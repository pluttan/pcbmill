package gfx

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func TestHex(t *testing.T) {
	cases := map[string]color.NRGBA{
		"#1e1e2e":   {0x1e, 0x1e, 0x2e, 255},
		"#1e1e2ecc": {0x1e, 0x1e, 0x2e, 0xcc},
		"CDD6F4":    {0xcd, 0xd6, 0xf4, 255},
		"#fff":      {255, 255, 255, 255},
		"#12345":    {255, 0, 255, 255},
		"#zzzzzz":   {255, 0, 255, 255},
	}
	for in, want := range cases {
		if got := Hex(in); got != want {
			t.Errorf("Hex(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMixAlpha(t *testing.T) {
	a, b := Hex("#000000"), Hex("#ffffff00")
	if got := Mix(a, b, 0.5); got != (color.NRGBA{128, 128, 128, 128}) {
		t.Errorf("Mix = %v", got)
	}
	if Mix(a, b, 0) != a || Mix(a, b, 1) != b || Mix(a, b, -3) != a {
		t.Error("Mix endpoints")
	}
	if got := Alpha(Hex("#ffffff"), 0.5); got.A != 128 {
		t.Errorf("Alpha = %v", got)
	}
}

// coverageSum fills poly into a fresh raster and sums its coverage.
func coverageSum(b image.Rectangle, poly []Pt) (*raster, float64) {
	r := getRaster(b)
	r.polygon(poly)
	sum := 0.0
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			sum += r.coverage(x, y)
		}
	}
	return r, sum
}

func square(x, y, s float64) []Pt {
	return []Pt{{x, y}, {x + s, y}, {x + s, y + s}, {x, y + s}}
}

func TestSquareCoverage(t *testing.T) {
	_, sum := coverageSum(image.Rect(0, 0, 16, 16), square(3, 3, 10))
	if math.Abs(sum-100) > 1e-4 {
		t.Fatalf("10x10 square coverage %v, want 100", sum)
	}
	r, sum := coverageSum(image.Rect(0, 0, 16, 16), square(2.5, 3, 10))
	if math.Abs(sum-100) > 1e-4 {
		t.Fatalf("shifted square coverage %v, want 100", sum)
	}
	for y := 3; y < 13; y++ {
		if l, rr := r.coverage(2, y), r.coverage(12, y); math.Abs(l-0.5) > 1e-5 || math.Abs(rr-0.5) > 1e-5 {
			t.Fatalf("edge pixels at row %d: %v %v, want 0.5", y, l, rr)
		}
		if m := r.coverage(7, y); math.Abs(m-1) > 1e-5 {
			t.Fatalf("interior %v", m)
		}
	}
	// Reversed winding must give the same coverage.
	p := square(3, 3, 10)
	p[1], p[3] = p[3], p[1]
	if _, s := coverageSum(image.Rect(0, 0, 16, 16), p); math.Abs(s-100) > 1e-4 {
		t.Fatalf("reversed square %v", s)
	}
}

func TestPartiallyOutside(t *testing.T) {
	// A square sticking out on all sides: visible part is the buffer.
	_, sum := coverageSum(image.Rect(0, 0, 8, 8), []Pt{{-5, -3}, {20, -1}, {14, 30}, {-9, 12}})
	if math.Abs(sum-64) > 1e-3 {
		t.Fatalf("clipped coverage %v, want 64", sum)
	}
	// A diagonal triangle half inside: area of the visible part.
	_, sum = coverageSum(image.Rect(0, 0, 10, 10), []Pt{{-10, 0}, {10, 0}, {-10, 20}})
	if math.Abs(sum-50) > 1e-3 {
		t.Fatalf("triangle coverage %v, want 50", sum)
	}
}

func TestFillPathCanvas(t *testing.T) {
	c := NewCanvas(20, 20)
	var p Path
	p.MoveTo(5, 5)
	p.LineTo(15, 5)
	p.LineTo(15, 15)
	p.LineTo(5, 15)
	p.Close()
	c.FillPath(&p, Hex("#ffffff"))
	total := 0
	for i := 3; i < len(c.Img.Pix); i += 4 {
		total += int(c.Img.Pix[i])
	}
	if total != 100*255 {
		t.Fatalf("alpha sum %d, want %d", total, 100*255)
	}
}

func TestClip(t *testing.T) {
	c := NewCanvas(40, 40)
	s := c.Sub(image.Rect(10, 10, 20, 20))
	col := Hex("#ffffff")
	s.FillRect(image.Rect(0, 0, 40, 40), col)
	s.FillRoundRect(-5, -5, 50, 50, 8, col)
	s.StrokeRoundRect(2, 2, 30, 30, 6, 3, col)
	s.FillCircle(15, 15, 30, col)
	s.StrokeCircle(15, 15, 8, 2, col)
	s.Line(-10, -10, 50, 50, 3, col)
	s.Shadow(0, 0, 40, 40, 6, 10, col)
	s.FillRectV(image.Rect(0, 0, 40, 40), col, col)
	s.Clear(col)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	s.DrawImage(image.Pt(5, 5), img)
	s.DrawImageScaled(image.Rect(-10, -10, 60, 60), img)
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			in := image.Pt(x, y).In(s.Clip)
			if a := c.Img.Pix[c.Img.PixOffset(x, y)+3]; !in && a != 0 {
				t.Fatalf("pixel %d,%d outside clip touched", x, y)
			}
		}
	}
	// A clip outside the image must not panic.
	c.Sub(image.Rect(100, 100, 200, 200)).FillRoundRect(90, 90, 50, 50, 5, col)
	(&Canvas{Img: c.Img, Clip: image.Rect(-50, -50, 500, 500)}).FillCircle(0, 0, 100, col)
}

