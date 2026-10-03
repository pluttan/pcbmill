package main

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestCamSamples: the small sample boards give programs that the leveller and
// the scan take, every operation on both sides.
func TestCamSamples(t *testing.T) {
	hm, _ := MapFromPoints([]Point{{0, 0, 0}, {300, 0, 0.1}, {0, 300, -0.05}, {300, 300, 0.08}})
	all := boardSamples(t)
	for _, name := range []string{"2032Charger", "ex-eagle", "ex-kicad-flashpads"} {
		b, err := loadBoard(all[name])
		if err != nil {
			t.Fatal(name, err)
		}
		kinds := map[string]bool{}
		for _, side := range []int{sideTop, sideBottom} {
			c := defaultCam()
			c.Side, c.DrillMode = side, drillBoth
			for _, raw := range camBoard(b, c, func(string) {}) {
				op := placeOp(raw, []place{{box: b.box, mirror: side == sideBottom}})
				kinds[strings.SplitN(op.file, "_", 3)[1]] = true
				lines := op.gcode()
				if lines[0] != "T1M6" || lines[len(lines)-1] != "M30" {
					t.Errorf("%s %s: начало %q конец %q", name, op.file, lines[0], lines[len(lines)-1])
				}
				if _, err := ApplyMap(lines, hm, 1); err != nil {
					t.Errorf("%s %s: %v", name, op.file, err)
				}
				x0, x1, y0, y1, err := CutBounds(lines)
				if err != nil {
					t.Errorf("%s %s: %v", name, op.file, err)
				}
				bw, bh := b.box.X1-b.box.X0, b.box.Y1-b.box.Y0
				if x0 < -0.5 || y0 < -0.5 || x1 > bw+0.5 || y1 > bh+0.5 {
					t.Errorf("%s %s: рез X %.2f..%.2f Y %.2f..%.2f за платой %.1f x %.1f", name, op.file, x0, x1, y0, y1, bw, bh)
				}
				cut, min := op.minutes()
				t.Logf("%s %-14s путей %4d строк %6d рез %6.0f мм %5.1f мин %v %v", name, op.file, len(op.paths), len(lines), cut, min, op.notes, op.warn)
			}
		}
		for _, k := range []string{"iso", "drill", "vcut"} {
			if !kinds[k] {
				t.Errorf("%s: нет программы %s", name, k)
			}
		}
	}
}

