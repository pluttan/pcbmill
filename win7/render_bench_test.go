package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pcbscan/r3"
	"pcbscan/ui"
)

// renderJob: the test board with all the probes on a 100 x 100 sheet, its
// programs written, and a height map over it, as the screen has them.
func renderJob(tb testing.TB) (*Job, *HeightMap) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	if _, err := os.Stat(zip); err != nil {
		tb.Skip("нет testboard/pcbmill-test.zip: make testboard")
	}
	j := newJob(filepath.Join(tb.TempDir(), "job"))
	j.Srcs = []jobSrc{{zip, 1}}
	if err := j.layout(false, true, true); err != nil {
		tb.Fatal(err)
	}
	c := defaultCam()
	c.IsoTest = true
	ops, err := j.cam(c, func(string) {})
	if err != nil {
		tb.Fatal(err)
	}
	if err := j.writeProgs(ops); err != nil {
		tb.Fatal(err)
	}
	var pts []Point
	for y := 0.0; y <= j.H; y += 5 {
		for x := 0.0; x <= j.W; x += 5 {
			pts = append(pts, Point{X: x, Y: y, Z: 0.05 * math.Sin(x/20) * math.Cos(y/30)})
		}
	}
	hm, err := MapFromPoints(pts)
	if err != nil {
		tb.Fatal(err)
	}
	return j, hm
}

func sheetFrame(tb testing.TB, j *Job, hm *HeightMap, file string, w, h int) (*sheetScene, *r3.Frame, *r3.Camera) {
	var paths []cpath
	if file != "" {
		lines, err := readLines(j.progPath(file))
		if err != nil {
			tb.Fatal(err)
		}
		op, err := readProg(lines)
		if err != nil {
			tb.Fatal(err)
		}
		paths = op.paths
	}
	s := newSheetScene(j, false, sheetOverlay{paths: paths, stage: file[:1]})
	s.hm = hm
	cam := &r3.Camera{}
	cam.Yaw, cam.Pitch, cam.FOV = -math.Pi/2+0.55, 0.62, 0.6
	lo, hi := s.Bounds()
	cam.Fit(lo, hi, w, h)
	return s, r3.NewFrame(w, h), cam
}

// TestRenderTime: one frame of the sheet with the isolation of the test board
// over the height map, as the screen draws it while turning (1x) and at rest (2x).
func TestRenderTime(t *testing.T) {
	if theme == nil {
		th, err := ui.NewTheme()
		if err != nil {
			t.Fatal(err)
		}
		theme = th
		defer func() { theme = nil }()
	}
	j, hm := renderJob(t)
	for _, file := range []string{"1_iso", "1_iso_probe", "3_drill_1.0", "4_vcut"} {
		for _, ss := range []int{1, 2} {
			s, f, cam := sheetFrame(t, j, hm, file, 1200*ss, 800*ss)
			s.Render(f, cam, float64(ss)) // the copper texture is cached after this
			const n = 5
			t0 := time.Now()
			for i := 0; i < n; i++ {
				f.Clear(theme.Base, theme.Crust)
				s.Render(f, cam, float64(ss))
			}
			t.Logf("%-12s x%d  путей %5d  кадр %v", file, ss, len(s.ov.paths), time.Since(t0)/n)
		}
	}
}
