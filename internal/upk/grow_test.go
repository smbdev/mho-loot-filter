package upk

import (
	"bytes"
	"testing"
)

type export struct{ off, size int }

func exports(flat []byte) []export {
	fp := flagPos(flat)
	var out []export
	p := int(i32(flat, fp+offExpOffset))
	for range int(i32(flat, fp+offExpCount)) {
		out = append(out, export{int(i32(flat, p+36)), int(i32(flat, p+32))})
		p += 48 + 4*int(i32(flat, p+44)) + 20
	}
	return out
}

// sameExport reports whether an export moved by delta is unchanged apart from its bulk data offsets.
func sameExport(old, moved []byte, oldOff, delta int) bool {
	want := append([]byte{}, old...)
	for s := 0; s+4 <= len(old); s++ {
		if int(i32(old, s)) == oldOff+s+4 {
			le.PutUint32(want[s:], uint32(oldOff+s+4+delta))
		}
	}
	return bytes.Equal(want, moved)
}

func TestAddNameShiftsEverythingAfterTheNameTable(t *testing.T) {
	for _, f := range fixtures {
		flat := read(t, f+".flat")
		if NameIndex(flat, "AITToken") >= 0 {
			t.Fatalf("%s: fixture already has the name", f)
		}
		grown, idx, err := AddName(flat, "AITToken")
		if err != nil {
			t.Fatal(err)
		}
		delta := len(grown) - len(flat)
		if delta != 4+9+8 || NameIndex(grown, "aittoken") != idx {
			t.Fatalf("%s: delta %d, index %d/%d", f, delta, idx, NameIndex(grown, "aittoken"))
		}
		if again, i2, _ := AddName(grown, "AITTOKEN"); i2 != idx || len(again) != len(grown) {
			t.Fatalf("%s: adding an existing name changed the package", f)
		}
		before, after := exports(flat), exports(grown)
		for i, e := range before {
			a := after[i]
			if a.off != e.off+delta || a.size != e.size {
				t.Fatalf("%s: export %d moved %+v -> %+v", f, i, e, a)
			}
			if !sameExport(flat[e.off:e.off+e.size], grown[a.off:a.off+a.size], e.off, delta) {
				t.Fatalf("%s: export %d content changed", f, i)
			}
		}
		packed, err := Repack(read(t, f), grown)
		if err != nil {
			t.Fatal(err)
		}
		if back, err := Unpack(packed); err != nil || !bytes.Equal(back, grown) {
			t.Fatalf("%s: grown package does not round trip: %v", f, err)
		}
	}
}

func TestInsertInsideAnExportGrowsIt(t *testing.T) {
	flat := read(t, fixtures[0]+".flat")
	ex := exports(flat)
	target := ex[1]
	grown, err := Insert(flat, target.off+8, []byte{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	after := exports(grown)
	if after[1].off != target.off || after[1].size != target.size+4 {
		t.Fatalf("target export %+v -> %+v", target, after[1])
	}
	for i, e := range ex {
		if e.off > target.off && after[i].off != e.off+4 {
			t.Fatalf("export %d after the insert did not move", i)
		}
	}
	if !bytes.Equal(grown[target.off+8:target.off+12], []byte{1, 2, 3, 4}) {
		t.Fatal("inserted bytes missing")
	}
}

func TestInsertRejectsHeaderPositions(t *testing.T) {
	flat := read(t, fixtures[0]+".flat")
	if _, err := Insert(flat, 20, []byte{0}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenameReplacesInPlace(t *testing.T) {
	flat := read(t, "UC__MarvelItem_Insignia_XMen_SF.upk.flat")
	orig := append([]byte{}, flat...)
	const old, name = "marvelitem_insignia_xmen", "marvelitem_lf000000000001"
	if err := Rename(flat, old, name); err == nil {
		t.Fatal("expected error for a different length")
	}
	if err := Rename(flat, old, name[:len(old)]); err != nil {
		t.Fatal(err)
	}
	if len(flat) != len(orig) || NameIndex(flat, old) >= 0 || NameIndex(flat, name[:len(old)]) != NameIndex(orig, old) {
		t.Fatal("name not renamed in place")
	}
	var guid [16]byte
	guid[0] = 7
	SetGUID(flat, guid)
	if flat[flagPos(flat)+48] != 7 {
		t.Fatal("GUID not set")
	}
}

func TestRepackKeepsChunksOnExportBoundaries(t *testing.T) {
	orig := read(t, "MarvelGame.upk")
	flat := read(t, "MarvelGame.upk.flat")
	s, err := parse(orig)
	if err != nil || len(s.chunks) < 3 {
		t.Fatalf("MarvelGame.upk should have several chunks: %v", err)
	}
	starts := map[int]int{} // original export offset -> export index
	for i, e := range exports(flat) {
		starts[e.off] = i
	}
	at := int(s.chunks[1].uoff) + 100 // inside the second chunk
	grown, err := Insert(flat, at, make([]byte, 29))
	if err != nil {
		t.Fatal(err)
	}
	packed, err := Repack(orig, grown)
	if err != nil {
		t.Fatal(err)
	}
	ns, _ := parse(packed)
	after := exports(grown)
	for i, c := range s.chunks {
		if i == 0 {
			continue
		}
		if idx, ok := starts[int(c.uoff)]; ok && int(ns.chunks[i].uoff) != after[idx].off {
			t.Fatalf("chunk %d starts at %d, but its export moved to %d", i, ns.chunks[i].uoff, after[idx].off)
		}
	}
	if back, err := Unpack(packed); err != nil || !bytes.Equal(back, grown) {
		t.Fatalf("grown package does not round trip: %v", err)
	}
}
