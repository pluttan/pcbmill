package main

// The sheet in 3D as it lies on the table: the copper plate with the copper
// of every board on it, the outlines, the corner of Z0, and what the screen
// is about: the points of the height map, the paths of a program, a circle
// of a redo, the tool where the machine has it.

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"sync"

	"pcbscan/gfx"
	"pcbscan/r3"
	"pcbscan/ui"
)

// sheetOverlay: what the sheet shows besides the boards.
type sheetOverlay struct {
	points [][2]float64 // the height map
	paths  []cpath      // a program
	marks  []pt         // narrow places, tiny openings
	circle *[3]float64  // x, y, r: a redo
	stage  string       // the colour of the paths: "1", "2", "3", "4"
}

// sheetScene implements ui.Scene for a job.
type sheetScene struct {
	j      *Job
	mirror bool
	side   string
	ov     sheetOverlay
	done   []Point    // measured points (a scan going on)
	hm     *HeightMap // the copper as measured
	tool   bool       // show the tool where it is
	key    string
}

func newSheetScene(j *Job, mirror bool, ov sheetOverlay) *sheetScene {
	s := &sheetScene{j: j, mirror: mirror, ov: ov, side: lyTop}
	if mirror {
		s.side = lyBottom
	}
	return s
}

func (s *sheetScene) Version() int {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%d|%d|%d|%v|%d|%v", sheetKey(s.j, s.mirror, 0, 0), len(s.ov.points), len(s.ov.paths), len(s.ov.marks),
		s.ov.circle != nil, len(s.done), s.hm != nil)
	if len(s.ov.paths) > 0 {
		fmt.Fprintf(h, "|%p", &s.ov.paths[0])
	}
	if s.ov.circle != nil {
		fmt.Fprintf(h, "|%v", *s.ov.circle)
	}
	if s.tool {
		x, y, z, _ := toolNow()
		fmt.Fprintf(h, "|%.3f,%.3f,%.4f", x, y, z)
	}
	return int(h.Sum32() & 0x7fffffff)
}

func (s *sheetScene) Bounds() (r3.Vec, r3.Vec) {
	w, h := s.j.W, s.j.H
	if w <= 0 || h <= 0 {
		w, h = 100, 100
	}
	return r3.Vec{X: 0, Y: 0, Z: -2}, r3.Vec{X: w, Y: h, Z: 4}
}

// ==========================
// ===  Cached geometry   ===
// ==========================

var sheetCache struct {
	sync.Mutex
	key  string
	tex  *image.RGBA
	outl [][]r3.Vec
}

// sheetKey: the layout of a job, for caches.
func sheetKey(j *Job, mirror bool, _, _ int) string {
	k := fmt.Sprintf("%sx%s/%v", num(j.W), num(j.H), mirror)
	for _, p := range j.Places {
		k += fmt.Sprintf("|%d,%d,%s,%s", p.Src, p.Rot, num(p.X), num(p.Y))
	}
	for _, p := range j.probeBoxes() {
		k += fmt.Sprintf("|%s,%s,%s", p.name, num(p.at.X), num(p.at.Y))
	}
	return k
}

// copperTex: the copper of the boards over the sheet, an image for the top
// of the plate: the plate darker, the boards lighter, traces and pads bright.
func (s *sheetScene) geometry() (*image.RGBA, [][]r3.Vec) {
	sheetCache.Lock()
	defer sheetCache.Unlock()
	key := sheetKey(s.j, s.mirror, 0, 0) + s.side
	if sheetCache.key == key && sheetCache.tex != nil {
		return sheetCache.tex, sheetCache.outl
	}
	j := s.j
	W, H := math.Max(j.W, 1), math.Max(j.H, 1)
	px := math.Max(W, H) / 1400
	if px < 0.05 {
		px = 0.05
	}
	area := j.boardsRaster(s.mirror, px)
	cu := newRaster(rect{0, W, 0, H}, area.px)
	var outl [][]r3.Vec
	for _, p := range j.Places {
		if p.Src >= len(j.boards) || j.boards[p.Src] == nil {
			continue
		}
		b := j.boards[p.Src]
		pl := j.placeOf(p, s.mirror)
		if g := b.copper[s.side]; g != nil {
			for _, sh := range g.shapes {
				t := shape{dark: sh.dark, R: sh.R}
				if sh.R > 0 {
					t.C = pl.at(sh.C)
				}
				for _, poly := range sh.polys {
					q := make([]pt, len(poly))
					for i, v := range poly {
						q[i] = pl.at(v)
					}
					t.polys = append(t.polys, q)
				}
				cu.draw(t)
			}
		}
		for _, l := range b.outline {
			var line []r3.Vec
			for _, q := range l {
				w := pl.at(q)
				line = append(line, r3.Vec{X: w.X, Y: w.Y, Z: 0.02})
			}
			outl = append(outl, line)
		}
	}
	th := theme
	// the board between its copper is near black, so the traces stand out
	plate := th.CopperDark
	boardC := gfx.Mix(th.CopperDark, gfx.Hex("#000000"), 0.6)
	trace := th.Copper
	img := image.NewRGBA(image.Rect(0, 0, cu.w, cu.h))
	for jy := 0; jy < cu.h; jy++ {
		row := cu.h - 1 - jy // the image has its first row on top, the raster at the bottom
		for ix := 0; ix < cu.w; ix++ {
			c := plate
			if area.at(ix, jy) {
				c = boardC
			}
			if cu.at(ix, jy) {
				c = trace
			}
			// a brushed look: faint streaks along X
			n := float64((ix*7+jy*131)%17)/17*0.05 - 0.025
			o := img.PixOffset(ix, row)
			img.Pix[o] = shade8(c.R, n)
			img.Pix[o+1] = shade8(c.G, n)
			img.Pix[o+2] = shade8(c.B, n)
			img.Pix[o+3] = 255
		}
	}
	sheetCache.key, sheetCache.tex, sheetCache.outl = key, img, outl
	return img, outl
}

