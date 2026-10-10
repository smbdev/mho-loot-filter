// Package cursor restyles the game's mouse pointers: 64x64 pictures stored uncompressed in MarvelGame.upk. The
// blue arrow can take another colour (the coloured badges and the red attack arrow keep theirs) and every pointer
// can be drawn larger.
package cursor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"slices"
	"strings"

	"mholootfilter/internal/icons"
	"mholootfilter/internal/upk"
)

var le = binary.LittleEndian

// Style is how the pointers look. The zero Style is the game's own.
type Style struct {
	Color string `json:"color"` // one of Colors; "" keeps the game's blue
	Size  int    `json:"size"`  // one of Sizes, in percent; 0 keeps the game's size
}

// Colors are the arrow colours on offer, with the hue and saturation each one paints.
var Colors = []string{"", "yellow", "green", "pink", "purple", "white"}

var paint = map[string]struct{ hue, sat float64 }{
	"yellow": {52, 1}, "green": {115, .9}, "pink": {325, 1}, "purple": {275, .85}, "white": {0, 0},
}

// Sizes are the pointer sizes on offer, in percent of the game's.
var Sizes = []int{0, 150, 200}

func (s Style) Validate() error {
	if !slices.Contains(Colors, s.Color) {
		return fmt.Errorf("Unknown pointer colour %q", s.Color)
	}
	if !slices.Contains(Sizes, s.Size) {
		return fmt.Errorf("Unknown pointer size %d", s.Size)
	}
	return nil
}

// Default reports whether s leaves the pointers as the game has them.
func (s Style) Default() bool { return s == Style{} }

// Restyle returns img recoloured and scaled. The arrow's tip sits in the top left corner, so scaling from there
// keeps the click point where it was.
func Restyle(img image.Image, s Style) *image.NRGBA {
	src := image.NewNRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			src.Set(x, y, recolor(color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA), s.Color))
		}
	}
	scale := 1.0
	if s.Size != 0 {
		scale = float64(s.Size) / 100
	}
	n := src.Rect.Dx() // larger pointers need a canvas twice the size
	if s.Size != 0 {
		n *= 2
	}
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	if scale == 1 && n == src.Rect.Dx() {
		copy(out.Pix, src.Pix)
		return out
	}
	// bilinear, on premultiplied colour so transparent edges do not darken
	w, h := src.Rect.Dx(), src.Rect.Dy()
	at := func(x, y int) [4]float64 {
		x, y = max(0, min(x, w-1)), max(0, min(y, h-1))
		c := src.NRGBAAt(x, y)
		a := float64(c.A) / 255
		return [4]float64{float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, float64(c.A)}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := (float64(x)+.5)/scale-.5, (float64(y)+.5)/scale-.5
			if fx > float64(w) || fy > float64(h) {
				continue
			}
			x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
			tx, ty := fx-float64(x0), fy-float64(y0)
			var v [4]float64
			for i, p := range [4][4]float64{at(x0, y0), at(x0+1, y0), at(x0, y0+1), at(x0+1, y0+1)} {
				wx, wy := 1-tx, 1-ty
				if i%2 == 1 {
					wx = tx
				}
				if i >= 2 {
					wy = ty
				}
				for k := range v {
					v[k] += p[k] * wx * wy
				}
			}
			if v[3] < .5 {
				continue
			}
			a := v[3] / 255
			out.SetNRGBA(x, y, color.NRGBA{clamp(v[0] / a), clamp(v[1] / a), clamp(v[2] / a), clamp(v[3])})
		}
	}
	return out
}

func clamp(v float64) uint8 { return uint8(max(0, min(255, math.Round(v)))) }

// recolor paints the blue of the arrow in the chosen colour, keeping its shading.
func recolor(c color.NRGBA, name string) color.NRGBA {
	p, ok := paint[name]
	if !ok || c.A == 0 {
		return c
	}
	h, s, l := hsl(c)
	if s < .3 || h < 185 || h > 250 {
		return c
	}
	// the arrow's blue is dark; lift it so light colours stay light
	l = .3 + .7*l
	if name == "white" {
		l = .55 + .45*l
	}
	r, g, b := rgb(p.hue, p.sat, l)
	return color.NRGBA{r, g, b, c.A}
}

