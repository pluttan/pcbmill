package main

import (
	"fmt"
	"math"
	"time"
)

// Keys types into NcStudio: open its MDI window, type one line, press Enter.
type Keys interface {
	Grab() error // bring NcStudio forward; after this, losing focus aborts
	OpenMDI() error
	Line(s string) error
}

// NcStudio drives the machine through MDI lines. XY are absolute work coordinates
// (G90), Z is relative (G91) and tracked here starting from 0 at the start height.
type NcStudio struct {
	K       Keys
	Rapid   float64       // G00 speed set in NcStudio, mm/min, to know how long to wait
	Settle  time.Duration // per command: keystrokes + NcStudio reaction + acceleration
	Reopen  bool          // MDI window closes after Enter
	Abs     bool          // Z as G90 work coordinates: NcStudio Z was zeroed at the start height
	Sleep   func(time.Duration)
	x, y, z float64
	known   bool
	opened  bool
}

func NewNcStudio(k Keys) *NcStudio {
	return &NcStudio{K: k, Rapid: 1500, Settle: 350 * time.Millisecond, Reopen: true, Sleep: time.Sleep}
}

func (n *NcStudio) cmd(line string, seconds float64) error {
	if n.Reopen || !n.opened {
		if err := n.K.OpenMDI(); err != nil {
			return err
		}
		n.opened = true
	}
	if err := n.K.Line(line); err != nil {
		return err
	}
	n.Sleep(time.Duration(seconds*float64(time.Second)) + n.Settle)
	return nil
}

func (n *NcStudio) Pos() (float64, float64, float64) { return n.x, n.y, n.z }

func (n *NcStudio) RapidZ(z float64) error {
	dz := z - n.z
	if math.Abs(dz) < 1e-4 {
		return nil
	}
	line := fmt.Sprintf("G91 G00 Z%.4f", dz)
	if n.Abs {
		line = fmt.Sprintf("G90 G00 Z%.4f", z)
	}
	if err := n.cmd(line, math.Abs(dz)/n.Rapid*60); err != nil {
		return err
	}
	n.z = z
	return nil
}

func (n *NcStudio) RapidXY(x, y float64) error {
	dist := 300.0 // first move: start position unknown, wait generously
	if n.known {
		dist = math.Hypot(x-n.x, y-n.y)
	}
	if err := n.cmd(fmt.Sprintf("G90 G00 X%.4f Y%.4f", x, y), dist/n.Rapid*60+0.3); err != nil {
		return err
	}
	n.x, n.y, n.known = x, y, true
	return nil
}

// MoveXYRel: relative XY rapid for manual mode.
func (n *NcStudio) MoveXYRel(dx, dy float64) error {
	if err := n.cmd(fmt.Sprintf("G91 G00 X%.4f Y%.4f", dx, dy), math.Hypot(dx, dy)/n.Rapid*60+0.3); err != nil {
		return err
	}
	n.x, n.y = n.x+dx, n.y+dy
	return nil
}

func (n *NcStudio) MoveZ(z, feed float64) error {
	dz := z - n.z
	if math.Abs(dz) < 1e-5 {
		return nil
	}
	// relative steps let NcStudio's rounding pile up over a long scan; absolute ones cannot
	line := fmt.Sprintf("G91 G01 Z%.4f F%.0f", dz, feed)
	if n.Abs {
		line = fmt.Sprintf("G90 G01 Z%.4f F%.0f", z, feed)
	}
	if err := n.cmd(line, math.Abs(dz)/feed*60); err != nil {
		return err
	}
	n.z = z
	return nil
}

func (n *NcStudio) Grab() error { return n.K.Grab() }

// ResetZ: the operator zeroed Z in NcStudio at the current height.
func (n *NcStudio) ResetZ() { n.z = 0 }
