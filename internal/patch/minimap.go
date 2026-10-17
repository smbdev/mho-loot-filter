package patch

import (
	"encoding/binary"
	"fmt"

	"mholootfilter/internal/upk"
)

const (
	tokVirtualFunction = 0x1b
	tokReturn          = 0x04
	tokEndOfScript     = 0x53
	tokNotPreBool      = 0x81 // native 129
)

// avatarLoaded returns the offset of MarvelHUD.InitAvatarHUDData, which the game calls on every map load once the
// region is known and the map mode has been reset, and the object reference of HUD.PlayerOwner. Earlier events,
// such as SetMiniMapTexture, still see region Unknown, where IsPlayerInTown is always false.
func avatarLoaded(flat []byte) (fn int, playerOwner int32, err error) {
	exports, err := upk.Exports(flat)
	if err != nil {
		return 0, 0, err
	}
	imports, err := upk.Imports(flat)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range exports {
		if e.Name == "initavatarhuddata" && e.Outer > 0 && exports[e.Outer-1].Name == "marvelhud" {
			fn = e.Offset
		}
	}
	for i, im := range imports {
		if im.Name == "playerowner" && im.Outer < 0 && imports[-im.Outer-1].Name == "hud" {
			playerOwner = int32(-i - 1)
		}
	}
	if fn == 0 || playerOwner == 0 {
		return 0, 0, fmt.Errorf("MarvelHUD.InitAvatarHUDData or HUD.PlayerOwner not found")
	}
	return fn, playerOwner, nil
}

// overlayMap adds code to the end of MarvelHUD.InitAvatarHUDData that opens the overlay map that Tab shows in
// areas and terminals, and the minimap in hubs such as Avengers Tower:
//
//	SetMapMode(false); if (!PlayerOwner.IsPlayerInTown()) { ToggleMapMode(); SetMapMode(true); }
//
// Tab cycles minimap, overlay, hidden. SetMapMode does not bring the map back from hidden, ToggleMapMode does: the
// three calls end on the overlay from any of the three.
func overlayMap(flat []byte) ([]byte, error) {
	fn, playerOwner, err := avatarLoaded(flat)
	if err != nil {
		return nil, err
	}
	name := func(n string) []byte { // names take 8 bytes on disk and in memory
		i := upk.NameIndex(flat, n)
		if i < 0 {
			return nil
		}
		return binary.LittleEndian.AppendUint64(nil, uint64(i))
	}
	setMapMode, toggle, inTown := name("SetMapMode"), name("ToggleMapMode"), name("IsPlayerInTown")
	if setMapMode == nil || toggle == nil || inTown == nil {
		return nil, fmt.Errorf("SetMapMode, ToggleMapMode or IsPlayerInTown not in MarvelGame.upk")
	}
	le := binary.LittleEndian
	mem, storage := int(le.Uint32(flat[fn+40:])), int(le.Uint32(flat[fn+44:]))
	at := fn + 48 + storage - 3
	if flat[at] != tokReturn || flat[at+1] != tokReturnNothing || flat[at+2] != tokEndOfScript {
		return nil, fmt.Errorf("MarvelHUD.InitAvatarHUDData does not end in return")
	}
	s := &script{mem: mem - 3, labels: map[string]int{}, fixups: map[int]string{}} // jumps count from the function start
	set := func(on byte) { s.op(tokVirtualFunction); s.op(setMapMode...); s.op(on, tokEndParms) }
	set(tokFalse)
	s.op(tokJumpIfNot)
	s.target("end")
	s.op(tokNotPreBool, tokContext, tokInstanceVariable) // !PlayerOwner.IsPlayerInTown()
	s.ref(playerOwner)
	s.op(1+8+1, 0) // memory size of the call that follows
	s.ref(0)
	s.op(0, tokVirtualFunction)
	s.op(inTown...)
	s.op(tokEndParms, tokEndParms)
	s.op(tokVirtualFunction)
	s.op(toggle...)
	s.op(tokEndParms)
	set(tokTrue)
	s.label("end")
	for p, l := range s.fixups {
		le.PutUint16(s.b[p:], uint16(s.labels[l]))
	}
	le.PutUint32(flat[fn+40:], uint32(s.mem+3))
	le.PutUint32(flat[fn+44:], uint32(storage+len(s.b)))
	return upk.Insert(flat, at, s.b)
}
