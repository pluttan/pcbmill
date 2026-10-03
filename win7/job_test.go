package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pcbscan/ui"
)

// testJob: two kinds of boards, three pieces, on a sheet in a temp folder.
func testJob(t *testing.T) *Job {
	t.Helper()
	all := boardSamples(t)
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	j := newJob(filepath.Join(t.TempDir(), "job"))
	j.W, j.H = 140, 90
	j.Srcs = []jobSrc{{abs(all["2032Charger"]), 2}, {abs(all["ex-eagle"]), 1}}
	if err := j.layout(false, true); err != nil {
		t.Fatal(err)
	}
	return j
}

// TestLayout: every board inside the sheet past the margin, no two boards
// closer than the gap, and the corner of the zero outside all of them.
func TestLayout(t *testing.T) {
	j := testJob(t)
	if len(j.Places) != 3 {
		t.Fatalf("плат %d", len(j.Places))
	}
	r := j.boardsRaster(false, 0.25)
	// each board alone, to see that they do not overlap
	sum := 0
	for _, v := range r.bits {
		sum += int(v)
	}
	area := 0
	for _, p := range j.Places {
		one := &Job{W: j.W, H: j.H, Places: []jobPlace{p}, boards: j.boards, Srcs: j.Srcs}
		for _, v := range one.boardsRaster(false, 0.25).bits {
			area += int(v)
		}
		pl := j.placeOf(p, false)
		w, h := pl.size()
		if p.X < j.Margin-1e-9 || p.Y < j.Margin-1e-9 || p.X+w > j.W-j.Margin+1e-9 || p.Y+h > j.H-j.Margin+1e-9 {
			t.Errorf("плата %d за краем: %v %.1fx%.1f", p.Src, p, w, h)
		}
	}
	if area != sum {
		t.Errorf("платы наложились: %d пикселей по отдельности, %d вместе", area, sum)
	}
	// the gap: the distance from each board to the others
	for a := range j.Places {
		one := &Job{W: j.W, H: j.H, Places: []jobPlace{j.Places[a]}, boards: j.boards, Srcs: j.Srcs}
		ra := one.boardsRaster(false, 0.25)
		d := ra.distTo(1, nil)
		for b := range j.Places {
			if a == b {
				continue
			}
			other := &Job{W: j.W, H: j.H, Places: []jobPlace{j.Places[b]}, boards: j.boards, Srcs: j.Srcs}
			for k, v := range other.boardsRaster(false, 0.25).bits {
				if v != 0 && float64(d[k])/distScale*ra.px < j.Gap-0.5 {
					t.Fatalf("платы %d и %d ближе зазора %.1f", a, b, j.Gap)
				}
			}
		}
	}
	z := j.zero()
	if r.at(int(r.col(z.X)), int(r.row(z.Y))) {
		t.Error("угол нуля на плате")
	}
	if j.Probe == nil {
		t.Error("пробе маски не нашлось места")
	}
	// the job comes back from its file as it was
	if err := j.save(); err != nil {
		t.Fatal(err)
	}
	k, err := loadJob(j.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Places) != 3 || k.Places[1] != j.Places[1] || k.W != j.W || k.Srcs[0] != j.Srcs[0] || *k.Probe != *j.Probe {
		t.Errorf("из файла %+v, было %+v", k, j)
	}
}

// TestLayoutFull: a sheet too small says so instead of putting boards over each other.
func TestLayoutFull(t *testing.T) {
	j := testJob(t)
	j.W, j.H = 60, 60
	if err := j.layout(false, false); err == nil || !strings.Contains(err.Error(), "помещается") {
		t.Fatalf("на маленький лист: %v", err)
	}
}

