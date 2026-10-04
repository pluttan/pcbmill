package main

import (
	"time"

	"encoding/json"
	"math"
	"os"
	"pcbscan/ui"
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

// TestCamForm: the numbers of the programs right in the lines of the stage
// Платы: the arrows keep values on their steps and in their limits, a typed
// number is taken, a wrong one is refused, a switch steps, the numbers are in
// the file next to the exe at once; a changed sheet asks for Разложить.
func TestCamForm(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PCBSCAN_DIR", dir)
	d := testApp(t)
	done := make(chan bool)
	go func() { jobMain(opts{sim: true}); close(done) }()
	waitText(t, "Числа программ")
	sendKeys(d, ui.KeyRight) // into Платы, on the heading Платы на листе
	down := func(n int) {
		for i := 0; i < n; i++ {
			sendKeys(d, ui.KeyDown)
		}
	}
	// the headings are passed by: ширина ... Разложить, кончик, угол, обороты, глубина
	down(9)
	var left []interface{}
	for i := 0; i < 50; i++ {
		left = append(left, ui.KeyLeft)
	}
	// the window may lag behind the keys: wait for the file to say so
	camIs := func(what string, ok func(c camCfg) bool) {
		t.Helper()
		end := time.Now().Add(5 * time.Second)
		for !ok(loadCam(dir)) {
			if time.Now().After(end) {
				t.Fatalf("%s: %+v", what, loadCam(dir))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	sendKeys(d, left...)
	camIs("глубина ниже предела", func(c camCfg) bool { return c.IsoDepth == camFields[3].lo })
	sendKeys(d, ui.KeyRight)
	camIs("шаг", func(c camCfg) bool { return math.Abs(c.IsoDepth-0.03) < 1e-9 })
	sendKeys(d, "0,12", ui.KeyEnter, "9", ui.KeyEnter)
	waitText(t, "глубина: от")
	down(8) // to Сверловка / как
	sendKeys(d, ui.KeyRight, ui.KeyRight)
	camIs("ввод 0,12, отказ на 9, сверловка оба", func(c camCfg) bool { return c.IsoDepth == 0.12 && c.DrillMode == drillBoth })
	// the sheet waits for Разложить
	sendKeys(d, ui.KeyEsc, ui.KeyRight, ui.KeyDown, ui.KeyRight) // out, in again, ширина +5
	waitText(t, "изменено")
	if theJob.j.W != 100 || theJob.pend().sheet[0] != 105 {
		t.Errorf("ширина листа %v, в строке %v", theJob.j.W, theJob.pend().sheet[0])
	}
	sendKeys(d, ui.KeyEsc, ui.KeyEsc)
	waitText(t, "Выйти из pcbscan?")
	sendKeys(d, "д")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("не вышел")
	}
}
