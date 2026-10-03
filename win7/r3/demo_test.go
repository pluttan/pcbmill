package r3

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ==========================
// ===  Palette           ===
// ==========================

func hex(s string) color.NRGBA {
	var v [3]uint8
	for i := 0; i < 3; i++ {
		var b uint8
		for _, ch := range s[1+2*i : 3+2*i] {
			b <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				b |= uint8(ch - '0')
			default:
				b |= uint8(ch-'a') + 10
			}
		}
		v[i] = b
	}
	return color.NRGBA{v[0], v[1], v[2], 255}
}

var mocha = []color.NRGBA{
	hex("#89b4fa"), hex("#74c7ec"), hex("#94e2d5"), hex("#a6e3a1"),
	hex("#f9e2af"), hex("#fab387"), hex("#f38ba8"),
}

// membrane mimics the L-shaped membrane mode: the r^(2/3) singular term
// of the re-entrant corner times an envelope that vanishes on the border.
// The removed quadrant x>0, y>0 is NaN.
func membrane(x, y float64) float64 {
	if x > 0 && y > 0 {
		return math.NaN()
	}
	th := math.Atan2(y, x)
	if th < math.Pi/2 {
		th += 2 * math.Pi
	}
	r := math.Hypot(x, y)
	env := (1 - x*x) * (1 - y*y)
	z := 1.6 * env * math.Pow(r, 2.0/3) * math.Sin(2.0/3*(th-math.Pi/2))
	return z + 0.18*env*math.Sin(3*x)*math.Cos(2.5*y)
}

// ==========================
// ===  Benchmark         ===
// ==========================

type benchScene struct {
	cam  *Camera
	surf *Mesh
	path []Vec
	pts  []Vec
	cols []color.NRGBA
}

func newBenchScene() *benchScene {
	s := &benchScene{}
	n := 120
	s.surf = Surface(n, n, -50, -40, 50, 40, func(i, j int) float64 {
		x, y := float64(i)/float64(n-1)*2-1, float64(j)/float64(n-1)*2-1
		return 8 * math.Sin(3*x) * math.Cos(2.5*y)
	}, func(i, j int, z float64) color.NRGBA { return Colormap(mocha, (z+8)/16) })
	for k := 0; k <= 20000; k++ {
		t := float64(k) / 20000
		s.path = append(s.path, Vec{-45 + 90*t, 35 * math.Sin(40*t), 9})
	}
	for k := 0; k < 2000; k++ {
		s.pts = append(s.pts, Vec{float64(k%50)*2 - 50, float64(k/50)*2 - 40, 10})
		s.cols = append(s.cols, mocha[k%len(mocha)])
	}
	s.cam = &Camera{Yaw: -2.2, Pitch: 0.6, FOV: 0.7}
	mn, mx := s.surf.Bounds()
	s.cam.Fit(mn, mx, 1000, 700)
	return s
}

func (s *benchScene) draw(f *Frame) {
	f.Clear(hex("#1e1e2e"), hex("#11111b"))
	f.DrawMesh(s.cam, s.surf, s.cam.KeyLight())
	f.Lines(s.cam, s.path, true, LineStyle{Width: 1.5, Color: hex("#cdd6f4"), Hidden: 0.3, Bias: 0.1})
	f.Points(s.cam, s.pts, s.cols, 5, Disc, LineStyle{Hidden: 0.3})
}

func BenchmarkScene(b *testing.B) {
	s := newBenchScene()
	f := NewFrame(1000, 700)
	s.draw(f)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.draw(f)
	}
}

func BenchmarkMeshOnly(b *testing.B) {
	s := newBenchScene()
	f := NewFrame(1000, 700)
	l := s.cam.KeyLight()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Clear(hex("#1e1e2e"), hex("#11111b"))
		f.DrawMesh(s.cam, s.surf, l)
	}
}

