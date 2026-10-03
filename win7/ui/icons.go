package ui

// Icons drawn as lines and shapes, so they are sharp at any size and need
// no image files: one stroke width, round ends, like a line icon set.

import (
	"image"
	"image/color"
	"math"

	"pcbscan/gfx"
)

type Icon int

const (
	IconNone Icon = iota
	IconCheck
	IconDot
	IconRing
	IconNext
	IconBoards
	IconMap
	IconMill
	IconMask
	IconDrill
	IconVcut
	IconGear
	IconExit
	IconPlay
	IconStop
	IconWarn
	IconInfo
	IconFile
	IconRedo
	IconPlug
	IconTarget
	IconFit
	IconTop
	IconCube
	IconHand
	IconZero
	IconSim
	IconSheet
	IconNumbers
	IconBack
	IconPlus
	IconMinus
	IconClose
)

// Icon draws ic into r (a square works best).
func (c *Ctx) Icon(ic Icon, r image.Rectangle, col color.NRGBA) {
	s := float64(imin(r.Dx(), r.Dy()))
	ox := float64(r.Min.X) + (float64(r.Dx())-s)/2
	oy := float64(r.Min.Y) + (float64(r.Dy())-s)/2
	// a 24-unit design grid
	k := s / 24
	P := func(x, y float64) (float64, float64) { return ox + x*k, oy + y*k }
	w := math.Max(1.3, 1.9*k)
	ln := func(x0, y0, x1, y1 float64) {
		a, b := P(x0, y0)
		cc, d := P(x1, y1)
		c.C.Line(a, b, cc, d, w, col)
	}
	poly := func(closed bool, xy ...float64) {
		var pts []gfx.Pt
		for i := 0; i+1 < len(xy); i += 2 {
			x, y := P(xy[i], xy[i+1])
			pts = append(pts, gfx.Pt{X: x, Y: y})
		}
		c.C.Polyline(pts, w, col, closed)
	}
	fill := func(xy ...float64) {
		var p gfx.Path
		for i := 0; i+1 < len(xy); i += 2 {
			x, y := P(xy[i], xy[i+1])
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
		c.C.FillPath(&p, col)
	}
	circ := func(x, y, r float64) {
		a, b := P(x, y)
		c.C.StrokeCircle(a, b, r*k, w, col)
	}
	disc := func(x, y, r float64) {
		a, b := P(x, y)
		c.C.FillCircle(a, b, r*k, col)
	}
	rect := func(x, y, rw, rh, rad float64) {
		a, b := P(x, y)
		c.C.StrokeRoundRect(a, b, rw*k, rh*k, rad*k, w, col)
	}
	switch ic {
	case IconCheck:
		poly(false, 5, 12.5, 10, 17.5, 19, 7)
	case IconDot:
		disc(12, 12, 3.5)
	case IconRing:
		circ(12, 12, 6.5)
	case IconNext:
		poly(false, 9, 5, 16, 12, 9, 19)
	case IconBack:
		poly(false, 15, 5, 8, 12, 15, 19)
	case IconBoards:
		rect(3, 5, 12, 10, 1.5)
		rect(9, 9, 12, 10, 1.5)
	case IconMap:
		poly(false, 2, 16, 6, 10, 10, 14, 14, 7, 18, 11, 22, 8)
		poly(false, 2, 20, 22, 20)
	case IconMill:
		poly(false, 9, 2, 15, 2, 15, 11, 9, 11)
		poly(false, 10, 11, 10, 16, 12, 21, 14, 16, 14, 11)
		ln(9, 5, 15, 5)
	case IconMask:
		poly(true, 12, 3, 18, 12, 18, 15, 15, 20, 9, 20, 6, 15, 6, 12)
	case IconDrill:
		poly(false, 8, 2, 16, 2, 16, 8, 8, 8, 8, 2)
		poly(false, 11, 8, 11, 18, 12, 21, 13, 18, 13, 8)
		ln(11, 12, 13, 11)
		ln(11, 15, 13, 14)
	case IconVcut:
		poly(false, 2, 8, 9, 8, 12, 15, 15, 8, 22, 8)
		ln(2, 19, 22, 19)
	case IconGear:
		circ(12, 12, 3.2)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			ln(12+6.2*math.Cos(a), 12+6.2*math.Sin(a), 12+9*math.Cos(a), 12+9*math.Sin(a))
		}
		circ(12, 12, 6.3)
	case IconExit:
		poly(false, 13, 4, 5, 4, 5, 20, 13, 20)
		ln(10, 12, 21, 12)
		poly(false, 17, 8, 21, 12, 17, 16)
	case IconPlay:
		fill(8, 5, 19, 12, 8, 19)
	case IconStop:
		a, b := P(6, 6)
		c.C.FillRoundRect(a, b, 12*k, 12*k, 2*k, col)
	case IconWarn:
		poly(true, 12, 3, 22, 20, 2, 20)
		ln(12, 9, 12, 14)
		disc(12, 17, 1.2)
	case IconInfo:
		circ(12, 12, 9)
		ln(12, 11, 12, 17)
		disc(12, 7.5, 1.2)
	case IconFile:
		poly(true, 6, 2, 14, 2, 19, 7, 19, 22, 6, 22)
		poly(false, 14, 2, 14, 7, 19, 7)
	case IconRedo:
		var pts []gfx.Pt
		for i := 0; i <= 20; i++ {
			a := -math.Pi*0.15 + float64(i)/20*math.Pi*1.6
			x, y := P(12+7*math.Cos(a), 12+7*math.Sin(a))
			pts = append(pts, gfx.Pt{X: x, Y: y})
		}
		c.C.Polyline(pts, w, col, false)
		poly(false, 19, 3, 18.6, 8.5, 13.5, 7.5)
	case IconPlug:
		poly(false, 9, 3, 9, 8)
		poly(false, 15, 3, 15, 8)
		poly(false, 6, 8, 18, 8, 18, 12, 12, 17, 6, 12, 6, 8)
		ln(12, 17, 12, 22)
	case IconTarget:
		circ(12, 12, 7)
		ln(12, 2, 12, 7)
		ln(12, 17, 12, 22)
		ln(2, 12, 7, 12)
		ln(17, 12, 22, 12)
	case IconFit:
		poly(false, 3, 8, 3, 3, 8, 3)
		poly(false, 16, 3, 21, 3, 21, 8)
		poly(false, 21, 16, 21, 21, 16, 21)
		poly(false, 8, 21, 3, 21, 3, 16)
	case IconTop:
		rect(4, 4, 16, 16, 2)
		disc(12, 12, 2.2)
	case IconCube:
		poly(true, 12, 2, 21, 7, 21, 17, 12, 22, 3, 17, 3, 7)
		poly(false, 3, 7, 12, 12, 21, 7)
		ln(12, 12, 12, 22)
	case IconHand:
		poly(false, 12, 4, 12, 20)
		poly(false, 4, 12, 20, 12)
		poly(false, 9, 7, 12, 4, 15, 7)
		poly(false, 9, 17, 12, 20, 15, 17)
		poly(false, 7, 9, 4, 12, 7, 15)
		poly(false, 17, 9, 20, 12, 17, 15)
	case IconZero:
		poly(true, 12, 3, 21, 12, 12, 21, 3, 12)
		disc(12, 12, 2)
	case IconSim:
		rect(3, 4, 18, 13, 2)
		ln(8, 21, 16, 21)
		ln(12, 17, 12, 21)
	case IconSheet:
		rect(3, 3, 18, 18, 2)
		ln(3, 9, 21, 9)
		ln(9, 9, 9, 21)
	case IconNumbers:
		ln(4, 7, 20, 7)
		ln(4, 12, 20, 12)
		ln(4, 17, 14, 17)
	case IconPlus:
		ln(12, 5, 12, 19)
		ln(5, 12, 19, 12)
	case IconMinus:
		ln(5, 12, 19, 12)
	case IconClose:
		ln(6, 6, 18, 18)
		ln(18, 6, 6, 18)
	}
}