func shade8(v uint8, d float64) uint8 {
	f := float64(v) * (1 + d)
	if f > 255 {
		f = 255
	}
	if f < 0 {
		f = 0
	}
	return uint8(f)
}

// ==========================
// ===  Rendering         ===
// ==========================

var keyLight = r3.Light{Dir: r3.Vec{X: -0.45, Y: -0.6, Z: 0.85}, Ambient: 0.32, Diffuse: 0.75, Specular: 0.35, Shininess: 28}

// zScale: how much higher the measured copper is drawn than it is.
func (s *sheetScene) zScale() float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	add := func(z float64) {
		lo, hi = math.Min(lo, z), math.Max(hi, z)
	}
	for _, p := range s.done {
		add(p.Z)
	}
	if s.hm != nil {
		for _, r := range s.hm.z {
			for _, v := range r {
				add(v)
			}
		}
	}
	span := hi - lo
	if math.IsInf(span, 0) || span < 0.01 {
		span = 0.01
	}
	return math.Max(s.j.W, s.j.H) * 0.06 / span
}

func (s *sheetScene) Render(f *r3.Frame, cam *r3.Camera, px float64) {
	t := theme
	j := s.j
	W, H := math.Max(j.W, 1), math.Max(j.H, 1)
	tex, outl := s.geometry()
	// the plate: sides and bottom plain, the top the copper picture
	f.DrawMesh(cam, r3.Box(r3.Vec{X: 0, Y: 0, Z: -1.6}, r3.Vec{X: W, Y: H, Z: -0.01}, gfx.Mix(t.CopperDark, t.Crust, 0.35)), keyLight)
	top := r3.Plane(0, 0, W, H, 0, tex, color.NRGBA{255, 255, 255, 255})
	top.Lit = true
	f.DrawMesh(cam, top, keyLight)
	zs := s.zScale()
	// the measured copper, higher than it is, its lowest point on the plate
	var lo float64
	if s.hm != nil {
		lo = math.Inf(1)
		for _, r := range s.hm.z {
			for _, v := range r {
				lo = math.Min(lo, v)
			}
		}
		f.DrawMesh(cam, mapSurface(s.hm, zs, 0.02-lo*zs, 0.85), keyLight)
	}
	lift := func(x, y float64) float64 {
		if s.hm != nil {
			return (s.hm.At(x, y)-lo)*zs + 0.03
		}
		return 0.03
	}
	for _, l := range outl {
		pts := make([]r3.Vec, len(l))
		for i, q := range l {
			pts[i] = r3.Vec{X: q.X, Y: q.Y, Z: lift(q.X, q.Y)}
		}
		f.Lines(cam, pts, true, r3.LineStyle{Width: 1.2 * px, Color: gfx.Alpha(t.Lavender, 0.9), Hidden: 0.15, Bias: 0.05})
	}
	// the places of the machine probes: a frame round each
	for _, p := range j.probeBoxes() {
		x0, y0, x1, y1 := p.at.X, p.at.Y, p.at.X+p.w, p.at.Y+p.h
		var ring []r3.Vec
		for _, q := range [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}, {x0, y0}} {
			ring = append(ring, r3.Vec{X: q[0], Y: q[1], Z: lift(q[0], q[1])})
		}
		f.Lines(cam, ring, true, r3.LineStyle{Width: 1.2 * px, Color: gfx.Alpha(t.Peach, 0.9), Hidden: 0.3, Bias: 0.05})
	}
	// the paths of a program on the copper; under it they show through
	if len(s.ov.paths) > 0 {
		col := stageColor(s.ov.stage)
		var dots []r3.Vec
		for _, p := range s.ov.paths {
			if len(p.pts) == 1 || p.peck > 0 {
				q := p.pts[0]
				dots = append(dots, r3.Vec{X: q.X, Y: q.Y, Z: lift(q.X, q.Y)})
				continue
			}
			pts := make([]r3.Vec, len(p.pts))
			for i, q := range p.pts {
				pts[i] = r3.Vec{X: q.X, Y: q.Y, Z: lift(q.X, q.Y) + q.Z*0.5}
			}
			f.Lines(cam, pts, true, r3.LineStyle{Width: 1.3 * px, Color: col, Hidden: 0.35, Bias: 0.08})
		}
		if len(dots) > 0 {
			f.Points(cam, dots, uniform(len(dots), col), 5*px, r3.Ring, r3.LineStyle{Hidden: 0.4, Bias: 0.1})
		}
	}
	// the points of the height map: measured ones by height, the rest grey
	if len(s.ov.points) > 0 || len(s.done) > 0 {
		var rest []r3.Vec
		for _, q := range s.ov.points[imin(len(s.done), len(s.ov.points)):] {
			rest = append(rest, r3.Vec{X: q[0], Y: q[1], Z: lift(q[0], q[1])})
		}
		if len(rest) > 0 {
			f.Points(cam, rest, uniform(len(rest), gfx.Alpha(t.Overlay2, 0.9)), 4*px, r3.Disc, r3.LineStyle{Hidden: 0.3, Bias: 0.1})
		}
		if len(s.done) > 0 {
			lo, hi := zRange(s.done)
			pts := make([]r3.Vec, len(s.done))
			cols := make([]color.NRGBA, len(s.done))
			for i, p := range s.done {
				pts[i] = r3.Vec{X: p.X, Y: p.Y, Z: lift(p.X, p.Y) + 0.02}
				cols[i] = heightColor(p.Z, lo, hi)
			}
			f.Points(cam, pts, cols, 7*px, r3.Disc, r3.LineStyle{Hidden: 0.5, Bias: 0.1})
		}
	}
	if len(s.ov.marks) > 0 {
		var pts []r3.Vec
		for _, m := range s.ov.marks {
			pts = append(pts, r3.Vec{X: m.X, Y: m.Y, Z: lift(m.X, m.Y) + 0.05})
		}
		f.Points(cam, pts, uniform(len(pts), t.Red), 9*px, r3.Diamond, r3.LineStyle{Hidden: 0.6, Bias: 0.1})
	}
	if c := s.ov.circle; c != nil {
		var ring []r3.Vec
		for i := 0; i <= 96; i++ {
			a := float64(i) / 96 * 2 * math.Pi
			x, y := c[0]+c[2]*math.Cos(a), c[1]+c[2]*math.Sin(a)
			ring = append(ring, r3.Vec{X: x, Y: y, Z: lift(x, y) + 0.05})
		}
		f.Lines(cam, ring, true, r3.LineStyle{Width: 2.2 * px, Color: t.Pink, Hidden: 0.4, Bias: 0.1})
		f.Points(cam, []r3.Vec{{X: c[0], Y: c[1], Z: lift(c[0], c[1]) + 0.05}}, []color.NRGBA{t.Pink}, 6*px, r3.Disc, r3.LineStyle{Hidden: 0.4, Bias: 0.1})
	}
	// the corner of Z0
	z := j.zero()
	f.Points(cam, []r3.Vec{{X: z.X, Y: z.Y, Z: lift(z.X, z.Y) + 0.05}}, []color.NRGBA{t.Mauve}, 11*px, r3.Diamond, r3.LineStyle{Hidden: 0.6, Bias: 0.2})
	if s.tool {
		x, y, tz, ok := toolNow()
		if ok {
			base := 0.0
			if s.hm != nil {
				base = s.hm.At(x, y)
			}
			drawTool(f, cam, r3.Vec{X: x, Y: y, Z: lift(x, y) - 0.03 + overCopper(tz) - base})
		}
	}
}

