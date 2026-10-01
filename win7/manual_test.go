package main

import (
	"strings"
	"testing"
)

func TestParseShift(t *testing.T) {
	dx, dy, dz, err := parseShift("x5 z-0,5 y-2.5")
	if err != nil || dx != 5 || dy != -2.5 || dz != -0.5 {
		t.Fatal(dx, dy, dz, err)
	}
	for _, bad := range []string{"", "x", "x1 x2", "a5", "x5q", "x700", "z60"} {
		if _, _, _, err := parseShift(bad); err == nil {
			t.Errorf("%q принят", bad)
		}
	}
}

// Up first and down last: a combined move never drags the tip sideways near the copper.
func TestShiftOrder(t *testing.T) {
	sm, nc, _ := setup()
	if err := shift(nc, 3, 4, 2); err != nil {
		t.Fatal(err)
	}
	if err := shift(nc, -3, 0, -1); err != nil {
		t.Fatal(err)
	}
	want := []string{"G00 Z", "G00 X", "G00 X", "G01 Z-1"}
	if len(sm.lines) != len(want) {
		t.Fatal(sm.lines)
	}
	for i, w := range want {
		if !strings.Contains(sm.lines[i], w) {
			t.Fatalf("строка %d: %q, ждали %q", i, sm.lines[i], w)
		}
	}
}

func TestLowerToCopperStopsOnContact(t *testing.T) {
	sm, nc, _ := setup()
	if err := lowerToCopper(opts{search: 3, step: 0.05, fine: 0.01, clear: 0.3, below: 0.3}, nc, sm, 3); err != nil {
		t.Fatal(err)
	}
	if !sm.State() || sm.maxDig > 0.011 {
		t.Fatalf("касание %v, заглубление %.3f", sm.State(), sm.maxDig)
	}
	sm2, nc2, _ := setup()
	sm2.broken = true
	if err := lowerToCopper(opts{search: 3, step: 0.05, fine: 0.01}, nc2, sm2, 1); err == nil {
		t.Fatal("без контакта должна быть ошибка")
	}
	if nc2.z < -1.0001 {
		t.Fatalf("ушёл ниже заданной глубины: %.3f", nc2.z)
	}
}
