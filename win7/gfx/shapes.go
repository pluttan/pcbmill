package gfx

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// ==========================
// ===  Polygon helpers   ===
// ==========================

// fillPolys rasterizes polygons separately inside each region, so large
// shapes only pay for their antialiased borders, never their interior.
func (c *Canvas) fillPolys(polys [][]Pt, col color.NRGBA, regions ...image.Rectangle) {
	for _, reg := range regions {
		b := reg.Intersect(c.clip())
		if b.Empty() {
			continue
		}
		r := getRaster(b)
		for _, p := range polys {
			r.polygon(p)
		}
		r.paint(c.Img, col)
		putRaster(r)
	}
}

// roundRectPts lists the outline clockwise on screen; reverse flips it.
func roundRectPts(out []Pt, x, y, w, h, rad float64, reverse bool) []Pt {
	rad = math.Max(0, math.Min(rad, math.Min(w, h)/2))
	start := len(out)
	if rad == 0 {
		out = append(out, Pt{x, y}, Pt{x + w, y}, Pt{x + w, y + h}, Pt{x, y + h})
	} else {
		out = arcPts(out, Pt{x + rad, y + rad}, rad, math.Pi, 1.5*math.Pi)
		out = arcPts(out, Pt{x + w - rad, y + rad}, rad, 1.5*math.Pi, 2*math.Pi)
		out = arcPts(out, Pt{x + w - rad, y + h - rad}, rad, 0, 0.5*math.Pi)
		out = arcPts(out, Pt{x + rad, y + h - rad}, rad, 0.5*math.Pi, math.Pi)
	}
	if reverse {
		s := out[start:]
		for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
			s[i], s[j] = s[j], s[i]
		}
	}
	return out
}

func rectF(x0, y0, x1, y1 float64) image.Rectangle {
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
}

// ==========================
// ===  Rounded rects     ===
// ==========================

// FillRoundRect fills a rectangle with circular corners of radius.
func (c *Canvas) FillRoundRect(x, y, w, h, radius float64, col color.NRGBA) {
	if w <= 0 || h <= 0 || col.A == 0 {
		return
	}
	radius = math.Max(0, math.Min(radius, math.Min(w, h)/2))
	if radius == 0 {
		c.fillRectF(x, y, x+w, y+h, col)
		return
	}
	poly := roundRectPts(nil, x, y, w, h, radius, false)
	top := int(math.Ceil(y + radius))
	bot := int(math.Floor(y + h - radius))
	all := rectF(x, y, x+w, y+h)
	if top >= bot {
		c.fillPolys([][]Pt{poly}, col, all)
		return
	}
	c.fillPolys([][]Pt{poly}, col,
		image.Rect(all.Min.X, all.Min.Y, all.Max.X, top),
		image.Rect(all.Min.X, bot, all.Max.X, all.Max.Y))
	// Rows between the corners have vertical edges only: exact and cheap.
	c.fillRectF(x, float64(top), x+w, float64(bot), col)
}

// StrokeRoundRect strokes the outline centered on the rectangle edge.
func (c *Canvas) StrokeRoundRect(x, y, w, h, radius, width float64, col color.NRGBA) {
	if w < 0 || h < 0 || width <= 0 || col.A == 0 {
		return
	}
	hw := width / 2
	radius = math.Max(0, math.Min(radius, math.Min(w, h)/2))
	polys := [][]Pt{roundRectPts(nil, x-hw, y-hw, w+width, h+width, radius+hw, false)}
	if w > width && h > width {
		polys = append(polys, roundRectPts(nil, x+hw, y+hw, w-width, h-width, math.Max(0, radius-hw), true))
	}
	all := rectF(x-hw, y-hw, x+w+hw, y+h+hw)
	band := math.Max(radius, hw)
	top := int(math.Ceil(y + band))
	bot := int(math.Floor(y + h - band))
	if top >= bot {
		c.fillPolys(polys, col, all)
		return
	}
	left := int(math.Ceil(x + hw))
	right := int(math.Floor(x + w - hw))
	if left >= right {
		c.fillPolys(polys, col, all)
		return
	}
	// Corners as full-width bands, straight sides as thin strips; the
	// interior hole is never scanned.
	c.fillPolys(polys, col,
		image.Rect(all.Min.X, all.Min.Y, all.Max.X, top),
		image.Rect(all.Min.X, bot, all.Max.X, all.Max.Y),
		image.Rect(all.Min.X, top, left, bot),
		image.Rect(right, top, all.Max.X, bot))
}

