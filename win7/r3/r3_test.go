package r3

import (
	"image/color"
	"math"
	"math/rand"
	"testing"
)

// ==========================
// ===  Camera            ===
// ==========================

func testCams() []*Camera {
	return []*Camera{
		{Target: Vec{10, -5, 2}, Yaw: 0.7, Pitch: 0.5, Dist: 120, FOV: 0.8},
		{Target: Vec{0, 0, 0}, Yaw: -2.1, Pitch: -0.3, Dist: 50, OrthoH: 80},
	}
}

func TestProjectRayRoundTrip(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for _, c := range testCams() {
		for k := 0; k < 200; k++ {
			p := Vec{rnd.Float64()*60 - 30, rnd.Float64()*60 - 30, rnd.Float64()*20 - 10}
			sx, sy, d, ok := c.Project(p, 800, 600)
			if !ok {
				continue
			}
			o, dir := c.Ray(sx, sy, 800, 600)
			q := o.Add(dir.Scale(p.Sub(o).Dot(dir)))
			if q.Sub(p).Len() > 1e-6 {
				t.Fatalf("ray misses point %v by %g (depth %g)", p, q.Sub(p).Len(), d)
			}
			if hit, ok := c.PickPlane(sx, sy, 800, 600, p.Z); ok && hit.Sub(p).Len() > 1e-6 {
				t.Fatalf("PickPlane %v != %v", hit, p)
			}
		}
	}
}

func TestOrbitZoomPan(t *testing.T) {
	for _, c := range testCams() {
		tg := c.Target
		for i := 0; i < 1000; i++ {
			c.Orbit(37, 53)
		}
		if c.Target != tg || math.Abs(c.Pitch) > maxPitch+1e-12 || math.Abs(c.Yaw) > math.Pi+1e-9 {
			t.Fatalf("orbit broke camera: %+v", c)
		}
		c.Orbit(math.NaN(), 1)
		if math.IsNaN(c.Yaw + c.Pitch) {
			t.Fatal("NaN leaked into orbit")
		}
		c.Pitch = 0.4
		// zoom keeps the point under the cursor at Target depth in place
		w, h := 800, 600
		v := c.view(w, h)
		mx, my := 610.0, 120.0
		s := v.unitsPerPixel()
		p := c.Target.Add(v.r.Scale((mx - v.cx) * s)).Sub(v.u.Scale((my - v.cy) * s))
		for _, n := range []float64{3, -5, 1.5} {
			c.Zoom(n, mx, my, w, h)
			sx, sy, _, ok := c.Project(p, w, h)
			if !ok || math.Abs(sx-mx) > 1e-6 || math.Abs(sy-my) > 1e-6 {
				t.Fatalf("zoom moved cursor point to %g,%g", sx, sy)
			}
		}
		for i := 0; i < 500; i++ {
			c.Zoom(10, mx, my, w, h)
		}
		if c.Target.bad() || !(c.Dist >= minDist) {
			t.Fatalf("zoom extremes broke camera %+v", c)
		}
		c.Zoom(-1e6, mx, my, w, h)
		if c.Target.bad() {
			t.Fatalf("zoom out broke target %+v", c)
		}
		// pan drags the target point with the mouse
		c2 := &Camera{Target: Vec{1, 2, 3}, Yaw: 1, Pitch: 0.6, Dist: 90, FOV: c.FOV, OrthoH: 70}
		old := c2.Target
		c2.Pan(40, -25, w, h)
		sx, sy, _, _ := c2.Project(old, w, h)
		if math.Abs(sx-440) > 1e-6 || math.Abs(sy-275) > 1e-6 {
			t.Fatalf("pan: target went to %g,%g", sx, sy)
		}
	}
}

func TestFitFrames(t *testing.T) {
	for _, c := range testCams() {
		mn, mx := Vec{-50, -40, -1.5}, Vec{50, 40, 0}
		c.Fit(mn, mx, 1000, 700)
		for i := 0; i < 8; i++ {
			p := Vec{mn.X, mn.Y, mn.Z}
			if i&1 != 0 {
				p.X = mx.X
			}
			if i&2 != 0 {
				p.Y = mx.Y
			}
			if i&4 != 0 {
				p.Z = mx.Z
			}
			sx, sy, _, ok := c.Project(p, 1000, 700)
			if !ok || sx < 0 || sx > 1000 || sy < 0 || sy > 700 {
				t.Fatalf("corner %v outside after Fit: %g,%g", p, sx, sy)
			}
		}
	}
}

// ==========================
// ===  Rasterizer        ===
// ==========================

