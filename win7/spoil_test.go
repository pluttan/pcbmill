package main

import (
	"math"
	"testing"
)

// TestPocket: the cutter stays in the pocket, reaches its square corners,
// clears the middle, and goes down in passes to the depth.
func TestPocket(t *testing.T) {
	p := pocketCfg{W: 100.4, H: 100.4, Depth: 1.0, Tool: 8, Step: 0.4, Over: 3.5, Feed: 800, Plunge: 150, Spindle: 15000, CornersSq: 1}
	op, err := pocketOp(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.paths) != 3 {
		t.Errorf("проходов %d, ждали 3", len(op.paths))
	}
	r := p.Tool / 2
	corner := math.Inf(1)
	last := op.paths[len(op.paths)-1]
	if z := last.pts[0].Z; math.Abs(z+p.Depth) > 1e-9 {
		t.Errorf("последний проход на %.3f", z)
	}
	maxGap := 0.0
	for i, q := range last.pts {
		if q.X < r/math.Sqrt2-1e-9 || q.Y < r/math.Sqrt2-1e-9 || q.X > p.W-r/math.Sqrt2+1e-9 || q.Y > p.H-r/math.Sqrt2+1e-9 {
			t.Fatalf("фреза за карманом: %v", q)
		}
		corner = math.Min(corner, math.Hypot(q.X, q.Y))
		if i > 0 {
			a := last.pts[i-1]
			if d := math.Hypot(q.X-a.X, q.Y-a.Y); math.Abs(q.X-a.X) > 1e-9 && math.Abs(q.Y-a.Y) > 1e-9 && d > maxGap {
				maxGap = d // a diagonal step between rings
			}
		}
	}
	if math.Abs(corner-r) > 1e-6 {
		t.Errorf("угол: фреза в %.3f мм от угла, ждали %.3f", corner, r)
	}
	if maxGap > p.Over*math.Sqrt2+1e-6 {
		t.Errorf("между кольцами %.2f мм: останется гребень", maxGap)
	}
	_, min := op.minutes()
	t.Logf("путей %d, точек в слое %d, ~%.0f мин", len(op.paths), len(last.pts), min)
}
