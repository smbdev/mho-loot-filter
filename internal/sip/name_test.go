package sip

import (
	"testing"

	"mholootfilter/internal/db"
)

func ownValue(t *testing.T, proto []byte, field uint64) (uint64, bool) {
	t.Helper()
	if _, err := parse(proto, nil); err != nil {
		t.Fatal(err)
	}
	return Value(proto, field)
}

func TestSetFieldAddsAFieldThenReplacesItInPlace(t *testing.T) {
	_, pak := loadPak(t)
	d, _ := db.Load()
	p := d.Items[0].Protos[0]
	proto, err := pak.Read(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	const field = 0x1234
	added, err := SetField(proto, field, TypeLong, 7, p.Blueprint, p.Copy)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ownValue(t, added, field); !ok || v != 7 {
		t.Fatalf("added field reads %d %v, want 7", v, ok)
	}
	again, err := SetField(added, field, TypeLong, 9, 0, 0)
	if err != nil || len(again) != len(added) {
		t.Fatalf("replace: %v, length %d -> %d", err, len(added), len(again))
	}
	if v, _ := ownValue(t, again, field); v != 9 {
		t.Fatalf("replaced field reads %d, want 9", v)
	}
}

func TestSetFieldChangesTheGroundNameSize(t *testing.T) {
	_, pak := loadPak(t)
	d, _ := db.Load()
	ns := d.NameStyle
	proto, err := pak.Read(ns.Title)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := ownValue(t, proto, ns.FontSizeField); !ok || int(v) != ns.FontSize {
		t.Fatalf("title font size %d %v, want %d", v, ok, ns.FontSize)
	}
	out, err := SetField(proto, ns.FontSizeField, TypeLong, 22, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := ownValue(t, out, ns.FontSizeField); v != 22 || len(out) != len(proto) {
		t.Fatalf("font size %d, length %d -> %d: want 22 in place", v, len(proto), len(out))
	}
	if _, err := SetField(proto, 12345, TypeLong, 1, 0, 0); err == nil {
		t.Fatal("a field the prototype does not set needs a field group to go into")
	}
}
