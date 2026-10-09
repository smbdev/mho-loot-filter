package icons

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func game(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cooked := filepath.Join(dir, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	os.MkdirAll(cooked, 0o755)
	os.WriteFile(filepath.Join(cooked, "ICO__MarvelUIIcons_SF.upk"), testdata(t, "ICO__MarvelUIIcons_SF.upk"), 0o644)
	return dir
}

func TestIconDecodesToPNG(t *testing.T) {
	s := New(game(t))
	for _, icon := range []string{"MarvelUIIcons.Item_TeamInsigniaAvengers", "Item_Tabloid"} {
		b, err := s.PNG(icon)
		if err != nil {
			t.Fatalf("%s: %v", icon, err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if r := img.Bounds(); r.Dx() < 16 || r.Dy() < 16 {
			t.Fatalf("%s: %v", icon, r)
		}
		opaque, colours := 0, map[uint32]bool{}
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a > 0x8000 {
					opaque++
					colours[r>>12<<8|g>>12<<4|b>>12] = true
				}
			}
		}
		if opaque < 100 || len(colours) < 8 {
			t.Fatalf("%s does not look like a picture: %d opaque pixels, %d colours", icon, opaque, len(colours))
		}
	}
	if _, err := s.PNG("MarvelUIIcons.NoSuchIcon"); err == nil {
		t.Fatal("expected an error for a missing icon")
	}
	if _, err := New(t.TempDir()).PNG("MarvelUIIcons.Item_TeamInsigniaAvengers"); err == nil {
		t.Fatal("expected an error without the game")
	}
}
