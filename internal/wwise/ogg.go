package wwise

// Turning the game's Wwise Vorbis sounds into Ogg Vorbis files a browser can play. This follows ww2ogg by Adam
// Gashlin (BSD licence, see ww2ogg-LICENSE), limited to the layout the game's sounds use: the vorb data inside a
// 0x42-byte fmt chunk, 2-byte packet headers, a stripped setup header and codebooks from the aoTuV 6.03 library.
// Unlike ww2ogg it also writes each page's granule position, so players know how long the sound is.

import (
	_ "embed"
	"errors"
	"fmt"
)

//go:embed packed_codebooks_aoTuV_603.bin
var codebooks []byte

// bitReader reads a byte slice LSB first, as Vorbis packs its fields.
type bitReader struct {
	b   []byte
	pos int // in bits
}

var errOutOfBits = errors.New("sound data ends early")

func (r *bitReader) get(n int) (uint32, error) {
	var v uint32
	for i := 0; i < n; i++ {
		if r.pos/8 >= len(r.b) {
			return 0, errOutOfBits
		}
		if r.b[r.pos/8]>>(r.pos%8)&1 != 0 {
			v |= 1 << i
		}
		r.pos++
	}
	return v, nil
}

// oggWriter builds Ogg pages from bits written LSB first.
type oggWriter struct {
	out     []byte
	payload []byte
	cur     byte
	nbits   int
	pages   uint32
	granule uint64
	err     error
}

const maxPayload = 255 * 255

func (w *oggWriter) put(v uint32, n int) {
	for i := 0; i < n; i++ {
		if v>>i&1 != 0 {
			w.cur |= 1 << w.nbits
		}
		w.nbits++
		if w.nbits == 8 {
			w.flushBits()
		}
	}
}

func (w *oggWriter) flushBits() {
	if w.nbits == 0 {
		return
	}
	if len(w.payload) == maxPayload {
		w.err = errors.New("packet too large for an Ogg page")
		return
	}
	w.payload = append(w.payload, w.cur)
	w.cur, w.nbits = 0, 0
}

// flushPage ends the current packet with a page of its own.
func (w *oggWriter) flushPage(last bool) {
	w.flushBits()
	if len(w.payload) == 0 {
		return
	}
	// one lacing value per started 255 bytes, plus a final short one (0 when the packet fills its segments)
	segments := min(len(w.payload)/255+1, 255)
	var flags byte
	if w.pages == 0 {
		flags |= 2
	}
	if last {
		flags |= 4
	}
	page := []byte{'O', 'g', 'g', 'S', 0, flags}
	page = le.AppendUint64(page, w.granule)
	page = le.AppendUint32(page, 1) // stream serial number
	page = le.AppendUint32(page, w.pages)
	page = le.AppendUint32(page, 0) // checksum, filled in below
	page = append(page, byte(segments))
	for left := len(w.payload); len(page) < 27+segments; left -= 255 {
		page = append(page, byte(min(left, 255)))
	}
	page = append(page, w.payload...)
	le.PutUint32(page[22:], oggCRC(page))
	w.out = append(w.out, page...)
	w.pages++
	w.payload = w.payload[:0]
}

var crcTable = func() (t [256]uint32) {
	for i := range t {
		r := uint32(i) << 24
		for range 8 {
			if r&0x80000000 != 0 {
				r = r<<1 ^ 0x04c11db7
			} else {
				r <<= 1
			}
		}
		t[i] = r
	}
	return t
}()

func oggCRC(b []byte) uint32 {
	var c uint32
	for _, x := range b {
		c = c<<8 ^ crcTable[byte(c>>24)^x]
	}
	return c
}

func ilog(v uint32) int {
	n := 0
	for ; v != 0; v >>= 1 {
		n++
	}
	return n
}

