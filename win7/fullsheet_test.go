package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBoardFillsSheet: a board as big as the blank: no margin, the zero 2 mm
// in from the corner, no V-groove, every cut on the blank.
func TestBoardFillsSheet(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	b, err := loadBoard(zip)
	if err != nil {
		t.Skip(err)
	}
	j := newJob(filepath.Join(t.TempDir(), "job"))
	j.W, j.H, j.Margin = b.box.X1-b.box.X0, b.box.Y1-b.box.Y0, 0
	j.Srcs = []jobSrc{{zip, 1}}
	if err := j.layout(false, false, false); err != nil {
		t.Fatal(err)
	}
	if z := j.zero(); z.X != zeroInset || z.Y != zeroInset {
		t.Errorf("ноль в X%.1f Y%.1f", z.X, z.Y)
	}
	if !j.fillsSheet(false) {
		t.Fatalf("плата не на весь лист: %+v", j.used(false))
	}
	c := defaultCam()
	c.IsoTest, c.MaskTest = false, false
	ops, err := j.cam(c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if strings.HasPrefix(op.file, "4_vcut") {
			t.Errorf("V-рез по краю заготовки: %s", op.file)
		}
		x0, x1, y0, y1, err := CutBounds(op.gcode())
		if err != nil {
			t.Fatal(err)
		}
		if x0 < -0.5 || y0 < -0.5 || x1 > j.W+0.5 || y1 > j.H+0.5 {
			t.Errorf("%s режет X %.1f..%.1f Y %.1f..%.1f на листе %vx%v", op.file, x0, x1, y0, y1, j.W, j.H)
		}
		t.Logf("%s", op.file)
	}
}