// TestPlaceTurns: the four quarter turns and the mirror keep a board in its
// box and keep loops with the copper on the left of the tool.
func TestPlaceTurns(t *testing.T) {
	b, err := loadBoard(boardSamples(t)["2032Charger"])
	if err != nil {
		t.Fatal(err)
	}
	sq := []pt{{b.box.X0, b.box.Y0}, {b.box.X1, b.box.Y0}, {b.box.X1, b.box.Y1}, {b.box.X0, b.box.Y1}, {b.box.X0, b.box.Y0}}
	op := &camOp{paths: rawPaths([][]pt{sq}, -0.1)}
	area := func(ps []p3) float64 {
		s := 0.0
		for i := 1; i < len(ps); i++ {
			s += ps[i-1].X*ps[i].Y - ps[i].X*ps[i-1].Y
		}
		return s / 2
	}
	for _, mirror := range []bool{false, true} {
		for rot := 0; rot < 4; rot++ {
			p := place{box: b.box, mirror: mirror, rot: rot, dx: 10, dy: 20}
			w, h := p.size()
			got := placeOp(op, []place{p}).paths[0].pts
			for _, q := range got {
				if q.X < 10-1e-9 || q.Y < 20-1e-9 || q.X > 10+w+1e-9 || q.Y > 20+h+1e-9 {
					t.Fatalf("поворот %d зеркало %v: точка %v вне %vx%v", rot, mirror, q, w, h)
				}
			}
			if area(got) <= 0 {
				t.Errorf("поворот %d зеркало %v: обход перевернулся", rot, mirror)
			}
		}
	}
}

// TestScanPoints: the zero first, every other point near a board, none far.
func TestScanPoints(t *testing.T) {
	j := testJob(t)
	pts := j.scanPoints(false)
	if z := j.zero(); pts[0] != [2]float64{z.X, z.Y} {
		t.Fatalf("первая точка %v, а угол %v", pts[0], z)
	}
	u := j.used(false)
	full := GridFor(u.X0, u.X1, u.Y0, u.Y1, j.Step).Points()
	if len(pts)-1 >= len(full) {
		t.Errorf("точек %d из прямоугольника %d: форма плат не учтена", len(pts)-1, len(full))
	}
	r := j.boardsRaster(false, 0.5)
	d := r.distTo(1, nil)
	for _, q := range pts[1:] {
		i, k := int(r.col(q[0])+0.5), int(r.row(q[1])+0.5)
		if dist := float64(d[k*r.w+i]) / distScale * r.px; dist > j.Step {
			t.Errorf("точка %v в %.1f мм от плат", q, dist)
		}
	}
	t.Logf("точек %d (прямоугольник %d)", len(pts), len(full))
}

