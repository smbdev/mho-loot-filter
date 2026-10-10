// Package wwise replaces a sound inside the Wwise sound banks and file packages (.pck) the game ships.
package wwise

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Alert is the sound the filter plays when an alerted item drops, built by tools/make_alert.py.
//
//go:embed alert.wem
var Alert []byte

const (
	hircSound  = 2
	pluginPCM  = 0x00010001 // Wwise PCM codec
	embedded   = 0          // media stored in the bank's DATA chunk
	dataAlign  = 16
	propVolume = 0 // AkPropID_Volume, in dB
)

var le = binary.LittleEndian

type chunk struct {
	tag  string
	body []byte
}

func chunks(b []byte) ([]chunk, error) {
	var out []chunk
	for p := 0; p < len(b); {
		if p+8 > len(b) {
			return nil, errors.New("truncated sound bank")
		}
		n := int(le.Uint32(b[p+4:]))
		if p+8+n > len(b) {
			return nil, errors.New("truncated sound bank")
		}
		out = append(out, chunk{string(b[p : p+4]), b[p+8 : p+8+n]})
		p += 8 + n
	}
	return out, nil
}

// ReplaceSound returns bank with the embedded media of sound object soundID replaced by wem, a PCM .wem file,
// played at volume dB.
func ReplaceSound(bank []byte, soundID uint32, wem []byte, volume float32) ([]byte, error) {
	cs, err := chunks(bank)
	if err != nil {
		return nil, err
	}
	var didx, data, hirc []byte
	for _, c := range cs {
		switch c.tag {
		case "DIDX":
			didx = c.body
		case "DATA":
			data = c.body
		case "HIRC":
			hirc = append([]byte{}, c.body...)
		}
	}
	if didx == nil || data == nil || hirc == nil {
		return nil, errors.New("sound bank without embedded media")
	}

	// Find the sound and make sure no other object plays the same media.
	var sound []byte
	users := map[uint32]int{}
	for p, i, n := 4, 0, int(le.Uint32(hirc)); i < n; i++ {
		size := int(le.Uint32(hirc[p+1:]))
		if hirc[p] == hircSound {
			body := hirc[p+9 : p+5+size]
			users[le.Uint32(body[5:])]++
			if le.Uint32(hirc[p+5:]) == soundID {
				sound = body
			}
		}
		p += 5 + size
	}
	if sound == nil {
		return nil, fmt.Errorf("sound %d not found", soundID)
	}
	media := le.Uint32(sound[5:])
	if sound[4] != embedded || users[media] != 1 {
		return nil, fmt.Errorf("sound %d does not own embedded media", soundID)
	}
	le.PutUint32(sound[0:], pluginPCM)
	le.PutUint32(sound[9:], uint32(len(wem)))
	if err := setVolume(sound, volume); err != nil {
		return nil, err
	}

	newDidx := append([]byte{}, didx...)
	var newData []byte
	found := false
	for i := 0; i+12 <= len(didx); i += 12 {
		id, off, size := le.Uint32(didx[i:]), int(le.Uint32(didx[i+4:])), int(le.Uint32(didx[i+8:]))
		if off+size > len(data) {
			return nil, errors.New("media index out of range")
		}
		m := data[off : off+size]
		if id == media {
			m, found = wem, true
		}
		for len(newData)%dataAlign != 0 {
			newData = append(newData, 0)
		}
		le.PutUint32(newDidx[i+4:], uint32(len(newData)))
		le.PutUint32(newDidx[i+8:], uint32(len(m)))
		newData = append(newData, m...)
	}
	if !found {
		return nil, fmt.Errorf("media %d not in the bank", media)
	}

	var out []byte
	for _, c := range cs {
		body := c.body
		switch c.tag {
		case "DIDX":
			body = newDidx
		case "DATA":
			body = newData
		case "HIRC":
			body = hirc
		}
		out = append(le.AppendUint32(append(out, c.tag...), uint32(len(body))), body...)
	}
	return out, nil
}

// setVolume sets a sound object's own volume in dB.
// After the 14-byte source come the effects override and count, an attachment flag, bus, parent, a flags byte,
// then the property count, ids and float values.
func setVolume(sound []byte, volume float32) error {
	if len(sound) < 27 || sound[15] != 0 {
		return errors.New("unsupported sound object layout")
	}
	n := int(sound[26])
	if len(sound) < 27+5*n {
		return errors.New("truncated sound object")
	}
	for i := 0; i < n; i++ {
		if sound[27+i] == propVolume {
			le.PutUint32(sound[27+n+4*i:], math.Float32bits(volume))
			return nil
		}
	}
	return errors.New("sound object has no volume to set")
}

