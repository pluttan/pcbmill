package r3

import (
	"image/color"
	"math"
)

// ==========================
// ===  Styles            ===
// ==========================

// LineStyle controls lines and markers. Hidden is the alpha factor for parts
// behind the surface (0 = not drawn); Bias pulls depth towards the camera in
// mm so paths lying on a surface do not z-fight.
type LineStyle struct {
	Width  float64
	Color  color.NRGBA
	Hidden float64
	Bias   float64
}

// Marker is the shape drawn by Points.
type Marker int

const (
	Disc Marker = iota
	Square
	Diamond
	Ring
)

// seg is a clipped screen segment; k is 1/d in perspective, d in ortho,
// both linear along the screen segment.
type seg struct {
	ax, ay, bx, by float64
	ka, kb         float64
	ca, cb         [4]float64
	minY, maxY     int32
	marker         bool
}

// ==========================
// ===  Lines             ===
// ==========================

// Lines draws a polyline (strip) or independent pairs, antialiased with
// round ends and a per-pixel depth test. Lines do not write depth.
func (f *Frame) Lines(c *Camera, pts []Vec, strip bool, s LineStyle) {
	f.lines(c, pts, nil, strip, s)
}

// LinesColored is Lines with a colour per point, interpolated along segments.
func (f *Frame) LinesColored(c *Camera, pts []Vec, cols []color.NRGBA, strip bool, s LineStyle) {
	if len(cols) < len(pts) {
		cols = nil
	}
	f.lines(c, pts, cols, strip, s)
}

func colf(c color.NRGBA) [4]float64 {
	return [4]float64{float64(c.R), float64(c.G), float64(c.B), float64(c.A) / 255}
}

func (f *Frame) lines(c *Camera, pts []Vec, cols []color.NRGBA, strip bool, s LineStyle) {
	if c == nil || len(pts) < 2 {
		return
	}
	v := c.view(f.W, f.H)
	width := s.Width
	if !(width > 0) {
		width = 1
	}
	width = minf(width, 256)
	pad := width/2 + 1
	step := 2
	if strip {
		step = 1
	}
	ns := (len(pts) - 1 + step - 1) / step
	if step == 2 {
		ns = len(pts) / 2
	}
	if cap(f.segs) < ns {
		f.segs = make([]seg, ns)
	}
	f.segs = f.segs[:ns]
	base := colf(s.Color)
	const chunk = 2048
	parallel((ns+chunk-1)/chunk, func(k int) {
		for j := k * chunk; j < ns && j < (k+1)*chunk; j++ {
			i := j * step
			ca, cb := base, base
			if cols != nil {
				ca, cb = colf(cols[i]), colf(cols[i+1])
			}
			f.makeSeg(&f.segs[j], &v, pts[i], pts[i+1], ca, cb, pad)
		}
	})
	f.drawSegs(&v, width, s)
}

// makeSeg clips the segment to the near plane and the guard band
// (Liang-Barsky in clip space) and stores it in screen space.
func (f *Frame) makeSeg(sg *seg, v *view, p0, p1 Vec, ca, cb [4]float64, pad float64) {
	sg.minY, sg.maxY = 1, 0 // empty row range: binning skips it
	if p0.bad() || p1.bad() {
		return
	}
	var a, b cvert
	a.xs, a.ys, a.d, a.w = v.toView(p0)
	b.xs, b.ys, b.d, b.w = v.toView(p1)
	t0, t1 := 0.0, 1.0
	for k := 0; k < 5; k++ {
		da, db := clipVal(v, &a, k), clipVal(v, &b, k)
		if da < 0 && db < 0 {
			return
		}
		if da < 0 {
			t0 = maxf(t0, da/(da-db))
		} else if db < 0 {
			t1 = minf(t1, da/(da-db))
		}
	}
	if !(t0 <= t1) {
		return
	}
	a2, b2 := a, b
	if t0 > 0 {
		a2 = lerpPos(&a, &b, t0)
	}
	if t1 < 1 {
		b2 = lerpPos(&a, &b, t1)
	}
	if !(a2.w > 0 && b2.w > 0) {
		return
	}
	sg.ax, sg.ay = v.cx+a2.xs/a2.w, v.cy+a2.ys/a2.w
	sg.bx, sg.by = v.cx+b2.xs/b2.w, v.cy+b2.ys/b2.w
	sg.marker = false
	for k := 0; k < 4; k++ {
		sg.ca[k] = ca[k] + (cb[k]-ca[k])*t0
		sg.cb[k] = ca[k] + (cb[k]-ca[k])*t1
	}
	if v.ortho {
		sg.ka, sg.kb = a2.d, b2.d
	} else {
		sg.ka, sg.kb = 1/a2.d, 1/b2.d
	}
	lo, hi := minf(sg.ay, sg.by)-pad, maxf(sg.ay, sg.by)+pad
	if hi < 0 || lo > float64(f.H) || maxf(sg.ax, sg.bx)+pad < 0 || minf(sg.ax, sg.bx)-pad > float64(f.W) {
		return
	}
	sg.minY = int32(maxi(0, int(lo)))
	sg.maxY = int32(mini(f.H-1, int(hi)))
}

