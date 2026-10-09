package upk

import (
	"bytes"
	"errors"
)

// Header fields of a decompressed package, relative to the package flags.
const (
	offNameCount  = 4
	offNameOffset = 8
	offExpCount   = 12
	offExpOffset  = 16
	offImpOffset  = 24
	offDepends    = 28
	offGuids      = 32
	offThumbnails = 44
	offGenCount   = 64 // generations follow the package GUID
)

func flagPos(flat []byte) int { return 16 + int(i32(flat, 12)) }

// Insert returns flat with data inserted at position at, keeping every absolute offset in the package valid:
// the header table offsets, the export table's serial offsets and sizes, and the file offsets that inline
// bulk data (textures, meshes) stores next to its payload.
func Insert(flat []byte, at int, data []byte) ([]byte, error) {
	fp := flagPos(flat)
	firstExport := len(flat)
	delta := int32(len(data))
	shift := func(p int) {
		if v := i32(flat, p); v != 0 && int(v) >= at {
			le.PutUint32(flat[p:], uint32(v+delta))
		}
	}
	if at < fp+offGenCount || at > len(flat) {
		return nil, errors.New("insert position outside the package body")
	}
	out := make([]byte, 0, len(flat)+len(data))
	flat = append([]byte{}, flat...)

	// Inline bulk data stores its own absolute file offset right before its payload, so a stored value equal to
	// its position + 4 marks one. Find them before anything moves.
	expCount, expOff := int(i32(flat, fp+offExpCount)), int(i32(flat, fp+offExpOffset))
	p := expOff
	for i := 0; i < expCount; i++ {
		firstExport = min(firstExport, int(i32(flat, p+36)))
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	var bulk []int
	for q := max(firstExport, at); q+4 <= len(flat); q++ {
		if int(i32(flat, q)) == q+4 {
			bulk = append(bulk, q)
		}
	}

	shift(8) // total header size
	for _, f := range []int{offNameOffset, offExpOffset, offImpOffset, offDepends, offGuids, offThumbnails} {
		shift(fp + f)
	}
	p = expOff
	for i := 0; i < expCount; i++ {
		size, off := int(i32(flat, p+32)), int(i32(flat, p+36))
		if off < at && at < off+size {
			le.PutUint32(flat[p+32:], uint32(int32(size)+delta))
		}
		shift(p + 36)
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	for _, q := range bulk {
		le.PutUint32(flat[q:], uint32(int32(q+4)+delta))
	}
	out = append(append(append(out, flat[:at]...), data...), flat[at:]...)
	return out, nil
}

// NameIndex returns the index of name in the package's name table (case-insensitive, like Unreal names), or -1.
func NameIndex(flat []byte, name string) int {
	fp := flagPos(flat)
	p := int(i32(flat, fp+offNameOffset))
	for i := range int(i32(flat, fp+offNameCount)) {
		n := int(i32(flat, p))
		if n > 0 && bytes.EqualFold(flat[p+4:p+3+n], []byte(name)) {
			return i
		}
		if n < 0 {
			n = -2 * n
		}
		p += 4 + n + 8
	}
	return -1
}

// AddName appends name to the name table unless it is already there, and returns the package and the name's index.
// Names added after the last export generation was saved are counted in that generation too, as the cooker does.
func AddName(flat []byte, name string) ([]byte, int, error) {
	if i := NameIndex(flat, name); i >= 0 {
		return flat, i, nil
	}
	fp := flagPos(flat)
	count := int(i32(flat, fp+offNameCount))
	p := int(i32(flat, fp+offNameOffset))
	var flags []byte
	for range count {
		n := int(i32(flat, p))
		if n < 0 {
			n = -2 * n
		}
		flags = flat[p+4+n : p+4+n+8]
		p += 4 + n + 8
	}
	entry := le.AppendUint32(nil, uint32(len(name)+1))
	entry = append(append(append(entry, name...), 0), flags...)
	out, err := Insert(flat, p, entry)
	if err != nil {
		return nil, 0, err
	}
	le.PutUint32(out[fp+offNameCount:], uint32(count+1))
	gens := int(i32(out, fp+offGenCount))
	if gens > 0 {
		last := fp + offGenCount + 4 + (gens-1)*12
		le.PutUint32(out[last+4:], uint32(count+1))
	}
	return out, count, nil
}

// Rename changes a name in the name table to another of the same length, so nothing moves.
func Rename(flat []byte, old, name string) error {
	if len(old) != len(name) {
		return errors.New("rename needs a name of the same length")
	}
	i := NameIndex(flat, old)
	if i < 0 {
		return errors.New("name not found: " + old)
	}
	fp := flagPos(flat)
	p := int(i32(flat, fp+offNameOffset))
	for ; i > 0; i-- {
		n := int(i32(flat, p))
		if n < 0 {
			n = -2 * n
		}
		p += 4 + n + 8
	}
	copy(flat[p+4:], name)
	return nil
}

// SetGUID replaces the package GUID, which the game uses to tell packages apart.
func SetGUID(flat []byte, guid [16]byte) {
	copy(flat[flagPos(flat)+48:], guid[:])
}

// exportOffsets returns the serial offset of every export, in export table order.
func exportOffsets(flat []byte) []int {
	fp := flagPos(flat)
	p := int(i32(flat, fp+offExpOffset))
	n := int(i32(flat, fp+offExpCount))
	out := make([]int, 0, n)
	for range n {
		out = append(out, int(i32(flat, p+36)))
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	return out
}