// TestIsoLikePython: the isolation of the top is as long as pcbmill makes it
// with shapely buffers, and the narrow places are the same.
func TestIsoLikePython(t *testing.T) {
	raw, err := os.ReadFile("testdata/gerbers/ref_iso.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string][3]float64
	json.Unmarshal(raw, &ref)
	all := boardSamples(t)
	for name, want := range ref {
		b, err := loadBoard(all[name])
		if err != nil {
			t.Fatal(name, err)
		}
		op, _ := isolation(b, lyTop, defaultCam(), nil)
		l := 0.0
		for _, p := range op.paths {
			for i := 1; i < len(p.pts); i++ {
				l += math.Hypot(p.pts[i].X-p.pts[i-1].X, p.pts[i].Y-p.pts[i-1].Y)
			}
		}
		if math.Abs(l-want[0])/want[0] > 0.005 || len(op.marks) != int(want[2]) {
			t.Errorf("%s: рез %.1f мм, мест %d; у pcbmill %.1f мм, мест %.0f", name, l, len(op.marks), want[0], want[2])
		}
	}
}

// TestIsoClearance: every point of the first pass is half a cut from the
// copper, the copper is on the left (climb milling with the spindle turning
// clockwise), and nothing of the tool touches the copper.
func TestIsoClearance(t *testing.T) {
	b, err := loadBoard(boardSamples(t)["2032Charger"])
	if err != nil {
		t.Fatal(err)
	}
	c := defaultCam()
	c.Passes = 1
	op, _ := isolation(b, lyTop, c, nil)
	w := c.cutWidth(c.IsoDepth)
	g := b.copper[lyTop]
	cu, _ := g.bounds()
	r := newRaster(rect{cu.X0 - 2, cu.X1 + 2, cu.Y0 - 2, cu.Y1 + 2}, 0.005)
	for _, s := range g.shapes {
		r.draw(s)
	}
	field := r.distTo(1, nil)
	near := func(q pt) float64 { // mm from the copper, at a table point
		x, y := q.X, q.Y
		i, j := int(math.Round(r.col(x))), int(math.Round(r.row(y)))
		return (float64(field[j*r.w+i])/distScale - 0.5) * r.px
	}
	worst, left, right := 1.0, 0, 0
	var ds []float64
	for _, p := range op.paths {
		for i := 1; i < len(p.pts); i++ {
			a, e := p.pts[i-1], p.pts[i]
			for k := 0; k < 4; k++ {
				f := float64(k) / 4
				q := pt{a.X + (e.X-a.X)*f, a.Y + (e.Y-a.Y)*f}
				worst = math.Min(worst, near(q))
				ds = append(ds, near(q))
			}
			l := math.Hypot(e.X-a.X, e.Y-a.Y)
			if l < 0.05 {
				continue
			}
			m := pt{(a.X + e.X) / 2, (a.Y + e.Y) / 2}
			nx, ny := -(e.Y-a.Y)/l*w*0.45, (e.X-a.X)/l*w*0.45
			if near(pt{m.X + nx, m.Y + ny}) < near(pt{m.X - nx, m.Y - ny}) {
				left++
			} else {
				right++
			}
		}
	}
	// the grid is 0.01 mm: a point may stray by a pixel where two pads pinch the path
	sort.Float64s(ds)
	if p1 := ds[len(ds)/100]; worst < w/2-0.015 || p1 < w/2-0.01 {
		t.Errorf("фреза ближе к меди: %.3f мм (1%% точек ближе %.3f) при половине реза %.3f", worst, p1, w/2)
	}
	if left < 20*right {
		t.Errorf("медь слева %d раз, справа %d: обход не попутный", left, right)
	}
	t.Logf("ближе всего к меди %.4f мм (половина реза %.4f), медь слева %d, справа %d", worst, w/2, left, right)
}

// TestCamForm: the form fits every window, scrolls to the chosen line, the
// arrows keep values on their steps and in their limits, and the numbers
// come back from the file next to the exe.
func TestCamForm(t *testing.T) {
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		f := &camForm{c: defaultCam()}
		for i := 0; i < f.count(); i++ {
			fr := f.view(sz[0], sz[1])
			if len(fr) != sz[1] {
				t.Fatalf("%v: %d строк", sz, len(fr))
			}
			onScreen(t, "форма", fr, sz[0])
			all := plainFrame(fr)
			want := camButtons[0]
			if i < len(camFields) {
				want = camFields[i].label
			} else {
				want = camButtons[i-len(camFields)]
			}
			if !strings.Contains(all, want) {
				t.Fatalf("%v: выбрано %d, не видно %q\n%s", sz, i, want, all)
			}
			f.key(k(KeyDown))
		}
	}
	f := &camForm{c: defaultCam()}
	f.sel = 3 // the depth of isolation
	for i := 0; i < 50; i++ {
		f.key(k(KeyLeft))
	}
	if f.c.IsoDepth != camFields[3].lo {
		t.Errorf("глубина %v, ниже предела", f.c.IsoDepth)
	}
	f.key(k(KeyRight))
	if math.Abs(f.c.IsoDepth-0.03) > 1e-9 {
		t.Errorf("шаг: %v", f.c.IsoDepth)
	}
	for _, x := range keys("0", ",", "1", "2", KeyEnter) {
		f.key(x)
	}
	if f.c.IsoDepth != 0.12 {
		t.Errorf("ввод 0,12: %v %q", f.c.IsoDepth, f.msg)
	}
	for _, x := range keys("9", KeyEnter) {
		f.key(x)
	}
	if f.c.IsoDepth != 0.12 || f.msg == "" {
		t.Errorf("9 мм приняты: %v", f.c.IsoDepth)
	}
	f.sel = 11 // the drilling switch
	f.key(k(KeyRight))
	f.key(k(KeyRight))
	if f.c.DrillMode != drillBoth {
		t.Errorf("сверловка %d", f.c.DrillMode)
	}
	dir := t.TempDir()
	if err := saveCam(dir, f.c); err != nil {
		t.Fatal(err)
	}
	if got := loadCam(dir); got != f.c {
		t.Errorf("из файла %+v, а было %+v", got, f.c)
	}
	if done, end := f.key(k(KeyEsc)); done || !end {
		t.Error("Esc")
	}
}