func BenchmarkLinesOnly(b *testing.B) {
	s := newBenchScene()
	f := NewFrame(1000, 700)
	f.Clear(hex("#1e1e2e"), hex("#11111b"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Lines(s.cam, s.path, true, LineStyle{Width: 1.5, Color: hex("#cdd6f4"), Hidden: 0.3})
	}
}

// ==========================
// ===  Demo images       ===
// ==========================

const demoDir = "/home/pluttan/.claude/jobs/2dbbf38d/tmp"

func savePNG(t *testing.T, name string, img image.Image) {
	fp, err := os.Create(filepath.Join(demoDir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	if err := png.Encode(fp, img); err != nil {
		t.Fatal(err)
	}
}

func TestDemoMatlab(t *testing.T) {
	if os.Getenv("R3_DEMO") == "" {
		t.Skip("set R3_DEMO=1")
	}
	const n = 61
	at := func(i int) float64 { return float64(i)/(n-1)*2 - 1 }
	lo, hi := math.Inf(1), math.Inf(-1)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			if z := membrane(at(i), at(j)); !math.IsNaN(z) {
				lo, hi = math.Min(lo, z), math.Max(hi, z)
			}
		}
	}
	m := Surface(n, n, -1, -1, 1, 1, func(i, j int) float64 { return membrane(at(i), at(j)) },
		func(i, j int, z float64) color.NRGBA { return Colormap(mocha, (z-lo)/(hi-lo)) })
	for i := range m.V { // stretch height for the logo look
		m.V[i].P.Z *= 1.5
	}
	W, H := 900, 700
	f := NewFrame(2*W, 2*H)
	c := &Camera{Yaw: 0.55, Pitch: 0.42, FOV: 0.6}
	mn, mx := m.Bounds()
	c.Fit(mn, mx, f.W, f.H)
	c.Zoom(1, float64(f.W)/2, float64(f.H)/2, f.W, f.H)
	f.Clear(hex("#1e1e2e"), hex("#11111b"))
	f.DrawMesh(c, m, c.KeyLight())
	// faint grid lines along every 4th row and column of the mesh
	var grid []Vec
	for k := 0; k < n; k += 4 {
		for _, dir := range []bool{true, false} {
			var prev *Vec
			for s := 0; s < n; s++ {
				i, j := k, s
				if dir {
					i, j = s, k
				}
				p := m.V[j*n+i].P
				if math.IsNaN(p.Z) {
					prev = nil
					continue
				}
				if prev != nil {
					grid = append(grid, *prev, p)
				}
				q := p
				prev = &q
			}
		}
	}
	f.Lines(c, grid, false, LineStyle{Width: 1.6, Color: color.NRGBA{17, 17, 27, 90}, Bias: 0.01})
	savePNG(t, "r3_matlab.png", f.Downsample2())
}

// tracesTexture paints a toy PCB: dark FR4 with copper traces and pads.
func tracesTexture(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{38, 52, 42, 255}
	cu := color.RGBA{214, 150, 92, 255}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, 255
	}
	set := func(x, y int) {
		if x >= 0 && y >= 0 && x < w && y < h {
			img.SetRGBA(x, y, cu)
		}
	}
	disc := func(cx, cy, r int) {
		for y := -r; y <= r; y++ {
			for x := -r; x <= r; x++ {
				if x*x+y*y <= r*r {
					set(cx+x, cy+y)
				}
			}
		}
	}
	track := func(x0, y0, x1, y1, r int) {
		steps := maxi(abs(x1-x0), abs(y1-y0))
		for s := 0; s <= steps; s++ {
			disc(x0+(x1-x0)*s/maxi(steps, 1), y0+(y1-y0)*s/maxi(steps, 1), r)
		}
	}
	for k := 0; k < 9; k++ {
		y := 60 + k*45
		x := 80 + (k%3)*30
		track(x, y, x+180, y, 4)
		track(x+180, y, x+240, y+40, 4)
		track(x+240, y+40, 700, y+40, 4)
		disc(x, y, 12)
		disc(700, y+40, 12)
	}
	for k := 0; k < 14; k++ {
		disc(760+(k%2)*40, 80+k*30, 9)
	}
	return img
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestDemoScene(t *testing.T) {
	if os.Getenv("R3_DEMO") == "" {
		t.Skip("set R3_DEMO=1")
	}
	W, H := 1000, 700
	f := NewFrame(2*W, 2*H)
	c := &Camera{Yaw: -2.0, Pitch: 0.62, FOV: 0.65}
	c.Fit(Vec{0, 0, -1.5}, Vec{100, 80, 25}, f.W, f.H)
	c.Zoom(1.3, float64(f.W)/2, float64(f.H)*0.55, f.W, f.H)
	drawPCB(t, f, c, "r3_scene.png")
	// close-up: the eye sits right next to the bit, the plate crosses the
	// near plane
	c.Target = Vec{62, 41, 3}
	c.Dist = 9
	c.Pitch = 0.25
	drawPCB(t, f, c, "r3_close.png")
}

func drawPCB(t *testing.T, f *Frame, c *Camera, name string) {
	peach := hex("#fab387")
	copper := color.NRGBA{uint8(float64(peach.R) * 0.7), uint8(float64(peach.G) * 0.62), uint8(float64(peach.B) * 0.55), 255}
	plate := Box(Vec{0, 0, -1.5}, Vec{100, 80, 0}, copper)
	top := Plane(0, 0, 100, 80, 0.001, tracesTexture(1000, 800), color.NRGBA{255, 255, 255, 255})
	bit := Cone(Vec{62, 41, 0.6}, 1.6, 6, 48, hex("#bac2de"))
	shank := Cylinder(Vec{62, 41, 6.6}, 1.6, 22, 48, hex("#a6adc8"))
	collar := Cylinder(Vec{62, 41, 18}, 3.2, 5, 48, hex("#f9e2af"))
	l := c.KeyLight()
	f.Clear(hex("#1e1e2e"), hex("#11111b"))
	f.DrawMesh(c, plate, l)
	f.DrawMesh(c, top, l)
	f.DrawMesh(c, bit, l)
	f.DrawMesh(c, shank, l)
	f.DrawMesh(c, collar, l)
	// toolpath: a spiral of passes 0.2 mm below the top
	var path []Vec
	for k := 0; k <= 3000; k++ {
		a := float64(k) / 3000 * 2 * math.Pi * 7
		r := 6 + float64(k)/3000*24
		path = append(path, Vec{50 + r*math.Cos(a), 40 + r*0.75*math.Sin(a), -0.2})
	}
	path = append(path, Vec{50 + 30, 40, 5}, Vec{62, 41, 5}, Vec{62, 41, 0.6})
	f.Lines(c, path, true, LineStyle{Width: 3, Color: hex("#89dceb"), Hidden: 0.35, Bias: 0.05})
	var pts []Vec
	var cols []color.NRGBA
	for k := 0; k < 40; k++ {
		pts = append(pts, Vec{5 + float64(k%8)*12.5, 5 + float64(k/8)*17.5, 0})
		cols = append(cols, mocha[k%len(mocha)])
	}
	f.Points(c, pts, cols, 14, Ring, LineStyle{Hidden: 0.3, Bias: 0.05})
	f.Points(c, []Vec{{62, 41, 0.6}}, nil, 16, Diamond, LineStyle{Color: hex("#f38ba8"), Hidden: 0.6, Bias: 0.05})
	f.Shade(hex("#11111b"), 0.25)
	savePNG(t, name, f.Downsample2())
}
