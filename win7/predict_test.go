package main

import (
	"math"
	"strings"
	"testing"
)

func setupAbs() (*simMachine, *NcStudio, *Scanner) {
	sm, nc, s := setup()
	nc.Abs = true
	s.Predict = true
	return sm, nc, s
}

func TestPredictShape(t *testing.T) {
	pts := GridPoints(0, 60, 0, 40, 10)
	sm0, nc0, s0 := setup()
	nc0.Abs = true
	if _, err := s0.Grid(pts, nil); err != nil {
		t.Fatal(err)
	}
	sm, _, s := setupAbs()
	rows, err := s.Grid(pts, nil)
	if err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for _, r := range rows {
		want := warped(r.X, r.Y) - warped(rows[0].X, rows[0].Y)
		worst = math.Max(worst, math.Abs((r.Z-rows[0].Z)-want))
	}
	if worst > 0.011 {
		t.Errorf("рельеф: ошибка %.4f", worst)
	}
	if sm.maxDig > 0.051 { // the first point is still found with coarse steps, as before
		t.Errorf("вдавливание %.4f", sm.maxDig)
	}
	before, after := float64(len(sm0.lines))/float64(len(rows)), float64(len(sm.lines))/float64(len(rows))
	if after > before*0.7 {
		t.Errorf("не быстрее: было %.1f команд на точку, стало %.1f", before, after)
	}
	t.Logf("команд на точку: было %.1f, стало %.1f; ошибка %.4f, вдавливание %.4f", before, after, worst, sm.maxDig)
}

// a bump the neighbours do not foresee: met during the approach, pressed at most by its height
func TestPredictBump(t *testing.T) {
	sm, _, s := setupAbs()
	sm.surface = func(x, y float64) float64 {
		if x > 25 && x < 35 && y > 5 && y < 15 {
			return warped(x, y) + 0.12
		}
		return warped(x, y)
	}
	rows, err := s.Grid(GridPoints(0, 60, 0, 40, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		want := sm.surface(r.X, r.Y) - sm.surface(rows[0].X, rows[0].Y)
		if math.Abs((r.Z-rows[0].Z)-want) > 0.011 {
			t.Errorf("X%.0f Y%.0f: %.4f вместо %.4f", r.X, r.Y, r.Z-rows[0].Z, want)
		}
	}
	if sm.maxDig > 0.12 {
		t.Errorf("вдавливание %.4f", sm.maxDig)
	}
	t.Logf("вдавливание на бугре 0.12 мм: %.3f", sm.maxDig)
}

// with absolute Z a dropped command cannot send the long approach into the copper
func TestPredictSkipped(t *testing.T) {
	for _, drop := range []string{"G90 G00 Z", "G90 G00 X", "G90 G01 Z"} {
		sm, _, s := setupAbs()
		n := 0
		sm.dropIf = func(l string) bool {
			if strings.HasPrefix(l, drop) {
				n++
				return n == 7
			}
			return false
		}
		rows, err := s.Grid(GridPoints(0, 60, 0, 40, 10), nil)
		if sm.maxDig > 0.051 {
			t.Errorf("пропуск %q: вдавливание %.3f", drop, sm.maxDig)
		}
		// a dropped X/Y move leaves the tool over the previous point: it measures that copper
		// and nothing can tell (no position feedback), the same as with the old search
		if err == nil && drop != "G90 G00 X" {
			for _, r := range rows {
				want := warped(r.X, r.Y) - warped(rows[0].X, rows[0].Y)
				if math.Abs((r.Z-rows[0].Z)-want) > 0.011 {
					t.Errorf("пропуск %q прошёл молча, X%.0f Y%.0f неверна", drop, r.X, r.Y)
				}
			}
		}
		t.Logf("пропуск %q: %v", drop, err)
	}
}

func TestPredictPlane(t *testing.T) {
	// along the first row the points are on one line: extend it
	rows := []Point{{0, 0, -0.06}, {10, 0, -0.10}, {20, 0, -0.14}}
	if p := predict(rows, 30, 0); math.Abs(p-(-0.18)) > 1e-9 {
		t.Errorf("по линии: %.4f", p)
	}
	// a tilted plane is reproduced exactly from any four points of it
	pl := func(x, y float64) float64 { return 0.003*x - 0.002*y - 0.1 }
	rows = []Point{{0, 0, pl(0, 0)}, {10, 0, pl(10, 0)}, {20, 0, pl(20, 0)}, {20, 10, pl(20, 10)}}
	if p := predict(rows, 10, 10); math.Abs(p-pl(10, 10)) > 1e-9 {
		t.Errorf("по плоскости: %.4f вместо %.4f", p, pl(10, 10))
	}
}
