package gfx

import (
	"image"
	"image/color"
	"math"
)

// ==========================
// ===  Path              ===
// ==========================

// Pt is a point in pixel coordinates (pixel centers sit at +0.5).
type Pt struct{ X, Y float64 }

const (
	cmdMove = iota
	cmdLine
	cmdQuad
	cmdCubic
	cmdClose
)

// Path is a sequence of subpaths made of lines and Bezier curves.
type Path struct {
	cmds []uint8
	pts  []Pt
	min  Pt
	max  Pt
}

func (p *Path) grow(x, y float64) {
	if len(p.pts) == 0 {
		p.min, p.max = Pt{x, y}, Pt{x, y}
	}
	p.min.X = math.Min(p.min.X, x)
	p.min.Y = math.Min(p.min.Y, y)
	p.max.X = math.Max(p.max.X, x)
	p.max.Y = math.Max(p.max.Y, y)
	p.pts = append(p.pts, Pt{x, y})
}

// MoveTo starts a new subpath.
func (p *Path) MoveTo(x, y float64) {
	p.cmds = append(p.cmds, cmdMove)
	p.grow(x, y)
}

// LineTo adds a straight segment.
func (p *Path) LineTo(x, y float64) {
	p.start()
	p.cmds = append(p.cmds, cmdLine)
	p.grow(x, y)
}

// QuadTo adds a quadratic Bezier with control point (cx, cy).
func (p *Path) QuadTo(cx, cy, x, y float64) {
	p.start()
	p.cmds = append(p.cmds, cmdQuad)
	p.grow(cx, cy)
	p.grow(x, y)
}

// CubicTo adds a cubic Bezier.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) {
	p.start()
	p.cmds = append(p.cmds, cmdCubic)
	p.grow(c1x, c1y)
	p.grow(c2x, c2y)
	p.grow(x, y)
}

// Close ends the current subpath with a segment back to its start.
func (p *Path) Close() {
	if len(p.cmds) > 0 {
		p.cmds = append(p.cmds, cmdClose)
	}
}

// start makes drawing without MoveTo begin at the origin, like canvas APIs.
func (p *Path) start() {
	if len(p.cmds) == 0 {
		p.MoveTo(0, 0)
	}
}

// bounds is the integer box of all points, including control points.
func (p *Path) bounds(pad float64) image.Rectangle {
	if len(p.pts) == 0 {
		return image.Rectangle{}
	}
	return image.Rect(int(math.Floor(p.min.X-pad)), int(math.Floor(p.min.Y-pad)),
		int(math.Ceil(p.max.X+pad))+1, int(math.Ceil(p.max.Y+pad))+1)
}

// flatTol is the maximum distance of a flattened curve from the true one.
const flatTol = 0.05

// flatten turns the path into polylines; emit gets a reused buffer.
func (p *Path) flatten(emit func(poly []Pt, closed bool)) {
	var poly []Pt
	flush := func(closed bool) {
		if len(poly) > 0 {
			emit(poly, closed)
		}
		poly = poly[:0]
	}
	k := 0
	for _, c := range p.cmds {
		switch c {
		case cmdMove:
			flush(false)
			poly = append(poly, p.pts[k])
			k++
		case cmdLine:
			poly = append(poly, p.pts[k])
			k++
		case cmdQuad:
			poly = flattenQuad(poly, poly[len(poly)-1], p.pts[k], p.pts[k+1])
			k += 2
		case cmdCubic:
			poly = flattenCubic(poly, poly[len(poly)-1], p.pts[k], p.pts[k+1], p.pts[k+2])
			k += 3
		case cmdClose:
			if len(poly) > 0 {
				first := poly[0]
				emit(poly, true)
				poly = append(poly[:0], first)
			}
		}
	}
	if len(poly) > 1 {
		flush(false)
	}
}

func flattenQuad(out []Pt, p0, p1, p2 Pt) []Pt {
	dd := math.Hypot(p0.X-2*p1.X+p2.X, p0.Y-2*p1.Y+p2.Y)
	n := int(math.Ceil(math.Sqrt(dd / (4 * flatTol))))
	if n < 1 {
		n = 1
	} else if n > 200 {
		n = 200
	}
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		out = append(out, Pt{u*u*p0.X + 2*u*t*p1.X + t*t*p2.X, u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y})
	}
	return out
}