func (s *sheetScene) Overlay(c *ui.Ctx, r image.Rectangle, cam *r3.Camera) {
	t := c.T
	z := s.j.zero()
	if sx, sy, _, ok := cam.Project(r3.Vec{X: z.X, Y: z.Y, Z: 0.1}, r.Dx(), r.Dy()); ok {
		c.Label(r.Min.X+int(sx)+t.Si(9), r.Min.Y+int(sy)-t.Si(8), "Z0", ui.Style{Kind: ui.Bold, Size: ui.SizeSmall, Color: t.Mauve})
	}
	for _, p := range s.j.probeBoxes() {
		if sx, sy, _, ok := cam.Project(r3.Vec{X: p.at.X, Y: p.at.Y + p.h, Z: 0.1}, r.Dx(), r.Dy()); ok {
			c.Label(r.Min.X+int(sx)+t.Si(4), r.Min.Y+int(sy)-t.Si(18), p.name, ui.Style{Kind: ui.Bold, Size: ui.SizeSmall, Color: t.Peach})
		}
	}
	axisTriad(c, r, cam)
	if s.tool {
		x, y, tz, ok := toolNow()
		s := fmt.Sprintf("Z %+.3f", tz)
		if ok {
			s = fmt.Sprintf("X %.2f   Y %.2f   Z %+.3f", x, y, tz)
		}
		lab := ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}
		w := c.Measure(s, lab) + t.Si(26)
		br := image.Rect(r.Min.X+t.Si(10), r.Max.Y-t.Si(40), r.Min.X+t.Si(10)+w, r.Max.Y-t.Si(10))
		c.C.FillRoundRect(float64(br.Min.X), float64(br.Min.Y), float64(br.Dx()), float64(br.Dy()), t.S(8), gfx.Alpha(t.Crust, 0.8))
		c.LabelIn(ui.InsetXY(br, t.Si(10), 0), s, lab, 0)
		if toolMoving() {
			c.Animate()
		}
	}
}

