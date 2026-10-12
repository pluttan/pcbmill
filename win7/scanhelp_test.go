package main

import (
	"errors"
	"strings"
	"testing"
)

// TestCornersFirst: the zero stays first, the four corners follow, nothing lost.
func TestCornersFirst(t *testing.T) {
	var pts [][2]float64
	pts = append(pts, [2]float64{2, 2})
	for y := 10.0; y <= 50; y += 10 {
		for x := 10.0; x <= 80; x += 10 {
			pts = append(pts, [2]float64{x, y})
		}
	}
	out := cornersFirst(pts)
	if len(out) != len(pts) || out[0] != pts[0] {
		t.Fatalf("точек %d, первая %v", len(out), out[0])
	}
	want := [][2]float64{{10, 10}, {80, 10}, {80, 50}, {10, 50}}
	for i, w := range want {
		if out[1+i] != w {
			t.Errorf("угол %d: %v, ждали %v", i, out[1+i], w)
		}
	}
	_, odd := cornerReport([]Point{{2, 2, 0}, {10, 10, -0.02}, {80, 10, 0.03}, {80, 50, 0.01}, {10, 50, -0.9}})
	if !odd {
		t.Error("перепад 0.9 мм не замечен")
	}
}

// TestScanHelp: a stuck contact at the first point names the forgotten zero.
func TestScanHelp(t *testing.T) {
	h := scanHelp(fail("контакт не размыкается после подъёма на 0.25 мм: зажим замкнут на фрезу?"), true, "scan.new.csv")
	all := strings.Join(h.lines(), "\n")
	for _, w := range []string{"Почему так бывает", "обнулить", "Что сделать", "scan.new.csv"} {
		if !strings.Contains(all, w) {
			t.Errorf("нет %q:\n%s", w, all)
		}
	}
	var got helpErr
	if !errors.As(error(h), &got) {
		t.Error("helpErr не узнаётся")
	}
}
