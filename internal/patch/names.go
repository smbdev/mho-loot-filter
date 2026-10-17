package patch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"

	"mholootfilter/internal/upk"
)

// NameColours are the colours the filter offers for item names on the ground, as HTML colours.
var NameColours = map[string]string{
	"white": "FFFFFF", "grey": "CCCCCC", "red": "EE2222", "orange": "FF9900", "yellow": "FCDB1C",
	"green": "00FF00", "cyan": "00FFFF", "blue": "1E90FF", "purple": "CC00FF", "pink": "FF34B3",
}

// ColourHex returns the six hex digits a name colour is written with: a key of NameColours, or "#RRGGBB".
func ColourHex(c string) (string, bool) {
	if hex, ok := NameColours[c]; ok {
		return hex, true
	}
	if len(c) != 7 || c[0] != '#' {
		return "", false
	}
	for _, r := range c[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", false
		}
	}
	return strings.ToUpper(c[1:]), true
}

// More UnrealScript tokens: native functions of Core.Object called by their native index.
const (
	tokLocalVariable  = 0x00
	tokArrayElement   = 0x10
	tokIntZero        = 0x25
	tokStructMember   = 0x35
	tokEmptyParmValue = 0x4a // an optional parameter left out
	tokConcatStrStr   = 0x70 // native 112: A $ B
	tokInStr          = 0x7e // native 126: InStr(S, T, optional bSearchFromRight, bIgnoreCase, StartPos)
	tokMid            = 0x7f // native 127: Mid(S, I, optional J)
	tokLeft           = 0x80 // native 128: Left(S, I)
	tokAddIntInt      = 0x92 // native 146
	tokGreaterEqualII = 0x99 // native 153
)

// The ground label's HTML, as the game builds it: the item name is wrapped in a font tag that sets its colour.
//
//	<p align='Left'><font face='Strat2Medium' color='#FFFFFF' size='16'><font color='#CC00FF'>Name</font></font></p>
const (
	colourTag    = "<font color='#"
	colourTagLen = len(colourTag) + 6 + 2 // the tag up to the name: colour digits and '>
	stackMark    = " ["                   // a stack count follows the name: Nanotech Filament [27]
)

// tooltipShown returns the offset of MarvelGFxActorTooltipComp.CallShowTooltip, which hands an entity's label to
// the HUD movie, and the object references its label code needs.
func tooltipShown(flat []byte) (fn int, refs [5]int32, err error) {
	exports, err := upk.Exports(flat)
	if err != nil {
		return 0, refs, err
	}
	find := func(path ...string) int32 {
		for j, e := range exports {
			if e.Name != path[len(path)-1] {
				continue
			}
			o, ok := e.Outer, true
			for k := len(path) - 2; k >= 0 && ok; k-- {
				ok = o > 0 && exports[o-1].Name == path[k]
				if ok {
					o = exports[o-1].Outer
				}
			}
			if ok && o == 0 {
				return int32(j + 1)
			}
		}
		return 0
	}
	f := find("marvelgfxactortooltipcomp", "callshowtooltip")
	refs = [5]int32{
		find("marvelgfxactortooltipcomp", "callshowtooltip", "data"),
		find("marvelentity", "tooltipdata"),
		find("marvelentity", "tooltipdata", "tooltipsections"),
		find("marvelentity", "tooltipsection"),
		find("marvelentity", "tooltipsection", "htmltext"),
	}
	if f == 0 || slices.Contains(refs[:], 0) {
		return 0, refs, fmt.Errorf("MarvelGFxActorTooltipComp.CallShowTooltip or the tooltip data not found")
	}
	return exports[f-1].Offset, refs, nil
}

// latin1 returns text as the single-byte string constants of the game's script hold it.
func latin1(text string) ([]byte, bool) {
	out := make([]byte, 0, len(text))
	for _, r := range text {
		if r == 0 || r > 0xff {
			return nil, false
		}
		out = append(out, byte(r))
	}
	return out, true
}

// labelNames is the name list for one colour as the label code searches it: "|Name A|Name B|". Names the code
// cannot hold are left out; a name with & is listed as the label writes it too.
func labelNames(names []string) []byte {
	var b bytes.Buffer
	b.WriteByte('|')
	for _, n := range names {
		forms := []string{n}
		if strings.Contains(n, "&") {
			forms = append(forms, strings.ReplaceAll(n, "&", "&amp;"))
		}
		for _, form := range forms {
			text, ok := latin1(form)
			if !ok || n == "" || bytes.IndexByte(text, '|') >= 0 {
				continue
			}
			b.Write(text)
			b.WriteByte('|')
		}
	}
	return b.Bytes()
}

