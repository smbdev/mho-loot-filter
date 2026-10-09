package patch

import (
	"bytes"
	"encoding/binary"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
)

func TestHideByRarityAppendsToPostAdapterInit(t *testing.T) {
	d, _ := db.Load()
	orig := fixture(t, "MarvelGame.upk")
	rules := []RarityRule{
		{Classes: []string{"marvelitem_loot_origin_fame"}, Rarities: []byte{1, 2, 3, 4}},
		{Classes: []string{"marvelitem_insignia_avengers", "marvelitem_insignia_xmen"}, Rarities: []byte{3}},
	}
	out, err := MarvelGame(orig, d, map[string]bool{"Common": true}, rules)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := upk.Unpack(out)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := upk.Unpack(orig)
	if upk.NameIndex(flat, "marvelitem_loot_origin_fame") >= 0 {
		t.Fatal("class names belong in the code, not the name table")
	}
	rs := d.RarityScript
	le := binary.LittleEndian
	var fn, mem, storage int
	for shift := 0; shift < len(flat)-len(before); shift++ { // the function moved by the size of the added names
		f := rs.Function + shift
		m, st := int(le.Uint32(flat[f+40:])), int(le.Uint32(flat[f+44:]))
		if st > rs.Storage && len(flat)-len(before) == shift+st-rs.Storage {
			fn, mem, storage = f, m, st
			break
		}
	}
	if fn == 0 {
		t.Fatal("PostAdapterInit not found after the added names")
	}
	code := flat[fn+48 : fn+48+storage]
	if !bytes.Equal(code[:rs.Storage-3], before[rs.Function+48:rs.Function+48+rs.Storage-3]) {
		t.Fatal("original code changed")
	}
	if !bytes.HasSuffix(code, []byte{0x04, 0x0b, 0x53}) || mem <= rs.Memory || storage <= rs.Storage {
		t.Fatalf("bad function sizes or ending: mem %d storage %d", mem, storage)
	}
	// Decode the added code and check that every jump lands on an instruction or on the final return.
	added := code[rs.Storage-3 : storage-3]
	starts, targets := map[int]bool{mem - 3: true}, []int{}
	for i, m := 0, rs.Memory-3; i < len(added); {
		starts[m] = true
		step, extra := 0, 0
		switch added[i] {
		case tokJumpIfNot:
			targets = append(targets, int(le.Uint16(added[i+1:])))
			switch added[i+3] {
			case tokIsA: // IsA(name("class"))
				end := bytes.IndexByte(added[i+7:], 0)
				if added[i+4] != tokPrimitiveCast || added[i+5] != castStringToName || added[i+6] != tokStringConst || end < 1 || added[i+7+end+1] != tokEndParms {
					t.Fatalf("bad IsA at %d", i)
				}
				step = 3 + 4 + end + 2
			case tokEqualIntInt: // Rarity == n
				step, extra = 3+9, refMemory-4
			default:
				t.Fatalf("unexpected condition %#x at %d", added[i+3], i)
			}
		case tokJump:
			targets = append(targets, int(le.Uint16(added[i+1:])))
			step = 3
		case tokFinalFunction: // SetHidden(true)
			step, extra = 7, refMemory-4
		case tokContext: // m_tooltipComp.HideTooltip()
			step, extra = 19, 3*(refMemory-4)
		case tokLet: // m_tooltipComp = None
			step, extra = 7, refMemory-4
		default:
			t.Fatalf("unexpected token %#x at %d", added[i], i)
		}
		i, m = i+step, m+step+extra
		if i == len(added) && m != mem-3 {
			t.Fatalf("decoded memory size %d, function says %d", m, mem-3)
		}
	}
	for _, tg := range targets {
		if !starts[tg] {
			t.Fatalf("jump to %d does not land on an instruction", tg)
		}
	}
}

func TestHideByRarityRefusesAnotherVersion(t *testing.T) {
	d, _ := db.Load()
	rs := d.RarityScript
	rs.Storage++
	flat, _ := upk.Unpack(fixture(t, "MarvelGame.upk"))
	if _, err := hideByRarity(flat, rs, []RarityRule{{Classes: []string{"x"}, Rarities: []byte{1}}}); err == nil {
		t.Fatal("expected an error when the function is not where the database says")
	}
}
