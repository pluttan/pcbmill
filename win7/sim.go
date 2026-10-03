package main

// Emulation: the machine and the MCU without a machine. A virtual NcStudio
// reads the MDI lines the program types (G90/G91, G0/G1, X Y Z F, several
// commands split by ';' as NcStudio 5 takes them) and moves a virtual tool at
// the real speeds, time sped up -simspeed times; a virtual MCU reports contact
// of that tool with a virtual copper blank. Faults are switched on from the
// program (menu 8, digits 1-6 on the work screens) to watch every protection stop the work. The tests
// drive the same machine with Speed 0: every move done at once.

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// simMove is one straight move of the tool, timed in real time.
type simMove struct {
	from, to [3]float64
	t0       time.Time
	dur      time.Duration
	done     float64 // the part of the path already checked for contact
}

type simMachine struct {
	mu      sync.Mutex
	x, y, z float64 // tool tip; z in the board frame
	offset  float64 // work Z0 in the board frame: G90 Z targets are relative to it
	surface func(x, y float64) float64

	abs, rapidMode bool    // G90 / G91 and G0 / G1 in force, as NcStudio keeps them
	feed           float64 // the last F, mm/min
	Rapid          float64 // G0 speed, mm/min
	Speed          float64 // time scale; 0: every move done at once (tests)
	mv             *simMove

	hits   int
	state  bool
	maxDig float64 // deepest the tip went under the copper
	lines  []string

	// test hooks
	dropIf   func(string) bool // the first line it says yes to is lost
	dieAfter int               // the MCU goes silent after this many lines (0: never)
	relErr   float64           // NcStudio rounding: every relative Z move lands this much off
	mechUp   float64           // the Z axis itself creeps up by this much per command
	broken   bool              // the clip is off the copper: never a contact

	// faults switched on from the screen
	shorted, noisy, skipLift bool
	deadAt                   time.Time // the MCU stopped answering (zero: it answers)
	deadState                bool
	deadHits                 int
	spikeTill, lastNoise     time.Time
	rnd                      *rand.Rand
	skipped                  string // the command the machine lost, for the screen
}

func newSimMachine(surface func(x, y float64) float64) *simMachine {
	return &simMachine{surface: surface, abs: true, rapidMode: true, feed: 100, Rapid: 1500,
		rnd: rand.New(rand.NewSource(1))}
}

// The blank of the emulation: 100x100 mm with X0 Y0 2 mm in from its corner,
// tilted 0.16 mm along X and 0.09 mm along Y per 100 mm, bowed and with a bump
// of 0.05 mm. The tool starts 1.5 mm over the copper at X0 Y0, Z zeroed there.
const (
	blankX0, blankX1 = -2.0, 98.0
	blankY0, blankY1 = -2.0, 98.0
	simStart         = 1.5
)

func blankRelief(x, y float64) float64 {
	bump := 0.05 * math.Exp(-((x-55)*(x-55)+(y-40)*(y-40))/(2*18*18))
	bow := -0.03 * ((x - 48) / 50) * ((x - 48) / 50)
	return 0.0016*x + 0.0009*y + bump + bow
}

// simSurface: copper height under (x, y); off the blank only the table, far below.
func simSurface(x, y float64) float64 {
	if x < blankX0 || x > blankX1 || y < blankY0 || y > blankY1 {
		return -50
	}
	return -simStart + blankRelief(x, y) - blankRelief(0, 0)
}

var reSimWord = regexp.MustCompile(`([A-Z])\s*([-+]?\d*\.?\d+)`)

// ==========================
// ===  Virtual NcStudio  ===
// ==========================

func (s *simMachine) Grab() error { return nil }

// OpenMDI: the real window takes a moment to come up.
func (s *simMachine) OpenMDI() error {
	if s.Speed > 0 {
		s.sleep(120 * time.Millisecond)
	}
	return nil
}

// sleep is a pause of machine time: -simspeed times shorter in real time.
func (s *simMachine) sleep(d time.Duration) {
	if s.Speed > 0 {
		time.Sleep(time.Duration(float64(d) / s.Speed))
	}
}

// Line runs one MDI line; a move still under way is finished first.
func (s *simMachine) Line(l string) error {
	s.mu.Lock()
	s.lines = append(s.lines, l)
	if s.dropIf != nil && s.dropIf(l) {
		s.dropIf = nil
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	for _, part := range strings.Split(l, ";") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		s.waitIdle()
		s.mu.Lock()
		s.exec(strings.ToUpper(part))
		s.mu.Unlock()
	}
	return nil
}

func (s *simMachine) waitIdle() {
	for {
		s.mu.Lock()
		now := time.Now()
		s.advance(now)
		mv := s.mv
		s.mu.Unlock()
		if mv == nil {
			return
		}
		rem := mv.t0.Add(mv.dur).Sub(now)
		if rem < time.Millisecond {
			rem = time.Millisecond
		}
		time.Sleep(rem)
	}
}