func hsl(c color.NRGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	l = (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	s = d / (1 - math.Abs(2*l-1))
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func rgb(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	return clamp((r + m) * 255), clamp((g + m) * 255), clamp((b + m) * 255)
}

// IsPointer reports whether an export name is one of the game's pointer pictures.
func IsPointer(name string) bool {
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "cursor_") || strings.HasPrefix(name, "cursors_") || strings.HasPrefix(name, "drag_cursor_")
}

// argb is the uncompressed pixel format every restyled pointer is written in.
const argb = "pf_a8r8g8b8"

// texture is a single-mip square Texture2D export, with the positions of the values a restyle changes.
type texture struct {
	size, origSize [2]int // positions of SizeX/SizeY and OriginalSizeX/OriginalSizeY values
	mipTail        int    // position of MipTailBaseIdx's value, or -1
	format         string // pixel format, lower case
	formatAt       int    // position of Format's value (a name)
	header         int    // the mip's bulk data header
	pixels, n      int    // the mip's pixels and their length; its width and height follow them
	side           int
}

// bytesFor is the length of a side x side mip in format.
func bytesFor(format string, side int) int {
	switch format {
	case "pf_dxt1":
		return side * side / 2
	case "pf_dxt5":
		return side * side
	}
	return side * side * 4
}

// parse finds the parts of a pointer texture at export offset off.
func parse(flat []byte, names []string, off, size int) (texture, error) {
	t := texture{mipTail: -1}
	name := func(at int) string {
		i := int(le.Uint32(flat[at:]))
		if i < 0 || i >= len(names) {
			return ""
		}
		return strings.ToLower(names[i])
	}
	end := off + size
	p := off + 4
	for {
		if p+24 > end {
			return t, errors.New("truncated properties")
		}
		prop := name(p)
		if prop == "none" {
			p += 8
			break
		}
		kind, n := name(p+8), int(le.Uint32(flat[p+16:]))
		v := p + 24
		switch kind {
		case "byteproperty":
			v += 8
			if prop == "format" {
				t.format, t.formatAt = name(v), v
			}
		case "structproperty":
			v += 8
		case "boolproperty":
			n = 1
		case "":
			return t, errors.New("unknown property type")
		}
		switch prop {
		case "sizex":
			t.size[0] = v
		case "sizey":
			t.size[1] = v
		case "originalsizex":
			t.origSize[0] = v
		case "originalsizey":
			t.origSize[1] = v
		case "miptailbaseidx":
			t.mipTail = v
		}
		p = v + n
	}
	if (t.format != argb && t.format != "pf_dxt1" && t.format != "pf_dxt5") || t.size[0] == 0 || t.size[1] == 0 {
		return t, fmt.Errorf("unsupported pixel format %q", t.format)
	}
	if le.Uint32(flat[p:])&1 == 0 { // source art kept in the package
		p += 16 + int(le.Uint32(flat[p+8:]))
	} else {
		p += 16
	}
	if le.Uint32(flat[p:]) != 1 {
		return t, errors.New("pointer textures have one mip")
	}
	t.header = p + 4
	t.pixels = t.header + 16
	t.n = int(le.Uint32(flat[t.header+8:]))
	if le.Uint32(flat[t.header:]) != 0 || t.pixels+t.n+8 > end {
		return t, errors.New("mip is not stored in the package")
	}
	t.side = int(le.Uint32(flat[t.pixels+t.n:]))
	if bytesFor(t.format, t.side) != t.n || int(le.Uint32(flat[t.pixels+t.n+4:])) != t.side {
		return t, errors.New("pointer textures are square")
	}
	return t, nil
}

func (t texture) image(flat []byte) (*image.NRGBA, error) {
	return icons.Decode(t.format, flat[t.pixels:t.pixels+t.n], t.side, t.side)
}

func encode(img *image.NRGBA) []byte {
	px := make([]byte, len(img.Pix))
	for i := 0; i < len(px); i += 4 {
		px[i], px[i+1], px[i+2], px[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	return px
}

// Patch returns the unpacked package flat with every pointer picture in it restyled.
func Patch(flat []byte, s Style) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Default() {
		return flat, nil
	}
	names, err := upk.Names(flat)
	if err != nil {
		return nil, err
	}
	argbName := upk.NameIndex(flat, argb)
	exps, err := upk.Exports(flat)
	if err != nil {
		return nil, err
	}
	// last first: growing a texture moves every export after it, but none before
	var pointers []upk.Export
	for _, e := range exps {
		if IsPointer(e.Name) {
			pointers = append(pointers, e)
		}
	}
	slices.SortFunc(pointers, func(a, b upk.Export) int { return b.Offset - a.Offset })
	for _, e := range pointers {
		t, err := parse(flat, names, e.Offset, e.Size)
		if err != nil {
			return nil, fmt.Errorf("pointer %s: %w", e.Name, err)
		}
		src, err := t.image(flat)
		if err != nil {
			return nil, fmt.Errorf("pointer %s: %w", e.Name, err)
		}
		if t.format != argb && argbName < 0 {
			return nil, fmt.Errorf("pointer %s: the package has no name for uncompressed pictures", e.Name)
		}
		img := Restyle(src, s)
		px := encode(img)
		if grow := len(px) - t.n; grow > 0 {
			if flat, err = upk.Insert(flat, t.pixels+t.n, make([]byte, grow)); err != nil {
				return nil, err
			}
		} else {
			flat = append([]byte{}, flat...)
		}
		if t.format != argb { // compressed pointers are written uncompressed
			le.PutUint32(flat[t.formatAt:], uint32(argbName))
			le.PutUint32(flat[t.formatAt+4:], 0)
		}
		side := uint32(img.Rect.Dx())
		copy(flat[t.pixels:], px)
		le.PutUint32(flat[t.pixels+len(px):], side)
		le.PutUint32(flat[t.pixels+len(px)+4:], side)
		le.PutUint32(flat[t.header+4:], uint32(len(px))) // element count
		le.PutUint32(flat[t.header+8:], uint32(len(px))) // size on disk
		for _, at := range append(t.size[:], t.origSize[:]...) {
			le.PutUint32(flat[at:], side)
		}
		if t.mipTail >= 0 {
			le.PutUint32(flat[t.mipTail:], uint32(math.Log2(float64(side))))
		}
	}
	return flat, nil
}

// Package returns the compressed package original with its pointers restyled. The game's own style gives back
// original itself: compressing it again would not give the same bytes.
func Package(original []byte, s Style) ([]byte, error) {
	if s.Default() {
		return original, s.Validate()
	}
	flat, err := upk.Unpack(original)
	if err != nil {
		return nil, err
	}
	if flat, err = Patch(flat, s); err != nil {
		return nil, err
	}
	return upk.Repack(original, flat)
}

// HUDPackage holds the pointers for swapping items, an invalid target and dragging; the rest are in MarvelGame.upk.
const HUDPackage = "MarvelHUD_SF.upk"

// Preview returns the pointer picture name from the unpacked package flat, restyled, as a PNG.
func Preview(flat []byte, name string, s Style) ([]byte, error) {
	names, err := upk.Names(flat)
	if err != nil {
		return nil, err
	}
	exps, err := upk.Exports(flat)
	if err != nil {
		return nil, err
	}
	for _, e := range exps {
		if e.Name != strings.ToLower(name) {
			continue
		}
		t, err := parse(flat, names, e.Offset, e.Size)
		if err != nil {
			continue
		}
		src, err := t.image(flat)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		err = png.Encode(&buf, Restyle(src, s))
		return buf.Bytes(), err
	}
	return nil, fmt.Errorf("no pointer %s", name)
}
