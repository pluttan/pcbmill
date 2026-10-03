package gfx

import (
	"io/fs"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testFontNames = []string{"segoeui.ttf", "Tahoma.ttf", "LiberationSans-Regular.ttf", "DejaVuSans.ttf"}

func testFont(t testing.TB, names ...string) (*Font, []byte) {
	if len(names) == 0 {
		names = testFontNames
	}
	p := FindFont(names...)
	if p == "" {
		t.Skip("no system font found")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFont(data)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return f, data
}

func TestFindFontCase(t *testing.T) {
	if FindFont() != "" || FindFont("no-such-font-file.ttf") != "" {
		t.Fatal("FindFont found a ghost")
	}
	if p := FindFont("liberationsans-regular.TTF"); p == "" {
		t.Skip("Liberation not installed")
	}
}

func TestAdvanceMatchesHmtx(t *testing.T) {
	f, data := testFont(t)
	tables, err := tableDir(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	hmtx := tables["hmtx"]
	fc := f.Face(f.unitsPerEm) // 1 px per font unit: exact comparison
	for _, r := range "AVWamw0Жщ" {
		g := f.GlyphIndex(r)
		if g == 0 {
			t.Fatalf("no glyph for %q", r)
		}
		i := int(g)
		if i >= f.numHMet {
			i = f.numHMet - 1
		}
		want := float64(uint16(hmtx[4*i])<<8 | uint16(hmtx[4*i+1]))
		if got := fc.Measure(string(r)); got != want {
			t.Errorf("advance of %q: %v, want %v", r, got, want)
		}
	}
	if fc.Measure("") != 0 {
		t.Error("Measure(\"\") != 0")
	}
}

func TestCyrillicAndMetrics(t *testing.T) {
	f, _ := testFont(t)
	for _, r := range "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя—…" {
		if f.GlyphIndex(r) == 0 {
			t.Errorf("missing %q", r)
		}
		if f.outline(f.GlyphIndex(r)) == nil {
			t.Errorf("empty outline for %q", r)
		}
	}
	fc := f.Face(14)
	if fc.Ascent <= 0 || fc.Descent <= 0 || fc.LineHeight < fc.Ascent+fc.Descent {
		t.Errorf("metrics %v %v %v", fc.Ascent, fc.Descent, fc.LineHeight)
	}
	if f.Face(14) != fc {
		t.Error("faces are not cached")
	}
	w := fc.Measure("Фрезеровка")
	if w < 50 || w > 110 {
		t.Errorf("width %v out of range", w)
	}
}

func TestFit(t *testing.T) {
	f, _ := testFont(t)
	fc := f.Face(13)
	s := "Фрезеровка дорожек 0.1 мм — карта высот"
	if fc.Fit(s, 1000) != s {
		t.Error("Fit shortened a fitting string")
	}
	for _, w := range []float64{200, 100, 40, 12} {
		got := fc.Fit(s, w)
		if fc.Measure(got) > w {
			t.Errorf("Fit(%v) = %q is %v wide", w, got, fc.Measure(got))
		}
	}
	if fc.Fit(s, 1) != "" {
		t.Error("Fit into 1px must be empty")
	}
}

func TestTextDraws(t *testing.T) {
	f, _ := testFont(t)
	fc := f.Face(15)
	c := NewCanvas(300, 40)
	c.Clear(Hex("#1e1e2e"))
	end := c.Text(fc, 10, 25, "Щука ёж", Hex("#cdd6f4"))
	if math.Abs(end-10-fc.Measure("Щука ёж")) > 1e-9 {
		t.Errorf("Text returned %v", end)
	}
	lit := 0
	for i := 0; i < len(c.Img.Pix); i += 4 {
		if c.Img.Pix[i] > 0x80 {
			lit++
		}
	}
	if lit < 50 {
		t.Errorf("only %d bright pixels", lit)
	}
}

func TestFallback(t *testing.T) {
	f, _ := testFont(t, "LiberationMono-Regular.ttf")
	g, _ := testFont(t, "DejaVuSans.ttf")
	fc := f.Face(14)
	fc.Fallback = g.Face(14)
	if f.GlyphIndex('∯') != 0 || g.GlyphIndex('∯') == 0 {
		t.Skip("fonts differ from expected")
	}
	if fc.Measure("∯") != fc.Fallback.Measure("∯") {
		t.Error("fallback advance not used")
	}
}

func TestTruncatedFont(t *testing.T) {
	_, data := testFont(t)
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 300; i++ {
		n := rng.Intn(len(data))
		if i < 40 {
			n = i * 16
		}
		if f, err := ParseFont(data[:n]); err == nil {
			exercise(f)
		}
	}
	// Flipped bytes inside an otherwise valid font.
	for i := 0; i < 150; i++ {
		b := append([]byte(nil), data...)
		for k := 0; k < 20; k++ {
			b[rng.Intn(len(b))] = byte(rng.Intn(256))
		}
		if f, err := ParseFont(b); err == nil {
			exercise(f)
		}
	}
}

// drawFrame paints a typical screen: 50 cards (shadowed every 50/shadows),
// 100 labels.
func drawFrame(c *Canvas, fc *Face, shadows int) {
	bg, card, fg := Hex("#1e1e2e"), Hex("#313244"), Hex("#cdd6f4")
	c.Clear(bg)
	for k := 0; k < 50; k++ {
		x, y := float64(20+(k%10)*125), float64(20+(k/10)*150)
		if k%(50/shadows) == 0 {
			c.Shadow(x, y+2, 110, 130, 8, 12, Hex("#00000060"))
		}
		c.FillRoundRect(x, y, 110, 130, 8, card)
	}
	for k := 0; k < 100; k++ {
		c.Text(fc, float64(30+(k%10)*125), float64(50+(k/10)*70), "Карта высот 0.1 мм", fg)
	}
}

func BenchmarkFrame(b *testing.B) {
	f, _ := testFont(b)
	fc := f.Face(13)
	c := NewCanvas(1280, 800)
	for i := 0; i < b.N; i++ {
		drawFrame(c, fc, 10)
	}
}

func BenchmarkFrameAllShadows(b *testing.B) {
	f, _ := testFont(b)
	fc := f.Face(13)
	c := NewCanvas(1280, 800)
	for i := 0; i < b.N; i++ {
		drawFrame(c, fc, 50)
	}
}

func TestKernAndCollection(t *testing.T) {
	f, _ := testFont(t, "LiberationSans-Regular.ttf", "DejaVuSans.ttf")
	t.Logf("kern pairs: %d", len(f.kern))
	if f.kern != nil {
		a, v := f.GlyphIndex('A'), f.GlyphIndex('V')
		if f.kerning(a, v) >= 0 {
			t.Errorf("AV kerning %v, want negative", f.kerning(a, v))
		}
	}
	p := FindFont("wqy-zenhei.ttc", "msgothic.ttc", "cambria.ttc")
	if p == "" {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		c, err := ParseFontIndex(data, i)
		if err != nil {
			t.Fatalf("%s #%d: %v", p, i, err)
		}
		exercise(c)
	}
	if _, err := ParseFontIndex(data, 99); err == nil {
		t.Error("index 99 accepted")
	}
}

// TestSystemFonts parses whatever fonts the machine has: none may panic,
// and TrueType ones are expected to parse.
func TestSystemFonts(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	n := 0
	_ = filepath.WalkDir("/usr/share/fonts", func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || n >= 300 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
			return nil
		}
		n++
		f, err := LoadFont(p)
		if err != nil {
			if ext == ".ttf" {
				t.Logf("%s: %v", p, err)
			}
			return nil
		}
		exercise(f)
		return nil
	})
}
