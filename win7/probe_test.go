package main

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestIsoProbe: the probe of isolation takes its place on the sheet clear of
// the boards and the mask probe; every feed and plunge of it is in the
// program and stays there after the height map.
func TestIsoProbe(t *testing.T) {
	j := testJob(t)
	if err := j.layout(false, true, true); err != nil {
		t.Fatal(err)
	}
	if j.IsoProbe == nil || j.Probe == nil {
		t.Fatalf("пробы не на листе: %v %v", j.IsoProbe, j.Probe)
	}
	a, b := *j.IsoProbe, *j.Probe
	if a.X < b.X+probeW && b.X < a.X+isoProbeW && a.Y < b.Y+probeH && b.Y < a.Y+isoProbeH {
		t.Errorf("пробы наложились: %v %v", a, b)
	}
	if a.X+isoProbeW > j.W-j.Margin+1e-9 || a.Y+isoProbeH > j.H-j.Margin+1e-9 {
		t.Errorf("проба подач за краем: %v", a)
	}
	r := j.boardsRaster(false, 0.25)
	for y := a.Y; y < a.Y+isoProbeH; y += 0.5 {
		for x := a.X; x < a.X+isoProbeW; x += 0.5 {
			if r.at(int((x-r.x0)/r.px), int((y-r.y0)/r.px)) {
				t.Fatalf("проба подач на плате в X%.1f Y%.1f", x, y)
			}
		}
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
	m := j.stageFiles("1_")
	if len(m) < 2 || !strings.Contains(m[0], "_probe") {
		t.Fatalf("проба подач не первая: %v", m)
	}
	lines, err := readLines(m[0])
	if err != nil {
		t.Fatal(err)
	}
	h, err := MapFromPoints([]Point{{0, 0, 0}, {j.W, 0, 0.05}, {0, j.H, 0.02}, {j.W, j.H, 0.07}})
	if err != nil {
		t.Fatal(err)
	}
	lev, err := ApplyMap(lines, h, 1)
	if err != nil {
		t.Fatal(err)
	}
	for name, prog := range map[string][]string{"программа": lines, "по карте": lev} {
		all := strings.Join(prog, "\n")
		for _, f := range append(append([]float64{}, isoProbeFeeds...), isoProbePlunges...) {
			if !strings.Contains(all, "F"+strconv.FormatFloat(f, 'f', 1, 64)) && !strings.Contains(all, "F"+f4(f)) {
				t.Errorf("%s: нет подачи F%s", name, num(f))
			}
		}
	}
	depth, cut, min, err := progInfo(lines)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(depth-isoProbeDepths[len(isoProbeDepths)-1]) > 1e-6 {
		t.Errorf("глубина пробы %.3f", depth)
	}
	t.Logf("проба подач в X%.1f Y%.1f, рез X %.1f..%.1f Y %.1f..%.1f, ~%.1f мин", a.X, a.Y, cut.X0, cut.X1, cut.Y0, cut.Y1, min)
}
