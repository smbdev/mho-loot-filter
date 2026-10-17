package patch

import (
	"bytes"
	"encoding/binary"
	"testing"

	"mholootfilter/internal/cursor"
	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
)

func TestLabelColoursRecolourNamedItemsBeforeTheLabelIsShown(t *testing.T) {
	d, _ := db.Load()
	orig := fixture(t, "MarvelGame.upk")
	colours := map[string][]string{"orange": {"Nanotech Filament", "Salt & Pepper"}, "cyan": {"Insignia of Thor"}}
	out, err := MarvelGame(orig, d, nil, nil, cursor.Style{}, false, colours)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := upk.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := upk.Unpack(orig)
	names, _ := upk.Names(before)
	if after, _ := upk.Names(flat); len(after) != len(names) {
		t.Fatal("the name table must stay as it is")
	}
	le := binary.LittleEndian
	fn0, _, _ := tooltipShown(before)
	fn, _, _ := tooltipShown(flat)
	mem0, st0 := int(le.Uint32(before[fn0+40:])), int(le.Uint32(before[fn0+44:]))
	mem, st := int(le.Uint32(flat[fn+40:])), int(le.Uint32(flat[fn+44:]))
	body := flat[fn+48 : fn+48+st]
	added := mem - mem0
	if added <= 0 || st <= st0 {
		t.Fatalf("no code added: memory %d -> %d", mem0, mem)
	}
	// The original code follows the added code, its two jumps moved by the added size.
	orig0 := before[fn0+48 : fn0+48+st0]
	rest := body[3+st-st0:]
	if !bytes.Equal(body[:3], orig0[:3]) || rest[0] != tokJumpIfNot || int(le.Uint16(rest[1:])) != 0x47+added ||
		int(le.Uint16(body[st-5:])) != 0x47+added || !bytes.Equal(body[st-3:], orig0[st0-3:]) {
		t.Fatalf("original code not kept with its jumps moved: % x", rest[:3])
	}
	for _, want := range []string{"|Nanotech Filament|Salt & Pepper|Salt &amp; Pepper|", "|Insignia of Thor|", "FF9900", "00FFFF", "<font color='#", "</font>"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("code does not hold %q", want)
		}
	}
	if bytes.Contains(body, []byte("|Gem|")) {
		t.Fatal("only coloured items belong in the code")
	}
}

func TestNoColoursLeaveTheTooltipCodeAlone(t *testing.T) {
	d, _ := db.Load()
	orig := fixture(t, "MarvelGame.upk")
	out, err := MarvelGame(orig, d, nil, nil, cursor.Style{}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := upk.Unpack(out)
	before, _ := upk.Unpack(orig)
	fn0, _, _ := tooltipShown(before)
	fn, _, _ := tooltipShown(flat)
	if !bytes.Equal(before[fn0:fn0+48+int(binary.LittleEndian.Uint32(before[fn0+44:]))], flat[fn:fn+48+int(binary.LittleEndian.Uint32(flat[fn+44:]))]) {
		t.Fatal("CallShowTooltip changed without colours")
	}
}

func TestColourHexTakesPaletteNamesAndHexColours(t *testing.T) {
	cases := map[string]string{"orange": "FF9900", "#12ab34": "12AB34", "#E69F00": "E69F00"}
	for in, want := range cases {
		if got, ok := ColourHex(in); !ok || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "plaid", "#12ab3", "#12ab345", "12ab34", "#12ab3g", "#'><x"} {
		if _, ok := ColourHex(bad); ok {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestLabelColoursTakeHexColours(t *testing.T) {
	d, _ := db.Load()
	out, err := MarvelGame(fixture(t, "MarvelGame.upk"), d, nil, nil, cursor.Style{}, false, map[string][]string{"#12ab34": {"Nanotech Filament"}})
	if err != nil {
		t.Fatal(err)
	}
	flat, _ := upk.Unpack(out)
	fn, _, _ := tooltipShown(flat)
	body := flat[fn+48 : fn+48+int(binary.LittleEndian.Uint32(flat[fn+44:]))]
	if !bytes.Contains(body, []byte("12AB34")) || !bytes.Contains(body, []byte("|Nanotech Filament|")) {
		t.Fatal("the hex colour and its names are not in the label code")
	}
}
