package upk

import (
	"bytes"
	"testing"
)

var fixtures = []string{
	"UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk",
	"UC__MarvelItem_Loot_FortuneCard_SF.upk",
	"UC__MarvelItem_Insignia_XMen_SF.upk",
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	return testdata(t, name)
}

func TestUnpackMatchesReference(t *testing.T) {
	for _, f := range fixtures {
		got, err := Unpack(read(t, f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !bytes.Equal(got, read(t, f+".flat")) {
			t.Fatalf("%s: unpacked bytes differ from liblzo2 reference", f)
		}
	}
}

func TestRepackRoundTrips(t *testing.T) {
	for _, f := range fixtures {
		orig := read(t, f)
		flat := read(t, f+".flat")
		flat[len(flat)-1] ^= 0xff // prove a change survives
		packed, err := Repack(orig, flat)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		back, err := Unpack(packed)
		if err != nil || !bytes.Equal(back, flat) {
			t.Fatalf("%s: repack round trip failed: %v", f, err)
		}
		if !bytes.Equal(packed[:16], orig[:16]) {
			t.Fatalf("%s: header prefix changed", f)
		}
	}
}

func TestUnpackRejectsGarbage(t *testing.T) {
	if _, err := Unpack([]byte("not a package at all")); err == nil {
		t.Fatal("expected error")
	}
}
