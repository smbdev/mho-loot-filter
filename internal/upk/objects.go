package upk

import (
	"errors"
	"strconv"
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
		full := names[name]
		if num := i32(flat, p+16); num > 0 { // the name's number: Art_366 is stored as "Art" and 367
			full += "_" + strconv.Itoa(int(num)-1)
		}
		out = append(out, Export{Name: strings.ToLower(full), Outer: i32(flat, p+8),
			Size: int(i32(flat, p+32)), Offset: int(i32(flat, p+36))})
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	return out, nil
}

// Import is one object a package uses from another package.
type Import struct {
	Name  string // object name, lower case
	Outer int32  // reference to the outer object: -(import index + 1) for an import, 0 at package level
}

// Imports returns the package's import table with names resolved. Object references to import i are -(i+1).
func Imports(flat []byte) ([]Import, error) {
	names, err := Names(flat)
	if err != nil {
		return nil, err
	}
	fp := flagPos(flat)
	p := int(i32(flat, fp+offImpOffset))
	n := int(i32(flat, fp+offImpCount))
	out := make([]Import, 0, n)
	for range n {
		if p+28 > len(flat) {
			return nil, errors.New("truncated import table")
		}
		name := int(i32(flat, p+20))
		if name < 0 || name >= len(names) {
			return nil, errors.New("bad import name")
		}
		out = append(out, Import{Name: strings.ToLower(names[name]), Outer: i32(flat, p+16)})
		p += 28
	}
	return out, nil
}
