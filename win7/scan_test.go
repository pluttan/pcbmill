package main

import (
	"math"
	"strings"
	"testing"
	"time"
)

func warped(x, y float64) float64 {
	return 0.003*x - 0.002*y + 0.15*math.Sin(x/70*3.1)*math.Sin(y/60*3.1) - 1.5
}

func setup() (*simMachine, *NcStudio, *Scanner) {
	sm := newSimMachine(warped)
	nc := NewNcStudio(sm)
	nc.Sleep = func(time.Duration) {}
	return sm, nc, NewScanner(nc, sm)
}

func TestScanShape(t *testing.T) {
	sm, _, s := setup()
	pts := GridPoints(0, 60, 0, 40, 10)
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
	if sm.maxDig > 0.051 {
		t.Errorf("вдавливание %.4f", sm.maxDig)
	}
	for _, l := range sm.lines {
		if !strings.HasPrefix(l, "G91 G0") && !strings.HasPrefix(l, "G90 G00 X") {
			t.Errorf("неожиданная команда %q", l)
		}
	}
	t.Logf("точек %d, ошибка %.4f, вдавливание %.4f, команд на точку %d",
		len(rows), worst, sm.maxDig, len(sm.lines)/len(rows))
}

func TestBrokenClip(t *testing.T) {
	sm, _, s := setup()
	sm.broken = true
	_, err := s.Grid(GridPoints(0, 20, 0, 20, 10), nil)
	if err == nil || !strings.Contains(err.Error(), "не найдена") {
		t.Fatalf("ожидался останов: %v", err)
	}
	if sm.z < 0 {
		t.Errorf("фреза осталась внизу: %.3f", sm.z)
	}
}

func TestShorted(t *testing.T) {
	sm, _, s := setup()
	sm.state = true
	if _, err := s.Grid(GridPoints(0, 20, 0, 20, 10), nil); err == nil {
		t.Fatal("замкнутый провод не остановил")
	}
	if len(sm.lines) != 0 {
		t.Errorf("при замкнутом проводе ушли команды: %v", sm.lines)
	}
}

func TestSkippedLift(t *testing.T) {
	sm, _, s := setup()
	seenFine := false
	sm.dropIf = func(l string) bool {
		if strings.HasPrefix(l, "G91 G01") {
			seenFine = true
		}
		return seenFine && strings.HasPrefix(l, "G91 G00 Z") && !strings.Contains(l, "Z-")
	}
	_, err := s.Grid(GridPoints(0, 30, 0, 10, 10), nil)
	if err == nil {
		t.Fatal("пропущенный подъём не замечен")
	}
	if sm.maxDig > 0.06 {
		t.Errorf("после пропуска вдавливание %.3f", sm.maxDig)
	}
	t.Log(err)
}

func TestLevelMatchesPython(t *testing.T) {
	pts := []Point{{0, 0, -1.5}, {10, 0, -1.45}, {0, 10, -1.52}, {10, 10, -1.40}}
	hm, _ := MapFromPoints(pts)
	if v := hm.At(5, 5); math.Abs(v-0.0325) > 1e-9 {
		t.Errorf("билинейная: %.5f", v)
	}
	out, err := ApplyMap([]string{"T1M6", "G0Z4.000", "G0X0.000Y0.000S8000M3", "G0Z1.000",
		"G0X1.000Y1.000", "G1Z-0.100F200.0", "G1X3.000Y1.000F300.0", "G0Z4.000", "M30"}, hm, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"T1M6", "G0Z4.000", "G0X0.000Y0.000S8000M3", "G0X0Y0Z1", "G0X1.000Y1.000",
		"G1X1Y1Z-0.0963F200", "G1X2Y1Z-0.0906F300", "G1X3Y1Z-0.0849", "G0Z4.000", "M30"}
	if strings.Join(out, "|") != strings.Join(want, "|") {
		t.Errorf("\n got %v\nwant %v", out, want)
	}
}

func TestMCUUnplugged(t *testing.T) {
	sm, _, s := setup()
	sm.dieAfter = 5 // USB pulled while going down to the first point
	_, err := s.Grid(GridPoints(0, 20, 0, 20, 10), nil)
	if err == nil || !strings.Contains(err.Error(), "МК") {
		t.Fatalf("отключение МК не остановило: %v", err)
	}
	if len(sm.lines) > 5 {
		t.Errorf("после отключения ушло ещё %d команд", len(sm.lines)-5)
	}
	if sm.maxDig > 0 {
		t.Errorf("фреза вошла в медь на %.3f", sm.maxDig)
	}
}

func TestClipFallsOffMidScan(t *testing.T) {
	sm, _, s := setup()
	pts := GridPoints(0, 30, 0, 20, 10)
	n := 0
	_, err := s.Grid(pts, func(Point) {
		n++
		if n == 3 {
			sm.broken = true // clip slips off the copper after the third point
		}
	})
	if err == nil {
		t.Fatal("отвалившийся зажим не остановил обход")
	}
	// the limit is 0.3 under the lowest copper found; local copper may sit higher by the warp
	if sm.maxDig > 0.3+0.15 {
		t.Errorf("фреза ушла в плату на %.3f мм", sm.maxDig)
	}
	t.Logf("%v; глубже меди на %.3f мм", err, sm.maxDig)
}

// relative rounding piles up; absolute Z does not, and the recheck sees what is left
func TestAbsoluteZNoDrift(t *testing.T) {
	pts := GridPoints(0, 60, 0, 40, 10)
	sm, nc, s := setup()
	sm.relErr = 0.0005
	rows, _ := s.Grid(pts, nil)
	dRel, _ := s.Recheck(rows)

	sm2, nc2, s2 := setup()
	sm2.relErr = 0.0005
	nc2.Abs = true
	rows2, _ := s2.Grid(pts, nil)
	dAbs, _ := s2.Recheck(rows2)
	t.Logf("сдвиг первой точки: G91 %+.4f, G90 %+.4f", dRel, dAbs)
	if math.Abs(dAbs) > 0.011 {
		t.Errorf("с абсолютным Z всё равно уехало на %.4f", dAbs)
	}
	if math.Abs(dRel) < 0.03 {
		t.Errorf("модель округления не дала дрейфа: %.4f", dRel)
	}
	_, _ = sm, nc
}

func TestRecheckSeesMechanicalCreep(t *testing.T) {
	sm, nc, s := setup()
	nc.Abs = true
	sm.mechUp = 0.0005 // the axis itself wanders: no command style can hide it
	rows, err := s.Grid(GridPoints(0, 60, 0, 40, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Recheck(rows)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(d) < 0.03 {
		t.Errorf("уход оси не замечен: %.4f", d)
	}
	t.Logf("уход оси за обход: %+.4f мм", d)
}
