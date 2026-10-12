package main

import (
	"fmt"
	"math"
	"testing"
)

// TestIsoNarrowDiagonal: a diagonal gap of 0.2 mm, hardly wider than the
// cut, is one line of the tool, not hundreds of rings with a plunge each.
func TestIsoNarrowDiagonal(t *testing.T) {
	d := 0.1 * math.Sqrt2 // half the gap along X and Y
	mm := func(v float64) int { return int(math.Round(v * 1e6)) }
	region := func(ps [][2]float64) string {
		s := "G36*\n"
		for i, p := range ps {
			op := "D01"
			if i == 0 {
				op = "D02"
			}
			s += fmt.Sprintf("X%dY%d%s*\n", mm(p[0]), mm(p[1]), op)
		}
		return s + fmt.Sprintf("X%dY%dD01*\nG37*\n", mm(ps[0][0]), mm(ps[0][1]))
	}
	src := "%FSLAX46Y46*%\n%MOMM*%\n%LPD*%\nG01*\n" +
		region([][2]float64{{0, 0}, {20 - d, 0}, {0, 20 - d}}) +
		region([][2]float64{{20, d}, {20, 20}, {d, 20}}) + "M02*\n"
	g, err := parseGerber(src)
	if err != nil {
		t.Fatal(err)
	}
	b := &board{name: "gap", copper: map[string]*gerber{lyTop: g}, box: rect{0, 20, 0, 20}}
	c := defaultCam()
	c.Passes = 1
	op, _ := isolation(b, lyTop, c, nil)
	if len(op.paths) > 4 {
		t.Errorf("путей %d: зазор распался на куски", len(op.paths))
	}
	long := 0.0
	for _, p := range op.paths {
		var l []pt
		for _, q := range p.pts {
			l = append(l, pt{q.X, q.Y})
		}
		long = math.Max(long, pathLen(l))
	}
	if long < 25 {
		t.Errorf("самый длинный путь %.1f мм: зазор длиной 28 мм не пройден целиком", long)
	}
	t.Logf("путей %d, самый длинный %.1f мм", len(op.paths), long)
}
