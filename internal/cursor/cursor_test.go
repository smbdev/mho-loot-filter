package cursor

import (
	"bytes"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"mholootfilter/internal/upk"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("testdata/%s not found: run python tools/make_fixtures.py \"<game folder>\" first", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPointersAreRestyledInPlaceAndStillLoad(t *testing.T) {
	orig := testdata(t, "MarvelGame.upk")
	flat, err := upk.Unpack(orig)
	if err != nil {
		t.Fatal(err)
	}
	if same, _ := Patch(flat, Style{}); !bytes.Equal(same, flat) {
		t.Fatal("the game's own style changed the package")
	}
	for _, s := range []Style{{Color: "yellow"}, {Color: "pink", Size: 200}, {Size: 150}} {
		out, err := Patch(flat, s)
		if err != nil {
			t.Fatalf("%+v: %v", s, err)
		}
		if _, err := upk.Repack(orig, out); err != nil {
			t.Fatalf("%+v: repack: %v", s, err)
		}
		// every pointer reads back at its new size, and the exports after them are intact
		exps, _ := upk.Exports(out)
		before, _ := upk.Exports(flat)
		pointers := 0
		for i, e := range exps {
			if IsPointer(e.Name) {
				pointers++
				png, err := Preview(out, e.Name, Style{})
				if err != nil {
					t.Fatalf("%+v: %s: %v", s, e.Name, err)
				}
				if s.Size != 0 && len(png) == 0 {
					t.Fatal("empty preview")
				}
				continue
			}
			if e.Size != before[i].Size || !bytes.Equal(out[e.Offset:e.Offset+e.Size], flat[before[i].Offset:before[i].Offset+e.Size]) && e.Size < 64 {
				t.Fatalf("%+v: export %s changed", s, e.Name)
			}
		}
		if pointers != 21 {
			t.Errorf("found %d pointers, want 21", pointers)
		}
		want := 64
		if s.Size != 0 {
			want = 128
		}
		img, err := png.Decode(bytes.NewReader(must(Preview(out, "cursor_attack", Style{}))))
		if err != nil || img.Bounds().Dx() != want {
			t.Errorf("%+v: attack pointer is %v, want %d wide", s, img.Bounds(), want)
		}
	}
	if (Style{Color: "orange"}).Validate() == nil || (Style{Size: 120}).Validate() == nil {
		t.Error("unknown styles accepted")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// TestSheet writes every style of a few pointers to $CURSOR_SHEET for looking at them.
func TestSheet(t *testing.T) {
	dir := os.Getenv("CURSOR_SHEET")
	if dir == "" {
		t.Skip("set CURSOR_SHEET to a folder")
	}
	flat, err := upk.Unpack(testdata(t, "MarvelGame.upk"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range Colors {
		for _, size := range Sizes {
			for _, n := range []string{"cursor_default", "cursor_attack", "cursor_pickup", "cursor_targetally"} {
				b, err := Preview(flat, n, Style{Color: c, Size: size})
				if err != nil {
					t.Fatal(err)
				}
				os.WriteFile(filepath.Join(dir, n+"_"+c+"_"+string(rune('0'+size/50))+".png"), b, 0o644)
			}
		}
	}
}

func TestCompressedPointersAreWrittenUncompressed(t *testing.T) {
	orig := testdata(t, "MarvelHUD_SF.upk")
	flat, err := upk.Unpack(orig)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Style{{Color: "green"}, {Color: "purple", Size: 150}} {
		out, err := Patch(flat, s)
		if err != nil {
			t.Fatalf("%+v: %v", s, err)
		}
		if _, err := upk.Repack(orig, out); err != nil {
			t.Fatalf("%+v: repack: %v", s, err)
		}
		names, _ := upk.Names(out)
		exps, _ := upk.Exports(out)
		found := 0
		for _, e := range exps {
			if !IsPointer(e.Name) {
				continue
			}
			found++
			tx, err := parse(out, names, e.Offset, e.Size)
			if err != nil || tx.format != argb {
				t.Fatalf("%+v: %s: %v %q", s, e.Name, err, tx.format)
			}
		}
		if found != 3 {
			t.Errorf("found %d pointers, want swap, invalid target and drag", found)
		}
	}
}
