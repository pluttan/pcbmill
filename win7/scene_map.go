package main

// The height map the way MATLAB draws surf(peaks): a lit surface coloured by
// height with its grid on it, in a box whose back walls carry the grid and
// the numbers in mm, a colour bar at the side. Heights are a few hundredths
// of a millimetre over a hundred millimetres of sheet, so they are drawn
// many times higher; the numbers say the real ones.

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"

	"pcbscan/gfx"
	"pcbscan/r3"
	"pcbscan/ui"
)

type mapScene struct {
	hm      *HeightMap
	pts     []Point // where the tool touched
	outl    [][]pt  // board outlines, to know where the boards are
	lo, hi  float64
	x0, x1  float64
	y0, y1  float64
	zs      float64 // drawn height per real mm
	tool    bool
	version int
}

func newMapScene(hm *HeightMap, pts []Point, j *Job, mirror bool) *mapScene {
	s := &mapScene{hm: hm, pts: pts}
	s.lo, s.hi = math.Inf(1), math.Inf(-1)
	for _, r := range hm.z {
		for _, v := range r {
			s.lo, s.hi = math.Min(s.lo, v), math.Max(s.hi, v)
		}
	}
	s.x0, s.x1 = hm.xs[0], hm.xs[len(hm.xs)-1]
	s.y0, s.y1 = hm.ys[0], hm.ys[len(hm.ys)-1]
	if s.x1-s.x0 < 1 {
		s.x1 = s.x0 + 1
	}
	if s.y1-s.y0 < 1 {
		s.y1 = s.y0 + 1
	}
	span := s.hi - s.lo
	if span < 0.01 {
		span = 0.01
	}
	s.zs = math.Max(s.x1-s.x0, s.y1-s.y0) * 0.32 / span
	if j != nil {
		for _, p := range j.Places {
			if p.Src >= len(j.boards) || j.boards[p.Src] == nil {
				continue
			}
			pl := j.placeOf(p, mirror)
			for _, l := range j.boards[p.Src].outline {
				var q []pt
				for _, v := range l {
					q = append(q, pl.at(v))
				}
				s.outl = append(s.outl, q)
			}
		}
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%p|%d|%d", hm, len(pts), len(s.outl))
	s.version = int(h.Sum32() & 0x7fffffff)
	return s
}

func (s *mapScene) Version() int {
	if s.tool {
		x, y, z, _ := toolNow()
		return s.version ^ int(math.Float64bits(x*31+y*17+z*1000)&0x7fffffff)
	}
	return s.version
}

// the drawn z of a real height; the floor of the box is under the lowest
func (s *mapScene) dz(z float64) float64 { return (z - s.lo) * s.zs }

func (s *mapScene) top() float64 { return s.dz(s.hi) * 1.08 }

func (s *mapScene) Bounds() (r3.Vec, r3.Vec) {
	return r3.Vec{X: s.x0, Y: s.y0, Z: 0}, r3.Vec{X: s.x1, Y: s.y1, Z: s.top()}
}

// mapSurface: the height map as a lit coloured surface, zs times higher,
// lifted by base.
func mapSurface(hm *HeightMap, zs, base, gloss float64) *r3.Mesh {
	x0, x1 := hm.xs[0], hm.xs[len(hm.xs)-1]
	y0, y1 := hm.ys[0], hm.ys[len(hm.ys)-1]
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range hm.z {
		for _, v := range r {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	nx := int(math.Min(140, math.Max(8, (x1-x0)/0.8)))
	ny := int(math.Min(140, math.Max(8, (y1-y0)/0.8)))
	at := func(i, j int) (float64, float64) {
		return x0 + (x1-x0)*float64(i)/float64(nx-1), y0 + (y1-y0)*float64(j)/float64(ny-1)
	}
	m := r3.Surface(nx, ny, x0, y0, x1, y1, func(i, j int) float64 {
		x, y := at(i, j)
		return base + hm.At(x, y)*zs
	}, func(i, j int, z float64) color.NRGBA {
		x, y := at(i, j)
		return heightColor(hm.At(x, y), lo, hi)
	})
	m.Lit, m.Smooth, m.TwoSided = true, true, true
	return m
}

var surfLight = r3.Light{Dir: r3.Vec{X: -0.5, Y: -0.75, Z: 0.9}, Ambient: 0.3, Diffuse: 0.72, Specular: 0.28, Shininess: 36}

func (s *mapScene) Render(f *r3.Frame, cam *r3.Camera, px float64) {
	t := theme
	top := s.top()
	eye := cam.Eye()
	// the back walls: the ones away from the eye
	bx, fx := s.x0, s.x1
	if eye.X < (s.x0+s.x1)/2 {
		bx, fx = s.x1, s.x0
	}
	by, fy := s.y0, s.y1
	if eye.Y < (s.y0+s.y1)/2 {
		by, fy = s.y1, s.y0
	}
	_, _ = fx, fy
	wall := gfx.Alpha(t.Surface0, 1)
	f.DrawMesh(cam, r3.Plane(s.x0, s.y0, s.x1, s.y1, -0.001, nil, gfx.Mix(t.Base, t.Surface0, 0.5)), surfLight)
	f.DrawMesh(cam, wallX(bx, s.y0, s.y1, top, gfx.Mix(t.Base, wall, 0.55)), surfLight)
	f.DrawMesh(cam, wallY(by, s.x0, s.x1, top, gfx.Mix(t.Base, wall, 0.45)), surfLight)
	grid := r3.LineStyle{Width: 1 * px, Color: gfx.Alpha(t.Surface2, 0.9), Bias: 0.02}
	xt, yt, zt := s.ticks()
	for _, x := range xt {
		f.Lines(cam, []r3.Vec{{X: x, Y: s.y0, Z: 0}, {X: x, Y: s.y1, Z: 0}, {X: x, Y: by, Z: 0}, {X: x, Y: by, Z: top}}, false, grid)
	}
	for _, y := range yt {
		f.Lines(cam, []r3.Vec{{X: s.x0, Y: y, Z: 0}, {X: s.x1, Y: y, Z: 0}, {X: bx, Y: y, Z: 0}, {X: bx, Y: y, Z: top}}, false, grid)
	}
	for _, z := range zt {
		d := s.dz(z)
		f.Lines(cam, []r3.Vec{{X: bx, Y: s.y0, Z: d}, {X: bx, Y: s.y1, Z: d}, {X: s.x0, Y: by, Z: d}, {X: s.x1, Y: by, Z: d}}, false, grid)
	}
	// the surface and its grid, like MATLAB's edges
	surf := mapSurface(s.hm, s.zs, -s.lo*s.zs, 1)
	f.DrawMesh(cam, surf, surfLight)
	edge := r3.LineStyle{Width: 0.9 * px, Color: gfx.Alpha(t.Crust, 0.45), Bias: 0.15}
	for _, x := range s.hm.xs {
		var l []r3.Vec
		for k := 0; k <= 60; k++ {
			y := s.y0 + (s.y1-s.y0)*float64(k)/60
			l = append(l, r3.Vec{X: x, Y: y, Z: s.dz(s.hm.At(x, y))})
		}
		f.Lines(cam, l, true, edge)
	}
	for _, y := range s.hm.ys {
		var l []r3.Vec
		for k := 0; k <= 60; k++ {
			x := s.x0 + (s.x1-s.x0)*float64(k)/60
			l = append(l, r3.Vec{X: x, Y: y, Z: s.dz(s.hm.At(x, y))})
		}
		f.Lines(cam, l, true, edge)
	}
	// where the boards are, lying on the surface
	for _, l := range s.outl {
		var q []r3.Vec
		for k := 1; k < len(l); k++ {
			a, b := l[k-1], l[k]
			n := int(math.Hypot(b.X-a.X, b.Y-a.Y)/0.7) + 1
			for i := 0; i <= n; i++ {
				x, y := a.X+(b.X-a.X)*float64(i)/float64(n), a.Y+(b.Y-a.Y)*float64(i)/float64(n)
				if x < s.x0 || x > s.x1 || y < s.y0 || y > s.y1 {
					continue
				}
				q = append(q, r3.Vec{X: x, Y: y, Z: s.dz(s.hm.At(x, y))})
			}
		}
		if len(q) > 1 {
			f.Lines(cam, q, true, r3.LineStyle{Width: 1.4 * px, Color: gfx.Alpha(t.Text, 0.75), Bias: 0.25, Hidden: 0.2})
		}
	}
	if len(s.pts) > 0 {
		z0 := s.pts[0].Z
		var ps []r3.Vec
		for _, p := range s.pts {
			ps = append(ps, r3.Vec{X: p.X, Y: p.Y, Z: s.dz(p.Z-z0) + 0.05})
		}
		f.Points(cam, ps, uniform(len(ps), gfx.Alpha(t.Crust, 0.9)), 4.5*px, r3.Disc, r3.LineStyle{Hidden: 0.25, Bias: 0.3})
	}
	if s.tool {
		x, y, z, ok := toolNow()
		if ok {
			drawTool(f, cam, r3.Vec{X: x, Y: y, Z: s.dz(s.hm.At(x, y)) + overCopper(z) - s.hm.At(x, y)})
		}
	}
}

func wallX(x, y0, y1, h float64, col color.NRGBA) *r3.Mesh {
	n := r3.Vec{X: 1}
	v := []r3.Vertex{{P: r3.Vec{X: x, Y: y0}, N: n, C: col}, {P: r3.Vec{X: x, Y: y1}, N: n, C: col},
		{P: r3.Vec{X: x, Y: y1, Z: h}, N: n, C: col}, {P: r3.Vec{X: x, Y: y0, Z: h}, N: n, C: col}}
	return &r3.Mesh{V: v, Idx: []int32{0, 1, 2, 0, 2, 3}, TwoSided: true}
}

func wallY(y, x0, x1, h float64, col color.NRGBA) *r3.Mesh {
	n := r3.Vec{Y: 1}
	v := []r3.Vertex{{P: r3.Vec{X: x0, Y: y}, N: n, C: col}, {P: r3.Vec{X: x1, Y: y}, N: n, C: col},
		{P: r3.Vec{X: x1, Y: y, Z: h}, N: n, C: col}, {P: r3.Vec{X: x0, Y: y, Z: h}, N: n, C: col}}
	return &r3.Mesh{V: v, Idx: []int32{0, 1, 2, 0, 2, 3}, TwoSided: true}
}

// niceStep: 1, 2 or 5 times a power of ten, about n steps over span.
func niceStep(span float64, n int) float64 {
	if span <= 0 {
		return 1
	}
	raw := span / float64(n)
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if raw <= m*p {
			return m * p
		}
	}
	return 10 * p
}

func ticksOf(a, b float64, n int) []float64 {
	st := niceStep(b-a, n)
	var out []float64
	for v := math.Ceil(a/st-1e-9) * st; v <= b+1e-9; v += st {
		out = append(out, v)
	}
	return out
}

func (s *mapScene) ticks() (xs, ys, zs []float64) {
	return ticksOf(s.x0, s.x1, 5), ticksOf(s.y0, s.y1, 5), ticksOf(s.lo, s.hi, 3)
}

func (s *mapScene) Overlay(c *ui.Ctx, r image.Rectangle, cam *r3.Camera) {
	t := c.T
	lab := ui.Style{Kind: ui.Regular, Size: ui.SizeSmall, Color: t.Subtext0}
	eye := cam.Eye()
	fx, fy := s.x1, s.y1 // the front edges carry the numbers
	if eye.X < (s.x0+s.x1)/2 {
		fx = s.x0
	}
	if eye.Y < (s.y0+s.y1)/2 {
		fy = s.y0
	}
	put := func(p r3.Vec, txt string) {
		if sx, sy, _, ok := cam.Project(p, r.Dx(), r.Dy()); ok {
			w := c.Measure(txt, lab)
			c.Label(r.Min.X+int(sx)-w/2, r.Min.Y+int(sy)+t.Si(4), txt, lab)
		}
	}
	putLeft := func(p r3.Vec, txt string) { // the heights: left of the corner edge
		if sx, sy, _, ok := cam.Project(p, r.Dx(), r.Dy()); ok {
			w := c.Measure(txt, lab)
			c.Label(r.Min.X+int(sx)-w-t.Si(8), r.Min.Y+int(sy)-c.LineH(lab)/2, txt, lab)
		}
	}
	xt, yt, zt := s.ticks()
	for _, x := range xt {
		put(r3.Vec{X: x, Y: fy, Z: 0}, num(math.Round(x*10)/10))
	}
	for _, y := range yt {
		put(r3.Vec{X: fx, Y: y, Z: 0}, num(math.Round(y*10)/10))
	}
	cx, cy := s.x0, fy
	if fx == s.x0 {
		cx = s.x1
	}
	for _, z := range zt {
		putLeft(r3.Vec{X: cx, Y: cy, Z: s.dz(z)}, fmt.Sprintf("%+.3f", z))
	}
	// the colour bar
	bw, bh := t.Si(14), r.Dy()/2
	bx, by := r.Max.X-t.Si(64), r.Min.Y+(r.Dy()-bh)/2
	stops := heightStops()
	for k := 0; k < bh; k++ {
		col := r3.Colormap(stops, 1-float64(k)/float64(bh-1))
		c.C.FillRect(image.Rect(bx, by+k, bx+bw, by+k+1), col)
	}
	c.C.StrokeRoundRect(float64(bx)-0.5, float64(by)-0.5, float64(bw)+1, float64(bh)+1, 2, 1, t.Surface2)
	for _, z := range zt {
		if s.hi-s.lo < 1e-9 {
			break
		}
		yy := by + int(float64(bh-1)*(1-(z-s.lo)/(s.hi-s.lo)))
		c.C.FillRect(image.Rect(bx+bw, yy, bx+bw+t.Si(4), yy+1), t.Subtext0)
		c.Label(bx+bw+t.Si(6), yy-t.Si(8), fmt.Sprintf("%+.3f", z), lab)
	}
	c.Label(bx-t.Si(4), by-t.Si(24), "мм", ui.Style{Kind: ui.Bold, Size: ui.SizeSmall, Color: t.Subtext1})
	info := fmt.Sprintf("перепад %.3f мм, высоты в %.0f раз выше", s.hi-s.lo, s.zs)
	c.Label(r.Min.X+t.Si(14), r.Min.Y+t.Si(12), info, ui.Style{Kind: ui.Regular, Size: ui.SizeSmall, Color: t.Overlay2})
	axisTriad(c, r, cam)
}