// ==========================
// ===  Shared pieces     ===
// ==========================

func stageColor(stage string) color.NRGBA {
	t := theme
	switch stage {
	case "2":
		return t.Mauve
	case "3":
		return t.Yellow
	case "4":
		return t.Red
	}
	return t.Green
}

func uniform(n int, c color.NRGBA) []color.NRGBA {
	out := make([]color.NRGBA, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func zRange(ps []Point) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range ps {
		lo, hi = math.Min(lo, p.Z), math.Max(hi, p.Z)
	}
	return lo, hi
}

// heightStops: the colours of heights, low to high, as MATLAB paints a surface.
func heightStops() []color.NRGBA {
	t := theme
	return []color.NRGBA{t.Blue, t.Sapphire, t.Teal, t.Green, t.Yellow, t.Peach, t.Red}
}

func heightColor(z, lo, hi float64) color.NRGBA {
	if hi-lo < 1e-9 {
		return r3.Colormap(heightStops(), 0.5)
	}
	return r3.Colormap(heightStops(), (z-lo)/(hi-lo))
}

// drawTool: a V-bit with its shank and the collet nut over it, tip at p.
func drawTool(f *r3.Frame, cam *r3.Camera, p r3.Vec) {
	t := theme
	steel := gfx.Mix(t.Subtext1, t.Overlay2, 0.3)
	f.DrawMesh(cam, r3.Cone(p, 1.6, 2.8, 24, t.Yellow), keyLight)
	f.DrawMesh(cam, r3.Cylinder(r3.Vec{X: p.X, Y: p.Y, Z: p.Z + 2.8}, 1.6, 14, 24, steel), keyLight)
	f.DrawMesh(cam, r3.Cylinder(r3.Vec{X: p.X, Y: p.Y, Z: p.Z + 16.8}, 5, 8, 6, gfx.Mix(t.Overlay1, t.Surface2, 0.4)), keyLight)
}

// axisTriad: X, Y, Z arrows in the corner turning with the camera.
func axisTriad(c *ui.Ctx, r image.Rectangle, cam *r3.Camera) {
	t := c.T
	o := r3.Vec{X: 0, Y: 0, Z: 0}
	ox, oy, _, ok := cam.Project(cam.Target, r.Dx(), r.Dy())
	if !ok {
		return
	}
	cx, cy := float64(r.Min.X)+t.S(44), float64(r.Max.Y)-t.S(44)
	if c2 := (r3.Camera{}); c2 == *cam {
		return
	}
	ax := []struct {
		d   r3.Vec
		col color.NRGBA
		s   string
	}{{r3.Vec{X: 1}, t.Red, "X"}, {r3.Vec{Y: 1}, t.Green, "Y"}, {r3.Vec{Z: 1}, t.Blue, "Z"}}
	_ = o
	for _, a := range ax {
		// direction on screen of a unit along the axis at the orbit centre
		ex, ey, _, ok2 := cam.Project(cam.Target.Add(a.d.Scale(cam.Dist*0.05)), r.Dx(), r.Dy())
		if !ok2 {
			continue
		}
		dx, dy := ex-ox, ey-oy
		l := math.Hypot(dx, dy)
		if l < 1e-6 {
			continue
		}
		k := t.S(26) * math.Min(1, l/(t.S(20)))
		dx, dy = dx/l*k, dy/l*k
		c.C.Line(cx, cy, cx+dx, cy+dy, t.S(2), a.col)
		c.Label(int(cx+dx*1.25)-t.Si(4), int(cy+dy*1.25)-t.Si(8), a.s, ui.Style{Kind: ui.Bold, Size: ui.SizeSmall, Color: a.col})
	}
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
