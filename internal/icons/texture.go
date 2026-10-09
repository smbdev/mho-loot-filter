// Package icons reads item icons from the game's own icon packages and turns them into PNG images. Nothing is
// copied into the app: the icons come from the player's install when the window asks for them.
package icons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
)

var le = binary.LittleEndian

// texture decodes a Texture2D export (its properties, then its source art and mip maps) into a PNG of its largest
// mip. Only images stored inside the package are supported, which is how the game keeps its UI icons.
func texture(data []byte, names []string) ([]byte, error) {
	format, p, err := properties(data, names)
	if err != nil {
		return nil, err
	}
	// source art: an empty bulk data header, or one followed by its data
	if p+16 > len(data) {
		return nil, errors.New("truncated texture")
	}
	if le.Uint32(data[p:])&1 == 0 {
		p += 16 + int(le.Uint32(data[p+8:]))
	} else {
		p += 16
	}
	if p+20 > len(data) || le.Uint32(data[p:]) == 0 {
		return nil, errors.New("texture has no mip maps")
	}
	flags, size := le.Uint32(data[p+4:]), int(le.Uint32(data[p+12:]))
	p += 20
	if flags != 0 || p+size+8 > len(data) {
		return nil, errors.New("texture data is not stored in the package")
	}
	pixels := data[p : p+size]
	w, h := int(le.Uint32(data[p+size:])), int(le.Uint32(data[p+size+4:]))
	img, err := decode(format, pixels, w, h)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// properties skips an export's tagged properties, starting after its 4-byte header, and returns its pixel format
// and the offset after the None tag.
func properties(data []byte, names []string) (format string, p int, err error) {
	name := func(at int) string {
		i := int(le.Uint32(data[at:]))
		if i < 0 || i >= len(names) {
			return ""
		}
		return strings.ToLower(names[i])
	}
	p = 4
	for {
		if p+8 > len(data) {
			return "", 0, errors.New("truncated properties")
		}
		prop := name(p)
		if prop == "none" {
			return format, p + 8, nil
		}
		if p+24 > len(data) {
			return "", 0, errors.New("truncated property")
		}
		kind, size := name(p+8), int(le.Uint32(data[p+16:]))
		p += 24
		switch kind {
		case "structproperty":
			p += 8
		case "byteproperty":
			p += 8
			if prop == "format" && size == 8 && p+8 <= len(data) {
				format = name(p)
			}
		case "boolproperty":
			size = 1
		case "":
			return "", 0, errors.New("unknown property type")
		}
		p += size
	}
}

func decode(format string, px []byte, w, h int) (image.Image, error) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	switch format {
	case "pf_a8r8g8b8":
		if len(px) < w*h*4 {
			return nil, errors.New("short image")
		}
		for i := 0; i < w*h; i++ {
			img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = px[i*4+2], px[i*4+1], px[i*4], px[i*4+3]
		}
	case "pf_dxt1", "pf_dxt5":
		block := 8
		if format == "pf_dxt5" {
			block = 16
		}
		bw, bh := (w+3)/4, (h+3)/4
		if len(px) < bw*bh*block {
			return nil, errors.New("short image")
		}
		for by := 0; by < bh; by++ {
			for bx := 0; bx < bw; bx++ {
				b := px[(by*bw+bx)*block:]
				var alpha [16]uint8
				for i := range alpha {
					alpha[i] = 255
				}
				if block == 16 {
					alpha = dxt5Alpha(b[:8])
					b = b[8:]
				}
				colors := dxtColors(b, block == 8)
				bits := le.Uint32(b[4:])
				for i := 0; i < 16; i++ {
					x, y := bx*4+i%4, by*4+i/4
					if x >= w || y >= h {
						continue
					}
					c := colors[(bits>>(2*i))&3]
					c.A = min(c.A, alpha[i])
					img.SetNRGBA(x, y, c)
				}
			}
		}
	default:
		return nil, fmt.Errorf("unsupported texture format %q", format)
	}
	return img, nil
}

func rgb565(v uint16) color.NRGBA {
	r, g, b := int(v>>11), int(v>>5&63), int(v&31)
	return color.NRGBA{uint8(r * 255 / 31), uint8(g * 255 / 63), uint8(b * 255 / 31), 255}
}

// dxtColors returns the four colours of a DXT colour block. DXT1 blocks whose first colour is not greater than the
// second have three colours and transparency.
func dxtColors(b []byte, dxt1 bool) [4]color.NRGBA {
	c0, c1 := le.Uint16(b), le.Uint16(b[2:])
	a, z := rgb565(c0), rgb565(c1)
	mix := func(p, q uint8, wp, wq int) uint8 { return uint8((int(p)*wp + int(q)*wq) / (wp + wq)) }
	if c0 > c1 || !dxt1 {
		return [4]color.NRGBA{a, z,
			{mix(a.R, z.R, 2, 1), mix(a.G, z.G, 2, 1), mix(a.B, z.B, 2, 1), 255},
			{mix(a.R, z.R, 1, 2), mix(a.G, z.G, 1, 2), mix(a.B, z.B, 1, 2), 255}}
	}
	return [4]color.NRGBA{a, z, {mix(a.R, z.R, 1, 1), mix(a.G, z.G, 1, 1), mix(a.B, z.B, 1, 1), 255}, {}}
}

func dxt5Alpha(b []byte) [16]uint8 {
	a0, a1 := int(b[0]), int(b[1])
	var table [8]int
	table[0], table[1] = a0, a1
	if a0 > a1 {
		for i := 1; i <= 6; i++ {
			table[i+1] = ((7-i)*a0 + i*a1) / 7
		}
	} else {
		for i := 1; i <= 4; i++ {
			table[i+1] = ((5-i)*a0 + i*a1) / 5
		}
		table[6], table[7] = 0, 255
	}
	bits := uint64(0)
	for i := 0; i < 6; i++ {
		bits |= uint64(b[2+i]) << (8 * i)
	}
	var out [16]uint8
	for i := range out {
		out[i] = uint8(table[(bits>>(3*i))&7])
	}
	return out
}