// Extends reports whether cur is orig with bank bankID replaced by ReplaceBank: orig's bytes are kept apart from
// the bank's lookup entry, and only new data follows them.
func Extends(cur, orig []byte, bankID uint32) bool {
	e, _, err := pckBank(orig, bankID)
	if err != nil || len(cur) < len(orig) {
		return false
	}
	if _, _, err := pckBank(cur, bankID); err != nil {
		return false
	}
	return bytes.Equal(cur[:e+8], orig[:e+8]) && bytes.Equal(cur[e+16:len(orig)], orig[e+16:])
}

// bankTable returns where a file package's bank lookup table starts and how many banks it lists.
func bankTable(pck []byte) (lut, n int, err error) {
	if len(pck) < 28 || string(pck[:4]) != "AKPK" {
		return 0, 0, errors.New("not a Wwise file package")
	}
	langs, banks := int(le.Uint32(pck[12:])), int(le.Uint32(pck[16:]))
	lut = 28 + langs
	if lut+banks > len(pck) || banks < 4 {
		return 0, 0, errors.New("truncated file package")
	}
	n = int(le.Uint32(pck[lut:]))
	if 4+n*20 > banks {
		return 0, 0, errors.New("bad bank table")
	}
	return lut, n, nil
}

// pckBank locates bank bankID in a file package: the position of its lookup entry and its data.
func pckBank(pck []byte, bankID uint32) (entry int, bank []byte, err error) {
	lut, n, err := bankTable(pck)
	if err != nil {
		return 0, nil, err
	}
	for i := 0; i < n; i++ {
		e := lut + 4 + i*20
		if le.Uint32(pck[e:]) != bankID {
			continue
		}
		block, size, start := int(le.Uint32(pck[e+4:])), int(le.Uint32(pck[e+8:])), int(le.Uint32(pck[e+12:]))
		if block <= 0 || start*block+size > len(pck) {
			return 0, nil, errors.New("bank out of range")
		}
		return e, pck[start*block : start*block+size], nil
	}
	return 0, nil, fmt.Errorf("bank %d not in the file package", bankID)
}

// Bank returns the contents of bank bankID in the file package pck.
func Bank(pck []byte, bankID uint32) ([]byte, error) {
	_, b, err := pckBank(pck, bankID)
	return b, err
}

// ReplaceBank returns pck with bank bankID replaced by bank. The new bank goes at the end of the package and the
// lookup table points at it; the old copy stays in place, unused.
func ReplaceBank(pck []byte, bankID uint32, bank []byte) ([]byte, error) {
	e, _, err := pckBank(pck, bankID)
	if err != nil {
		return nil, err
	}
	block := int(le.Uint32(pck[e+4:]))
	out := append([]byte{}, pck...)
	for len(out)%block != 0 {
		out = append(out, 0)
	}
	le.PutUint32(out[e+8:], uint32(len(bank)))
	le.PutUint32(out[e+12:], uint32(len(out)/block))
	return append(out, bank...), nil
}

// Rate is the sample rate of alert sounds.
const Rate = 44100

// PCM returns a mono 16-bit sound as a Wwise PCM .wem file, laid out like the PCM sounds the game ships, raised so
// its loudest sample is just under full scale.
func PCM(samples []int16) []byte {
	peak := 1
	for _, s := range samples {
		peak = max(peak, abs(int(s)))
	}
	gain := 0.97 * 32767 / float64(peak)
	fmtChunk := le.AppendUint16(nil, 0xFFFE) // WAVE_FORMAT_EXTENSIBLE
	fmtChunk = le.AppendUint16(fmtChunk, 1)
	fmtChunk = le.AppendUint32(fmtChunk, Rate)
	fmtChunk = le.AppendUint32(fmtChunk, Rate*2)
	fmtChunk = le.AppendUint16(fmtChunk, 2)
	fmtChunk = le.AppendUint16(fmtChunk, 16)
	fmtChunk = le.AppendUint16(fmtChunk, 6)
	fmtChunk = le.AppendUint16(fmtChunk, 0)
	fmtChunk = le.AppendUint32(fmtChunk, 0x4101) // AkChannelConfig: 1 channel, standard layout, front center
	data := make([]byte, 0, 2*len(samples))
	for _, s := range samples {
		data = le.AppendUint16(data, uint16(int16(math.Round(float64(s)*gain))))
	}
	return riff(chunkBytes("fmt ", fmtChunk), chunkBytes("JUNK", make([]byte, 4)), chunkBytes("data", data))
}

