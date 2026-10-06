package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTestBoard: the test board of the mill (make testboard) goes on a 100 x
// 100 blank with all the probes, and every stage gets its programs.
func TestTestBoard(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	if _, err := os.Stat(zip); err != nil {
		t.Skip("нет testboard/pcbmill-test.zip: make testboard")
	}
	b, err := loadBoard(zip)
	if err != nil {
		t.Fatal(err)
	}
	w, h := b.box.X1-b.box.X0, b.box.Y1-b.box.Y0
	if w < 83.9 || w > 84.1 || h < 57.9 || h > 58.1 {
		t.Errorf("плата %.2f x %.2f, ждали 84 x 58", w, h)
	}
	for _, k := range []string{lyTop, lyTopMask, lyOutline} {
		if len(b.files[k]) == 0 {
			t.Errorf("нет слоя %s", k)
		}
	}
	j := newJob(filepath.Join(t.TempDir(), "job"))
	j.Srcs = []jobSrc{{zip, 1}}
	if err := j.layout(false, true, true); err != nil {
		t.Fatal(err)
	}
	if j.DepthProbe == nil || j.IsoProbe == nil || j.DrillProbe == nil || j.VProbe == nil || j.Probe == nil {
		t.Fatalf("не все пробы на листе 100x100: %v %v %v %v", j.IsoProbe, j.DrillProbe, j.VProbe, j.Probe)
	}
	c := defaultCam()
	c.IsoTest = true
	ops, err := j.cam(c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.writeProgs(ops); err != nil {
		t.Fatal(err)
	}
	for _, s := range stages {
		files := j.stageFiles(s.prefix)
		var names []string
		for _, f := range files {
			names = append(names, strings.TrimSuffix(filepath.Base(f), ".tap"))
		}
		t.Logf("%-12s %s", s.title, strings.Join(names, " "))
		if len(files) < 2 && s.prefix != "2_" {
			t.Errorf("%s: %v", s.title, names)
		}
	}
	holes := 0
	for _, hl := range b.holes {
		if drillSize(hl.D) != 1.0 {
			t.Errorf("отверстие %.2f мм, на плате все 1.0", hl.D)
		}
		holes++
	}
	for _, op := range ops {
		_, min := op.minutes()
		t.Logf("%-18s ~%4.0f мин  %s", op.file, min, strings.Join(op.notes, "; "))
	}
	t.Logf("отверстий %d, пробы: глубина X%.0f Y%.0f, изоляция X%.0f Y%.0f, сверло X%.0f Y%.0f, V-рез X%.0f Y%.0f, маска X%.0f Y%.0f",
		holes, j.DepthProbe.X, j.DepthProbe.Y, j.IsoProbe.X, j.IsoProbe.Y, j.DrillProbe.X, j.DrillProbe.Y, j.VProbe.X, j.VProbe.Y, j.Probe.X, j.Probe.Y)
}