func flattenCubic(out []Pt, p0, p1, p2, p3 Pt) []Pt {
	d1 := math.Hypot(p0.X-2*p1.X+p2.X, p0.Y-2*p1.Y+p2.Y)
	d2 := math.Hypot(p1.X-2*p2.X+p3.X, p1.Y-2*p2.Y+p3.Y)
	dd := math.Max(d1, d2)
	n := int(math.Ceil(math.Sqrt(3 * dd / (4 * flatTol))))
	if n < 1 {
		n = 1
	} else if n > 300 {
		n = 300
	}
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out = append(out, Pt{a*p0.X + b*p1.X + c*p2.X + d*p3.X, a*p0.Y + b*p1.Y + c*p2.Y + d*p3.Y})
	}
	return out
}

// ==========================
// ===  Fill and stroke   ===
// ==========================

// FillPath fills p with the nonzero rule (coverage is |winding area|,
// clamped, which equals nonzero for non-cancelling overlaps).
func (c *Canvas) FillPath(p *Path, col color.NRGBA) {
	if col.A == 0 {
		return
	}
	b := p.bounds(0).Intersect(c.clip())
	if b.Empty() {
		return
	}
	r := getRaster(b)
	p.flatten(func(poly []Pt, _ bool) { r.polygon(poly) })
	r.paint(c.Img, col)
	putRaster(r)
}

// StrokePath outlines p with round joins and caps.
func (c *Canvas) StrokePath(p *Path, width float64, col color.NRGBA) {
	if col.A == 0 || width <= 0 {
		return
	}
	b := p.bounds(width/2 + 1).Intersect(c.clip())
	if b.Empty() {
		return
	}
	r := getRaster(b)
	var s stroker
	p.flatten(func(poly []Pt, closed bool) { s.stroke(r, poly, width/2, closed) })
	r.paint(c.Img, col)
	putRaster(r)
}

// Line draws a segment with round caps.
func (c *Canvas) Line(x0, y0, x1, y1, width float64, col color.NRGBA) {
	c.Polyline([]Pt{{x0, y0}, {x1, y1}}, width, col, false)
}

// Polyline strokes connected segments with round joins.
func (c *Canvas) Polyline(pts []Pt, width float64, col color.NRGBA, closed bool) {
	if len(pts) == 0 || col.A == 0 || width <= 0 {
		return
	}
	var p Path
	p.MoveTo(pts[0].X, pts[0].Y)
	for _, q := range pts[1:] {
		p.LineTo(q.X, q.Y)
	}
	if closed {
		p.Close()
	}
	c.StrokePath(&p, width, col)
}

// ==========================
// ===  Stroker           ===
// ==========================

// stroker builds the outline of a polyline: left offset forward, cap,
// right offset backward, cap. Inner joins pass through the vertex so the
// overlap has positive winding and simply saturates.
type stroker struct {
	pts  []Pt
	out  []Pt
	norm []Pt
}

