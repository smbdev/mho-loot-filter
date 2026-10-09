// Package assetcache edits AssetPackageCache.bin, the game's map from a Calligraphy asset GUID to its Unreal class
// and from a class to the packages that must be loaded for it. A class missing from it is never loaded.
package assetcache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// File is the cache's name in CookedPCConsole.
const File = "AssetPackageCache.bin"

var le = binary.LittleEndian

type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) u32() int {
	if r.err != nil || r.pos+4 > len(r.b) {
		r.err = errors.New("truncated asset package cache")
		return 0
	}
	v := int(le.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v
}

func (r *reader) str() string {
	n := r.u32()
	if r.err != nil || n < 1 || r.pos+n > len(r.b) {
		r.err = errors.New("truncated asset package cache")
		return ""
	}
	s := string(r.b[r.pos : r.pos+n-1])
	r.pos += n
	return s
}

// layout returns where the asset section and the class section end, and the packages of every class.
func layout(cache []byte) (assetsEnd, classesEnd int, packages map[string][]string, err error) {
	r := &reader{b: cache}
	for n := r.u32(); n > 0 && r.err == nil; n-- {
		r.pos += 8
		r.str()
	}
	assetsEnd = r.pos
	packages = map[string][]string{}
	for n := r.u32(); n > 0 && r.err == nil; n-- {
		class := r.str()
		var pk []string
		for k := r.u32(); k > 0 && r.err == nil; k-- {
			pk = append(pk, r.str())
		}
		packages[strings.ToLower(class)] = pk
	}
	return assetsEnd, r.pos, packages, r.err
}

func appendStr(b []byte, s string) []byte {
	return append(append(le.AppendUint32(b, uint32(len(s)+1)), s...), 0)
}

// Packages returns the packages the game loads for class, a full class path such as
// "marvelgameitems.MarvelItem_Insignia_Avengers".
func Packages(cache []byte, class string) ([]string, error) {
	_, _, packages, err := layout(cache)
	if err != nil {
		return nil, err
	}
	pk, ok := packages[strings.ToLower(class)]
	if !ok {
		return nil, fmt.Errorf("%s is not in the asset package cache", class)
	}
	return pk, nil
}

// AddClass returns cache with an asset GUID mapped to class and class mapped to packages.
func AddClass(cache []byte, guid uint64, class string, packages []string) ([]byte, error) {
	assetsEnd, classesEnd, _, err := layout(cache)
	if err != nil {
		return nil, err
	}
	asset := appendStr(le.AppendUint64(nil, guid), class)
	entry := le.AppendUint32(appendStr(nil, class), uint32(len(packages)))
	for _, p := range packages {
		entry = appendStr(entry, p)
	}
	out := make([]byte, 0, len(cache)+len(asset)+len(entry))
	out = append(append(out, cache[:assetsEnd]...), asset...)
	out = append(append(out, cache[assetsEnd:classesEnd]...), entry...)
	out = append(out, cache[classesEnd:]...)
	le.PutUint32(out, le.Uint32(cache)+1)
	q := assetsEnd + len(asset)
	le.PutUint32(out[q:], le.Uint32(cache[assetsEnd:])+1)
	return out, nil
}

// Extends reports whether cur is orig with classes added by AddClass and nothing else changed.
func Extends(cur, orig []byte) bool {
	oa, oc, _, err := layout(orig)
	if err != nil {
		return false
	}
	ca, cc, _, err := layout(cur)
	if err != nil || ca < oa || cc-ca < oc-oa {
		return false
	}
	return bytes.Equal(cur[4:oa], orig[4:oa]) &&
		bytes.Equal(cur[ca+4:ca+(oc-oa)], orig[oa+4:oc]) &&
		bytes.Equal(cur[cc:], orig[oc:])
}
