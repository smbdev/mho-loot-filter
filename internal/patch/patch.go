// Package patch builds patched game file contents. It does no file I/O.
package patch

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
)

// ExeHashOffset is where MarvelGame.upk's SHA1 sits in the 2.16a Win64 exe, right after "marvelgame.upk\0".
const ExeHashOffset = 46895160

type Flags struct{ Glow, Model, Name bool }

func (f Flags) None() bool { return !f.Glow && !f.Model && !f.Name }

func Sha1Hex(b []byte) string { s := sha1.Sum(b); return hex.EncodeToString(s[:]) }

func zero(flat []byte, offs []int) error {
	for _, o := range offs {
		if o < 0 || o+4 > len(flat) {
			return fmt.Errorf("offset %d out of range", o)
		}
		copy(flat[o:o+4], []byte{0, 0, 0, 0})
	}
	return nil
}

// Package returns original with the requested references cleared. original must be the untouched game file.
func Package(original []byte, t *db.Type, f Flags) ([]byte, error) {
	if Sha1Hex(original) != t.OrigSha1 {
		return nil, fmt.Errorf("%s is not the original 2.16a file", t.File)
	}
	flat, err := upk.Unpack(original)
	if err != nil {
		return nil, err
	}
	var offs []int
	if f.Glow {
		offs = append(offs, t.Glow...)
	}
	if f.Model {
		offs = append(offs, t.Model...)
	}
	if f.Name {
		offs = append(offs, t.Name...)
	}
	if err := zero(flat, offs); err != nil {
		return nil, err
	}
	return upk.Repack(original, flat)
}

// MarvelGame returns MarvelGame.upk with the glow of every rarity in hide cleared.
func MarvelGame(original []byte, offsets map[string][]int, hide map[string]bool) ([]byte, error) {
	flat, err := upk.Unpack(original)
	if err != nil {
		return nil, err
	}
	for r, h := range hide {
		if h {
			if err := zero(flat, offsets[r]); err != nil {
				return nil, err
			}
		}
	}
	return upk.Repack(original, flat)
}

// ExeHash returns exe with MarvelGame.upk's stored SHA1 replaced by the SHA1 of marvelGame.
func ExeHash(exe, marvelGame []byte) ([]byte, error) {
	name := []byte("marvelgame.upk\x00")
	if len(exe) < ExeHashOffset+20 || !bytes.Equal(exe[ExeHashOffset-len(name):ExeHashOffset], name) {
		return nil, fmt.Errorf("unexpected exe version: hash table entry not found")
	}
	out := append([]byte{}, exe...)
	sum := sha1.Sum(marvelGame)
	copy(out[ExeHashOffset:], sum[:])
	return out, nil
}
