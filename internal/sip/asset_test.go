package sip

import (
	"bytes"
	"testing"
)

func TestAddAssetAppendsAndCounts(t *testing.T) {
	list := []byte{0x54, 0x59, 0x50, 0x0b, 1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8, 0, 1, 0, 'A'}
	out, err := AddAsset(list, 0x1122334455667788, 9, "MarvelItem_LF01")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out[:4], list[:4]) || le.Uint16(out[4:]) != 2 || !bytes.Equal(out[6:len(list)], list[6:]) {
		t.Fatal("header or existing entries changed")
	}
	e := out[len(list):]
	if le.Uint64(e) != 0x1122334455667788 || le.Uint64(e[8:]) != 9 || e[16] != 0 || le.Uint16(e[17:]) != 15 ||
		string(e[19:]) != "MarvelItem_LF01" {
		t.Fatalf("bad entry % x", e)
	}
}
