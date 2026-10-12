package main

// Height scan for NcStudio: no probe input and no API, so the tool goes down in
// short separate MDI moves and the MCU is asked about contact between them.
// Z is never read from the machine: every Z move is relative and summed here.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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
	FeedNear   float64      // the long approach move: its depth is fixed by the target, speed only sets the wait
	Margin     float64      // approach in one move to this much above the predicted copper
	FineLimit  float64      // fine steps below the approach before falling back to the coarse search
	Predict    bool         // Z is absolute (G90): a skipped command cannot shift the count, so one long approach move is safe
	Stuck      int          // points touched again after a contact that would not open
	Skipped    [][2]float64 // points the operator let go after failed tries
	// Ask: a point failed pointTries times; askRetry, askSkip or askStop.
	// nil: the error ends the scan.
	Ask      func(i int, p [2]float64, err error, canSkip bool) int
	start    float64 // Z where the scan began
	Far      int     // the first points are far apart (the corners): travel high, full search, no "higher" check
	floor    float64
	hasFloor bool
	Log      func(string)
}

func NewScanner(m Machine, t Touch) *Scanner {
	return &Scanner{M: m, T: t, Clear: 0.3, Search: 3.0, Below: 0.3,
		StepCoarse: 0.05, StepFine: 0.01,
		FeedCoarse: 20, FeedFine: 20, FeedNear: 100, Margin: 0.04, FineLimit: 0.06, Log: func(string) {}}
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
	return s.liftUntilOpenN(z, step, 5)
}

func (s *Scanner) liftUntilOpenN(z, step float64, n int) error {
	for k := 1; k <= n; k++ {
		if err := s.M.MoveZ(z+step*float64(k), s.FeedFine); err != nil {
			return err
		}
		if !s.T.State() {
			return nil
		}
	}
	return s.liftFar(z)
}

// farLift: the travel height over the highest copper so far between the
// corners: a blank tilted 0.5 mm across still passes under it.
const farLift = 1.0

// stuckLift: how high the tool goes when the contact will not open.
const stuckLift = 2.0

// errStuck: the contact opened only well above the copper: the touch is not
// to be trusted, the point is measured again from there.
var errStuck = ScanError{"контакт разомкнулся только высоко над медью"}

// liftFar: the contact stays closed after the usual lift (the tip stuck in
// the copper, a burr, a twitch of the wire): up in 0.1 mm steps to stuckLift
// over z. Open on the way: errStuck, the point is touched again.
func (s *Scanner) liftFar(z float64) error {
	for h := 0.1; h <= stuckLift+1e-9; h += 0.1 {
		if s.z() >= z+h {
			continue
		}
		if err := s.M.MoveZ(z+h, s.FeedCoarse); err != nil {
			return err
		}
		if err := s.T.Alive(); err != nil {
			return err
		}
		if !s.T.State() {
			s.Stuck++
			s.Log(fmt.Sprintf("контакт разомкнулся на %.1f мм над точкой касания: снимаю точку заново", h))
			return errStuck
		}
	}
	return fail("контакт не размыкается и на %.1f мм над медью: провод касания замкнут на фрезу или на раму", stuckLift)
}

// TouchAt finds the copper under (x, y) and returns its Z; a contact that
// stuck gets one more try from where it opened.
func (s *Scanner) TouchAt(x, y, zClear float64) (float64, error) {
	z, err := s.touchAt(x, y, zClear)
	if errors.Is(err, errStuck) {
		return s.touchAt(x, y, s.z())
	}
	return z, err
}