// quantvals is the number of values in a lookup type 1 codebook (from Tremor).
func quantvals(entries, dims uint32) uint32 {
	bits := ilog(entries)
	vals := entries >> (uint32(bits-1) * (dims - 1) / dims)
	for {
		acc, acc1 := uint64(1), uint64(1)
		for range dims {
			acc *= uint64(vals)
			acc1 *= uint64(vals) + 1
		}
		if acc <= uint64(entries) && acc1 > uint64(entries) {
			return vals
		}
		if acc > uint64(entries) {
			vals--
		} else {
			vals++
		}
	}
}

// rebuildCodebook expands codebook id of the packed library into a standard Vorbis codebook.
func rebuildCodebook(id uint32, w *oggWriter) error {
	end := int(le.Uint32(codebooks[len(codebooks)-4:]))
	count := (len(codebooks) - end) / 4
	if int(id) >= count-1 {
		return fmt.Errorf("unknown codebook %d", id)
	}
	from, to := int(le.Uint32(codebooks[end+4*int(id):])), int(le.Uint32(codebooks[end+4*int(id)+4:]))
	r := &bitReader{b: codebooks[from:to]}
	f := func(n int) uint32 {
		v, err := r.get(n)
		if err != nil && w.err == nil {
			w.err = err
		}
		return v
	}

	dims, entries := f(4), f(14)
	w.put(0x564342, 24)
	w.put(dims, 16)
	w.put(entries, 24)
	ordered := f(1)
	w.put(ordered, 1)
	if ordered != 0 {
		w.put(f(5), 5) // initial length
		for cur := uint32(0); cur < entries; {
			n := f(ilog(entries - cur))
			w.put(n, ilog(entries-cur))
			cur += n
			if cur > entries {
				return errors.New("codebook entries out of range")
			}
		}
	} else {
		lengthBits, sparse := f(3), f(1)
		if lengthBits == 0 || lengthBits > 5 {
			return errors.New("bad codeword length")
		}
		w.put(sparse, 1)
		for range entries {
			present := uint32(1)
			if sparse != 0 {
				present = f(1)
				w.put(present, 1)
			}
			if present != 0 {
				w.put(f(int(lengthBits)), 5)
			}
		}
	}
	lookup := f(1)
	w.put(lookup, 4)
	if lookup == 1 {
		w.put(f(32), 32) // minimum
		w.put(f(32), 32) // delta
		valueBits := f(4)
		w.put(valueBits, 4)
		w.put(f(1), 1) // sequence flag
		for range quantvals(entries, dims) {
			w.put(f(int(valueBits)+1), int(valueBits)+1)
		}
	}
	if w.err != nil {
		return w.err
	}
	if r.pos/8+1 != to-from {
		return errors.New("codebook size mismatch")
	}
	return nil
}

