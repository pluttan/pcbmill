package main

import (
	"math"
	"strings"
	"testing"
)

// liveSim is the emulation as the program runs it: moves take (scaled) time,
// NcStudio waits through the machine's clock, Z absolute, predicted approach.
func liveSim(speed float64) (*simMachine, *NcStudio, *Scanner) {
	sm := newSimMachine(simSurface)
	sm.Speed = speed
	sm.Reset()
	nc := NewNcStudio(sm)
	nc.Sleep = sm.sleep
	nc.Abs = true
	s := NewScanner(nc, sm)
	s.Predict = true
	return sm, nc, s
}

func TestSimLineParsing(t *testing.T) {
	sm := newSimMachine(simSurface)
	sm.Reset()
	sm.Line("G91 G00 X5 Y-2; Z1")      // two commands in one line, G91 stays in force
	sm.Line("G01 Z-0.5 F300; G90 X10") // modal G1, then back to absolute
	x, y, z, moving := sm.WorkPos()
	if math.Abs(x-10) > 1e-9 || math.Abs(y+2) > 1e-9 || math.Abs(z-0.5) > 1e-9 || moving {
		t.Fatalf("X%.3f Y%.3f Z%.3f moving %v", x, y, z, moving)
	}
	if sm.rapidMode || sm.feed != 300 {
		t.Fatal("G01 F300 не запомнены")
	}
	sm.Line("G90 G00 Z0")
	if _, _, z, _ := sm.WorkPos(); math.Abs(z) > 1e-9 {
		t.Fatal(z)
	}
}

// Moves take the time of their length at the real speed, divided by Speed.
func TestSimTiming(t *testing.T) {
	sm := newSimMachine(simSurface)
	sm.Speed = 10
	sm.Reset()
	sm.Line("G91 G00 X25") // 25 mm at 1500 mm/min: 1 s, 0.1 s real
	if _, _, _, moving := sm.WorkPos(); !moving {
		t.Fatal("ход закончился сразу")
	}
	sm.waitIdle()
	if x, _, _, moving := sm.WorkPos(); moving || math.Abs(x-25) > 1e-9 {
		t.Fatal(x, moving)
	}
}

// The scan on the emulator gives the blank's relief within one fine step.
func TestSimScanMatchesBlank(t *testing.T) {
	sm, _, s := liveSim(400)
	g := GridFor(0.63, 61.26, 1.8, 46.05, 10)
	rows, err := s.Grid(g.Points(), nil)
	if err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for _, r := range rows {
		want := simSurface(r.X, r.Y) - simSurface(rows[0].X, rows[0].Y)
		worst = math.Max(worst, math.Abs((r.Z-rows[0].Z)-want))
	}
	if worst > 0.011 {
		t.Errorf("карта расходится с заготовкой на %.4f", worst)
	}
	if sm.maxDig > 0.051 {
		t.Errorf("вдавливание %.4f", sm.maxDig)
	}
	d, err := s.Recheck(rows)
	if err != nil || math.Abs(d) > 0.011 {
		t.Errorf("повторное касание: %+.4f %v", d, err)
	}
	t.Logf("точек %d, ошибка %.4f, вдавливание %.4f, сдвиг %+.4f", len(rows), worst, sm.maxDig, d)
}

// Every fault of the emulation stops the scan with its own message.
func TestSimFaults(t *testing.T) {
	g := GridFor(0.63, 61.26, 1.8, 46.05, 10)
	for _, c := range []struct {
		f     simFault
		after int // points measured before the fault comes
		want  string
	}{
		{fDead, 2, "МК не отвечает"},
		{fClip, 2, "медь не найдена"},
		{fShort, 2, "контакт"},
		{fNoise, 0, "онтакт"}, // the tries end on "контакт до переезда" or "касание ... контакт"
	} {
		sm, _, s := liveSim(400)
		if c.after == 0 {
			sm.Toggle(c.f)
		}
		n := 0
		_, err := s.Grid(g.Points(), func(Point) {
			if n++; n == c.after {
				sm.Toggle(c.f)
			}
		})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", simFaults[c.f].short, err)
			continue
		}
		if sm.maxDig > 0.3+0.06 {
			t.Errorf("%s: фреза ушла в медь на %.3f", simFaults[c.f].short, sm.maxDig)
		}
		t.Logf("%-13s после %2d точек: %v", simFaults[c.f].short, n, err)
	}
	// a skipped lift: the contact stays closed, the tool goes on up, the
	// point is touched again; the map comes out right and the count says so
	{
		sm, _, s := liveSim(400)
		n := 0
		rows, err := s.Grid(g.Points(), func(Point) {
			if n++; n == 2 {
				sm.Toggle(fSkip)
			}
		})
		if err != nil {
			t.Fatalf("ПРОПУСК: %v", err)
		}
		worst := 0.0
		for _, r := range rows {
			worst = math.Max(worst, math.Abs((r.Z-rows[0].Z)-(simSurface(r.X, r.Y)-simSurface(rows[0].X, rows[0].Y))))
		}
		if worst > 0.011 || s.Stuck == 0 || sm.maxDig > 0.36 {
			t.Errorf("ПРОПУСК: ошибка карты %.4f, точек заново %d, вдавливание %.3f", worst, s.Stuck, sm.maxDig)
		}
		t.Logf("ПРОПУСК: карта верна (%.4f), точек снято заново %d", worst, s.Stuck)
	}
	// the creeping axis passes the scan, the recheck shows it
	sm, _, s := liveSim(400)
	sm.Toggle(fCreep)
	rows, err := s.Grid(g.Points(), nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Recheck(rows)
	if err != nil || math.Abs(d) < 0.03 {
		t.Fatalf("уход оси не замечен: %+.4f %v", d, err)
	}
	t.Logf("Z ПОЛЗЁТ: сдвиг первой точки %+.4f", d)
}

// Off the blank there is no copper: a grid wider than the blank stops at its edge.
func TestSimBlankEdge(t *testing.T) {
	_, _, s := liveSim(400)
	_, err := s.Grid(GridFor(-5, 20, 0, 10, 5).Points(), nil)
	if err == nil || !strings.Contains(err.Error(), "край заготовки") {
		t.Fatal(err)
	}
}

// The blank is the sheet of the job: on a sheet wider than 100 mm the map
// finds copper out at its far corner too.
func TestSimBlankFollowsSheet(t *testing.T) {
	defer simFit(100, 100)
	simFit(160, 120)
	_, _, s := liveSim(400)
	rows, err := s.Grid(GridFor(140, 155, 100, 115, 15).Points(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("точек %d", len(rows))
	}
	_, err = s.Grid(GridFor(150, 170, 100, 110, 10).Points(), nil)
	if err == nil || !strings.Contains(err.Error(), "край заготовки") {
		t.Fatalf("за краем листа 160 мм: %v", err)
	}
}
