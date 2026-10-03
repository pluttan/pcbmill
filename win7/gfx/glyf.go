package gfx

import "math"

// ==========================
// ===  Glyph outlines    ===
// ==========================

// gpt is an outline point in font units; off-curve points are quadratic
// control points.
type gpt struct {
	x, y float32
	on   bool
}

// outline is a decoded glyph: contour i spans pts[ends[i-1]:ends[i]].
type outline struct {
	pts  []gpt
	ends []int
}

const (
	maxGlyphDepth  = 8
	maxGlyphPoints = 1 << 16
)

// outline returns the cached decoded outline of glyph g (nil if empty
// or broken: a bad glyph renders as nothing rather than failing text).
func (f *Font) outline(g uint16) *outline {
	f.mu.Lock()
	o, ok := f.shapes[g]
	f.mu.Unlock()
	if ok {
		return o
	}
	o = f.safeLoad(g)
	f.mu.Lock()
	f.shapes[g] = o
	f.mu.Unlock()
	return o
}

func (f *Font) safeLoad(g uint16) (o *outline) {
	defer func() {
		if recover() != nil {
			o = nil
		}
	}()
	o = &outline{}
	if !f.load(o, g, 0) || len(o.pts) == 0 {
		return nil
	}
	return o
}

// glyphData returns the glyf bytes of g.
func (f *Font) glyphData(g uint16) ([]byte, bool) {
	if int(g) >= f.numGlyphs {
		return nil, false
	}
	var a, b int
	if f.locLong {
		a, b = int(u32(f.loca, 4*int(g))), int(u32(f.loca, 4*int(g)+4))
	} else {
		a, b = 2*int(u16(f.loca, 2*int(g))), 2*int(u16(f.loca, 2*int(g)+2))
	}
	if a < 0 || b < a || b > len(f.glyf) {
		return nil, false
	}
	return f.glyf[a:b], true
}

// load appends glyph g to o; composites recurse with a depth limit.
func (f *Font) load(o *outline, g uint16, depth int) bool {
	if depth > maxGlyphDepth {
		return false
	}
	data, ok := f.glyphData(g)
	if !ok {
		return false
	}
	if len(data) == 0 {
		return true // space and other blank glyphs
	}
	r := &reader{b: data}
	nc := int(r.i16())
	r.skip(8)
	if r.bad {
		return false
	}
	if nc >= 0 {
		return loadSimple(o, r, nc)
	}
	return f.loadComposite(o, r, depth)
}

func loadSimple(o *outline, r *reader, nc int) bool {
	base := len(o.pts)
	ends := make([]int, nc)
	prev := -1
	for i := range ends {
		e := int(r.u16())
		if e < prev {
			return false
		}
		ends[i] = e
		prev = e
	}
	np := prev + 1
	if r.bad || np > maxGlyphPoints || base+np > 4*maxGlyphPoints {
		return false
	}
	r.skip(int(r.u16())) // hinting instructions
	flags := make([]uint8, 0, np)
	for len(flags) < np && !r.bad {
		fl := r.u8()
		flags = append(flags, fl)
		if fl&8 != 0 {
			n := int(r.u8())
			if len(flags)+n > np {
				return false
			}
			for k := 0; k < n; k++ {
				flags = append(flags, fl)
			}
		}
	}
	if r.bad {
		return false
	}
	xs := make([]int32, np)
	var v int32
	for i, fl := range flags {
		switch {
		case fl&2 != 0:
			d := int32(r.u8())
			if fl&16 == 0 {
				d = -d
			}
			v += d
		case fl&16 == 0:
			v += int32(r.i16())
		}
		xs[i] = v
	}
	v = 0
	for i, fl := range flags {
		switch {
		case fl&4 != 0:
			d := int32(r.u8())
			if fl&32 == 0 {
				d = -d
			}
			v += d
		case fl&32 == 0:
			v += int32(r.i16())
		}
		o.pts = append(o.pts, gpt{float32(xs[i]), float32(v), fl&1 != 0})
	}
	if r.bad {
		o.pts = o.pts[:base]
		return false
	}
	for _, e := range ends {
		o.ends = append(o.ends, base+e+1)
	}
	return true
}