// WemToOgg converts a Wwise Vorbis .wem file into an Ogg Vorbis file.
func WemToOgg(wem []byte) ([]byte, error) {
	if len(wem) < 12 || string(wem[:4]) != "RIFF" || string(wem[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF WAVE sound")
	}
	var fmtc, data []byte
	for p := 12; p+8 <= len(wem); {
		n := int(le.Uint32(wem[p+4:]))
		if p+8+n > len(wem) {
			return nil, errors.New("sound chunk truncated")
		}
		switch string(wem[p : p+4]) {
		case "fmt ":
			fmtc = wem[p+8 : p+8+n]
		case "data":
			data = wem[p+8 : p+8+n]
		case "vorb":
			return nil, errors.New("unsupported Wwise Vorbis layout")
		}
		p += 8 + n
	}
	if len(fmtc) != 0x42 || data == nil {
		return nil, errors.New("unsupported sound format")
	}
	if le.Uint16(fmtc) != 0xFFFF {
		return nil, errors.New("not Wwise Vorbis")
	}
	channels, rate, avgBytes := uint32(le.Uint16(fmtc[2:])), le.Uint32(fmtc[4:]), le.Uint32(fmtc[8:])
	vorb := fmtc[0x18:]
	samples := le.Uint32(vorb)
	modSignal := le.Uint32(vorb[4:])
	modPackets := modSignal != 0x4A && modSignal != 0x4B && modSignal != 0x69 && modSignal != 0x70
	setupAt, audioAt := int(le.Uint32(vorb[0x10:])), int(le.Uint32(vorb[0x14:]))
	bs0, bs1 := uint32(vorb[0x28]), uint32(vorb[0x29])
	if channels == 0 || setupAt+2 > len(data) || audioAt > len(data) || bs0 > 15 || bs1 > 15 {
		return nil, errors.New("bad sound header")
	}

	w := &oggWriter{}
	vorbis := func(kind uint32) {
		w.put(kind, 8)
		for _, c := range []byte("vorbis") {
			w.put(uint32(c), 8)
		}
	}
	// identification
	vorbis(1)
	w.put(0, 32)
	w.put(channels, 8)
	w.put(rate, 32)
	w.put(0, 32)
	w.put(avgBytes*8, 32)
	w.put(0, 32)
	w.put(bs0, 4)
	w.put(bs1, 4)
	w.put(1, 1)
	w.flushPage(false)
	// comment
	vorbis(3)
	vendor := "converted from Audiokinetic Wwise by MHO Loot Filter"
	w.put(uint32(len(vendor)), 32)
	for _, c := range []byte(vendor) {
		w.put(uint32(c), 8)
	}
	w.put(0, 32)
	w.put(1, 1)
	w.flushPage(false)

	// setup, rebuilt from its stripped form
	setupSize := int(le.Uint16(data[setupAt:]))
	if setupAt+2+setupSize > len(data) {
		return nil, errors.New("setup packet truncated")
	}
	r := &bitReader{b: data[setupAt+2 : setupAt+2+setupSize]}
	var rerr error
	f := func(n int) uint32 {
		v, err := r.get(n)
		if err != nil && rerr == nil {
			rerr = err
		}
		return v
	}
	copyBits := func(n int) uint32 { v := f(n); w.put(v, n); return v }
	vorbis(5)
	books := copyBits(8) + 1
	for range books {
		if err := rebuildCodebook(f(10), w); err != nil {
			return nil, err
		}
	}
	w.put(0, 6)  // time domain transforms
	w.put(0, 16) // placeholder
	floors := copyBits(6) + 1
	for range floors {
		w.put(1, 16) // floor type 1
		partitions := copyBits(5)
		classes := make([]uint32, partitions)
		var maxClass uint32
		for j := range classes {
			classes[j] = copyBits(4)
			maxClass = max(maxClass, classes[j])
		}
		dims := make([]uint32, maxClass+1)
		for j := range dims {
			dims[j] = copyBits(3) + 1
			sub := copyBits(2)
			if sub != 0 && copyBits(8) >= books {
				return nil, errors.New("bad floor masterbook")
			}
			for range 1 << sub {
				if b := int(copyBits(8)) - 1; b >= int(books) {
					return nil, errors.New("bad floor subclass book")
				}
			}
		}
		copyBits(2) // multiplier
		rangeBits := int(copyBits(4))
		for _, c := range classes {
			for range dims[c] {
				copyBits(rangeBits)
			}
		}
	}
	residues := copyBits(6) + 1
	for range residues {
		kind := f(2)
		if kind > 2 {
			return nil, errors.New("bad residue type")
		}
		w.put(kind, 16)
		copyBits(24) // begin
		copyBits(24) // end
		copyBits(24) // partition size
		classifications := copyBits(6) + 1
		if copyBits(8) >= books {
			return nil, errors.New("bad residue classbook")
		}
		cascade := make([]uint32, classifications)
		for j := range cascade {
			low := copyBits(3)
			var high uint32
			if copyBits(1) != 0 {
				high = copyBits(5)
			}
			cascade[j] = high*8 + low
		}
		for _, c := range cascade {
			for k := range 8 {
				if c&(1<<k) != 0 && copyBits(8) >= books {
					return nil, errors.New("bad residue book")
				}
			}
		}
	}
	mappings := copyBits(6) + 1
	for range mappings {
		w.put(0, 16) // mapping type 0
		submaps := uint32(1)
		if copyBits(1) != 0 {
			submaps = copyBits(4) + 1
		}
		if copyBits(1) != 0 { // square polar coupling
			steps := copyBits(8) + 1
			bits := ilog(channels - 1)
			for range steps {
				m, a := copyBits(bits), copyBits(bits)
				if m == a || m >= channels || a >= channels {
					return nil, errors.New("bad channel coupling")
				}
			}
		}
		if copyBits(2) != 0 {
			return nil, errors.New("mapping reserved field set")
		}
		if submaps > 1 {
			for range channels {
				if copyBits(4) >= submaps {
					return nil, errors.New("bad mapping mux")
				}
			}
		}
		for range submaps {
			copyBits(8) // time config
			if copyBits(8) >= floors {
				return nil, errors.New("bad floor mapping")
			}
			if copyBits(8) >= residues {
				return nil, errors.New("bad residue mapping")
			}
		}
	}
	modes := copyBits(6) + 1
	blockflag := make([]bool, modes)
	modeBits := ilog(modes - 1)
	for i := range blockflag {
		blockflag[i] = copyBits(1) != 0
		w.put(0, 16) // window type
		w.put(0, 16) // transform type
		if copyBits(8) >= mappings {
			return nil, errors.New("bad mode mapping")
		}
	}
	w.put(1, 1)
	if rerr != nil {
		return nil, rerr
	}
	if (r.pos+7)/8 != setupSize || setupAt+2+setupSize != audioAt {
		return nil, errors.New("setup packet size mismatch")
	}
	w.flushPage(false)

	// audio packets, each on a page of its own
	block := [2]uint64{1 << bs0, 1 << bs1}
	var prevFlag, started bool
	modeOf := func(at, size int) (uint32, bool) {
		if size == 0 || at >= len(data) {
			return 0, false
		}
		if modPackets {
			return uint32(data[at]) & (1<<modeBits - 1), true
		}
		return uint32(data[at]) >> 1 & (1<<modeBits - 1), true
	}
	for at := audioAt; at < len(data); {
		if at+2 > len(data) {
			return nil, errors.New("packet header truncated")
		}
		size := int(le.Uint16(data[at:]))
		body, next := at+2, at+2+size
		if next > len(data) {
			return nil, errors.New("packet truncated")
		}
		mode, ok := modeOf(body, size)
		if ok && int(mode) >= len(blockflag) {
			return nil, errors.New("bad packet mode")
		}
		flag := ok && blockflag[mode]
		if ok { // samples this packet completes: a quarter of each of the two windows that overlap
			if started {
				w.granule += block[b2i(prevFlag)]/4 + block[b2i(flag)]/4
			}
			started = true
		}
		if modPackets && size > 0 {
			w.put(0, 1) // packet type: audio
			w.put(mode, modeBits)
			if flag { // long window: previous and next window types
				nextFlag := false
				if next+2 <= len(data) {
					if n, ok := modeOf(next+2, int(le.Uint16(data[next:]))); ok && int(n) < len(blockflag) {
						nextFlag = blockflag[n]
					}
				}
				w.put(uint32(b2i(prevFlag)), 1)
				w.put(uint32(b2i(nextFlag)), 1)
			}
			w.put(uint32(data[body])>>modeBits, 8-modeBits)
		} else if size > 0 {
			w.put(uint32(data[body]), 8)
		}
		if ok {
			prevFlag = flag
		}
		for _, c := range data[min(body+1, next):next] {
			w.put(uint32(c), 8)
		}
		last := next >= len(data)
		if last {
			w.granule = min(w.granule, uint64(samples))
		}
		w.flushPage(last)
		at = next
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.out, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
