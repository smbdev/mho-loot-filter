package patch

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"strings"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
	"mholootfilter/internal/wwise"
)

func fixture(t *testing.T, n string) []byte {
	return testdata(t, n)
}

func relicType(t *testing.T) *db.Type {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	ty := d.Types["marvelitem_loot_origin_reliccritdamage"]
	if ty == nil {
		for _, v := range d.Types {
			if v.File == "UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk" {
				ty = v
			}
		}
	}
	if ty == nil {
		t.Fatal("relic type missing from itemdb.json")
	}
	return ty
}

func TestPackageZeroesOnlyRequestedOffsets(t *testing.T) {
	orig := fixture(t, "UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk")
	ty := relicType(t)
	out, err := Package(orig, ty, Flags{Model: true})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := upk.Unpack(orig)
	after, _ := upk.Unpack(out)
	for _, o := range ty.Model {
		if !bytes.Equal(after[o:o+4], []byte{0, 0, 0, 0}) {
			t.Fatalf("model ref at %d not cleared", o)
		}
	}
	diff := 0
	for i := range before {
		if before[i] != after[i] {
			diff++
		}
	}
	if diff > 4*len(ty.Model) {
		t.Fatalf("%d bytes changed, expected at most %d", diff, 4*len(ty.Model))
	}
}

func TestPackageRefusesWrongOriginal(t *testing.T) {
	other := fixture(t, "UC__MarvelItem_Loot_FortuneCard_SF.upk")
	if _, err := Package(other, relicType(t), Flags{Model: true}); err == nil {
		t.Fatal("expected SHA1 mismatch error")
	}
}

func TestExeHashWritesSha1AfterName(t *testing.T) {
	exe := make([]byte, ExeHashOffset+40)
	copy(exe[ExeHashOffset-15:], []byte("marvelgame.upk\x00"))
	mg := []byte("pretend marvelgame")
	out, err := ExeHash(exe, mg)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(mg)
	if !bytes.Equal(out[ExeHashOffset:ExeHashOffset+20], sum[:]) {
		t.Fatal("hash not written")
	}
	if _, err := ExeHash(make([]byte, ExeHashOffset+40), mg); err == nil {
		t.Fatal("expected error for exe without hash entry")
	}
}

func typeOf(t *testing.T, file string) *db.Type {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range d.Types {
		if v.File == file {
			return v
		}
	}
	t.Fatalf("%s missing from itemdb.json", file)
	return nil
}

func TestSoundRewritesAnExistingAudioType(t *testing.T) {
	const file = "UC__MarvelItem_Loot_FortuneCard_SF.upk"
	orig, ty := fixture(t, file), typeOf(t, file)
	if ty.Audio == 0 {
		t.Fatal("fortune cards set their own audio type")
	}
	out, err := Package(orig, ty, Flags{Sound: true})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := upk.Unpack(orig)
	after, err := upk.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	delta := len(after) - len(before)
	idx := upk.NameIndex(after, AlertAudioType)
	if delta <= 0 || idx < 0 {
		t.Fatalf("name not added: delta %d, index %d", delta, idx)
	}
	if got := binary.LittleEndian.Uint64(after[ty.Audio+delta:]); got != uint64(idx) {
		t.Fatalf("audio type is name %d, want %d", got, idx)
	}
}

func TestSoundAddsAudioTypeWhenInherited(t *testing.T) {
	const file = "UC__MarvelItem_Insignia_XMen_SF.upk"
	orig, ty := fixture(t, file), typeOf(t, file)
	if ty.Audio != 0 {
		t.Fatal("insignias inherit their audio type")
	}
	out, err := Package(orig, ty, Flags{Sound: true, Name: true})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := upk.Unpack(orig)
	after, err := upk.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	none := binary.LittleEndian.Uint32(before[ty.AudioEnd:])
	at := ty.AudioEnd + (len(after) - len(before)) - 40
	name := func(p int) int { return int(binary.LittleEndian.Uint32(after[at+p:])) }
	if name(0) != upk.NameIndex(after, "audiotype") || name(8) != upk.NameIndex(after, "byteproperty") ||
		name(16) != 8 || name(24) != upk.NameIndex(after, "eaudioitemtype") || name(32) != upk.NameIndex(after, AlertAudioType) {
		t.Fatalf("bad property tag % x", after[at:at+40])
	}
	if uint32(name(40)) != none {
		t.Fatal("property not followed by the None tag")
	}
	for _, o := range ty.Name {
		o += len(after) - len(before) - 40 // moved by the added names only; the tag is inserted after it
		if !bytes.Equal(after[o:o+4], []byte{0, 0, 0, 0}) {
			t.Fatal("name reference not cleared alongside the sound")
		}
	}
}

func TestCloneNameKeepsLength(t *testing.T) {
	for _, key := range []string{"marvelitem_loot", "marvelitem_insignia_avengers"} {
		name, err := CloneName(key, 578)
		if err != nil || len(name) != len(key) || !IsClone(CloneFile(name)) {
			t.Fatalf("%s: %q %v", key, name, err)
		}
	}
	if a, _ := CloneName("marvelitem_loot", 1); a != "MarvelItem_LF01" {
		t.Fatalf("got %s", a)
	}
	if _, err := CloneName("marvelitem_loot", 36*36); err == nil {
		t.Fatal("expected error when the number does not fit")
	}
	if IsClone("UC__MarvelItem_Loot_SF.upk") {
		t.Fatal("game package taken for a copy")
	}
}

func TestCloneRenamesTheClass(t *testing.T) {
	const file, key = "UC__MarvelItem_Insignia_XMen_SF.upk", "marvelitem_insignia_xmen"
	orig, ty := fixture(t, file), typeOf(t, file)
	clone, _ := CloneName(key, 7)
	out, err := Clone(orig, ty, key, clone, Flags{Sound: true})
	if err != nil {
		t.Fatal(err)
	}
	flat, err := upk.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(clone)
	for _, n := range []string{low, "default__" + low, AlertAudioType} {
		if upk.NameIndex(flat, n) < 0 {
			t.Fatalf("%s missing from the copy", n)
		}
	}
	if upk.NameIndex(flat, key) >= 0 || upk.NameIndex(flat, "default__"+key) >= 0 {
		t.Fatal("copy still names the original class")
	}
	before, _ := upk.Unpack(orig)
	fp := 16 + int(binary.LittleEndian.Uint32(flat[12:]))
	if bytes.Equal(flat[fp+48:fp+64], before[fp+48:fp+64]) {
		t.Fatal("copy kept the original package GUID")
	}
}

func TestSoundsReplaceTokenAndRaritySounds(t *testing.T) {
	pck := fixture(t, "SFX_Shared_INT.pck")
	out, err := Sounds(pck, []uint32{TokenSound, RaritySounds["Cosmic"], RaritySounds["Unique"]}, wwise.Alert, AlertVolume)
	if err != nil {
		t.Fatal(err)
	}
	if !wwise.Extends(out, pck, ItemSoundBank) {
		t.Fatal("sound package changed beyond the item bank")
	}
	bank, _ := wwise.Bank(out, ItemSoundBank)
	if n := bytes.Count(bank, wwise.Alert[:64]); n != 3 {
		t.Fatalf("alert stored %d times, want 3", n)
	}
}
