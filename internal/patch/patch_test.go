package patch

import (
	"bytes"
	"crypto/sha1"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
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
