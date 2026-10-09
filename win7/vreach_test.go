package main

import (
	"math"
	"path/filepath"
	"testing"
)

// TestVcutReach: on the test sheet the sides of the board go on to the edge
// of the sheet or stop short of a probe, never into one, never off the sheet.
func TestVcutReach(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	j := newJob(filepath.Join(t.TempDir(), "job"))
	j.Srcs = []jobSrc{{zip, 1}}
	if err := j.layout(false, true, true); err != nil {
		t.Skip(err)
	}
	c := defaultCam()
	ext := j.vcutReach(false, c)
	if len(ext) == 0 {
		t.Fatal("канавок к краю нет")
	}
	edge := 0
	for _, p := range ext {
		a, b := p.pts[0], p.pts[1]
		for _, q := range []p3{a, b} {
			if q.X < -1e-9 || q.Y < -1e-9 || q.X > j.W+1e-9 || q.Y > j.H+1e-9 {
				t.Fatalf("канавка за листом: %v", q)
			}
		}
		if b.X == 0 || b.Y == 0 || b.X == j.W || b.Y == j.H {
			edge++
		}
		for _, s := range j.probeBoxes() {
			for k := 0.05; k < 1; k += 0.05 { // the inside of the groove, not its start on the board
				x, y := a.X+(b.X-a.X)*k, a.Y+(b.Y-a.Y)*k
				if x > s.at.X && x < s.at.X+s.w && y > s.at.Y && y < s.at.Y+s.h {
					t.Fatalf("канавка %v-%v режет пробу %s", a, b, s.name)
				}
			}
		}
	}
	lv := len(levels(c.VDepth, c.VStep))
	t.Logf("отрезков %d, до края листа %d", len(ext)/lv, edge/lv)
	if edge == 0 {
		t.Error("ни одна канавка не дошла до края листа")
	}
	ops, err := j.cam(c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.file != "4_vcut" {
			continue
		}
		x0, x1, y0, y1, err := CutBounds(op.gcode())
		if err != nil {
			t.Fatal(err)
		}
		if x0 < -0.5 || y0 < -0.5 || x1 > j.W+0.5 || y1 > j.H+0.5 || math.IsInf(x0, 0) {
			t.Errorf("4_vcut режет X %.1f..%.1f Y %.1f..%.1f на листе %vx%v", x0, x1, y0, y1, j.W, j.H)
		}
		_, min := op.minutes()
		t.Logf("4_vcut ~%.1f мин", min)
	}
}
