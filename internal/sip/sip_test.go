package sip

import (
	"bytes"
	"testing"

	"mholootfilter/internal/db"
)

const (
	logan = "Calligraphy/Entity/Items/Consumables/Prototypes/FortuneCard/LoganFortuneCard.prototype"
	axe   = "Calligraphy/Entity/Items/Runewords/BaseItems/RunewordBase001.prototype"
)

func loadPak(t *testing.T) ([]byte, *Pak) {
	t.Helper()
	raw := testdata(t, "Calligraphy.sip")
	p, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, p
}

func protoFor(t *testing.T, d *db.DB, path string) db.Proto {
	for _, it := range d.Items {
		for _, p := range it.Protos {
			if p.Path == path {
				return p
			}
		}
	}
	t.Fatalf("%s not in the item database", path)
	return db.Proto{}
}

func fieldSet(d *db.DB) map[uint64]bool {
	set := map[uint64]bool{}
	for _, f := range d.UnrealClassFields {
		set[f] = true
	}
	return set
}

func TestOpenBytesRoundTrip(t *testing.T) {
	raw, p := loadPak(t)
	if !bytes.Equal(p.Bytes(), raw) {
		t.Fatal("unchanged pak does not serialise byte-identical")
	}
}

func retargetAndCheck(t *testing.T, path string) (before, after []byte) {
	d, _ := db.Load()
	_, p := loadPak(t)
	before, err := p.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	sink := d.Sinks["shown"].Asset
	after, err = Retarget(before, sink, protoFor(t, d, path), fieldSet(d))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := UnrealClass(after, fieldSet(d))
	if !ok || got != sink {
		t.Fatalf("UnrealClass after retarget = %d (found %v), want %d", got, ok, sink)
	}
	return before, after
}

func TestRetargetOwnField(t *testing.T) {
	before, after := retargetAndCheck(t, logan)
	if len(before) != len(after) {
		t.Fatalf("own field should be swapped in place: %d -> %d bytes", len(before), len(after))
	}
}

func TestRetargetInheritedField(t *testing.T) {
	d, _ := db.Load()
	_, p := loadPak(t)
	orig, _ := p.Read(axe)
	if _, ok := UnrealClass(orig, fieldSet(d)); ok {
		t.Fatal("fixture assumption broken: the axe should inherit UnrealClass")
	}
	before, after := retargetAndCheck(t, axe)
	if len(after) <= len(before) {
		t.Fatal("inherited field should be inserted")
	}
}

func TestReplaceKeepsOtherEntries(t *testing.T) {
	d, _ := db.Load()
	raw, p := loadPak(t)
	orig, _ := p.Read(axe)
	changed, err := Retarget(orig, d.Sinks["hidden"].Asset, protoFor(t, d, axe), fieldSet(d))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Replace(axe, changed); err != nil {
		t.Fatal(err)
	}
	q, err := Open(p.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := q.Read(axe); !bytes.Equal(got, changed) {
		t.Fatal("replaced entry does not read back")
	}
	o, _ := Open(raw)
	for _, name := range o.Names() {
		if name != axe && !bytes.Equal(o.Blob(name), q.Blob(name)) {
			t.Fatalf("entry %s changed", name)
		}
	}
}
