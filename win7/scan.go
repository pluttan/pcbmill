package main

// Height scan for NcStudio: no probe input and no API, so the tool goes down in
// short separate MDI moves and the MCU is asked about contact between them.
// Z is never read from the machine: every Z move is relative and summed here.

import (
	"fmt"
	"math"
)

type Machine interface {
	Pos() (x, y, z float64)
	RapidZ(z float64) error
	RapidXY(x, y float64) error
	MoveZ(z, feed float64) error
}

type Touch interface {
	Alive() error // the sensor answered recently; "no contact" from a dead one means nothing
	State() bool
	Hits() int // rising edges so far: contact during travel is caught even if brief
}

type ScanError struct{ msg string }

func (e ScanError) Error() string { return e.msg }

func fail(f string, a ...interface{}) error { return ScanError{fmt.Sprintf(f, a...)} }

type Scanner struct {
	M          Machine
	T          Touch
	Clear      float64 // lift between points
	Search     float64 // how far down to look for copper at the first point
	Below      float64 // later points: never deeper than this under the lowest copper found
	StepCoarse float64
	StepFine   float64
	FeedCoarse float64
	FeedFine   float64
	floor      float64
	hasFloor   bool
	Log        func(string)
}

func NewScanner(m Machine, t Touch) *Scanner {
	return &Scanner{M: m, T: t, Clear: 0.3, Search: 3.0, Below: 0.3,
		StepCoarse: 0.05, StepFine: 0.01,
		FeedCoarse: 20, FeedFine: 20, Log: func(string) {}}
}

func (s *Scanner) z() float64 { _, _, z := s.M.Pos(); return z }

// travel is a move that must never touch copper; contact means a lost command.
func (s *Scanner) travel(f func() error) error {
	if err := s.T.Alive(); err != nil {
		return err
	}
	h := s.T.Hits()
	if err := f(); err != nil {
		return err
	}
	if s.T.Hits() != h || s.T.State() {
		return fail("касание во время переезда: станок не выполнил подъём (команда пропущена?) " +
			"или заготовка выше ожидаемого")
	}
	return nil
}

// stepDown returns the Z where contact appeared, or ok=false.
func (s *Scanner) stepDown(depth, step, feed float64) (float64, bool, error) {
	z0 := s.z()
	n := int(math.Ceil(depth/step - 1e-9))
	for k := 1; k <= n; k++ {
		z := z0 - math.Min(depth, float64(k)*step)
		if s.hasFloor && z < s.floor {
			return 0, false, nil
		}
		if err := s.T.Alive(); err != nil {
			return 0, false, err
		}
		if err := s.M.MoveZ(z, feed); err != nil {
			return 0, false, err
		}
		if err := s.T.Alive(); err != nil { // answer must be fresh, not from before the step
			return 0, false, err
		}
		if s.T.State() {
			return z, true, nil
		}
	}
	return 0, false, nil
}

func (s *Scanner) liftUntilOpen(z, step float64) error {
	for k := 1; k <= 5; k++ {
		if err := s.M.MoveZ(z+step*float64(k), s.FeedFine); err != nil {
			return err
		}
		if !s.T.State() {
			return nil
		}
	}
	return fail("контакт не размыкается после подъёма на %.2f мм: зажим замкнут на фрезу?", step*5)
}

// TouchAt finds the copper under (x, y) and returns its Z.
func (s *Scanner) TouchAt(x, y, zClear float64) (float64, error) {
	if s.T.State() {
		return 0, fail("контакт до переезда: фрезу не подняло или провод замкнут")
	}
	if err := s.travel(func() error { return s.M.RapidZ(zClear) }); err != nil {
		return 0, err
	}
	if err := s.travel(func() error { return s.M.RapidXY(x, y) }); err != nil {
		return 0, err
	}
	z, ok, err := s.stepDown(s.Search, s.StepCoarse, s.FeedCoarse)
	if err != nil {
		return 0, err
	}
	if !ok {
		s.M.RapidZ(zClear)
		return 0, fail("медь не найдена в X%.2f Y%.2f до Z%.2f: зажим отвалился, грязная медь "+
			"или край заготовки", x, y, s.z())
	}
	if err := s.liftUntilOpen(z, s.StepCoarse); err != nil {
		return 0, err
	}
	zf, ok, err := s.stepDown(s.StepCoarse*3, s.StepFine, s.FeedFine)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fail("повторное касание не случилось в X%.2f Y%.2f", x, y)
	}
	if err := s.liftUntilOpen(zf, s.StepCoarse); err != nil {
		return 0, err
	}
	if err := s.travel(func() error { return s.M.RapidZ(zf + s.Clear) }); err != nil {
		return 0, err
	}
	return zf, nil
}

type Point struct{ X, Y, Z float64 }

// Grid touches every point; out gets each row as soon as it is measured.
func (s *Scanner) Grid(pts [][2]float64, out func(Point)) ([]Point, error) {
	zClear := s.z()
	s.floor, s.hasFloor = zClear-s.Search-1, true
	var rows []Point
	for i, p := range pts {
		if len(rows) > 0 {
			// travel just above the copper here and at the nearest measured point: a short
			// lift means few search steps, and a skipped lift still shows on the first step
			near, last := rows[0], rows[len(rows)-1]
			for _, r := range rows {
				if math.Hypot(r.X-p[0], r.Y-p[1]) < math.Hypot(near.X-p[0], near.Y-p[1]) {
					near = r
				}
			}
			zClear = math.Max(near.Z, last.Z) + s.Clear
		}
		z, err := s.TouchAt(p[0], p[1], zClear)
		if err != nil {
			return rows, err
		}
		hi, lo := z, z
		for _, r := range rows {
			hi, lo = math.Max(hi, r.Z), math.Min(lo, r.Z)
		}
		if len(rows) > 0 {
			prevHi := rows[0].Z
			for _, r := range rows {
				prevHi = math.Max(prevHi, r.Z)
			}
			if z > prevHi+s.Clear*0.6 {
				// copper right under the lift height: a lift was skipped and the count is off
				return rows, fail("в X%.2f Y%.2f медь на %.3f мм выше всех прежних точек: похоже, станок "+
					"пропустил команду. Увеличьте паузу (settle)", p[0], p[1], z-prevHi)
			}
		}
		r := Point{p[0], p[1], z}
		rows = append(rows, r)
		if out != nil {
			out(r)
		}
		_ = hi
		// a clip fallen off the copper reads as "no contact": with the copper level known,
		// going far below it can only mean digging into the board
		s.floor = lo - s.Below
		s.Log(fmt.Sprintf("%3d/%d  X%7.2f Y%7.2f  Z%8.4f", i+1, len(pts), p[0], p[1], z))
	}
	return rows, nil
}

// Recheck touches the first point again after the scan: anything but zero is how
// far Z wandered while scanning, and the map is only as good as that number.
func (s *Scanner) Recheck(rows []Point) (float64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	hi := rows[0].Z
	for _, r := range rows {
		hi = math.Max(hi, r.Z)
	}
	s.hasFloor = false // the drift is exactly what is unknown here: search the full depth
	z, err := s.TouchAt(rows[0].X, rows[0].Y, hi+s.Clear)
	if err != nil {
		return 0, err
	}
	return z - rows[0].Z, nil
}