func TestStrokeSymmetry(t *testing.T) {
	// A 1 px horizontal line centred on a pixel row covers that row fully.
	c := NewCanvas(20, 10)
	c.Line(4, 5.5, 16, 5.5, 1, Hex("#ffffff"))
	for x := 5; x < 15; x++ {
		if a := c.Img.Pix[c.Img.PixOffset(x, 5)+3]; a != 255 {
			t.Fatalf("pixel %d alpha %d", x, a)
		}
		if a := c.Img.Pix[c.Img.PixOffset(x, 4)+3]; a != 0 {
			t.Fatalf("row above touched at %d: %d", x, a)
		}
	}
	// Polyline with sharp turns must not leave holes or panic.
	c.Polyline([]Pt{{1, 1}, {18, 1}, {1, 2}, {18, 8}, {18, 8}, {2, 8}}, 2, Hex("#ffffff"), true)
}

func TestRoundRectArea(t *testing.T) {
	c := NewCanvas(60, 60)
	c.FillRoundRect(5.25, 5.5, 40, 30, 8, Hex("#ffffff"))
	total := 0.0
	for i := 3; i < len(c.Img.Pix); i += 4 {
		total += float64(c.Img.Pix[i]) / 255
	}
	want := 40*30 - (4-math.Pi)*64
	if math.Abs(total-want) > 2 {
		t.Fatalf("round rect area %v, want %v", total, want)
	}
}

func TestGarbageFont(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		b := make([]byte, rng.Intn(400))
		rng.Read(b)
		if i%3 == 0 && len(b) >= 4 {
			copy(b, []byte{0, 1, 0, 0})
		}
		if f, err := ParseFont(b); err == nil {
			exercise(f)
		}
	}
}

// exercise touches every lazy path of a font that parsed.
func exercise(f *Font) {
	fc := f.Face(13)
	c := NewCanvas(64, 32)
	s := "Hello, Фрезеровка … ЁёЩщ"
	fc.Measure(s)
	fc.Fit(s, 30)
	c.Text(fc, 2, 20, s, Hex("#cdd6f4"))
	for g := 0; g < f.numGlyphs && g < 300; g++ {
		f.outline(uint16(g))
	}
}