// TestJobPrograms: the programs of the sheet in their files, inside the sheet,
// readable back, levelled by a map of the job.
func TestJobPrograms(t *testing.T) {
	j := testJob(t)
	c := defaultCam()
	ops, err := j.cam(c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.writeProgs(ops); err != nil {
		t.Fatal(err)
	}
	for _, s := range stages {
		if len(j.stageFiles(s.prefix)) == 0 {
			t.Errorf("этап %s без файлов", s.title)
		}
	}
	if c.MaskTest {
		if m := j.stageFiles("2_"); len(m) < 2 || !strings.Contains(m[0], "_probe") {
			t.Errorf("проба маски не первая: %v", m)
		}
	}
	for _, f := range j.stageFiles("") {
		lines, err := readLines(f)
		if err != nil {
			t.Fatal(err)
		}
		depth, cut, min, err := progInfo(lines)
		if err != nil {
			t.Fatal(f, err)
		}
		lo := j.Margin - 1
		if strings.Contains(f, "sheet_2_mask.") { // the bare circle at the zero
			lo = j.Margin/2 - zeroSpotR - 0.01
			z := j.zero()
			if math.Abs(cut.X0-(z.X-zeroSpotR)) > 0.1 || math.Abs(cut.Y0-(z.Y-zeroSpotR)) > 0.1 {
				t.Errorf("маска не вскрывает круг в ноле: рез от X%.2f Y%.2f", cut.X0, cut.Y0)
			}
		}
		if cut.X0 < lo || cut.Y0 < lo || cut.X1 > j.W-j.Margin+1 || cut.Y1 > j.H-j.Margin+1 {
			t.Errorf("%s: рез X %.1f..%.1f Y %.1f..%.1f за листом", filepath.Base(f), cut.X0, cut.X1, cut.Y0, cut.Y1)
		}
		title, _, _, _ := progNotes(f)
		t.Logf("%-28s %-22s глубина %.3f ~%.0f мин, %s", filepath.Base(f), toolOf(f, c), depth, min, title)
	}
	// a map of this sheet levels them; one with another zero is refused
	pts := j.scanPoints(false)
	var rows []string
	for _, q := range pts {
		rows = append(rows, num(q[0])+","+num(q[1])+",0.01")
	}
	os.WriteFile(j.mapPath(), []byte(strings.Join(rows, "\n")), 0644)
	if _, err := jobMap(j); err != nil {
		t.Fatal(err)
	}
	j.Margin += 2
	if _, err := jobMap(j); err == nil {
		t.Error("карта с чужим нулём принята")
	}
}

// TestRedo: the paths of a program inside a circle, deeper, in a new file
// that reads back; outside the circle nothing.
func TestRedo(t *testing.T) {
	j := testJob(t)
	ops, err := j.cam(defaultCam(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.writeProgs(ops); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{j.progPath("1_iso"), j.progPath("3_drill_1.0"), j.progPath("4_vcut")} {
		src, _ := readLines(f)
		before, _ := readProg(src)
		var c pt
		for _, p := range before.paths { // a circle round some cut
			c = pt{p.pts[0].X, p.pts[0].Y}
			break
		}
		dst, n, err := redoProg(f, c, 6, 0.1)
		if err != nil {
			t.Fatal(f, err)
		}
		lines, _ := readLines(dst)
		after, err := readProg(lines)
		if err != nil {
			t.Fatal(err)
		}
		if len(after.paths) > n || len(after.paths) == 0 { // pieces that meet are one path in the file
			t.Fatalf("%s: путей %d, сказано %d", dst, len(after.paths), n)
		}
		deep := 0.0
		for _, p := range before.paths {
			for _, q := range p.pts {
				deep = math.Max(deep, -q.Z)
			}
		}
		for _, p := range after.paths {
			for _, q := range p.pts {
				if math.Hypot(q.X-c.X, q.Y-c.Y) > 6+1e-3 {
					t.Fatalf("%s: точка %v вне круга", filepath.Base(dst), q)
				}
				if -q.Z > deep+0.1+1e-6 || -q.Z < 0.1 {
					t.Fatalf("%s: глубина %.3f, было до %.3f", filepath.Base(dst), -q.Z, deep)
				}
			}
		}
		if after.feed != before.feed || after.plunge != before.plunge || after.spindle != before.spindle {
			t.Errorf("%s: подачи %v/%v/%v, было %v/%v/%v", filepath.Base(dst), after.feed, after.plunge, after.spindle,
				before.feed, before.plunge, before.spindle)
		}
		if strings.Contains(f, "drill") && after.paths[0].peck == 0 {
			t.Errorf("%s: сверловка без выводов стружки", filepath.Base(dst))
		}
		if !strings.Contains(dst, "_redo1") {
			t.Errorf("имя %s", dst)
		}
		t.Logf("%s: %d путей", filepath.Base(dst), n)
	}
	if _, _, err := redoProg(j.progPath("1_iso"), pt{-50, -50}, 3, 0.1); err == nil {
		t.Error("пустой круг дал файл")
	}
}

// TestAskYes: only the word yes says yes; the Russian layout gets a hint.
func TestAskYes(t *testing.T) {
	for _, c := range []struct {
		keys []interface{}
		want bool
	}{
		{[]interface{}{"yes", ui.KeyEnter}, true},
		{[]interface{}{"YES", ui.KeyEnter}, true},
		{[]interface{}{"y", ui.KeyEnter}, false},
		{[]interface{}{ui.KeyEnter, "yes", ui.KeyEnter}, true}, // empty: asked again
		{[]interface{}{"нуы", ui.KeyEnter, "yes", ui.KeyEnter}, true},
		{[]interface{}{"yes", ui.KeyEsc}, false},
		{[]interface{}{"да", ui.KeyEnter}, false},
	} {
		d := testApp(t)
		got := make(chan bool)
		go func() { got <- askYes("Открыть?", "строка") }()
		waitText(t, "Напечатайте yes")
		sendKeys(d, c.keys...)
		select {
		case g := <-got:
			if g != c.want {
				t.Errorf("%v: %v", c.keys, g)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%v: нет ответа", c.keys)
		}
	}
}

// jobScreen: the job screen over an empty job, for the dialogs to lie on.
func jobScreen(t *testing.T) *jobUI {
	t.Helper()
	return &jobUI{j: newJob(filepath.Join(t.TempDir(), "job")), c: defaultCam()}
}