// lerpPos interpolates only the position part of a clip vertex.
func lerpPos(a, b *cvert, t float64) cvert {
	return cvert{
		xs: a.xs + (b.xs-a.xs)*t, ys: a.ys + (b.ys-a.ys)*t,
		d: a.d + (b.d-a.d)*t, w: a.w + (b.w-a.w)*t,
	}
}

// depthKey turns the interpolated k into a biased z-buffer key.
func depthKey(v *view, k, bias float64) float32 {
	d := k
	if !v.ortho {
		d = 1 / k
	}
	return float32(v.key(d - bias))
}

// drawSegs rasterizes f.segs band-parallel into the coverage buffer, keeping
// the strongest coverage per pixel so joints of a polyline do not double up,
// then composites each band once.
func (f *Frame) drawSegs(v *view, width float64, s LineStyle) {
	if len(f.segs) == 0 {
		return
	}
	nb, bh := f.bands()
	for len(f.touch) < nb {
		f.touch = append(f.touch, nil)
	}
	for len(f.sbins) < nb {
		f.sbins = append(f.sbins, nil)
	}
	for b := 0; b < nb; b++ {
		f.sbins[b] = f.sbins[b][:0]
	}
	for i := range f.segs {
		sg := &f.segs[i]
		for b := int(sg.minY) / bh; b <= int(sg.maxY)/bh && b < nb; b++ {
			f.sbins[b] = append(f.sbins[b], int32(i))
		}
	}
	hidden := clampf(s.Hidden, 0, 1)
	hw := maxf(width/2, 0.5)
	thin := minf(width, 1)
	parallel(nb, func(b int) {
		y0, y1 := b*bh, mini((b+1)*bh, f.H)
		touch := f.touch[b][:0]
		for _, i := range f.sbins[b] {
			sg := &f.segs[i]
			if sg.marker {
				touch = f.markerBand(v, sg, y0, y1, hidden, s.Bias, touch)
			} else {
				touch = f.segBand(v, sg, y0, y1, hw, thin, hidden, s.Bias, touch)
			}
		}
		f.composite(touch)
		f.touch[b] = touch
	})
}

func (f *Frame) segBand(v *view, sg *seg, y0, y1 int, hw, thin, hidden, bias float64, touch []int32) []int32 {
	R := hw + 0.5
	dx, dy := sg.bx-sg.ax, sg.by-sg.ay
	l2 := dx*dx + dy*dy
	inv := 0.0
	if l2 > 1e-12 {
		inv = 1 / l2
	}
	ya, yb := maxi(int(sg.minY), y0), mini(int(sg.maxY), y1-1)
	for y := ya; y <= yb; y++ {
		yc := float64(y) + 0.5
		// x extent of the capsule on this row, from the part of the
		// segment within R vertically.
		ta, tb := 0.0, 1.0
		if math.Abs(dy) > 1e-9 {
			ta, tb = (yc-R-sg.ay)/dy, (yc+R-sg.ay)/dy
			if ta > tb {
				ta, tb = tb, ta
			}
			ta, tb = maxf(ta, 0), minf(tb, 1)
			if ta > tb {
				continue
			}
		} else if math.Abs(sg.ay-yc) > R {
			continue
		}
		xa, xb := sg.ax+dx*ta, sg.ax+dx*tb
		lo := maxi(0, int(math.Floor(minf(xa, xb)-R)))
		hi := mini(f.W-1, int(math.Ceil(maxf(xa, xb)+R)))
		for x := lo; x <= hi; x++ {
			px := float64(x) + 0.5
			t := ((px-sg.ax)*dx + (yc-sg.ay)*dy) * inv
			t = clampf(t, 0, 1)
			ex, ey := px-(sg.ax+dx*t), yc-(sg.ay+dy*t)
			cov := R - math.Sqrt(ex*ex+ey*ey)
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			i := y*f.W + x
			alpha := cov * thin * (sg.ca[3] + (sg.cb[3]-sg.ca[3])*t)
			if !(depthKey(v, sg.ka+(sg.kb-sg.ka)*t, bias) >= f.Z[i]) {
				alpha *= hidden
			}
			touch = f.accum(i, alpha, sg, t, touch)
		}
	}
	return touch
}

