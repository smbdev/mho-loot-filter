// Package patch builds patched game file contents. It does no file I/O.
package patch

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
	"mholootfilter/internal/wwise"
)

// ExeHashOffset is where MarvelGame.upk's SHA1 sits in the 2.16a Win64 exe, right after "marvelgame.upk\0".
const ExeHashOffset = 46895160

type Flags struct{ Glow, Model, Name, Sound bool }

func (f Flags) None() bool { return !f.Glow && !f.Model && !f.Name && !f.Sound }

// AlertAudioType is the drop sound the alert replaces. No item uses it in 2.16a.
const AlertAudioType = "aittoken"

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
	if f.Sound {
		if flat, err = setAudioType(flat, t, AlertAudioType); err != nil {
			return nil, err
		}
	}
	return upk.Repack(original, flat)
}

// setAudioType sets the drop sound of a type: it rewrites the AudioType value when the default object sets one,
// and otherwise adds the property in front of the None tag that ends the default object's properties.
func setAudioType(flat []byte, t *db.Type, value string) ([]byte, error) {
	at := t.Audio
	if at == 0 {
		at = t.AudioEnd
	}
	if at <= 0 || at+8 > len(flat) {
		return nil, fmt.Errorf("%s: no drop sound offset", t.File)
	}
	names := []string{value}
	if t.Audio == 0 {
		names = append(names, "audiotype", "byteproperty", "eaudioitemtype")
	}
	idx := map[string]int{}
	for _, n := range names {
		before := len(flat)
		var err error
		if flat, idx[n], err = upk.AddName(flat, n); err != nil {
			return nil, err
		}
		at += len(flat) - before // the name table sits before every export, so the offset moves with it
	}
	if t.Audio != 0 {
		binary.LittleEndian.PutUint64(flat[at:], uint64(idx[value]))
		return flat, nil
	}
	var tag []byte
	for _, v := range []int{idx["audiotype"], 0, idx["byteproperty"], 0, 8, 0, idx["eaudioitemtype"], 0, idx[value], 0} {
		tag = binary.LittleEndian.AppendUint32(tag, uint32(v))
	}
	return upk.Insert(flat, at, tag)
}

// MarvelGame returns MarvelGame.upk with the glow of every rarity in hide cleared and, when rules are given, code
// that hides items by class and rarity.
func MarvelGame(original []byte, d *db.DB, hide map[string]bool, rules []RarityRule) ([]byte, error) {
	flat, err := upk.Unpack(original)
	if err != nil {
		return nil, err
	}
	for r, h := range hide {
		if h {
			if err := zero(flat, d.Rarities[r]); err != nil {
				return nil, err
			}
		}
	}
	if len(rules) > 0 { // after clearing glows: the offsets are those of the original layout
		rs := d.RarityScript
		n := len(flat)
		if flat, err = hideByRarity(flat, rs, rules); err != nil {
			return nil, err
		}
		at := rs.LineCheck
		if at > rs.Function {
			at += len(flat) - n
		}
		if flat, err = clickableMeshes(flat, at, rs.BoolProperty); err != nil {
			return nil, err
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

// The drop sounds live in one bank of SFX_Shared_INT.pck, one sound object per audio type.
const (
	SoundPackage  = "SFX_Shared_INT.pck"
	ItemSoundBank = 1382876039
	TokenSound    = 934977114 // drop sound of AlertAudioType
	AlertVolume   = 8         // dB, the default; loud enough to stand out over a fight
)

// RaritySounds are the drop sounds of Cosmic and Unique items: their rarity sets the sound, not the item.
var RaritySounds = map[string]uint32{"Cosmic": 802757657, "Unique": 162153762}

// Sounds returns the sound package with alert, a PCM .wem file, played at volume dB by each of the given sound
// objects.
func Sounds(pck []byte, sounds []uint32, alert []byte, volume float32) ([]byte, error) {
	bank, err := wwise.Bank(pck, ItemSoundBank)
	if err != nil {
		return nil, err
	}
	for _, s := range sounds {
		if bank, err = wwise.ReplaceSound(bank, s, alert, volume); err != nil {
			return nil, err
		}
	}
	return wwise.ReplaceBank(pck, ItemSoundBank, bank)
}

const clonePrefix = "MarvelItem_LF"

// CloneName returns the class name of copy n of item type key: "MarvelItem_LF" and n in base 36, padded to the
// length of key so the copied package keeps its layout.
func CloneName(key string, n int) (string, error) {
	digits := strings.ToUpper(strconv.FormatInt(int64(n), 36))
	pad := len(key) - len(clonePrefix) - len(digits)
	if pad < 0 {
		return "", fmt.Errorf("%s: class name too short to copy", key)
	}
	return clonePrefix + strings.Repeat("0", pad) + digits, nil
}

// IsClone reports whether a package file name is one of the filter's class copies.
func IsClone(file string) bool {
	return strings.HasPrefix(strings.ToLower(file), strings.ToLower("UC__"+clonePrefix))
}

// CloneFile is the package file of a class copy.
func CloneFile(clone string) string { return "UC__" + clone + "_SF.upk" }

// Clone returns the package of item type t (class key) as a new class named clone, with f applied.
// The game finds the copy through the class name, so the class, its defaults and the package are renamed,
// and the package gets a GUID of its own.
func Clone(original []byte, t *db.Type, key, clone string, f Flags) ([]byte, error) {
	b, err := Package(original, t, f)
	if err != nil {
		return nil, err
	}
	flat, err := upk.Unpack(b)
	if err != nil {
		return nil, err
	}
	low := strings.ToLower(clone)
	for _, pair := range [][2]string{{key, low}, {"default__" + key, "default__" + low}, {"uc__" + key + "_sf", "uc__" + low + "_sf"}} {
		if err := upk.Rename(flat, pair[0], pair[1]); err != nil && pair[0] != "uc__"+key+"_sf" {
			return nil, fmt.Errorf("%s: %w", t.File, err)
		}
	}
	sum := sha1.Sum([]byte("mholootfilter " + low))
	upk.SetGUID(flat, [16]byte(sum[:16]))
	return upk.Repack(original, flat)
}

// CloneIDs returns the Calligraphy asset id and GUID of a class copy, fixed by its name.
func CloneIDs(clone string) (id, guid uint64) {
	sum := sha1.Sum([]byte("mholootfilter asset " + strings.ToLower(clone)))
	return binary.LittleEndian.Uint64(sum[:]) | 1<<63, binary.LittleEndian.Uint64(sum[8:])
}
