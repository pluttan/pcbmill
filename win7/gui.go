package main

// The window of the program: started once, every screen draws into it. The
// work (the machine, the files) goes on this goroutine; the window has its
// own and stays alive while the work waits for NcStudio.

import (
	"fmt"
	"image"
	"image/color"
	"sync"

	"pcbscan/gfx"
	"pcbscan/ui"
)

var (
	app   *ui.App
	theme *ui.Theme
)

// mcuState: what the status line says about the MCU.
type mcuState int

const (
	mcuNone mcuState = iota
	mcuOpen
	mcuContact
	mcuDead
)

type SimInfo struct {
	X, Y, Z float64
	Moving  bool
	Faults  []string
}

type Status struct {
	Port string
	MCU  mcuState
	Nc   bool
	Sim  *SimInfo
}

var statusFn func() Status

// startGUI opens the window (native on Windows, a browser page elsewhere).
func startGUI(o opts) error {
	th, err := ui.NewTheme()
	if err != nil {
		return err
	}
	d, err := openDisplay(o)
	if err != nil {
		return err
	}
	theme = th
	app = ui.NewApp(d, th)
	app.OnClose = func() { askQuit() }
	return nil
}

// quitAsk: a close of the window comes from the window goroutine; the work
// goroutine asks and exits, so the question is only flagged here.
var (
	quitMu  sync.Mutex
	quitReq bool
)

func askQuit() {
	quitMu.Lock()
	quitReq = true
	quitMu.Unlock()
	if mainActs != nil {
		select {
		case mainActs <- func() error { quitJob(); return nil }:
		default:
		}
	}
}

// ==========================
// ===  The head bar      ===
// ==========================

// crumb: where the operator is, shown in the head bar.
var crumb = "задание"

// chrome draws the window background and the head bar; returns the body.
func chrome(c *ui.Ctx, title string) image.Rectangle {
	t := c.T
	r := image.Rect(0, 0, c.W, c.H)
	c.C.Clear(t.Mantle)
	bar := ui.CutTop(&r, t.Si(48))
	c.C.FillRect(bar, t.Crust)
	c.C.FillRect(image.Rect(bar.Min.X, bar.Max.Y-1, bar.Max.X, bar.Max.Y), gfx.Alpha(t.Surface0, 0.8))
	x := bar.Min.X + t.Si(16)
	logo := image.Rect(x, bar.Min.Y+t.Si(12), x+t.Si(24), bar.Min.Y+t.Si(36))
	drawLogo(c, logo)
	x += t.Si(34)
	x += c.Label(x, bar.Min.Y+t.Si(13), "pcbscan", ui.Style{Kind: ui.Bold, Size: ui.SizeLarge, Color: t.Text})
	if title != "" {
		x += t.Si(12)
		c.Icon(ui.IconNext, image.Rect(x, bar.Min.Y+t.Si(16), x+t.Si(16), bar.Min.Y+t.Si(32)), t.Overlay0)
		x += t.Si(22)
		rest := image.Rect(x, bar.Min.Y, bar.Max.X-statusWidth(c), bar.Max.Y)
		c.LabelIn(rest, title, ui.Style{Kind: ui.Regular, Size: ui.SizeBody, Color: t.Subtext0}, 0)
	}
	drawStatus(c, bar)
	return r
}

// drawLogo: the mark of the program, a small lit height map.
func drawLogo(c *ui.Ctx, r image.Rectangle) {
	t := c.T
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	c.C.FillRoundRect(x, y, w, h, t.S(6), t.Surface0)
	var p gfx.Path
	p.MoveTo(x+w*0.12, y+h*0.72)
	p.CubicTo(x+w*0.3, y+h*0.25, x+w*0.45, y+h*0.95, x+w*0.62, y+h*0.4)
	p.CubicTo(x+w*0.72, y+h*0.12, x+w*0.82, y+h*0.35, x+w*0.88, y+h*0.5)
	p.LineTo(x+w*0.88, y+h*0.85)
	p.LineTo(x+w*0.12, y+h*0.85)
	p.Close()
	c.C.FillPath(&p, t.Peach)
	c.C.Line(x+w*0.12, y+h*0.72, x+w*0.3, y+h*0.48, t.S(1.2), t.Yellow)
}

func statusWidth(c *ui.Ctx) int { return c.T.Si(330) }

// drawStatus: chips for the MCU, NcStudio and the emulation at the right.
func drawStatus(c *ui.Ctx, bar image.Rectangle) {
	if statusFn == nil {
		return
	}
	t := c.T
	st := statusFn()
	type chip struct {
		s   string
		dot color.NRGBA
	}
	var chips []chip
	mcu := chip{"МК нет", t.Overlay0}
	switch st.MCU {
	case mcuOpen:
		mcu = chip{"МК", t.Green}
	case mcuContact:
		mcu = chip{"МК: касание", t.Yellow}
	case mcuDead:
		mcu = chip{"МК не отвечает", t.Red}
	}
	if st.Port != "" && st.MCU != mcuNone && st.Sim == nil {
		mcu.s += " " + st.Port
	}
	chips = append(chips, mcu)
	if st.Nc {
		chips = append(chips, chip{"NcStudio", t.Green})
	} else {
		chips = append(chips, chip{"NcStudio нет", t.Overlay0})
	}
	if st.Sim != nil {
		chips = append(chips, chip{"эмуляция", t.Mauve})
	}
	x := bar.Max.X - t.Si(14)
	y := bar.Min.Y + (bar.Dy()-t.Si(22))/2
	for i := len(chips) - 1; i >= 0; i-- {
		w := c.Measure(chips[i].s, ui.Style{Kind: ui.Regular, Size: ui.SizeSmall}) + t.Si(28)
		x -= w
		c.Chip(x, y, chips[i].s, chips[i].dot, t.Surface0)
		x -= t.Si(8)
	}
}

// toolText: where the tool is, for the pages that move it.
func toolText() string {
	x, y, z, ok := toolNow()
	if !ok {
		return fmt.Sprintf("Z %+.3f", z)
	}
	return fmt.Sprintf("X %.2f  Y %.2f  Z %+.3f", x, y, z)
}