// accum keeps the strongest coverage per pixel; ties go to the later
// primitive so overlapping markers stack in draw order.
func (f *Frame) accum(i int, alpha float64, sg *seg, t float64, touch []int32) []int32 {
	a8 := uint32(alpha*255 + 0.5)
	if a8 == 0 {
		return touch
	}
	if a8 > 255 {
		a8 = 255
	}
	old := f.acc[i]
	if a8 < old>>24 {
		return touch
	}
	if old == 0 {
		touch = append(touch, int32(i))
	}
	r := uint32(to8(sg.ca[0] + (sg.cb[0]-sg.ca[0])*t))
	g := uint32(to8(sg.ca[1] + (sg.cb[1]-sg.ca[1])*t))
	b := uint32(to8(sg.ca[2] + (sg.cb[2]-sg.ca[2])*t))
	f.acc[i] = a8<<24 | r<<16 | g<<8 | b
	return touch
}

func (f *Frame) composite(touch []int32) {
	pix := f.Img.Pix
	for _, i := range touch {
		v := f.acc[i]
		f.acc[i] = 0
		// integer blend: uint32->float is a slow call on 386
		a := int32(v >> 24)
		k := 255 - a
		o := int(i/int32(f.W))*f.Img.Stride + 4*int(i%int32(f.W))
		p := pix[o : o+4 : o+4]
		p[0] = uint8((int32(v>>16&0xff)*a + int32(p[0])*k + 127) / 255)
		p[1] = uint8((int32(v>>8&0xff)*a + int32(p[1])*k + 127) / 255)
		p[2] = uint8((int32(v&0xff)*a + int32(p[2])*k + 127) / 255)
		p[3] = uint8((255*a + int32(p[3])*k + 127) / 255)
	}
}

// ==========================
// ===  Markers           ===
// ==========================

// Points draws screen-size markers (size = diameter in pixels), depth tested
// like lines. cols may be nil to use s.Color.
func (f *Frame) Points(c *Camera, pts []Vec, cols []color.NRGBA, size float64, m Marker, s LineStyle) {
	if c == nil || len(pts) == 0 {
		return
	}
	if len(cols) < len(pts) {
		cols = nil
	}
	if !(size > 0) {
		size = 4
	}
	size = minf(size, 256)
	v := c.view(f.W, f.H)
	f.segs = f.segs[:0]
	base := colf(s.Color)
	rad := size / 2
	for i, p := range pts {
		if p.bad() {
			continue
		}
		xs, ys, d, w := v.toView(p)
		if !(d >= v.near) || !(w > 0) || math.Abs(xs) > v.guard*w || math.Abs(ys) > v.guard*w {
			continue
		}
		sg := seg{ax: v.cx + xs/w, ay: v.cy + ys/w, marker: true}
		sg.bx, sg.by = rad, float64(m) // marker radius and shape ride along
		sg.ca = base
		if cols != nil {
			sg.ca = colf(cols[i])
		}
		sg.cb = sg.ca
		if v.ortho {
			sg.ka = d
		} else {
			sg.ka = 1 / d
		}
		sg.kb = sg.ka
		lo, hi := sg.ay-rad-1, sg.ay+rad+1
		if hi < 0 || lo > float64(f.H) || sg.ax+rad+1 < 0 || sg.ax-rad-1 > float64(f.W) {
			continue
		}
		sg.minY = int32(maxi(0, int(lo)))
		sg.maxY = int32(mini(f.H-1, int(hi)))
		f.segs = append(f.segs, sg)
	}
	f.drawSegs(&v, 1, s)
}

func (f *Frame) markerBand(v *view, sg *seg, y0, y1 int, hidden, bias float64, touch []int32) []int32 {
	rad, shape := sg.bx, Marker(sg.by)
	key := depthKey(v, sg.ka, bias)
	ring := maxf(1.2, rad*0.35)
	ya, yb := maxi(int(sg.minY), y0), mini(int(sg.maxY), y1-1)
	lo := maxi(0, int(math.Floor(sg.ax-rad-1)))
	hi := mini(f.W-1, int(math.Ceil(sg.ax+rad+1)))
	for y := ya; y <= yb; y++ {
		ey := float64(y) + 0.5 - sg.ay
		for x := lo; x <= hi; x++ {
			ex := float64(x) + 0.5 - sg.ax
			var sd float64
			switch shape {
			case Square:
				sd = maxf(math.Abs(ex), math.Abs(ey)) - rad*0.85
			case Diamond:
				sd = (math.Abs(ex) + math.Abs(ey) - rad) * math.Sqrt2 / 2
			case Ring:
				sd = math.Abs(math.Sqrt(ex*ex+ey*ey)-(rad-ring/2)) - ring/2
			default:
				sd = math.Sqrt(ex*ex+ey*ey) - rad
			}
			cov := clampf(0.5-sd, 0, 1)
			if cov <= 0 {
				continue
			}
			i := y*f.W + x
			alpha := cov * sg.ca[3]
			if !(key >= f.Z[i]) {
				alpha *= hidden
			}
			touch = f.accum(i, alpha, sg, 0, touch)
		}
	}
	return touch
}
