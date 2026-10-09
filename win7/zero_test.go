package main

import (
	"errors"
	"math"
	"testing"
	"time"
)

// TestZeroRange: the touch for Z0 finds copper 4.5 mm down (the old search
// stopped at 3), starts from the copper itself, and Отмена stops it.
func TestZeroRange(t *testing.T) {
	for _, c := range []struct {
		name   string
		copper float64
	}{{"4.5 мм над медью", -4.5}, {"на меди", 0.02}} {
		sm := newSimMachine(func(x, y float64) float64 { return c.copper })
		nc := NewNcStudio(sm)
		nc.Sleep = func(time.Duration) {}
		p := &page{acts: make(chan string, 8)}
		s := NewScanner(zeroStop{nc, p}, sm)
		s.Search, s.StepCoarse = zeroSearch, 0.05
		_, _, z0 := nc.Pos()
		if sm.State() {
			if err := s.liftUntilOpenN(z0, 0.1, 20); err != nil {
				t.Fatal(c.name, err)
			}
			_, _, z0 = nc.Pos()
		}
		z, err := s.TouchAt(0, 0, z0)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if math.Abs(z-c.copper) > 0.02 {
			t.Errorf("%s: медь на %.3f, ждали %.3f", c.name, z, c.copper)
		}
	}
	sm := newSimMachine(func(x, y float64) float64 { return -4.5 })
	nc := NewNcStudio(sm)
	nc.Sleep = func(time.Duration) {}
	p := &page{acts: make(chan string, 8)}
	p.send("cancel")
	s := NewScanner(zeroStop{nc, p}, sm)
	s.Search = zeroSearch
	if _, err := s.TouchAt(0, 0, 0); !errors.Is(err, errZeroStop) {
		t.Errorf("Отмена не остановила касание: %v", err)
	}
}
