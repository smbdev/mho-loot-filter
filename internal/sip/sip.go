// Package sip reads and rewrites the game's Calligraphy.sip data archive and changes which item class
// an item prototype is drawn with.
package sip

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/pierrec/lz4/v4"

	"mholootfilter/internal/db"
)

var le = binary.LittleEndian

type entry struct {
	hash    []byte
	name    string
	modTime uint32
	size    int
	blob    []byte
}

// Pak is a KAPG archive. Unchanged entries keep their original compressed bytes.
type Pak struct {
	head    []byte
	entries []*entry
	index   map[string]*entry
}

func Open(data []byte) (*Pak, error) {
	if len(data) < 12 || string(data[:4]) != "KAPG" {
		return nil, errors.New("not a Calligraphy archive")
	}
	n := int(int32(le.Uint32(data[8:])))
	p := &Pak{head: append([]byte{}, data[:12]...), index: map[string]*entry{}}
	pos := 12
	type loc struct{ off, csize int }
	locs := make([]loc, 0, n)
	for i := 0; i < n; i++ {
		if pos+12 > len(data) {
			return nil, errors.New("truncated archive index")
		}
		e := &entry{hash: append([]byte{}, data[pos:pos+8]...)}
		nameLen := int(int32(le.Uint32(data[pos+8:])))
		pos += 12
		if nameLen < 0 || pos+nameLen+16 > len(data) {
			return nil, errors.New("truncated archive index")
		}
		e.name = string(data[pos : pos+nameLen])
		pos += nameLen
		e.modTime = le.Uint32(data[pos:])
		off := int(int32(le.Uint32(data[pos+4:])))
		csize := int(int32(le.Uint32(data[pos+8:])))
		e.size = int(int32(le.Uint32(data[pos+12:])))
		pos += 16
		p.entries = append(p.entries, e)
		p.index[e.name] = e
		locs = append(locs, loc{off, csize})
	}
	for i, l := range locs {
		start := pos + l.off
		if l.off < 0 || l.csize < 0 || start+l.csize > len(data) {
			return nil, fmt.Errorf("entry %s points outside the archive", p.entries[i].name)
		}
		p.entries[i].blob = data[start : start+l.csize]
	}
	return p, nil
}

func (p *Pak) Names() []string {
	out := make([]string, len(p.entries))
	for i, e := range p.entries {
		out[i] = e.name
	}
	return out
}

// Blob returns an entry's compressed bytes.
func (p *Pak) Blob(name string) []byte {
	if e := p.index[name]; e != nil {
		return e.blob
	}
	return nil
}

func (p *Pak) Read(name string) ([]byte, error) {
	e := p.index[name]
	if e == nil {
		return nil, fmt.Errorf("%s is not in the archive", name)
	}
	out := make([]byte, e.size)
	n, err := lz4.UncompressBlock(e.blob, out)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if n != e.size {
		return nil, fmt.Errorf("%s: decompressed to %d bytes, expected %d", name, n, e.size)
	}
	return out, nil
}

func (p *Pak) Replace(name string, data []byte) error {
	e := p.index[name]
	if e == nil {
		return fmt.Errorf("%s is not in the archive", name)
	}
	buf := make([]byte, lz4.CompressBlockBound(len(data)))
	n, err := lz4.CompressBlock(data, buf, nil)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s: data does not compress", name)
	}
	e.blob, e.size = buf[:n], len(data)
	return nil
}

// Bytes serialises the archive with entries in their original order.
func (p *Pak) Bytes() []byte {
	out := append([]byte{}, p.head...)
	off := 0
	for _, e := range p.entries {
		out = append(out, e.hash...)
		out = le.AppendUint32(out, uint32(len(e.name)))
		out = append(out, e.name...)
		out = le.AppendUint32(out, e.modTime)
		out = le.AppendUint32(out, uint32(off))
		out = le.AppendUint32(out, uint32(len(e.blob)))
		out = le.AppendUint32(out, uint32(e.size))
		off += len(e.blob)
	}
	for _, e := range p.entries {
		out = append(out, e.blob...)
	}
	return out
}

const (
	typeAsset    = 0x41 // 'A'
	typeRHStruct = 0x52 // embedded prototype instead of a 64-bit value
	flagParent   = 1
	flagData     = 2
	headerSize   = 4 // "PTP" + version
)

// group is the position of one top-level field group inside a prototype file.
type group struct {
	blueprint   uint64
	copy        uint8
	simpleCount int // offset of the simple-field count
	simpleEnd   int // offset just past the last simple field
}