func (s *stroker) stroke(r *raster, poly []Pt, hw float64, closed bool) {
	// Drop repeated points: they have no direction.
	s.pts = s.pts[:0]
	for _, q := range poly {
		if n := len(s.pts); n > 0 && math.Abs(q.X-s.pts[n-1].X) < 1e-9 && math.Abs(q.Y-s.pts[n-1].Y) < 1e-9 {
			continue
		}
		s.pts = append(s.pts, q)
	}
	pts := s.pts
	if closed && len(pts) > 2 && pts[0] == pts[len(pts)-1] {
		pts = pts[:len(pts)-1]
	}
	if len(pts) == 0 {
		return
	}
	if len(pts) == 1 {
		s.out = arcPts(s.out[:0], pts[0], hw, 0, 2*math.Pi)
		r.polygon(s.out)
		return
	}
	if closed && len(pts) < 3 {
		closed = false
	}
	nseg := len(pts) - 1
	if closed {
		nseg = len(pts)
	}
	s.norm = s.norm[:0]
	for i := 0; i < nseg; i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		dx, dy := b.X-a.X, b.Y-a.Y
		l := math.Hypot(dx, dy)
		s.norm = append(s.norm, Pt{-dy / l * hw, dx / l * hw})
	}
	if closed {
		s.out = s.side(s.out[:0], pts, hw, true)
		r.polygon(s.out)
		s.out = s.sideBack(s.out[:0], pts, hw, true)
		r.polygon(s.out)
		return
	}
	s.out = s.side(s.out[:0], pts, hw, false)
	last := pts[len(pts)-1]
	nl := s.norm[nseg-1]
	s.out = arcPts(s.out, last, hw, math.Atan2(nl.Y, nl.X), math.Atan2(nl.Y, nl.X)-math.Pi)
	s.out = s.sideBack(s.out, pts, hw, false)
	n0 := s.norm[0]
	s.out = arcPts(s.out, pts[0], hw, math.Atan2(-n0.Y, -n0.X), math.Atan2(-n0.Y, -n0.X)-math.Pi)
	r.polygon(s.out)
}

// side walks the +normal offset of every segment forward.
func (s *stroker) side(out []Pt, pts []Pt, hw float64, closed bool) []Pt {
	nseg := len(s.norm)
	for i := 0; i < nseg; i++ {
		a, b := pts[i], pts[(i+1)%len(pts)]
		n := s.norm[i]
		out = append(out, Pt{a.X + n.X, a.Y + n.Y}, Pt{b.X + n.X, b.Y + n.Y})
		if i+1 < nseg || closed {
			out = join(out, b, n, s.norm[(i+1)%nseg], hw)
		}
	}
	return out
}

// sideBack walks the -normal offset backward.
func (s *stroker) sideBack(out []Pt, pts []Pt, hw float64, closed bool) []Pt {
	nseg := len(s.norm)
	for i := nseg - 1; i >= 0; i-- {
		a, b := pts[i], pts[(i+1)%len(pts)]
		n := Pt{-s.norm[i].X, -s.norm[i].Y}
		out = append(out, Pt{b.X + n.X, b.Y + n.Y}, Pt{a.X + n.X, a.Y + n.Y})
		if i > 0 || closed {
			pn := s.norm[(i-1+nseg)%nseg]
			out = join(out, a, n, Pt{-pn.X, -pn.Y}, hw)
		}
	}
	return out
}

// join connects offset n0 to offset n1 around vertex v: an arc on the
// outer side, a detour through the vertex on the inner side.
func join(out []Pt, v, n0, n1 Pt, hw float64) []Pt {
	// The normal is the direction rotated by +90 degrees, so a positive
	// turn bends toward this side and makes it the inner one.
	cross := n0.X*n1.Y - n0.Y*n1.X
	if cross > 0 {
		out = append(out, v)
		return out
	}
	a0 := math.Atan2(n0.Y, n0.X)
	a1 := math.Atan2(n1.Y, n1.X)
	for a1 > a0 {
		a1 -= 2 * math.Pi
	}
	return arcPts(out, v, hw, a0, a1)
}

// arcPts appends points of an arc from angle a0 to a1 (either direction).
func arcPts(out []Pt, c Pt, r, a0, a1 float64) []Pt {
	span := a1 - a0
	n := arcSteps(r, math.Abs(span))
	for i := 0; i <= n; i++ {
		a := a0 + span*float64(i)/float64(n)
		out = append(out, Pt{c.X + r*math.Cos(a), c.Y + r*math.Sin(a)})
	}
	return out
}

// arcSteps picks a segment count that keeps the chord error tiny.
func arcSteps(r, span float64) int {
	if r <= 0 || span <= 0 {
		return 1
	}
	tol := 0.02
	if tol > r/4 {
		tol = r / 4
	}
	step := 2 * math.Acos(1-tol/r)
	n := int(math.Ceil(span / step))
	if n < 2 {
		n = 2
	}
	if n > 720 {
		n = 720
	}
	return n
}