// coverCount draws every triangle of m alone and counts how many times each
// pixel gets written.
func coverCount(c *Camera, m *Mesh, w, h int) []int {
	f := NewFrame(w, h)
	cnt := make([]int, w*h)
	for k := 0; k+2 < len(m.Idx); k += 3 {
		one := &Mesh{V: m.V, Idx: m.Idx[k : k+3], TwoSided: true}
		f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
		f.DrawMesh(c, one, Light{})
		for i, z := range f.Z {
			if z > -math.MaxFloat32 {
				cnt[i]++
			}
		}
	}
	return cnt
}

func TestFillExactlyOnce(t *testing.T) {
	white := color.NRGBA{255, 255, 255, 255}
	cams := []*Camera{
		{Yaw: -math.Pi / 2, Pitch: maxPitch, Dist: 100, OrthoH: 60},
		{Yaw: 0.3, Pitch: 0.7, Dist: 40, FOV: 1.0},
	}
	// a quad far larger than the view, as two triangles
	quad := Plane(-500, -500, 500, 500, 0, nil, white)
	// a jittered grid: many shared edges at odd sub-pixel positions
	rnd := rand.New(rand.NewSource(7))
	grid := Surface(9, 9, -200, -200, 200, 200, func(i, j int) float64 { return 0 }, nil)
	for i := range grid.V {
		p := &grid.V[i].P
		if p.X > -200 && p.X < 200 && p.Y > -200 && p.Y < 200 {
			p.X += rnd.Float64()*30 - 15
			p.Y += rnd.Float64()*30 - 15
			p.Z += rnd.Float64() * 3
		}
	}
	for ci, c := range cams {
		for mi, m := range []*Mesh{quad, grid} {
			cnt := coverCount(c, m, 97, 61)
			for i, n := range cnt {
				if n != 1 {
					t.Fatalf("cam %d mesh %d: pixel %d,%d written %d times", ci, mi, i%97, i/97, n)
				}
			}
		}
	}
}

func TestNearClipInside(t *testing.T) {
	// eye 1 mm above a huge floor, looking almost level: the floor passes
	// through the near plane and behind the eye.
	c := &Camera{Target: Vec{0, 0, 0.5}, Yaw: 0.2, Pitch: 0.05, Dist: 2, FOV: 1.2}
	floor := Plane(-1e5, -1e5, 1e5, 1e5, 0, nil, color.NRGBA{100, 150, 200, 255})
	box := Box(Vec{-1, -1, 0}, Vec{1, 1, 1}, color.NRGBA{200, 100, 50, 255})
	f := NewFrame(200, 150)
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.DrawMesh(c, floor, Light{})
	f.DrawMesh(c, box, Light{}) // camera inside the box: back faces culled
	f.Lines(c, []Vec{{-1e6, 0, 0}, {1e6, 0, 0}, {0, -1e6, 0}, {0, 1e6, 0}}, false, LineStyle{Width: 2, Color: color.NRGBA{255, 0, 0, 255}})
	f.Points(c, []Vec{{0, 0, 0}, {1, 1, 0}}, nil, 6, Ring, LineStyle{Color: color.NRGBA{0, 255, 0, 255}})
	// bottom row sees the floor, top row sees the sky
	for x := 0; x < 200; x++ {
		if !(f.Z[149*200+x] > -math.MaxFloat32) {
			t.Fatalf("floor missing at bottom pixel %d", x)
		}
		if f.Z[x] > -math.MaxFloat32 {
			t.Fatalf("garbage above horizon at %d", x)
		}
	}
	for _, z := range f.Z {
		if math.IsNaN(float64(z)) || math.IsInf(float64(z), 1) {
			t.Fatal("bad depth value")
		}
	}
}

func TestCullAndDepth(t *testing.T) {
	c := &Camera{Dist: 100, FOV: 0.8}
	f := NewFrame(64, 64)
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.DrawMesh(c, Box(Vec{-10, -10, -10}, Vec{10, 10, 10}, color.NRGBA{255, 255, 255, 255}), Light{})
	z := f.Z[32*64+32]
	if math.Abs(float64(z)-1.0/90) > 1e-6 {
		t.Fatalf("centre depth key %g, want front face 1/90", z)
	}
	c.FOV = 0
	c.OrthoH = 40
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.DrawMesh(c, Box(Vec{-10, -10, -10}, Vec{10, 10, 10}, color.NRGBA{255, 255, 255, 255}), Light{})
	if z := f.Z[32*64+32]; math.Abs(float64(z)+90) > 1e-3 {
		t.Fatalf("ortho centre depth key %g, want -90", z)
	}
}

