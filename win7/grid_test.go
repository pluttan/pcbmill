package main

import (
	"math"
	"testing"
)

func TestGridLayout(t *testing.T) {
	g := GridFor(0, 60, 0, 40, 10)
	if g.NX != 7 || g.NY != 5 || g.StepX() != 10 || g.StepY() != 10 {
		t.Fatalf("%+v", g)
	}
	// same points as the old one-step grid, same snake order
	old := GridPoints(0, 60, 0, 40, 10)
	pts := g.Points()
	if len(pts) != len(old) {
		t.Fatal(len(pts), len(old))
	}
	for i := range pts {
		if math.Hypot(pts[i][0]-old[i][0], pts[i][1]-old[i][1]) > 1e-9 {
			t.Fatalf("точка %d: %v вместо %v", i, pts[i], old[i])
		}
	}
	// separate X and Y steps, ends always included
	g.NX, g.NY = CountFor(0, 60, 5), CountFor(0, 40, 20)
	pts = g.Points()
	if len(pts) != 13*3 || pts[0] != [2]float64{0, 0} || pts[12] != [2]float64{60, 0} || pts[len(pts)-1] != [2]float64{60, 40} {
		t.Fatalf("%d точек, %v %v %v", len(pts), pts[0], pts[12], pts[len(pts)-1])
	}
	// one point along an axis: the low edge only
	g = Grid{5, 25, 3, 3, 3, 1}
	if p := g.Points(); len(p) != 3 || p[1] != [2]float64{15, 3} {
		t.Fatal(p)
	}
}
