package gfx

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestDemo renders a showcase PNG for eyeballing quality (GFX_DEMO=1).
func TestDemo(t *testing.T) {
	if os.Getenv("GFX_DEMO") != "1" {
		t.Skip("set GFX_DEMO=1 to render the demo image")
	}
	sans, _ := testFont(t)
	var mono *Font
	if p := FindFont("LiberationMono-Regular.ttf", "consola.ttf", "DejaVuSansMono.ttf"); p != "" {
		mono, _ = LoadFont(p)
	}
	var dejavu *Font
	if p := FindFont("DejaVuSans.ttf"); p != "" {
		dejavu, _ = LoadFont(p)
	}

	const W, H = 1100, 820
	c := NewCanvas(W, H)
	bg, card, fg := Hex("#1e1e2e"), Hex("#313244"), Hex("#cdd6f4")
	sub, accent, green, red := Hex("#a6adc8"), Hex("#89b4fa"), Hex("#a6e3a1"), Hex("#f38ba8")
	c.Clear(bg)

	// Text sizes.
	y := 30.0
	ru := "Фрезеровка дорожек 0.1 мм — карта высот, Ёё Щщ Жж"
	for _, px := range []float64{11, 12, 13, 15, 18, 24, 32} {
		fc := sans.Face(px)
		y += fc.LineHeight
		c.Text(sans.Face(11), 20, y, fmt.Sprintf("%.0f", px), sub)
		c.Text(fc, 44, y, ru, fg)
	}
	y += 16
	if mono != nil {
		fm := mono.Face(13)
		y += fm.LineHeight
		c.Text(fm, 20, y, "G01 X12.500 Y-3.250 Z-0.100 F120 ; моно 13px", green)
	}
	// Fit and fallback.
	f13 := sans.Face(13)
	y += 26
	long := "Очень длинная подпись, которая не помещается в отведённое место"
	c.FillRoundRect(18, y-15, 262, 22, 4, card)
	c.Text(f13, 26, y, f13.Fit(long, 246), fg)
	if dejavu != nil {
		f13.Fallback = dejavu.Face(13)
		c.Text(f13, 300, y, "Fallback: ∯ ⌀ ⏚ ✓ ≈ ±", accent)
		f13.Fallback = nil
	}

	// Cards with shadows.
	cy := y + 30
	for i, title := range []string{"Плата", "Сканирование", "Фрезеровка"} {
		x := 20 + float64(i)*230
		c.Shadow(x, cy+4, 210, 120, 10, 18, Hex("#00000080"))
		c.FillRoundRect(x, cy, 210, 120, 10, card)
		c.StrokeRoundRect(x+0.5, cy+0.5, 209, 119, 9.5, 1, Hex("#45475a"))
		c.Text(sans.Face(15), x+16, cy+30, title, fg)
		c.Text(f13, x+16, cy+52, "Готово: 87 %", sub)
		c.FillRoundRect(x+16, cy+70, 178, 6, 3, Hex("#45475a"))
		c.FillRoundRect(x+16, cy+70, 178*0.87, 6, 3, accent)
		c.FillRoundRect(x+16, cy+88, 84, 24, 6, accent)
		c.Text(f13, x+16+(84-f13.Measure("Старт"))/2, cy+104, "Старт", bg)
	}

	// Lines of various widths and angles.
	ly := cy + 160
	for i, w := range []float64{0.5, 1, 1.5, 2, 3, 4} {
		x0 := 20 + float64(i)*110
		for k := 0; k < 7; k++ {
			a := float64(k) * math.Pi / 12
			c.Line(x0+10, ly+90, x0+10+90*math.Cos(a), ly+90-90*math.Sin(a), w, fg)
		}
		c.Text(f13, x0+10, ly+110, fmt.Sprintf("%.1f px", w), sub)
	}

	// Circles, polyline, star, gradient, image.
	ox := 700.0
	c.FillCircle(ox+40, cy+40, 30, accent)
	c.StrokeCircle(ox+120, cy+40, 30, 2, green)
	c.StrokeCircle(ox+190, cy+40, 20, 1, red)
	for r := 1.0; r <= 6; r++ {
		c.FillCircle(ox+230+r*14, cy+40, r, fg)
	}
	var star Path
	for k := 0; k < 10; k++ {
		rr := 60.0
		if k%2 == 1 {
			rr = 24
		}
		a := float64(k)*math.Pi/5 - math.Pi/2
		px, py := ox+80+rr*math.Cos(a), cy+150+rr*math.Sin(a)
		if k == 0 {
			star.MoveTo(px, py)
		} else {
			star.LineTo(px, py)
		}
	}
	star.Close()
	c.FillPath(&star, Hex("#f9e2af"))
	c.StrokePath(&star, 1.5, Hex("#fab387"))
	var wave []Pt
	for k := 0; k <= 60; k++ {
		wave = append(wave, Pt{ox + 170 + float64(k)*3, cy + 150 + 30*math.Sin(float64(k)/6)})
	}
	c.Polyline(wave, 2, accent, false)
	var heart Path
	hx, hy := ox+300, cy+120
	heart.MoveTo(hx, hy+10)
	heart.CubicTo(hx, hy-10, hx-30, hy-10, hx-30, hy+10)
	heart.QuadTo(hx-30, hy+30, hx, hy+50)
	heart.QuadTo(hx+30, hy+30, hx+30, hy+10)
	heart.CubicTo(hx+30, hy-10, hx, hy-10, hx, hy+10)
	heart.Close()
	c.FillPath(&heart, red)
	c.FillRectV(image.Rect(int(ox), int(ly+130), int(ox)+360, int(ly+160)), Hex("#89b4fa"), Hex("#cba6f7"))
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < 64; i++ {
		v := uint8(0x40)
		if (i%8+i/8)%2 == 0 {
			v = 0xd0
		}
		copy(src.Pix[i*4:], []uint8{v, v, v, 255})
	}
	c.DrawImageScaled(image.Rect(int(ox), int(ly+175), int(ox)+64, int(ly+239)), src)
	c.DrawImage(image.Pt(int(ox)+80, int(ly+175)), src)

	// Dialog.
	dx, dy := 20.0, ly+130
	c.Shadow(dx, dy+8, 520, 110, 12, 32, Hex("#000000a0"))
	c.FillRoundRect(dx, dy, 520, 110, 12, Hex("#181825"))
	c.Text(sans.Face(18), dx+20, dy+34, "Начать фрезеровку?", fg)
	c.Text(f13, dx+20, dy+58, "Шпиндель включится, ось Z опустится к плате.", sub)
	c.FillRoundRect(dx+300, dy+72, 96, 28, 6, Hex("#45475a"))
	c.Text(f13, dx+300+(96-f13.Measure("Отмена"))/2, dy+90, "Отмена", fg)
	c.FillRoundRect(dx+408, dy+72, 96, 28, 6, green)
	c.Text(f13, dx+408+(96-f13.Measure("Начать"))/2, dy+90, "Начать", bg)

	dir := os.Getenv("TMPDIR")
	if d := "/home/pluttan/.claude/jobs/2dbbf38d/tmp"; dir == "" && dirExists(d) {
		dir = d
	}
	if dir == "" {
		dir = os.TempDir()
	}
	out := filepath.Join(dir, "gfx_demo.png")
	fh, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if err := png.Encode(fh, c.Img); err != nil {
		t.Fatal(err)
	}
	t.Logf("demo written to %s", out)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