func (s *Scanner) touchAt(x, y, zClear float64) (float64, error) {
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

// TouchNear is TouchAt for a point whose copper height is already predicted from
// its neighbours: one move down to just above it, then only fine steps. Copper
// above the prediction is met during that move and costs at most Margin of
// pressing; copper well below it falls back to the ordinary coarse search.
func (s *Scanner) TouchNear(x, y, zClear, pred float64) (float64, error) {
	z, err := s.touchNear(x, y, zClear, pred)
	if errors.Is(err, errStuck) {
		return s.touchAt(x, y, s.z())
	}
	return z, err
}

func (s *Scanner) touchNear(x, y, zClear, pred float64) (float64, error) {
	if s.T.State() {
		return 0, fail("контакт до переезда: фрезу не подняло или провод замкнут")
	}
	if err := s.travel(func() error { return s.M.RapidZ(zClear) }); err != nil {
		return 0, err
	}
	if err := s.travel(func() error { return s.M.RapidXY(x, y) }); err != nil {
		return 0, err
	}
	approach := pred + s.Margin
	if s.hasFloor {
		approach = math.Max(approach, s.floor)
	}
	if approach < s.z() {
		if err := s.T.Alive(); err != nil {
			return 0, err
		}
		h := s.T.Hits()
		if err := s.M.MoveZ(approach, s.FeedNear); err != nil {
			return 0, err
		}
		if err := s.T.Alive(); err != nil {
			return 0, err
		}
		if s.T.Hits() != h || s.T.State() {
			// copper higher than predicted: back off until open, then come down again
			if err := s.liftUntilOpenN(s.z(), s.StepFine, int(math.Ceil(zClear-s.z())/s.StepFine)+1); err != nil {
				return 0, err
			}
		}
	}
	zf, ok, err := s.stepDown(s.FineLimit, s.StepFine, s.FeedFine)
	if err != nil {
		return 0, err
	}
	if !ok {
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
		if zf, ok, err = s.stepDown(s.StepCoarse*3, s.StepFine, s.FeedFine); err != nil {
			return 0, err
		}
		if !ok {
			return 0, fail("повторное касание не случилось в X%.2f Y%.2f", x, y)
		}
	}
	// straight up to the travel height: one command instead of lift-until-open plus lift
	if err := s.M.RapidZ(zf + s.Clear); err != nil {
		return 0, err
	}
	if s.T.State() {
		return 0, s.liftFar(zf)
	}
	return zf, nil
}

// predict guesses the copper height at p from the plane through the four nearest
// measured points; along the first row (all on one line) it extends that line.
// On a real 90-point map this missed by at most 0.03 mm on the high side.
func predict(rows []Point, x, y float64) float64 {
	near := append([]Point(nil), rows...)
	d := func(r Point) float64 { return math.Hypot(r.X-x, r.Y-y) }
	sort.Slice(near, func(i, j int) bool { return d(near[i]) < d(near[j]) })
	if len(near) > 4 {
		near = near[:4]
	}
	if len(near) == 1 {
		return near[0].Z
	}
	if len(near) >= 3 {
		// least squares z = a + b*dx + c*dy around p
		var m [3][4]float64
		for _, r := range near {
			v := [3]float64{1, r.X - x, r.Y - y}
			for i := 0; i < 3; i++ {
				for j := 0; j < 3; j++ {
					m[i][j] += v[i] * v[j]
				}
				m[i][3] += v[i] * r.Z
			}
		}
		if a, ok := solve3(m); ok {
			return a
		}
	}
	a, b := near[0], near[1]
	dx, dy := a.X-b.X, a.Y-b.Y
	t := ((x-a.X)*dx + (y-a.Y)*dy) / (dx*dx + dy*dy)
	t = math.Max(-1.5, math.Min(1.5, t))
	return a.Z + (a.Z-b.Z)*t
}

// solve3 returns the first unknown of a 3x3 system, or ok=false when the points
// lie on one line and the plane is not defined.
func solve3(m [3][4]float64) (float64, bool) {
	scale := math.Max(m[1][1], m[2][2])
	for c := 0; c < 3; c++ {
		p := c
		for r := c + 1; r < 3; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		m[c], m[p] = m[p], m[c]
		if math.Abs(m[c][c]) < 1e-6*math.Max(scale, 1) {
			return 0, false
		}
		for r := 0; r < 3; r++ {
			if r != c {
				f := m[r][c] / m[c][c]
				for k := c; k < 4; k++ {
					m[r][k] -= f * m[c][k]
				}
			}
		}
	}
	return m[0][3] / m[0][0], true
}

type Point struct{ X, Y, Z float64 }

// Grid touches every point; out gets each row as soon as it is measured.
func (s *Scanner) Grid(pts [][2]float64, out func(Point)) ([]Point, error) {
	s.start = s.z()
	s.floor, s.hasFloor = s.start-s.Search-1, true
	var rows []Point
	for i, p := range pts {
		z, err := s.point(i, p, rows)
		skip := false
		for tries := 1; err != nil; tries++ {
			var se ScanError
			if !errors.As(err, &se) || strings.Contains(err.Error(), "остановлено") || s.T.Alive() != nil {
				return rows, err // the operator, or the MCU gone for good: not a move more
			}
			s.Log(fmt.Sprintf("точка %d не снята (%d): %v", i+1, tries, err))
			if e := s.recover(rows); e != nil {
				return rows, e
			}
			if tries >= pointTries {
				if s.Ask == nil {
					return rows, err
				}
				switch s.Ask(i, p, err, len(rows) > 0) {
				case askRetry:
					tries = 0
				case askSkip:
					skip = true
				default:
					return rows, err
				}
			}
			if skip {
				break
			}
			z, err = s.point(i, p, rows)
		}
		if skip {
			s.Skipped = append(s.Skipped, p)
			continue
		}
		lo := z
		for _, r := range rows {
			lo = math.Min(lo, r.Z)
		}
		r := Point{p[0], p[1], z}
		rows = append(rows, r)
		if out != nil {
			out(r)
		}
		// a clip fallen off the copper reads as "no contact": with the copper level known,
		// going far below it can only mean digging into the board
		s.floor = lo - s.Below
		s.Log(fmt.Sprintf("%3d/%d  X%7.2f Y%7.2f  Z%8.4f", i+1, len(pts), p[0], p[1], z))
	}
	return rows, nil
}

// pointTries: a point is touched this many times before the operator is asked.
const pointTries = 3

// Answers of Ask.
const (
	askStop = iota
	askRetry
	askSkip
)

// recover: after a failed point, up and clear of the copper before the next try.
func (s *Scanner) recover(rows []Point) error {
	hi := s.start - farLift
	for _, r := range rows {
		hi = math.Max(hi, r.Z)
	}
	if up := hi + farLift; s.z() < up {
		return s.M.RapidZ(up)
	}
	return nil
}

// point: one touch at p, the way its place in the order wants.
func (s *Scanner) point(i int, p [2]float64, rows []Point) (float64, error) {
	zClear := s.start
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
		if i < s.Far { // across the whole blank: it may rise more than a lift
			hi := rows[0].Z
			for _, r := range rows {
				hi = math.Max(hi, r.Z)
			}
			zClear = hi + farLift
		}
		zClear = math.Max(zClear, math.Min(s.z(), zClear+farLift)) // after a recover: no dive first
	}
	var z float64
	var err error
	if len(rows) == 0 || !s.Predict || i < s.Far {
		z, err = s.TouchAt(p[0], p[1], zClear)
	} else {
		pred := predict(rows, p[0], p[1])
		zClear = math.Max(zClear, pred+s.Margin+s.Clear/2)
		z, err = s.TouchNear(p[0], p[1], zClear, pred)
	}
	if err != nil {
		return 0, err
	}
	if len(rows) > 0 && i >= s.Far {
		prevHi := rows[0].Z
		for _, r := range rows {
			prevHi = math.Max(prevHi, r.Z)
		}
		if z > prevHi+s.Clear*0.6 {
			// copper right under the lift height: a lift was skipped and the count is off
			return 0, fail("в X%.2f Y%.2f медь на %.3f мм выше всех прежних точек: похоже, станок "+
				"пропустил команду", p[0], p[1], z-prevHi)
		}
	}
	return z, nil
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
