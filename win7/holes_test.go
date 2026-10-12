package main

import (
	"math"
	"strings"
	"testing"
)

// TestHolesByMill: a 1 mm hole is drilled, a 3 mm one goes round at 1 mm
// from the centre down to the full depth, a wide slot round its outline.
func TestHolesByMill(t *testing.T) {
	c := defaultCam()
	op := holesByMill([]hole{
		{P: pt{10, 10}, D: 1.0},
		{P: pt{20, 10}, D: 3.0},
		{P: pt{30, 10}, P2: pt{34, 10}, D: 2.0, Slot: true},
	}, c)
	depth := c.Thickness + 0.3
	if p := op.paths[0]; p.peck == 0 || len(p.pts) != 1 {
		t.Errorf("отверстие 1.0 не сверлится: %+v", p)
	}
	h := op.paths[1]
	low := 0.0
	for _, q := range h.pts {
		if r := math.Hypot(q.X-20, q.Y-10); math.Abs(r-1.0) > 1e-6 {
			t.Fatalf("круг радиусом %.3f, ждали 1.0", r)
		}
		low = math.Min(low, q.Z)
	}
	if math.Abs(low+depth) > 1e-6 || math.Abs(h.pts[len(h.pts)-1].Z+depth) > 1e-6 {
		t.Errorf("спираль до %.3f, ждали %.3f", low, -depth)
	}
	for i := 1; i < len(h.pts); i++ {
		if dz := h.pts[i-1].Z - h.pts[i].Z; dz > c.HoleStep/float64(circleN(1))+1e-6 {
			t.Fatalf("шаг вниз %.3f за отрезок", dz)
		}
	}
	slot := 0
	for _, p := range op.paths[2:] {
		slot++
		for _, q := range p.pts {
			d := math.Abs(q.Y - 10)
			if q.X < 30 {
				d = math.Hypot(q.X-30, q.Y-10)
			} else if q.X > 34 {
				d = math.Hypot(q.X-34, q.Y-10)
			}
			if math.Abs(d-0.5) > 1e-6 {
				t.Fatalf("паз: точка %v в %.3f от оси, ждали 0.5", q, d)
			}
		}
	}
	if slot != len(levels(depth, c.HoleStep)) {
		t.Errorf("паз за %d слоёв", slot)
	}
	if !strings.Contains(strings.Join(op.gcode(), "\n"), "M3") {
		t.Error("шпиндель не включён")
	}
	if toolOf("sheet_"+holesFile+".tap", c) != "фреза 1 мм" {
		t.Errorf("инструмент %q", toolOf("sheet_"+holesFile+".tap", c))
	}
}

// TestUVProgram: the zigzag covers the sheet edge to edge, no Z, no spindle.
func TestUVProgram(t *testing.T) {
	lines, min := uvProgram(uvCfg{Spot: 20, Feed: 20, Passes: 1, DX: 30}, 100, 100)
	all := strings.Join(lines, "\n")
	if strings.Contains(all, "Z") || strings.Contains(all, "M3\n") || strings.Contains(all, "S") {
		t.Fatalf("Z или шпиндель в засветке:\n%s", all)
	}
	if !strings.Contains(all, "X70.000Y100.000") || !strings.Contains(all, "X-30.000Y0.000") {
		t.Errorf("змейка не доходит до краёв листа со смещением фонаря:\n%s", all)
	}
	if min < 50 || min > 70 {
		t.Errorf("время %.0f мин", min)
	}
}