type layout struct {
	groupCount int // offset of the group count, -1 when the prototype has no data
	groups     []group
	classValue int // offset of the top-level UnrealClass value, -1 when inherited
	end        int
}

type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || r.pos+n > len(r.b) {
		r.err = errors.New("truncated prototype")
		return make([]byte, n)
	}
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return v
}

func (r *reader) u8() uint8   { return r.take(1)[0] }
func (r *reader) u16() int    { return int(le.Uint16(r.take(2))) }
func (r *reader) u64() uint64 { return le.Uint64(r.take(8)) }

// walk reads one prototype starting at r.pos. With top set it records the layout of its field groups.
func walk(r *reader, top bool, unrealClass map[uint64]bool) layout {
	l := layout{groupCount: -1, classValue: -1}
	flags := r.u8()
	if flags&flagParent != 0 {
		r.u64()
	}
	if flags&flagData == 0 {
		l.end = r.pos
		return l
	}
	l.groupCount = r.pos
	groups := r.u16()
	for g := 0; g < groups && r.err == nil; g++ {
		gr := group{blueprint: r.u64(), copy: r.u8(), simpleCount: r.pos}
		simple := r.u16()
		for i := 0; i < simple && r.err == nil; i++ {
			field := r.u64()
			kind := r.u8()
			if kind == typeRHStruct {
				walk(r, false, unrealClass)
				continue
			}
			if top && unrealClass[field] {
				l.classValue = r.pos
			}
			r.u64()
		}
		gr.simpleEnd = r.pos
		lists := r.u16()
		for i := 0; i < lists && r.err == nil; i++ {
			r.u64()
			kind := r.u8()
			count := r.u16()
			for j := 0; j < count && r.err == nil; j++ {
				if kind == typeRHStruct {
					walk(r, false, unrealClass)
				} else {
					r.u64()
				}
			}
		}
		if top {
			l.groups = append(l.groups, gr)
		}
	}
	l.end = r.pos
	return l
}

func parse(proto []byte, unrealClass map[uint64]bool) (layout, error) {
	if len(proto) < headerSize+1 || string(proto[:3]) != "PTP" {
		return layout{}, errors.New("not a prototype file")
	}
	r := &reader{b: proto, pos: headerSize}
	l := walk(r, true, unrealClass)
	return l, r.err
}

// UnrealClass returns the item class asset a prototype sets itself, if any.
func UnrealClass(proto []byte, unrealClass map[uint64]bool) (uint64, bool) {
	l, err := parse(proto, unrealClass)
	if err != nil || l.classValue < 0 {
		return 0, false
	}
	return le.Uint64(proto[l.classValue:]), true
}

// Retarget returns proto with its UnrealClass set to asset. A value the prototype sets itself is replaced in
// place; an inherited one is added to the field group slot names, creating that group if needed.
func Retarget(proto []byte, asset uint64, slot db.Proto, unrealClass map[uint64]bool) ([]byte, error) {
	l, err := parse(proto, unrealClass)
	if err != nil {
		return nil, err
	}
	out := append([]byte{}, proto...)
	if l.classValue >= 0 {
		le.PutUint64(out[l.classValue:], asset)
		return out, nil
	}
	field := le.AppendUint64(nil, slot.Field)
	field = append(field, typeAsset)
	field = le.AppendUint64(field, asset)

	for _, g := range l.groups {
		if g.blueprint == slot.Blueprint && g.copy == slot.Copy {
			le.PutUint16(out[g.simpleCount:], le.Uint16(out[g.simpleCount:])+1)
			return splice(out, g.simpleEnd, field), nil
		}
	}

	newGroup := le.AppendUint64(nil, slot.Blueprint)
	newGroup = append(newGroup, slot.Copy)
	newGroup = le.AppendUint16(newGroup, 1)
	newGroup = append(newGroup, field...)
	newGroup = le.AppendUint16(newGroup, 0)
	if l.groupCount < 0 {
		out[headerSize] |= flagData
		count := le.AppendUint16(nil, 1)
		return splice(out, l.end, append(count, newGroup...)), nil
	}
	le.PutUint16(out[l.groupCount:], le.Uint16(out[l.groupCount:])+1)
	return splice(out, l.end, newGroup), nil
}

func splice(b []byte, at int, insert []byte) []byte {
	out := make([]byte, 0, len(b)+len(insert))
	out = append(out, b[:at]...)
	out = append(out, insert...)
	return append(out, b[at:]...)
}
