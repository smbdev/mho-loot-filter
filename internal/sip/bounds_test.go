package sip

import (
	"bytes"
	"testing"

	"mholootfilter/internal/db"
)

// pickingFlag reports whether the prototype's own Bounds struct has ComplexPickingOnly set to true.
func pickingFlag(t *testing.T, proto []byte, pick db.Picking) bool {
	t.Helper()
	l, err := parse(proto, nil)
	if err != nil {
		t.Fatal(err)
	}
	at, own := l.structs[pick.BoundsField]
	if !own {
		return false
	}
	want := le.AppendUint64(nil, pick.FlagField)
	want = append(want, typeBool)
	want = le.AppendUint64(want, 1)
	return bytes.Contains(proto[at[0]:at[1]], want)
}

func boundsOf(t *testing.T, d *db.DB, path string) db.Bounds {
	t.Helper()
	p := protoFor(t, d, path)
	if p.Bounds == nil {
		t.Fatalf("%s has no bounds in the database", path)
	}
	return d.Bounds[*p.Bounds]
}

func TestUnclickableInheritedBounds(t *testing.T) {
	_, pak := loadPak(t)
	d, _ := db.Load()
	const thor = "Calligraphy/Entity/Items/Insignias/Prototypes/Insignia002.prototype"
	proto, _ := pak.Read(thor)
	def := boundsOf(t, d, thor)
	out, err := Unclickable(proto, def, d.Picking)
	if err != nil {
		t.Fatal(err)
	}
	if !pickingFlag(t, out, d.Picking) {
		t.Fatal("bounds not flagged")
	}
	if again, err := Unclickable(out, def, d.Picking); err != nil || !bytes.Equal(again, out) {
		t.Fatalf("second pass changed the prototype: %v", err)
	}
	c1, ok1 := UnrealClass(proto, fieldSet(d))
	if c2, ok2 := UnrealClass(out, fieldSet(d)); c1 != c2 || ok1 != ok2 {
		t.Fatal("UnrealClass disturbed")
	}
	pinned, err := PinBounds(proto, def, d.Picking)
	if err != nil || pickingFlag(t, pinned, d.Picking) {
		t.Fatalf("pin must copy the bounds unflagged: %v", err)
	}
	l, _ := parse(pinned, nil)
	at := l.structs[d.Picking.BoundsField]
	if hexed := pinned[at[0]:at[1]]; len(hexed)*2 != len(def.Data) {
		t.Fatal("pinned bounds differ from the original")
	}
}

func TestUnclickableOwnBounds(t *testing.T) {
	_, pak := loadPak(t)
	d, _ := db.Load()
	var path string
	for _, it := range d.Items {
		for _, p := range it.Protos {
			if p.BoundsOwn && path == "" {
				path = p.Path
			}
		}
	}
	proto, _ := pak.Read(path)
	out, err := Unclickable(proto, boundsOf(t, d, path), d.Picking)
	if err != nil || !pickingFlag(t, out, d.Picking) {
		t.Fatalf("%s: own bounds not flagged: %v", path, err)
	}
	if len(out) != len(proto)+17 {
		t.Fatalf("%s: grew by %d bytes, want one 17-byte field", path, len(out)-len(proto))
	}
	if pinned, _ := PinBounds(proto, boundsOf(t, d, path), d.Picking); !bytes.Equal(pinned, proto) {
		t.Fatal("pinning a prototype with its own bounds must not change it")
	}
}
