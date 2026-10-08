// Package upk unpacks and repacks the LZO-compressed Unreal Engine 3 packages used by Marvel Heroes Omega.
package upk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	lzo "github.com/rasky/go-lzo"
)

const (
	tag            = 0x9E2A83C1
	flagCompressed = 0x02000000
	compressionLZO = 2
)

type chunk struct{ uoff, usize, coff, csize int32 }

type summary struct {
	flagPos  int
	tablePos int // position of CompressionFlags
	chunks   []chunk
}

var le = binary.LittleEndian

func i32(b []byte, p int) int32 { return int32(le.Uint32(b[p:])) }

func parse(b []byte) (summary, error) {
	var s summary
	if len(b) < 16 || le.Uint32(b) != tag {
		return s, errors.New("not an Unreal package")
	}
	p := 12
	p += 4 + int(i32(b, p))
	s.flagPos = p
	p += 4 + 40 + 4 + 16
	if p+4 > len(b) {
		return s, errors.New("truncated header")
	}
	p += 4 + int(i32(b, p))*12 + 8
	s.tablePos = p
	if p+8 > len(b) {
		return s, errors.New("truncated header")
	}
	if i32(b, p) != compressionLZO {
		return s, fmt.Errorf("unsupported compression %d", i32(b, p))
	}
	n := int(i32(b, p+4))
	for i := 0; i < n; i++ {
		q := p + 8 + i*16
		s.chunks = append(s.chunks, chunk{i32(b, q), i32(b, q+4), i32(b, q+8), i32(b, q+12)})
	}
	if n == 0 {
		return s, errors.New("no chunks")
	}
	return s, nil
}

func decompressChunk(b []byte, c chunk) ([]byte, error) {
	q := int(c.coff)
	if le.Uint32(b[q:]) != tag {
		return nil, errors.New("bad chunk tag")
	}
	total := int(i32(b, q+12))
	q += 16
	type blk struct{ c, u int }
	var blocks []blk
	for sum := 0; sum < total; {
		bl := blk{int(i32(b, q)), int(i32(b, q+4))}
		blocks = append(blocks, bl)
		sum += bl.u
		q += 8
	}
	out := make([]byte, 0, total)
	for _, bl := range blocks {
		d, err := lzo.Decompress1X(bytes.NewReader(b[q:q+bl.c]), bl.c, bl.u)
		if err != nil {
			return nil, err
		}
		if len(d) != bl.u {
			return nil, errors.New("block size mismatch")
		}
		out = append(out, d...)
		q += bl.c
	}
	return out, nil
}

// Unpack returns the package decompressed, byte-identical to unpack() in tools/upk.py.
func Unpack(file []byte) ([]byte, error) {
	s, err := parse(file)
	if err != nil {
		return nil, err
	}
	first := int(s.chunks[0].uoff)
	hdr := append([]byte{}, file[:first]...)
	le.PutUint32(hdr[s.flagPos:], le.Uint32(hdr[s.flagPos:])&^flagCompressed)
	tableEnd := s.tablePos + 8 + len(s.chunks)*16
	var rest []byte // header bytes after the chunk table; none when the table runs past the first chunk's offset
	if tableEnd < first {
		rest = file[tableEnd:first]
	}
	hdr = append(append(hdr[:s.tablePos:s.tablePos], 0, 0, 0, 0, 0, 0, 0, 0), rest...)
	out := append(hdr, make([]byte, first-len(hdr))...)
	for _, c := range s.chunks {
		d, err := decompressChunk(file, c)
		if err != nil {
			return nil, err
		}
		if len(out) < int(c.uoff) {
			out = append(out, make([]byte, int(c.uoff)-len(out))...)
		}
		out = append(out[:c.uoff], d...)
	}
	return out, nil
}

// Repack compresses flat back into original's chunk layout (same chunk boundaries and block size).
func Repack(original, flat []byte) ([]byte, error) {
	s, err := parse(original)
	if err != nil {
		return nil, err
	}
	hdr := append([]byte{}, original[:s.chunks[0].coff]...)
	var body []byte
	pos := int32(len(hdr))
	for i, c := range s.chunks {
		bs := int(i32(original, int(c.coff)+4))
		raw := flat[c.uoff : c.uoff+c.usize]
		var sizes, blocks []byte
		total := 0
		for j := 0; j < len(raw); j += bs {
			end := min(j+bs, len(raw))
			comp := lzo.Compress1X(raw[j:end])
			sizes = le.AppendUint32(le.AppendUint32(sizes, uint32(len(comp))), uint32(end-j))
			blocks = append(blocks, comp...)
			total += len(comp)
		}
		ch := le.AppendUint32(nil, tag)
		ch = le.AppendUint32(ch, uint32(bs))
		ch = le.AppendUint32(ch, uint32(total))
		ch = le.AppendUint32(ch, uint32(c.usize))
		ch = append(append(ch, sizes...), blocks...)
		q := s.tablePos + 8 + i*16
		le.PutUint32(hdr[q:], uint32(c.uoff))
		le.PutUint32(hdr[q+4:], uint32(c.usize))
		le.PutUint32(hdr[q+8:], uint32(pos))
		le.PutUint32(hdr[q+12:], uint32(len(ch)))
		body = append(body, ch...)
		pos += int32(len(ch))
	}
	return append(hdr, body...), nil
}