// ==========================
// ===  Circles           ===
// ==========================

// FillCircle fills a disc centered at (cx, cy).
func (c *Canvas) FillCircle(cx, cy, r float64, col color.NRGBA) {
	if r <= 0 || col.A == 0 {
		return
	}
	poly := arcPts(nil, Pt{cx, cy}, r, 0, 2*math.Pi)
	c.fillPolys([][]Pt{poly}, col, rectF(cx-r, cy-r, cx+r+1, cy+r+1))
}

// StrokeCircle strokes a ring centered on radius r.
func (c *Canvas) StrokeCircle(cx, cy, r, width float64, col color.NRGBA) {
	if r <= 0 || width <= 0 || col.A == 0 {
		return
	}
	ro, ri := r+width/2, r-width/2
	polys := [][]Pt{arcPts(nil, Pt{cx, cy}, ro, 0, 2*math.Pi)}
	if ri > 0 {
		polys = append(polys, arcPts(nil, Pt{cx, cy}, ri, 2*math.Pi, 0))
	}
	c.fillPolys(polys, col, rectF(cx-ro, cy-ro, cx+ro+1, cy+ro+1))
}

// ==========================
// ===  Shadows           ===
// ==========================

// shadowMask is a blurred rounded rect alpha mask placed at (ox, oy).
type shadowMask struct {
	ox, oy int // offset of the mask from floor(x), floor(y)
	w, h   int
	a      []uint8
}

type shadowKey struct{ w, h, r, blur, fx, fy int32 }

var (
	shadowMu    sync.Mutex
	shadowCache = map[shadowKey]*shadowMask{}
)

// Shadow paints a soft shadow of a rounded rect like CSS box-shadow:
// blur is the CSS blur radius (Gaussian sigma = blur/2). Offset and
// spread are applied by the caller through x, y, w, h. Masks are cached
// because the same cards are redrawn every frame.
func (c *Canvas) Shadow(x, y, w, h, radius, blur float64, col color.NRGBA) {
	c.ShadowUnder(x, y, w, h, radius, blur, col, image.Rectangle{})
}

// ShadowUnder is Shadow under something opaque that will cover the
// rectangle cover: the shadow is blended only around it (a big card would
// otherwise blend its whole area for nothing).
func (c *Canvas) ShadowUnder(x, y, w, h, radius, blur float64, col color.NRGBA, cover image.Rectangle) {
	if w <= 0 || h <= 0 || col.A == 0 {
		return
	}
	if blur <= 0.25 {
		c.FillRoundRect(x, y, w, h, radius, col)
		return
	}
	q := func(v float64) int32 { return int32(math.Floor(v*4 + 0.5)) }
	fx, fy := x-math.Floor(x), y-math.Floor(y)
	k := shadowKey{q(w), q(h), q(radius), q(blur), q(fx), q(fy)}
	shadowMu.Lock()
	m := shadowCache[k]
	shadowMu.Unlock()
	if m == nil {
		m = makeShadow(float64(k.fx)/4, float64(k.fy)/4, float64(k.w)/4, float64(k.h)/4,
			float64(k.r)/4, float64(k.blur)/4)
		shadowMu.Lock()
		if len(shadowCache) >= 96 {
			shadowCache = map[shadowKey]*shadowMask{}
		}
		shadowCache[k] = m
		shadowMu.Unlock()
	}
	at := image.Pt(int(math.Floor(x))+m.ox, int(math.Floor(y))+m.oy)
	dst := image.Rect(at.X, at.Y, at.X+m.w, at.Y+m.h).Intersect(c.clip())
	ca := uint32(col.A)
	hole := cover.Intersect(dst)
	for yy := dst.Min.Y; yy < dst.Max.Y; yy++ {
		src := m.a[(yy-at.Y)*m.w+dst.Min.X-at.X:]
		if yy < hole.Min.Y || yy >= hole.Max.Y {
			blendMask(c.Img.Pix[c.Img.PixOffset(dst.Min.X, yy):], src[:dst.Dx()], col, ca)
			continue
		}
		blendMask(c.Img.Pix[c.Img.PixOffset(dst.Min.X, yy):], src[:hole.Min.X-dst.Min.X], col, ca)
		blendMask(c.Img.Pix[c.Img.PixOffset(hole.Max.X, yy):], src[hole.Max.X-dst.Min.X:dst.Dx()], col, ca)
	}
}