// colourLabels adds code to the start of MarvelGFxActorTooltipComp.CallShowTooltip that gives the ground labels
// of the items in colours (colour, see ColourHex -> item names) their colour: it reads the name out of the label's HTML,
// looks it up in each colour's name list, and rewrites the colour of the name's font tag. Item tooltips and slots
// are drawn from the game data, so they keep the game's colours.
func colourLabels(flat []byte, colours map[string][]string) ([]byte, error) {
	fn, refs, err := tooltipShown(flat)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	mem, storage := int(le.Uint32(flat[fn+40:])), int(le.Uint32(flat[fn+44:]))
	body := flat[fn+48 : fn+48+storage]
	// The function starts with three Nothing tokens, then jumps past the movie call when there is no movie;
	// both of its jumps target its return.
	const target = 0x47
	if storage < 12 || !bytes.HasPrefix(body, []byte{0x0b, 0x0b, 0x0b, tokJumpIfNot, target, 0}) ||
		body[storage-6] != tokJump || le.Uint16(body[storage-5:]) != target || body[storage-3] != tokReturn {
		return nil, fmt.Errorf("MarvelGFxActorTooltipComp.CallShowTooltip is not the code this filter knows")
	}

	s := &script{mem: 3, labels: map[string]int{}, fixups: map[int]string{}}
	str := func(text string) { s.op(tokStringConst); s.op([]byte(text)...); s.op(0) }
	html := func(modify byte) { // Data.tooltipSections[0].htmlText
		s.op(tokStructMember)
		s.ref(refs[4])
		s.ref(refs[3])
		s.op(0, modify, tokArrayElement, tokIntZero, tokStructMember)
		s.ref(refs[2])
		s.ref(refs[1])
		s.op(0, modify, tokLocalVariable)
		s.ref(refs[0])
	}
	inStr := func(a, b func()) {
		s.op(tokInStr)
		a()
		b()
		s.op(tokEmptyParmValue, tokEmptyParmValue, tokEmptyParmValue, tokEndParms)
	}
	text := func(t string) func() { return func() { str(t) } }
	tag := func() { inStr(func() { html(0) }, text(colourTag)) } // where the name's font tag starts
	plus := func(n byte) func() {
		return func() { s.op(tokAddIntInt); tag(); s.op(tokIntConstByte, n, tokEndParms) }
	}
	concat := func(a, b func()) { s.op(tokConcatStrStr); a(); b(); s.op(tokEndParms) }
	// name: Mid(html, tag+len, InStr(Mid(html, tag+len), "</font>")), then cut at a stack count:
	// Left(name $ " [", InStr(name $ " [", " ["))
	name := func() {
		s.op(tokMid)
		html(0)
		plus(byte(colourTagLen))()
		inStr(func() { s.op(tokMid); html(0); plus(byte(colourTagLen))(); s.op(tokEmptyParmValue, tokEndParms) }, text("</font>"))
		s.op(tokEndParms)
	}
	marked := func() { concat(name, text(stackMark)) }
	key := func() { // "|" $ name $ "|"
		concat(func() {
			concat(text("|"), func() { s.op(tokLeft); marked(); inStr(marked, text(stackMark)); s.op(tokEndParms) })
		}, text("|"))
	}

	s.op(tokJumpIfNot) // only labels with a coloured name
	s.target("end")
	s.op(tokGreaterEqualII)
	tag()
	s.op(tokIntZero, tokEndParms)
	keys := make([]string, 0, len(colours))
	for c := range colours {
		keys = append(keys, c)
	}
	slices.Sort(keys)
	for i, c := range keys {
		hex, ok := ColourHex(c)
		list := labelNames(colours[c])
		if !ok || len(list) < 2 {
			continue
		}
		next := fmt.Sprint("colour", i)
		s.op(tokJumpIfNot) // the name is in this colour's list
		s.target(next)
		s.op(tokGreaterEqualII, tokInStr, tokStringConst)
		s.op(list...)
		s.op(0)
		key()
		s.op(tokEmptyParmValue, tokEmptyParmValue, tokEmptyParmValue, tokEndParms, tokIntZero, tokEndParms)
		// html = Left(html, tag+14) $ hex $ Mid(html, tag+20)
		s.op(tokLet)
		html(1)
		concat(func() {
			concat(func() { s.op(tokLeft); html(0); plus(byte(len(colourTag)))(); s.op(tokEndParms) }, text(hex))
		}, func() { s.op(tokMid); html(0); plus(byte(len(colourTag) + 6))(); s.op(tokEmptyParmValue, tokEndParms) })
		s.op(tokJump)
		s.target("end")
		s.label(next)
	}
	s.label("end")
	for at, l := range s.fixups {
		le.PutUint16(s.b[at:], uint16(s.labels[l]))
	}
	added := s.mem - 3
	if target+added > 0xffff {
		return nil, fmt.Errorf("too many coloured names for the label code")
	}
	le.PutUint16(flat[fn+48+4:], uint16(target+added))
	le.PutUint16(flat[fn+48+storage-5:], uint16(target+added))
	le.PutUint32(flat[fn+40:], uint32(mem+added))
	le.PutUint32(flat[fn+44:], uint32(storage+len(s.b)))
	return upk.Insert(flat, fn+48+3, s.b)
}