// exec is one command: the modal words first, then the target, then the move.
func (s *simMachine) exec(cmd string) {
	ws := reSimWord.FindAllStringSubmatch(cmd, -1)
	if len(ws) == 0 {
		return
	}
	for _, w := range ws {
		v, _ := strconv.ParseFloat(w[2], 64)
		switch {
		case w[1] == "G" && v == 0:
			s.rapidMode = true
		case w[1] == "G" && v == 1:
			s.rapidMode = false
		case w[1] == "G" && v == 90:
			s.abs = true
		case w[1] == "G" && v == 91:
			s.abs = false
		case w[1] == "F" && v > 0:
			s.feed = v
		}
	}
	// a creeping axis moves the machine's own idea of zero, under any command style
	s.offset += s.mechUp
	s.z += s.mechUp
	to, hasZ := [3]float64{s.x, s.y, s.z}, false
	for _, w := range ws {
		v, _ := strconv.ParseFloat(w[2], 64)
		switch w[1] {
		case "X", "Y":
			i := map[string]int{"X": 0, "Y": 1}[w[1]]
			if s.abs {
				to[i] = v
			} else {
				to[i] += v
			}
		case "Z":
			hasZ = true
			if s.abs {
				to[2] = v + s.offset
			} else {
				to[2] += v
			}
		}
	}
	if hasZ && !s.abs {
		to[2] += s.relErr
	}
	if s.skipLift && hasZ && to[2] > s.z+1e-6 {
		s.skipLift, s.skipped = false, strings.TrimSpace(cmd)
		return
	}
	from := [3]float64{s.x, s.y, s.z}
	dist := math.Sqrt(sq(to[0]-from[0]) + sq(to[1]-from[1]) + sq(to[2]-from[2]))
	if dist < 1e-9 {
		return
	}
	rate := s.feed
	if s.rapidMode {
		rate = s.Rapid
	}
	sec := dist/rate*60 + 0.03 // the drives speed up and slow down
	if s.Speed <= 0 {
		s.mv = &simMove{from: from, to: to}
		s.advance(time.Now())
		return
	}
	s.mv = &simMove{from: from, to: to, t0: time.Now(), dur: time.Duration(sec / s.Speed * float64(time.Second))}
	s.advance(time.Now())
}

func sq(v float64) float64 { return v * v }

// advance moves the tool to where it is at now, looking for contact all along
// the way: a brief touch in the middle of a move counts as a hit too.
func (s *simMachine) advance(now time.Time) {
	if s.mv == nil {
		if s.Speed > 0 {
			s.touch() // faults switched meanwhile change what the MCU says
		}
		return
	}
	m := s.mv
	f := 1.0
	if m.dur > 0 {
		f = math.Min(1, float64(now.Sub(m.t0))/float64(m.dur))
	}
	if f <= m.done {
		return
	}
	at := func(k float64) [3]float64 {
		var p [3]float64
		for i := range p {
			p[i] = m.from[i] + (m.to[i]-m.from[i])*k
		}
		return p
	}
	a, b := at(m.done), at(f)
	n := int(math.Ceil(math.Sqrt(sq(b[0]-a[0])+sq(b[1]-a[1])+sq(b[2]-a[2])) / 0.005))
	if n < 1 {
		n = 1
	}
	if n > 4000 {
		n = 4000
	}
	for i := 1; i <= n; i++ {
		p := at(m.done + (f-m.done)*float64(i)/float64(n))
		s.x, s.y, s.z = p[0], p[1], p[2]
		s.touch()
	}
	m.done = f
	if f >= 1 {
		s.x, s.y, s.z = m.to[0], m.to[1], m.to[2]
		s.mv = nil
	}
}

// touch is the contact of the tool with the copper where it is now.
func (s *simMachine) touch() {
	surf := s.surface(s.x, s.y)
	s.maxDig = math.Max(s.maxDig, surf-s.z)
	c := s.z <= surf
	if s.noisy && !c && s.z-surf < 0.015 && s.rnd.Intn(3) == 0 {
		c = true // the contact bounces just over the copper
	}
	if s.broken {
		c = false
	}
	if s.shorted {
		c = true
	}
	if c && !s.state {
		s.hits++
	}
	s.state = c
}

// ==========================
// ===  Virtual MCU       ===
// ==========================

// mcuDown: the MCU has been silent long enough for Alive to see it.
func (s *simMachine) mcuDown(now time.Time) (float64, bool) {
	if s.deadAt.IsZero() {
		return 0, false
	}
	d := now.Sub(s.deadAt).Seconds() * math.Max(s.Speed, 1)
	return d, d > 0.5
}

func (s *simMachine) Alive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dieAfter > 0 && len(s.lines) >= s.dieAfter {
		return ScanError{"МК не отвечает"}
	}
	if _, down := s.mcuDown(time.Now()); down { // the seconds of the clock on the wall
		return fmt.Errorf("МК не отвечает %.1f с: выдернут USB или завис", time.Since(s.deadAt).Seconds())
	}
	return nil
}