func makeShadow(fx, fy, w, h, radius, blur float64) *shadowMask {
	sigma := blur / 2
	pad := int(math.Ceil(3*sigma)) + 2
	mw := int(math.Ceil(fx+w)) + 2*pad
	mh := int(math.Ceil(fy+h)) + 2*pad
	r := getRaster(image.Rect(0, 0, mw, mh))
	r.polygon(roundRectPts(nil, fx+float64(pad), fy+float64(pad), w, h, radius, false))
	buf := make([]float32, mw*mh)
	for yy := 0; yy < mh; yy++ {
		var s float32
		row := r.acc[yy*r.stride : yy*r.stride+mw]
		for xx, v := range row {
			s += v
			cv := s
			if cv < 0 {
				cv = -cv
			}
			if cv > 1 {
				cv = 1
			}
			buf[yy*mw+xx] = cv
		}
	}
	putRaster(r)
	tmp := make([]float32, len(buf))
	for _, size := range gaussBoxes(sigma) {
		boxBlur(buf, tmp, mw, mh, size/2, 1, mw) // horizontal
		boxBlur(tmp, buf, mh, mw, size/2, mw, 1) // vertical
	}
	m := &shadowMask{ox: -pad, oy: -pad, w: mw, h: mh, a: make([]uint8, len(buf))}
	for i, v := range buf {
		m.a[i] = clamp8(float64(v) * 255)
	}
	return m
}

// gaussBoxes returns three box widths whose cascade approximates a
// Gaussian of the given sigma.
func gaussBoxes(sigma float64) [3]int {
	const n = 3
	wi := math.Sqrt(12*sigma*sigma/n + 1)
	wl := int(math.Floor(wi))
	if wl%2 == 0 {
		wl--
	}
	if wl < 1 {
		wl = 1
	}
	wu := wl + 2
	fl := float64(wl)
	m := int(math.Floor((12*sigma*sigma-n*fl*fl-4*n*fl-3*n)/(-4*fl-4) + 0.5))
	var out [3]int
	for i := range out {
		if i < m {
			out[i] = wl
		} else {
			out[i] = wu
		}
	}
	return out
}

// boxBlur runs a moving average of radius rad over lines of src into dst.
// step is the stride along a line and lineStep the stride between lines,
// so one routine serves both directions with the same memory layout.
func boxBlur(src, dst []float32, length, lines, rad, step, lineStep int) {
	if rad <= 0 {
		copy(dst, src)
		return
	}
	inv := 1 / float32(2*rad+1)
	for l := 0; l < lines; l++ {
		base := l * lineStep
		var s float32
		for i := 0; i < rad && i < length; i++ {
			s += src[base+i*step]
		}
		for i := 0; i < length; i++ {
			if j := i + rad; j < length {
				s += src[base+j*step]
			}
			dst[base+i*step] = s * inv
			if j := i - rad; j >= 0 {
				s -= src[base+j*step]
			}
		}
	}
}