// WAV returns the samples of a PCM .wem file as a plain WAV file, for playing it outside the game.
func WAV(wem []byte) ([]byte, error) {
	cs, err := chunks(wem[min(12, len(wem)):])
	if len(wem) < 12 || string(wem[:4]) != "RIFF" || string(wem[8:12]) != "WAVE" || err != nil {
		return nil, errors.New("not a PCM sound")
	}
	for _, c := range cs {
		if c.tag == "data" {
			f := le.AppendUint16(nil, 1) // PCM
			f = le.AppendUint16(f, 1)
			f = le.AppendUint32(f, Rate)
			f = le.AppendUint32(f, Rate*2)
			f = le.AppendUint16(f, 2)
			f = le.AppendUint16(f, 16)
			return riff(chunkBytes("fmt ", f), chunkBytes("data", c.body)), nil
		}
	}
	return nil, errors.New("sound has no samples")
}

func chunkBytes(tag string, body []byte) []byte {
	return append(le.AppendUint32([]byte(tag), uint32(len(body))), body...)
}

func riff(parts ...[]byte) []byte {
	body := []byte("WAVE")
	for _, p := range parts {
		body = append(body, p...)
	}
	return append(le.AppendUint32([]byte("RIFF"), uint32(len(body))), body...)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// HircEvent is the object type of an event, which the game posts to play a sound.
const HircEvent = 4

// Object is one entry of a bank's HIRC chunk. IDOffset is where its ID sits in the bank.
type Object struct {
	Type     uint8
	ID       uint32
	IDOffset int
}

// Objects lists the objects of a sound bank.
func Objects(bank []byte) ([]Object, error) {
	cs, err := chunks(bank)
	if err != nil {
		return nil, err
	}
	pos := 0
	for _, c := range cs {
		pos += 8
		if c.tag != "HIRC" {
			pos += len(c.body)
			continue
		}
		if len(c.body) < 4 {
			return nil, errors.New("truncated HIRC")
		}
		n := int(le.Uint32(c.body))
		out := make([]Object, 0, n)
		p := 4
		for range n {
			if p+9 > len(c.body) {
				return nil, errors.New("truncated HIRC")
			}
			size := int(le.Uint32(c.body[p+1:]))
			out = append(out, Object{Type: c.body[p], ID: le.Uint32(c.body[p+5:]), IDOffset: pos + p + 5})
			p += 5 + size
		}
		return out, nil
	}
	return nil, nil
}

// Banks lists the IDs of the banks in a file package.
func Banks(pck []byte) ([]uint32, error) {
	lut, n, err := bankTable(pck)
	if err != nil {
		return nil, err
	}
	out := make([]uint32, n)
	for i := range n {
		out[i] = le.Uint32(pck[lut+4+i*20:])
	}
	return out, nil
}

// ShortID is the ID Wwise gives a name: the 32-bit FNV-1 hash of the lower-case name.
func ShortID(name string) uint32 {
	h := uint32(2166136261)
	for _, c := range []byte(strings.ToLower(name)) {
		h = h*16777619 ^ uint32(c)
	}
	return h
}

// SetEventID changes the ID of an event in bank bankID of a file package, in place. An event the game asks for by
// a name whose ID no longer exists plays nothing, so this mutes it; setting the ID back unmutes it.
// offset is where the ID sits in the game's own bank. A bank the filter rebuilt (the alert's) has it elsewhere, so
// then the event is looked up. It returns false when no event has ID from or to: the file is not as expected.
func SetEventID(pck []byte, bankID uint32, offset int, from, to uint32) (bool, error) {
	_, bank, err := pckBank(pck, bankID)
	if err != nil {
		return false, err
	}
	set := func(at int) bool {
		switch le.Uint32(bank[at:]) {
		case to:
			return true
		case from:
			le.PutUint32(bank[at:], to)
			return true
		}
		return false
	}
	if offset >= 0 && offset+4 <= len(bank) && set(offset) {
		return true, nil
	}
	objs, err := Objects(bank)
	if err != nil {
		return false, err
	}
	for _, o := range objs {
		if o.Type == HircEvent && (o.ID == from || o.ID == to) {
			return set(o.IDOffset), nil
		}
	}
	return false, nil
}
