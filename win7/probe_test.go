package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMachineProbes: the probes of isolation, drilling and V-cut take their
// places on the sheet clear of the boards, of each other and of the mask
// probe; every feed, plunge, speed and depth of them is in the programs and
// stays there after the height map.
func TestMachineProbes(t *testing.T) {
	j := testJob(t)
	j.W, j.H = 140, 120
	if err := j.layout(false, true, true); err != nil {
		t.Fatal(err)
	}
	type box struct {
		name string
		at   *pt
		w, h float64
	}
	boxes := []box{{"изоляция", j.IsoProbe, isoProbeW, isoProbeH}, {"сверловка", j.DrillProbe, drillProbeW, drillProbeH},
		{"V-рез", j.VProbe, vProbeW, vProbeH}, {"маска", j.Probe, probeW, probeH}}
	r := j.boardsRaster(false, 0.25)
	for i, a := range boxes {
		if a.at == nil {
			t.Fatalf("проба %s не на листе", a.name)
		}
		if a.at.X+a.w > j.W-j.Margin+1e-9 || a.at.Y+a.h > j.H-j.Margin+1e-9 {
			t.Errorf("проба %s за краем: %v", a.name, *a.at)
		}
		for _, b := range boxes[:i] {
			if a.at.X < b.at.X+b.w && b.at.X < a.at.X+a.w && a.at.Y < b.at.Y+b.h && b.at.Y < a.at.Y+a.h {
				t.Errorf("пробы %s и %s наложились", a.name, b.name)
			}
		}
		for y := a.at.Y; y < a.at.Y+a.h; y += 0.5 {
			for x := a.at.X; x < a.at.X+a.w; x += 0.5 {
				if r.at(int((x-r.x0)/r.px), int((y-r.y0)/r.px)) {
					t.Fatalf("проба %s на плате в X%.1f Y%.1f", a.name, x, y)
				}
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
	h, err := MapFromPoints([]Point{{0, 0, 0}, {j.W, 0, 0.05}, {0, j.H, 0.02}, {j.W, j.H, 0.07}})
	if err != nil {
		t.Fatal(err)
	}
	want := func(prefix string, words []string, mapped bool) {
		m := j.stageFiles(prefix)
		if len(m) < 2 || !strings.Contains(m[0], "_probe") {
			t.Fatalf("%s: проба не первая: %v", prefix, m)
		}
		lines, err := readLines(m[0])
		if err != nil {
			t.Fatal(err)
		}
		lev, err := ApplyMap(lines, h, 1)
		if err != nil {
			t.Fatal(err)
		}
		progs := map[string][]string{"программа": lines}
		if mapped { // the map adds the height of the copper to Z, depths change
			progs["по карте"] = lev
		}
		for name, prog := range progs {
			all := strings.Join(prog, "\n") + "\n"
			for _, w := range words {
				// the map writes F100 where the program had F100.0
				re := regexp.MustCompile(regexp.QuoteMeta(strings.TrimSuffix(w, ".0")) + `(\.0+)?[^0-9.]`)
				if !re.MatchString(all) {
					t.Errorf("%s %s: нет %s", m[0][strings.LastIndex(m[0], "sheet_"):], name, w)
				}
			}
		}
	}
	feed := func(f float64) string { return "F" + strconv.FormatFloat(f, 'f', 1, 64) }
	var iso []string
	for _, f := range append(append([]float64{}, isoProbeFeeds...), isoProbePlunges...) {
		iso = append(iso, feed(f))
	}
	for _, s := range isoProbeSpeeds {
		iso = append(iso, fmt.Sprintf("S%.0fM3", s))
	}
	want("1_", iso, true)
	var dr []string
	for _, f := range drillProbeFeeds {
		dr = append(dr, feed(f))
	}
	want("3_", dr, true)
	var vc []string
	for _, d := range vProbeDepths {
		vc = append(vc, "Z"+n3(-d))
	}
	want("4_", vc, false)
	lines, _ := readLines(j.stageFiles("1_")[0])
	depth, _, min, err := progInfo(lines)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(depth-isoProbeDepths[len(isoProbeDepths)-1]) > 1e-6 {
		t.Errorf("глубина пробы изоляции %.3f", depth)
	}
	t.Logf("изоляция X%.1f Y%.1f ~%.1f мин, сверловка X%.1f Y%.1f, V-рез X%.1f Y%.1f, маска X%.1f Y%.1f",
		j.IsoProbe.X, j.IsoProbe.Y, min, j.DrillProbe.X, j.DrillProbe.Y, j.VProbe.X, j.VProbe.Y, j.Probe.X, j.Probe.Y)
}
