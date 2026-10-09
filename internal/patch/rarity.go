package patch

import (
	"encoding/binary"
	"fmt"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
)

// RarityCodes are the values of MarvelItem.Rarity for each rarity.
var RarityCodes = map[string]byte{"Common": 1, "Uncommon": 2, "Rare": 3, "Epic": 4, "Cosmic": 5, "Unique": 6}

// RarityRule hides items drawn with any of Classes when they drop at any of Rarities (RarityCodes values).
type RarityRule struct {
	Classes  []string
	Rarities []byte
}

// UnrealScript tokens used by the generated code.
const (
	tokInstanceVariable = 0x01
	tokJump             = 0x06
	tokJumpIfNot        = 0x07
	tokLet              = 0x0f
	tokEndParms         = 0x16
	tokContext          = 0x19
	tokFinalFunction    = 0x1c
	tokStringConst      = 0x1f
	tokPrimitiveCast    = 0x38
	castStringToName    = 0x60
	tokTrue             = 0x27
	tokNoObject         = 0x2a
	tokIntConstByte     = 0x2c
	tokEqualIntInt      = 0x9a // native 154
	tokIsA              = 0xc5 // native 197
	tokReturnNothing    = 0x0b
	refMemory           = 8 // object references take 8 bytes in memory, 4 on disk
)

// script assembles UnrealScript bytecode. Jump targets are memory offsets within the function, so it tracks the
// memory size alongside the bytes and patches label references once every label is known.
type script struct {
	b      []byte
	mem    int
	labels map[string]int
	fixups map[int]string // byte offset of a 2-byte target -> label
}

func (s *script) op(b ...byte) { s.b = append(s.b, b...); s.mem += len(b) }
func (s *script) ref(v int32) {
	s.b = binary.LittleEndian.AppendUint32(s.b, uint32(v))
	s.mem += refMemory
}

// nameOf emits name("text"): a string constant cast to a name. The class names live in the code, so the package's
// name table, which the game does not accept many additions to, stays as it is.
func (s *script) nameOf(text string) {
	s.op(tokPrimitiveCast, castStringToName, tokStringConst)
	s.op([]byte(text)...)
	s.op(0)
}
func (s *script) label(l string) { s.labels[l] = s.mem }
func (s *script) target(l string) {
	s.fixups[len(s.b)] = l
	s.op(0, 0)
}

// hideByRarity adds code to the end of MarvelItem.PostAdapterInit that hides the item, its glow and its name label
// when its class and rarity match a rule. Rarity is set just before, so every item runs it with its real rarity.
func hideByRarity(flat []byte, rs db.RarityScript, rules []RarityRule) ([]byte, error) {
	fn := rs.Function
	le := binary.LittleEndian
	if int(le.Uint32(flat[fn+40:])) != rs.Memory || int(le.Uint32(flat[fn+44:])) != rs.Storage {
		return nil, fmt.Errorf("MarvelItem.PostAdapterInit is not where the item database expects it")
	}
	at := fn + 48 + rs.Storage - 3 // before the final return
	if flat[at] != 0x04 || flat[at+1] != tokReturnNothing {
		return nil, fmt.Errorf("MarvelItem.PostAdapterInit does not end in return")
	}

	s := &script{mem: rs.Memory - 3, labels: map[string]int{}, fixups: map[int]string{}}
	for k, r := range rules {
		hit, next := fmt.Sprint("hit", k), fmt.Sprint("rule", k+1)
		for j, c := range r.Classes {
			skip := fmt.Sprint("class", k, ".", j)
			s.op(tokJumpIfNot)
			s.target(skip)
			s.op(tokIsA)
			s.nameOf(c)
			s.op(tokEndParms, tokJump)
			s.target(hit)
			s.label(skip)
		}
		s.op(tokJump)
		s.target(next)
		s.label(hit)
		for j, rarity := range r.Rarities {
			skip := fmt.Sprint("rarity", k, ".", j)
			s.op(tokJumpIfNot)
			s.target(skip)
			s.op(tokEqualIntInt, tokInstanceVariable)
			s.ref(rs.Rarity)
			s.op(tokIntConstByte, rarity, tokEndParms, tokJump)
			s.target("hide")
			s.label(skip)
		}
		s.label(next)
	}
	s.op(tokJump)
	s.target("end")
	s.label("hide")
	s.op(tokFinalFunction) // SetHidden(true): no model or glow (clicks still find it: they use the prototype bounds)
	s.ref(rs.SetHidden)
	s.op(tokTrue, tokEndParms)
	s.op(tokContext, tokInstanceVariable) // m_tooltipComp.HideTooltip()
	s.ref(rs.Tooltip)
	s.op(1+refMemory+1, 0) // memory size of the call that follows
	s.ref(0)
	s.op(0, tokFinalFunction)
	s.ref(rs.HideTooltip)
	s.op(tokEndParms)
	s.op(tokLet, tokInstanceVariable) // m_tooltipComp = None: no name label from now on
	s.ref(rs.Tooltip)
	s.op(tokNoObject)
	s.label("end")
	for at, l := range s.fixups {
		target, ok := s.labels[l]
		if !ok || target > 0xffff {
			return nil, fmt.Errorf("hide-by-rarity code too long")
		}
		le.PutUint16(s.b[at:], uint16(target))
	}

	le.PutUint32(flat[fn+40:], uint32(s.mem+3))
	le.PutUint32(flat[fn+44:], uint32(rs.Storage+len(s.b)))
	return upk.Insert(flat, at, s.b)
}
