package assetcache

import (
	"bytes"
	"slices"
	"testing"
)

func TestAddClassIsFoundAndKeepsTheRest(t *testing.T) {
	cache := testdata(t, File)
	const src = "marvelgameitems.MarvelItem_Insignia_Avengers"
	pk, err := Packages(cache, src)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pk, []string{"uc__marvelitem_insigniabase_sf", "uc__marvelitem_insignia_avengers_sf"}) {
		t.Fatalf("unexpected packages %v", pk)
	}
	out, err := AddClass(cache, 42, "marvelgameitems.MarvelItem_LF01", []string{"a_sf", "b_sf"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Packages(out, "MarvelGameItems.MarvelItem_LF01"); err != nil || !slices.Equal(got, []string{"a_sf", "b_sf"}) {
		t.Fatalf("added class: %v %v", got, err)
	}
	if got, _ := Packages(out, src); !slices.Equal(got, pk) {
		t.Fatal("existing class changed")
	}
	_, end, _, _ := layout(cache)
	_, end2, _, _ := layout(out)
	if !bytes.Equal(cache[end:], out[end2:]) {
		t.Fatal("data after the class section changed")
	}
	if !bytes.Contains(out, append(le.AppendUint64(nil, 42), 32, 0, 0, 0)) {
		t.Fatal("asset GUID entry missing")
	}
	if !Extends(out, cache) || Extends(cache, out) {
		t.Fatal("Extends does not recognise an added class")
	}
	tampered := append([]byte{}, out...)
	tampered[50]++
	if Extends(tampered, cache) {
		t.Fatal("Extends accepted a changed entry")
	}
	if _, err := AddClass(cache[:100], 1, "x", nil); err == nil {
		t.Fatal("expected error for a truncated cache")
	}
}
