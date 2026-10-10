package wwise

import (
	"errors"
	"fmt"
)

// Finding what an event plays, for previewing it. An event's Play actions point at a sound or at a container;
// every object names its parent, so the sounds under a container are the ones whose parent chain reaches it.

const (
	hircAction     = 3
	actionPlay     = 0x0403
	streamEmbedded = 0
)

// Source is the audio of one sound object: media embedded in its bank, or a file streamed from a file package.
type Source struct {
	Media    uint32
	Embedded bool
}

// Node is one object of a bank's hierarchy: an event, an action, a sound, a container...
type Node struct {
	Kind byte
	Body []byte // after the ID
}

// Nodes reads a bank's objects, with their IDs in bank order.
func Nodes(bank []byte) (map[uint32]Node, []uint32, error) {
	cs, err := chunks(bank)
	if err != nil {
		return nil, nil, err
	}
	for _, c := range cs {
		if c.tag != "HIRC" {
			continue
		}
		if len(c.body) < 4 {
			return nil, nil, errors.New("truncated HIRC")
		}
		n := int(le.Uint32(c.body))
		nodes := make(map[uint32]Node, n)
		var order []uint32
		for p, i := 4, 0; i < n; i++ {
			if p+9 > len(c.body) {
				return nil, nil, errors.New("truncated HIRC")
			}
			size := int(le.Uint32(c.body[p+1:]))
			if p+5+size > len(c.body) || size < 4 {
				return nil, nil, errors.New("truncated HIRC")
			}
			id := le.Uint32(c.body[p+5:])
			nodes[id] = Node{c.body[p], c.body[p+9 : p+5+size]}
			order = append(order, id)
			p += 5 + size
		}
		return nodes, order, nil
	}
	return map[uint32]Node{}, nil, nil
}

// parent reads the parent ID from an object's node parameters, which start at `at`: an effects override flag and
// effect count (each effect 7 bytes, after a bypass byte), an attachment flag, an output bus, then the parent.
func parent(body []byte, at int) (uint32, bool) {
	if at+2 > len(body) {
		return 0, false
	}
	p := at + 2
	if n := int(body[at+1]); n > 0 {
		p += 1 + 7*n
	}
	p += 1 + 4
	if p+4 > len(body) {
		return 0, false
	}
	return le.Uint32(body[p:]), true
}

const hircMusicTrack = 11

// nodeAt is where an object type's node parameters start: after the 14-byte source of a sound, at once for the
// containers that hold sounds, after a flags byte for music segments and music containers.
var nodeAt = map[byte]int{2: 14, 5: 0, 6: 0, 7: 0, 9: 0, 10: 1, 12: 1, 13: 1}

// musicTrack reads a music track: its sources, then its playlist and clip automation, then its node parameters.
func musicTrack(body []byte) (sources []Source, node int, ok bool) {
	p := 1 // flags
	u32 := func() (int, bool) {
		if p+4 > len(body) {
			return 0, false
		}
		v := int(le.Uint32(body[p:]))
		p += 4
		return v, true
	}
	n, ok := u32()
	if !ok || p+14*n > len(body) {
		return nil, 0, false
	}
	for i := range n {
		s := body[p+14*i:]
		sources = append(sources, Source{Media: le.Uint32(s[5:]), Embedded: s[4] == streamEmbedded})
	}
	p += 14 * n
	items, ok := u32()
	if !ok {
		return nil, 0, false
	}
	p += 40 * items // track and source IDs, then play time, trims and duration as doubles
	if items > 0 {
		if _, ok = u32(); !ok { // sub-tracks
			return nil, 0, false
		}
	}
	clips, ok := u32()
	if !ok {
		return nil, 0, false
	}
	for range clips {
		p += 8 // clip index, automation type
		points, ok := u32()
		if !ok {
			return nil, 0, false
		}
		p += 12 * points
	}
	return sources, p, p <= len(body)
}

// Sources is the audio a sound or music track plays; other nodes have none.
func (n Node) Sources() []Source {
	switch n.Kind {
	case hircSound:
		if len(n.Body) >= 14 {
			return []Source{{Media: le.Uint32(n.Body[5:]), Embedded: n.Body[4] == streamEmbedded}}
		}
	case hircMusicTrack:
		src, _, _ := musicTrack(n.Body)
		return src
	}
	return nil
}