// noise: with the fault on, a short false contact about every two seconds.
func (s *simMachine) noise(now time.Time) {
	if !s.noisy || s.Speed <= 0 {
		return
	}
	if !s.lastNoise.IsZero() {
		dt := now.Sub(s.lastNoise).Seconds() * s.Speed
		if s.rnd.Float64() < dt/2 {
			if !s.state && now.After(s.spikeTill) {
				s.hits++
			}
			s.spikeTill = now.Add(30 * time.Millisecond)
		}
	}
	s.lastNoise = now
}

func (s *simMachine) State() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.deadAt.IsZero() {
		return s.deadState
	}
	s.advance(now)
	s.noise(now)
	return s.state || now.Before(s.spikeTill)
}

func (s *simMachine) Hits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.deadAt.IsZero() {
		return s.deadHits
	}
	now := time.Now()
	s.advance(now)
	s.noise(now)
	return s.hits
}

// ==========================
// ===  Operator, faults  ===
// ==========================

// WorkPos is what the NcStudio screen would show: X, Y and Z in work coordinates.
func (s *simMachine) WorkPos() (x, y, z float64, moving bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance(time.Now())
	return s.x, s.y, s.z - s.offset, s.mv != nil
}

// ZeroZ is the operator pressing "Z = 0" in NcStudio at the present height.
func (s *simMachine) ZeroZ() {
	s.waitIdle()
	s.mu.Lock()
	s.offset = s.z
	s.mu.Unlock()
}

// Reset puts the tool back where a work starts: over X0 Y0, 1.5 mm over the
// copper, Z zeroed there. The faults stay as they are.
func (s *simMachine) Reset() {
	s.waitIdle()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.x, s.y = 0, 0
	s.z = s.surface(0, 0) + simStart
	s.offset = s.z
	s.abs, s.rapidMode = true, true
	s.touch()
}

type simFault int

const (
	fDead simFault = iota
	fClip
	fShort
	fSkip
	fCreep
	fNoise
	fCount
)

// simFaults: the digit key, the short name for the status line, the description.
var simFaults = [fCount]struct {
	fkey        int
	short, long string
}{
	{1, "МК ОТВАЛИЛСЯ", "МК отвалился: USB выдернут, ответов нет"},
	{2, "ЗАЖИМ", "зажим отвалился: касания нет нигде"},
	{3, "ЗАМЫКАНИЕ", "провод замкнут: касание всегда"},
	{4, "ПРОПУСК", "станок пропустит следующий подъём Z"},
	{5, "Z ПОЛЗЁТ", "ось Z ползёт вверх на 0.2 мкм за команду"},
	{6, "ДРЕБЕЗГ", "дребезг и наводки на проводе"},
}

func (s *simMachine) On(f simFault) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch f {
	case fDead:
		return !s.deadAt.IsZero()
	case fClip:
		return s.broken
	case fShort:
		return s.shorted
	case fSkip:
		return s.skipLift
	case fCreep:
		return s.mechUp != 0
	case fNoise:
		return s.noisy
	}
	return false
}

func (s *simMachine) Toggle(f simFault) {
	on := s.On(f)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance(time.Now())
	switch f {
	case fDead:
		if on {
			s.deadAt = time.Time{}
		} else {
			s.deadAt, s.deadState, s.deadHits = time.Now(), s.state, s.hits
		}
	case fClip:
		s.broken = !on
	case fShort:
		s.shorted = !on
	case fSkip:
		s.skipLift, s.skipped = !on, ""
	case fCreep:
		s.mechUp = 0
		if !on {
			s.mechUp = 0.0002
		}
	case fNoise:
		s.noisy = !on
		s.lastNoise = time.Time{}
	}
	if !s.broken && !s.shorted {
		s.touch()
	}
}

// Faults are the short names of the faults switched on.
func (s *simMachine) Faults() []string {
	var out []string
	for f := simFault(0); f < fCount; f++ {
		if s.On(f) {
			out = append(out, simFaults[f].short)
		}
	}
	return out
}

// faultKey is the fault an F key switches, or false.
// faultKey: digits 1-6 switch the faults, on the screens that let them (faultKeys).
func faultKey(k Key) (simFault, bool) {
	if k.Code == KeyRune && k.R >= '1' && int(k.R-'1') < int(fCount) {
		return simFault(k.R - '1'), true
	}
	return 0, false
}

// simWindows is NcWindows for the emulation: the virtual NcStudio shows its
// coordinates as text, so menu item 7 finds them like on a real one.
func simWindows(string) ([]WinInfo, error) {
	x, y, z, _ := simM.WorkPos()
	return []WinInfo{
		{H: 0x101, Class: "Static", Text: "NcStudio (эмуляция)", ID: 100},
		{H: 0x102, Class: "Static", Text: fmt.Sprintf("%.3f", x), ID: 1001, Left: 80, Top: 60, Right: 180, Bott: 80},
		{H: 0x103, Class: "Static", Text: fmt.Sprintf("%.3f", y), ID: 1002, Left: 80, Top: 90, Right: 180, Bott: 110},
		{H: 0x104, Class: "Static", Text: fmt.Sprintf("%.3f", z), ID: 1003, Left: 80, Top: 120, Right: 180, Bott: 140},
		{H: 0x105, Class: "Button", Text: "Старт", ID: 2001},
	}, nil
}