func TestSurfaceHoles(t *testing.T) {
	m := Surface(10, 10, 0, 0, 9, 9, func(i, j int) float64 {
		if i >= 3 && i <= 5 && j >= 3 && j <= 5 {
			return math.NaN()
		}
		return float64(i*j) * 0.1
	}, nil)
	for _, i := range m.Idx {
		if math.IsNaN(m.V[i].P.Z) {
			t.Fatal("triangle uses a hole vertex")
		}
	}
	for _, v := range m.V {
		if !math.IsNaN(v.P.Z) && (math.IsNaN(v.N.Len()) || math.Abs(v.N.Len()-1) > 1e-9) {
			t.Fatalf("bad normal %v", v.N)
		}
	}
	full := (9 * 9) * 2
	if n := len(m.Idx) / 3; n >= full || n < full-4*4*2-8 {
		t.Fatalf("unexpected triangle count %d", n)
	}
	mn, mx := m.Bounds()
	if mn.bad() || mx.bad() {
		t.Fatal("bounds see NaN")
	}
	c := &Camera{Yaw: 1, Pitch: 0.8}
	c.Fit(mn, mx, 100, 80)
	f := NewFrame(100, 80)
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.DrawMesh(c, m, c.KeyLight())
}

func TestDegenerateInput(t *testing.T) {
	nan := math.NaN()
	m := &Mesh{
		V: []Vertex{
			{P: Vec{nan, 0, 0}}, {P: Vec{1, 1, 1}}, {P: Vec{1, 1, 1}},
			{P: Vec{0, 0, 0}}, {P: Vec{math.Inf(1), 0, 0}}, {P: Vec{0, 1, 0}},
		},
		Idx: []int32{0, 1, 2, 1, 2, 3, 3, 4, 5, 1, 2, 99, -1, 0, 1},
		Lit: true, Smooth: true, TwoSided: true,
	}
	for _, c := range append(testCams(), &Camera{Dist: nan, Yaw: nan, FOV: nan}, &Camera{}) {
		f := NewFrame(50, 40)
		f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
		f.DrawMesh(c, m, Light{})
		f.DrawMesh(c, &Mesh{}, Light{})
		f.Lines(c, []Vec{{nan, 0, 0}, {0, 0, 0}, {0, 0, 0}, {1, 1, 1}}, true, LineStyle{})
		f.Points(c, []Vec{{nan, 1, 1}, {0, 0, 0}}, nil, 0, Diamond, LineStyle{Hidden: 0.5})
		f.Shade(color.NRGBA{}, 0.5)
		f.Downsample2()
	}
	f := NewFrame(0, -3)
	f.Resize(3, 1)
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.Downsample2()
}

func TestLinesHidden(t *testing.T) {
	c := &Camera{Yaw: -math.Pi / 2, Pitch: 1.2, Dist: 100, FOV: 0.8}
	f := NewFrame(120, 120)
	f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
	f.DrawMesh(c, &Mesh{V: Plane(-10, -50, 10, 50, 0, nil, color.NRGBA{0, 0, 0, 255}).V,
		Idx: []int32{0, 1, 2, 0, 2, 3}, TwoSided: true}, Light{})
	red := color.NRGBA{255, 0, 0, 255}
	// one line under the black plate, x-ray at 0.5
	f.Lines(c, []Vec{{-40, 0, -1}, {40, 0, -1}}, true, LineStyle{Width: 3, Color: red, Hidden: 0.5})
	sx, sy, _, _ := c.Project(Vec{0, 0, -1}, 120, 120)
	o := int(sy)*f.Img.Stride + 4*int(sx)
	if r := f.Img.Pix[o]; r < 110 || r > 145 {
		t.Fatalf("hidden line red %d, want about half", r)
	}
	sx, sy, _, _ = c.Project(Vec{30, 0, -1}, 120, 120)
	o = int(sy)*f.Img.Stride + 4*int(sx)
	if r := f.Img.Pix[o]; r < 250 {
		t.Fatalf("visible line red %d, want full", r)
	}
	for _, a := range f.acc {
		if a != 0 {
			t.Fatal("coverage buffer not reset")
		}
	}
}

func TestTwoSidedFromBelow(t *testing.T) {
	c := &Camera{Pitch: -0.8, Dist: 50, FOV: 0.8}
	l := Light{Headlight: true, Diffuse: 1}
	for _, smooth := range []bool{false, true} {
		m := Plane(-100, -100, 100, 100, 0, nil, color.NRGBA{200, 200, 200, 255})
		m.Smooth = smooth
		f := NewFrame(40, 40)
		f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
		f.DrawMesh(c, m, l)
		want := 200 * math.Sin(0.8)
		if g := float64(f.Img.Pix[(20*40+20)*4+1]); math.Abs(g-want) > 3 {
			t.Fatalf("smooth=%v: back face lit %g, want %g", smooth, g, want)
		}
		m.TwoSided = false
		f.Clear(color.NRGBA{A: 255}, color.NRGBA{A: 255})
		f.DrawMesh(c, m, l)
		if f.Z[20*40+20] > -math.MaxFloat32 {
			t.Fatal("one-sided back face not culled")
		}
	}
}