// Plays lists, in order, the sounds and music tracks that event eventID plays: those under the targets of its
// Play actions. nodes may hold the objects of several banks, as an event can play sounds kept in another bank.
func Plays(nodes map[uint32]Node, order []uint32, eventID uint32) ([]uint32, error) {
	ev, ok := nodes[eventID]
	if !ok || ev.Kind != HircEvent || len(ev.Body) < 4 {
		return nil, fmt.Errorf("event %d not found", eventID)
	}
	targets := map[uint32]bool{}
	for i := range int(le.Uint32(ev.Body)) {
		if 4+4*i+4 > len(ev.Body) {
			return nil, errors.New("truncated event")
		}
		a, ok := nodes[le.Uint32(ev.Body[4+4*i:])]
		if ok && a.Kind == hircAction && len(a.Body) >= 6 && le.Uint16(a.Body) == actionPlay {
			targets[le.Uint32(a.Body[2:])] = true
		}
	}
	under := func(id uint32) bool {
		for range 64 { // parent chains are short; this only guards against loops
			if targets[id] {
				return true
			}
			o, ok := nodes[id]
			if !ok {
				return false
			}
			at, ok := nodeAt[o.Kind]
			if o.Kind == hircMusicTrack {
				_, at, ok = musicTrack(o.Body)
			}
			if !ok {
				return false
			}
			if id, ok = parent(o.Body, at); !ok || id == 0 {
				return false
			}
		}
		return false
	}
	var out []uint32
	for _, id := range order {
		if k := nodes[id].Kind; (k == hircSound || k == hircMusicTrack) && under(id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// EmbeddedMedia lists the media embedded in a bank: media ID -> offset from the bank's start and size.
func EmbeddedMedia(bank []byte) (map[uint32][2]int, error) {
	cs, err := chunks(bank)
	if err != nil {
		return nil, err
	}
	var didx []byte
	dataAt, dataLen, pos := -1, 0, 0
	for _, c := range cs {
		switch c.tag {
		case "DIDX":
			didx = c.body
		case "DATA":
			dataAt, dataLen = pos+8, len(c.body)
		}
		pos += 8 + len(c.body)
	}
	out := map[uint32][2]int{}
	for i := 0; i+12 <= len(didx); i += 12 {
		off, n := int(le.Uint32(didx[i+4:])), int(le.Uint32(didx[i+8:]))
		if dataAt < 0 || off+n > dataLen {
			return nil, errors.New("media index out of range")
		}
		out[le.Uint32(didx[i:])] = [2]int{dataAt + off, n}
	}
	return out, nil
}

// BankAt is where bank bankID sits in a file package.
func BankAt(pck []byte, bankID uint32) (offset int, err error) {
	e, _, err := pckBank(pck, bankID)
	if err != nil {
		return 0, err
	}
	return int(le.Uint32(pck[e+12:])) * int(le.Uint32(pck[e+4:])), nil
}

// StreamedFile is where a file package keeps a streamed sound.
type StreamedFile struct {
	Offset, Size int64
}

// StreamedFiles lists the streamed sounds of a file package from its header, which is all head needs to hold.
func StreamedFiles(head []byte) (map[uint32]StreamedFile, error) {
	lut, _, err := bankTable(head)
	if err != nil {
		return nil, err
	}
	banks := int(le.Uint32(head[16:]))
	st := lut + banks
	if st+4 > len(head) {
		return nil, errors.New("truncated file package")
	}
	n := int(le.Uint32(head[st:]))
	if st+4+20*n > len(head) {
		return nil, errors.New("truncated file package")
	}
	out := make(map[uint32]StreamedFile, n)
	for i := range n {
		e := st + 4 + 20*i
		block, size, start := int64(le.Uint32(head[e+4:])), int64(le.Uint32(head[e+8:])), int64(le.Uint32(head[e+12:]))
		out[le.Uint32(head[e:])] = StreamedFile{start * block, size}
	}
	return out, nil
}

// HeaderSize is how much of a file package StreamedFiles needs: its tables.
func HeaderSize(start []byte) (int, error) {
	if len(start) < 28 || string(start[:4]) != "AKPK" {
		return 0, errors.New("not a Wwise file package")
	}
	n := 28
	for _, at := range []int{12, 16, 20, 24} {
		n += int(le.Uint32(start[at:]))
	}
	return n, nil
}
