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
	exports, err := upk.Exports(flat)
	if err != nil {
		t.Fatal(err)
	}
	export := func(outer, name string) upk.Export {
		for _, e := range exports {
			if e.Name == name && e.Outer > 0 && exports[e.Outer-1].Name == outer {
				return e
			}
		}
		t.Fatalf("%s.%s not found", outer, name)
		return upk.Export{}
	}
	fn := export("marvelitem", "postadapterinit").Offset
	mem, storage := int(le.Uint32(flat[fn+40:])), int(le.Uint32(flat[fn+44:]))

	// The item mesh lets clicks hit its bounding box: a BoolProperty tag set to true before its None tag.
	mesh := export("default__marvelitem", "initialskeletalmesh")
	line := upk.NameIndex(flat, lineCheckName)
	tag := le.AppendUint64(nil, uint64(line))
	tag = le.AppendUint64(tag, uint64(rs.BoolProperty))
	tag = append(tag, make([]byte, 8)...)
	tag = append(tag, 1)
	tag = le.AppendUint64(tag, uint64(upk.NameIndex(flat, "None")))
	if line < 0 || !bytes.Contains(flat[mesh.Offset:mesh.Offset+mesh.Size], tag) {
		t.Fatal("item mesh does not switch on bounding-box clicks")
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
		case tokContext: // m_tooltipComp.HideTooltip() or Mesh.SetTraceBlocking(false, false)
			step, extra = 19, 3*(refMemory-4)
			if int32(le.Uint32(added[i+14:])) == rs.SetTraceBlocking {
				if !bytes.Equal(added[i+18:i+21], []byte{tokFalse, tokFalse, tokEndParms}) {
					t.Fatalf("bad SetTraceBlocking at %d", i)
				}
				step = 21
			}
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
