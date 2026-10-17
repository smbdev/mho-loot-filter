package patch

import (
	"bytes"
	"encoding/binary"
	"testing"

	"mholootfilter/internal/cursor"
	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
)

func TestOverlayMapOpensOutsideTownsWhenTheAvatarLoads(t *testing.T) {
	d, _ := db.Load()
	orig := fixture(t, "MarvelGame.upk")
	out, err := MarvelGame(orig, d, nil, nil, cursor.Style{}, true, nil)
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
	find := func(flat []byte) (int, int32) {
		fn, owner, err := avatarLoaded(flat)
		if err != nil {
			t.Fatal(err)
		}
		return fn, owner
	}
	le := binary.LittleEndian
	fn0, owner := find(before)
	fn, _ := find(flat)
	mem0, storage0 := int(le.Uint32(before[fn0+40:])), int(le.Uint32(before[fn0+44:]))
	mem, storage := int(le.Uint32(flat[fn+40:])), int(le.Uint32(flat[fn+44:]))

	if owner != -790 {
		t.Fatalf("PlayerOwner reference %d, want -790", owner)
	}
	// SetMapMode(false); if (!PlayerOwner.IsPlayerInTown()) { ToggleMapMode(); SetMapMode(true); }
	name := func(n string) []byte { return le.AppendUint64(nil, uint64(upk.NameIndex(flat, n))) }
	setMode := func(on byte) []byte {
		return append(append([]byte{tokVirtualFunction}, name("SetMapMode")...), on, tokEndParms)
	}
	inTown := append([]byte{tokContext, tokInstanceVariable}, le.AppendUint32(nil, uint32(owner))...)
	inTown = append(inTown, 10, 0, 0, 0, 0, 0, 0, tokVirtualFunction)
	inTown = append(append(inTown, name("IsPlayerInTown")...), tokEndParms)
	toggle := append(append([]byte{tokVirtualFunction}, name("ToggleMapMode")...), tokEndParms)
	cond := append([]byte{tokNotPreBool}, inTown...)
	cond = append(cond, tokEndParms)
	tail := append(append(setMode(tokTrue), tokReturn, tokReturnNothing), tokEndOfScript)
	code := append(append(append(setMode(tokFalse), tokJumpIfNot, 0, 0), cond...), toggle...)
	code = append(code, tail...)
	added := len(code) - 3 + 2*(refMemory-4) // two object references, 8 bytes each in memory
	if mem != mem0+added || storage != storage0+len(code)-3 {
		t.Fatalf("script sizes %d/%d, want %d/%d", mem, storage, mem0+added, storage0+len(code)-3)
	}
	end := fn + 48 + storage
	got := flat[end-len(code) : end]
	jump := le.Uint16(got[len(setMode(0))+1:])
	got = append([]byte{}, got...)
	le.PutUint16(got[len(setMode(0))+1:], 0)
	if !bytes.Equal(got, code) {
		t.Fatalf("function ends % x, want % x", got, code)
	}
	// the jump skips the toggle and the second SetMapMode, landing on the return
	if int(jump) != mem-3 {
		t.Fatalf("jump to %d, want the return at %d", jump, mem-3)
	}
}
