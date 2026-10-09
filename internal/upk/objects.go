package upk

import (
	"errors"
	"strings"
)

// Export is one object stored in a package.
type Export struct {
	Name         string // object name, lower case
	Outer        int32  // export index + 1 of the outer object, 0 at package level
	Offset, Size int    // serialized data in the unpacked package
}

// Names returns the package's name table.
func Names(flat []byte) ([]string, error) {
	fp := flagPos(flat)
	p := int(i32(flat, fp+offNameOffset))
	count := int(i32(flat, fp+offNameCount))
	names := make([]string, 0, count)
	for range count {
		if p+4 > len(flat) {
			return nil, errors.New("truncated name table")
		}
		n := int(i32(flat, p))
		switch {
		case n > 0 && p+4+n <= len(flat):
			names = append(names, string(flat[p+4:p+3+n]))
		case n < 0 && p+4-2*n <= len(flat): // UTF-16; item packages do not use it, so keep the length right only
			names = append(names, "")
			n = -2 * n
		default:
			return nil, errors.New("bad name entry")
		}
		p += 4 + n + 8
	}
	return names, nil
}

// Exports returns the package's export table with names resolved.
func Exports(flat []byte) ([]Export, error) {
	names, err := Names(flat)
	if err != nil {
		return nil, err
	}
	fp := flagPos(flat)
	p := int(i32(flat, fp+offExpOffset))
	n := int(i32(flat, fp+offExpCount))
	out := make([]Export, 0, n)
	for range n {
		if p+68 > len(flat) {
			return nil, errors.New("truncated export table")
		}
		name := int(i32(flat, p+12))
		if name < 0 || name >= len(names) {
			return nil, errors.New("bad export name")
		}
		out = append(out, Export{Name: strings.ToLower(names[name]), Outer: i32(flat, p+8),
			Size: int(i32(flat, p+32)), Offset: int(i32(flat, p+36))})
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	return out, nil
}
