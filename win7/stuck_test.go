package main

import (
	"math"
	"testing"
)

// stickyTouch: the contact of the sim, but at one point it stays closed until
// the tool is well up (a burr on the tip, a twitch of the wire).
type stickyTouch struct {
	*simMachine
	nc      *NcStudio
	at      [2]float64
	until   float64 // closed while z is below this
	armed   bool
	stuckAt float64
}

func (t *stickyTouch) State() bool {
	x, y, z := t.nc.Pos()
	if t.armed && math.Hypot(x-t.at[0], y-t.at[1]) < 0.01 && t.simMachine.State() {
		t.armed = false
		t.stuckAt = z
	}
	if t.stuckAt != 0 && z < t.stuckAt+t.until {
		return true
	}
	t.stuckAt = 0
	return t.simMachine.State()
}

// TestStuckContact: a contact that opens only 0.6 mm up does not stop the
// scan: the tool goes up, the point is touched again and comes out right.
func TestStuckContact(t *testing.T) {
	sm, nc, _ := setup()
	pts := GridPoints(0, 30, 0, 20, 10)
	st := &stickyTouch{simMachine: sm, nc: nc, at: pts[5], until: 0.6, armed: true}
	s := NewScanner(nc, st)
	rows, err := s.Grid(pts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.armed {
		t.Fatal("залипание не случилось")
	}
	for _, r := range rows {
		want := warped(r.X, r.Y) - warped(rows[0].X, rows[0].Y)
		if d := math.Abs((r.Z - rows[0].Z) - want); d > 0.012 {
			t.Errorf("X%.0f Y%.0f: ошибка %.3f", r.X, r.Y, d)
		}
	}
}

// TestAskRetry: the clip falls off after two points; the scan does not end:
// three tries, then the operator puts it back and says retry, and the map is
// whole. A point let go is left out, the rest goes on.
func TestAskRetry(t *testing.T) {
	g := GridFor(0.63, 61.26, 1.8, 46.05, 10)
	for _, answer := range []int{askRetry, askSkip} {
		sm, _, s := liveSim(400)
		asked := 0
		s.Ask = func(i int, p [2]float64, err error, canSkip bool) int {
			asked++
			sm.Toggle(fClip) // the clip back on the copper
			return answer
		}
		n := 0
		rows, err := s.Grid(g.Points(), func(Point) {
			if n++; n == 2 {
				sm.Toggle(fClip)
			}
		})
		if err != nil {
			t.Fatalf("ответ %d: %v", answer, err)
		}
		want := len(g.Points())
		if answer == askSkip {
			want--
		}
		if asked != 1 || len(rows) != want || len(s.Skipped) != len(g.Points())-want {
			t.Errorf("ответ %d: спросили %d раз, точек %d из %d, пропущено %d", answer, asked, len(rows), want, len(s.Skipped))
		}
		if sm.maxDig > 0.36 {
			t.Errorf("ответ %d: фреза ушла в медь на %.3f", answer, sm.maxDig)
		}
	}
}