func (f *Font) loadComposite(o *outline, r *reader, depth int) bool {
	for {
		flags := r.u16()
		sub := r.u16()
		var a1, a2 int
		if flags&1 != 0 {
			a1, a2 = int(r.i16()), int(r.i16())
		} else if flags&2 != 0 {
			a1, a2 = int(int8(r.u8())), int(int8(r.u8()))
		} else {
			a1, a2 = int(r.u8()), int(r.u8())
		}
		a, b, c, d := 1.0, 0.0, 0.0, 1.0
		f2 := func() float64 { return float64(r.i16()) / 16384 }
		switch {
		case flags&0x08 != 0:
			a = f2()
			d = a
		case flags&0x40 != 0:
			a, d = f2(), f2()
		case flags&0x80 != 0:
			a, b, c, d = f2(), f2(), f2(), f2()
		}
		if r.bad {
			return false
		}
		start := len(o.pts)
		if !f.load(o, sub, depth+1) {
			return false
		}
		for i := start; i < len(o.pts); i++ {
			p := &o.pts[i]
			x, y := float64(p.x), float64(p.y)
			p.x, p.y = float32(a*x+c*y), float32(b*x+d*y)
		}
		var dx, dy float64
		if flags&2 != 0 {
			dx, dy = float64(a1), float64(a2)
		} else {
			// Point matching: align child point a2 with parent point a1.
			pi, ci := a1, start+a2
			if pi < 0 || pi >= start || ci >= len(o.pts) {
				return false
			}
			dx = float64(o.pts[pi].x - o.pts[ci].x)
			dy = float64(o.pts[pi].y - o.pts[ci].y)
		}
		for i := start; i < len(o.pts); i++ {
			o.pts[i].x += float32(dx)
			o.pts[i].y += float32(dy)
		}
		if len(o.pts) > 4*maxGlyphPoints {
			return false
		}
		if flags&0x20 == 0 {
			return true
		}
	}
}

// ==========================
// ===  Outline to raster ===
// ==========================

// bounds returns the outline box in pixels (y flipped).
func (o *outline) bounds(s, dx float64, ym *yMap) (x0, y0, x1, y1 float64) {
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for _, p := range o.pts {
		x, y := float64(p.x)*s+dx, -ym.at(float64(p.y))
		x0, x1 = math.Min(x0, x), math.Max(x1, x)
		y0, y1 = math.Min(y0, y), math.Max(y1, y)
	}
	return
}

// fill feeds the contours to r: x scaled by s and shifted by dx, y mapped
// by ym and flipped so that font y-up becomes screen y-down.
func (o *outline) fill(r *raster, s, dx float64, ym *yMap) {
	tr := func(p gpt) Pt { return Pt{float64(p.x)*s + dx, -ym.at(float64(p.y))} }
	var poly []Pt
	from := 0
	for _, end := range o.ends {
		c := o.pts[from:end]
		from = end
		if len(c) < 2 {
			continue
		}
		// Start on an on-curve point; synthesize one if all are off.
		first := -1
		for i, p := range c {
			if p.on {
				first = i
				break
			}
		}
		var start Pt
		if first < 0 {
			a, b := tr(c[0]), tr(c[len(c)-1])
			start = Pt{(a.X + b.X) / 2, (a.Y + b.Y) / 2}
			first = 0
		} else {
			start = tr(c[first])
			first++
		}
		poly = append(poly[:0], start)
		var ctrl Pt
		hasCtrl := false
		for k := 0; k < len(c); k++ {
			p := c[(first+k)%len(c)]
			q := tr(p)
			if p.on {
				if hasCtrl {
					poly = flattenQuad(poly, poly[len(poly)-1], ctrl, q)
					hasCtrl = false
				} else {
					poly = append(poly, q)
				}
				continue
			}
			if hasCtrl {
				// Two off-curve points imply an on-curve midpoint.
				mid := Pt{(ctrl.X + q.X) / 2, (ctrl.Y + q.Y) / 2}
				poly = flattenQuad(poly, poly[len(poly)-1], ctrl, mid)
			}
			ctrl, hasCtrl = q, true
		}
		if hasCtrl {
			poly = flattenQuad(poly, poly[len(poly)-1], ctrl, start)
		}
		r.polygon(poly)
	}
}

// ==========================
// ===  Vertical snapping ===
// ==========================

// yMap scales font y to pixels piecewise so that the baseline, x-height
// and cap height land on whole pixels. This is the useful part of light
// autohinting: flat tops of lowercase letters become one crisp row
// instead of two grey ones, while widths and spacing stay untouched.
type yMap struct {
	xh, cap float64 // font units
	x, c    float64 // snapped pixels
	s       float64 // plain scale
}

func newYMap(f *Font, s float64) *yMap {
	m := &yMap{s: s}
	if f.xHeight <= 0 || f.capHeight <= f.xHeight || s*f.unitsPerEm > 48 {
		return m // large sizes are already sharp; keep exact shapes
	}
	m.xh, m.cap = f.xHeight, f.capHeight
	m.x = math.Max(1, math.Floor(f.xHeight*s+0.5))
	m.c = math.Max(m.x+1, math.Floor(f.capHeight*s+0.5))
	return m
}

func (m *yMap) at(y float64) float64 {
	switch {
	case m.xh == 0:
		return y * m.s
	case y <= m.xh:
		return y * m.x / m.xh
	case y <= m.cap:
		return m.x + (y-m.xh)*(m.c-m.x)/(m.cap-m.xh)
	}
	return m.c + (y-m.cap)*m.s
}
