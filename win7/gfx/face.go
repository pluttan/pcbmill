package gfx

import (
	"image"
	"image/color"
	"math"
	"sync"
	"unicode/utf8"
)

// ==========================
// ===  Face              ===
// ==========================

// Face is a font at one pixel size with its own glyph cache.
type Face struct {
	Ascent, Descent, LineHeight float64 // pixels; Descent positive
	Fallback                    *Face   // used for runes the font lacks

	font  *Font
	px    float64
	scale float64
	ymap  *yMap

	mu     sync.Mutex
	glyphs map[glyphKey]*glyphBitmap
}

// subSteps is the horizontal subpixel resolution of cached glyphs.
const subSteps = 4

type glyphKey struct {
	g   uint16
	sub uint8
}

// glyphBitmap is a coverage mask placed relative to the pen position
// (integer part) and the baseline.
type glyphBitmap struct {
	x0, y0 int
	w, h   int
	a      []uint8
}

// Face returns the (cached) face for an em size of px pixels.
func (f *Font) Face(px float64) *Face {
	if px <= 0 || math.IsNaN(px) {
		px = 1
	}
	key := int64(math.Floor(px*64 + 0.5))
	f.mu.Lock()
	defer f.mu.Unlock()
	if fc := f.faces[key]; fc != nil {
		return fc
	}
	px = float64(key) / 64
	s := px / f.unitsPerEm
	fc := &Face{
		font:   f,
		px:     px,
		scale:  s,
		glyphs: map[glyphKey]*glyphBitmap{},
	}
	fc.ymap = newYMap(f, s)
	fc.Ascent = f.ascent * s
	fc.Descent = -f.descent * s
	fc.LineHeight = (f.ascent - f.descent + f.lineGap) * s
	f.faces[key] = fc
	return fc
}

// resolve finds the face in the fallback chain that has rune r.
func (f *Face) resolve(r rune) (*Face, uint16) {
	cur := f
	for i := 0; cur != nil && i < 8; i++ {
		if g := cur.font.GlyphIndex(r); g != 0 {
			return cur, g
		}
		cur = cur.Fallback
	}
	return f, 0
}

// has reports whether the chain can show r without a .notdef box.
func (f *Face) has(r rune) bool {
	_, g := f.resolve(r)
	return g != 0
}

// layout walks s glyph by glyph, applying kerning within one face.
func (f *Face) layout(s string, x float64, fn func(fc *Face, g uint16, x float64)) float64 {
	var prevFace *Face
	var prev uint16
	for _, r := range s {
		if r < 0x20 {
			continue // control characters have no visible glyph
		}
		fc, g := f.resolve(r)
		if fc == prevFace {
			x += fc.font.kerning(prev, g) * fc.scale
		}
		if fn != nil {
			fn(fc, g, x)
		}
		x += fc.font.advance(g) * fc.scale
		prevFace, prev = fc, g
	}
	return x
}

// Measure returns the advance width of s in pixels.
func (f *Face) Measure(s string) float64 {
	return f.layout(s, 0, nil)
}

// Fit shortens s with an ellipsis so that it fits into w pixels.
func (f *Face) Fit(s string, w float64) string {
	if f.Measure(s) <= w {
		return s
	}
	ell := "…"
	if !f.has('…') {
		ell = "..."
	}
	ew := f.Measure(ell)
	if ew > w {
		return ""
	}
	// Binary search over rune counts: widths grow monotonically.
	idx := make([]int, 0, len(s)+1)
	for i := range s {
		idx = append(idx, i)
	}
	idx = append(idx, len(s))
	lo, hi := 0, len(idx)-1
	for lo < hi {
		m := (lo + hi + 1) / 2
		if f.Measure(s[:idx[m]])+ew <= w {
			lo = m
		} else {
			hi = m - 1
		}
	}
	head := s[:idx[lo]]
	for len(head) > 0 {
		r, n := utf8.DecodeLastRuneInString(head)
		if r != ' ' && r != '\t' {
			break
		}
		head = head[:len(head)-n]
	}
	return head + ell
}

// ==========================
// ===  Glyph rendering   ===
// ==========================

// textLUT maps raw coverage to drawn alpha. Blending happens in sRGB, so
// light-on-dark text loses weight; a gamma lift on partial coverage
// restores the stems without hinting.
var textLUT = func() *[256]uint8 {
	var l [256]uint8
	for i := range l {
		v := float64(i) / 255
		l[i] = clamp8(math.Pow(v, 1/textGamma) * 255)
	}
	return &l
}()

const textGamma = 1.45

func (f *Face) bitmap(g uint16, sub int) *glyphBitmap {
	k := glyphKey{g, uint8(sub)}
	f.mu.Lock()
	b := f.glyphs[k]
	f.mu.Unlock()
	if b != nil {
		return b
	}
	b = &glyphBitmap{}
	if o := f.font.outline(g); o != nil {
		dx := float64(sub) / subSteps
		x0, y0, x1, y1 := o.bounds(f.scale, dx, f.ymap)
		rect := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1))+1, int(math.Ceil(y1))+1)
		if !rect.Empty() && rect.Dx() < 4096 && rect.Dy() < 4096 {
			r := getRaster(rect)
			o.fill(r, f.scale, dx, f.ymap)
			b = &glyphBitmap{x0: rect.Min.X, y0: rect.Min.Y, w: rect.Dx(), h: rect.Dy(), a: r.alpha(textLUT)}
			putRaster(r)
		}
	}
	f.mu.Lock()
	f.glyphs[k] = b
	f.mu.Unlock()
	return b
}

// Text draws s with its baseline at y and returns the pen x after the
// last glyph. The baseline snaps to whole pixels for crisp horizontals;
// x keeps quarter-pixel precision so spacing stays even.
func (c *Canvas) Text(f *Face, x, y float64, s string, col color.NRGBA) float64 {
	if f == nil {
		return x
	}
	clip := c.clip()
	by := int(math.Floor(y + 0.5))
	if col.A == 0 || by-int(f.Ascent)-2 >= clip.Max.Y || by+int(f.Descent)+2 < clip.Min.Y {
		return f.layout(s, x, nil)
	}
	ca := uint32(col.A)
	return f.layout(s, x, func(fc *Face, g uint16, gx float64) {
		q := math.Floor(gx*subSteps + 0.5)
		ix := math.Floor(q / subSteps)
		sub := int(q - ix*subSteps)
		px := int(ix)
		if px+int(fc.px*2) < clip.Min.X || px-int(fc.px) > clip.Max.X {
			return
		}
		b := fc.bitmap(g, sub)
		if b.w == 0 {
			return
		}
		at := image.Pt(px+b.x0, by+b.y0)
		r := image.Rect(at.X, at.Y, at.X+b.w, at.Y+b.h).Intersect(clip)
		for yy := r.Min.Y; yy < r.Max.Y; yy++ {
			src := b.a[(yy-at.Y)*b.w+r.Min.X-at.X:]
			blendMask(c.Img.Pix[c.Img.PixOffset(r.Min.X, yy):], src[:r.Dx()], col, ca)
		}
	})
}
