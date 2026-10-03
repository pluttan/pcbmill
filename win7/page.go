package main

// A work page: what is being done (a head and lines about it), a journal of
// what happened, the 3D view of the machine and buttons. The work goroutine
// changes it with set and logLine and waits for a button with ask.

import (
	"image"
	"image/color"
	"strings"

	"pcbscan/gfx"
	"pcbscan/ui"
)

type pageBtn struct {
	id, label string
	kind      ui.BtnKind
	key       ui.Key // a key that presses it too
}

type page struct {
	title, head string
	info        []string
	log         []string
	nolog       bool
	progress    float64 // 0..1, below 0: none
	progText    string
	scene       ui.Scene
	big         func(c *ui.Ctx, r image.Rectangle)     // a panel instead of the journal (the wire check)
	extra       func(c *ui.Ctx, r image.Rectangle) int // widgets under the lines, returns the height used
	need        int                                    // the height the card took last frame (stacked layout)
	onKey       func(e ui.Event) bool                  // the page's own keys, on the window goroutine
	btns        []pageBtn
	acts        chan string
}

// cur is the page the journal goes to.
var cur *page

// newPage shows a page with lines about it.
func newPage(title string, info ...string) *page {
	p := &page{title: title, info: info, progress: -1, acts: make(chan string, 8)}
	crumb = title
	cur = p
	if app != nil {
		app.Show(p)
	}
	return p
}

// set changes the page under the lock of the window.
func (p *page) set(f func()) {
	if app == nil {
		f()
		return
	}
	app.Do(f)
}

// buttons: what the page offers now.
func (p *page) buttons(bs ...pageBtn) { p.set(func() { p.btns = bs }) }

// ask shows the buttons and waits for one; its id.
func (p *page) ask(bs ...pageBtn) string {
	p.drain()
	p.buttons(bs...)
	backToApp()
	id := <-p.acts
	p.buttons()
	return id
}

// drain forgets buttons pressed before the question.
func (p *page) drain() {
	for {
		select {
		case <-p.acts:
		default:
			return
		}
	}
}

// pressed: a button of the page was pressed since the last look, without waiting.
func (p *page) pressed() (string, bool) {
	select {
	case id := <-p.acts:
		return id, true
	default:
		return "", false
	}
}

// wait: the page stays until the operator has read it.
func (p *page) wait() {
	p.ask(pageBtn{id: "ok", label: "Готово", kind: ui.BtnPrimary, key: ui.KeyEnter})
}

func logLine(s string) {
	p := cur
	if p == nil {
		return
	}
	p.set(func() {
		p.log = append(p.log, s)
		if len(p.log) > 400 {
			p.log = p.log[len(p.log)-300:]
		}
	})
}

func sayStop(err error) { logLine("■ ОСТАНОВ: " + err.Error()) }
func sayWarn(s string)  { logLine("▲ ВНИМАНИЕ: " + s) }
func sayBad(s string)   { logLine("■ " + s) }
func sayOK(s string)    { logLine("√ " + s) }
func sayHint(s string)  { logLine(s) }

// ==========================
// ===  Drawing           ===
// ==========================

// lineMark: the sign a line starts with becomes an icon of its colour.
func lineMark(t *ui.Theme, s string) (ui.Icon, color.NRGBA, string) {
	switch {
	case strings.HasPrefix(s, "√ "):
		return ui.IconCheck, t.Green, s[len("√ "):]
	case strings.HasPrefix(s, "▲ "):
		return ui.IconWarn, t.Yellow, s[len("▲ "):]
	case strings.HasPrefix(s, "■ "):
		return ui.IconStop, t.Red, s[len("■ "):]
	case strings.HasPrefix(s, "► "):
		return ui.IconPlay, t.Blue, s[len("► "):]
	case strings.HasPrefix(s, "♦ "):
		return ui.IconZero, t.Mauve, s[len("♦ "):]
	}
	return ui.IconNone, t.Text, s
}

// lines draws text lines with their marks from y down in r; returns the new y.
func drawLines(c *ui.Ctx, r image.Rectangle, y int, ls []string, st ui.Style) int {
	t := c.T
	lh := c.LineH(st)
	for _, l := range ls {
		if l == "" {
			y += lh / 2
			continue
		}
		ic, col, rest := lineMark(t, l)
		x := r.Min.X
		w := r.Dx()
		if ic != ui.IconNone {
			c.Icon(ic, image.Rect(x, y+(lh-t.Si(16))/2, x+t.Si(16), y+(lh+t.Si(16))/2), col)
			x += t.Si(24)
			w -= t.Si(24)
		}
		s := st
		if ic == ui.IconWarn || ic == ui.IconStop {
			s.Color = gfx.Mix(col, t.Text, 0.35)
		}
		for _, wl := range c.Wrap(rest, w, s) {
			if y+lh > r.Max.Y {
				return y
			}
			c.Label(x, y, wl, s)
			y += lh
		}
	}
	return y
}

func (p *page) Draw(c *ui.Ctx) {
	t := c.T
	body := chrome(c, p.title)
	body = ui.Inset(body, t.Si(16))
	var left, right image.Rectangle
	if p.scene != nil && body.Dx() > t.Si(700) {
		lw := body.Dx() * 2 / 5
		if lw < t.Si(340) {
			lw = t.Si(340)
		}
		if lw > t.Si(520) {
			lw = t.Si(520)
		}
		left = ui.CutLeft(&body, lw)
		ui.CutLeft(&body, t.Si(16))
		right = body
	} else if p.scene != nil {
		sh := body.Dy() * 11 / 20
		if body.Dy()-sh < p.need { // the panel wants more: the view gives way down to a quarter
			sh = body.Dy() - p.need
			if sh < body.Dy()/4 {
				sh = body.Dy() / 4
			}
		}
		right = ui.CutTop(&body, sh)
		ui.CutTop(&body, t.Si(12))
		left = body
	} else {
		w := body.Dx()
		if w > t.Si(860) {
			w = t.Si(860)
		}
		left = image.Rect(body.Min.X+(body.Dx()-w)/2, body.Min.Y, body.Min.X+(body.Dx()+w)/2, body.Max.Y)
	}
	if p.scene != nil {
		c.Card(right)
		c.View3D("page/view", ui.Inset(right, t.Si(1)), p.scene)
	}
	c.Card(left)
	in := ui.Inset(left, t.Si(20))
	// the buttons at the bottom
	if len(p.btns) > 0 {
		foot := ui.CutBottom(&in, t.Si(40))
		x := foot.Max.X
		for i := len(p.btns) - 1; i >= 0; i-- {
			b := p.btns[i]
			bw := c.Measure(b.label, c.Body()) + t.Si(40)
			x -= bw
			br := image.Rect(x, foot.Min.Y, x+bw, foot.Max.Y)
			if c.ButtonX("page/"+b.id, br, b.label, b.kind, true, b.key == ui.KeyEnter) || (b.key != ui.KeyNone && c.Pressed(b.key)) {
				p.send(b.id)
			}
			x -= t.Si(10)
		}
		ui.CutBottom(&in, t.Si(14))
	}
	if e, ok := c.Key(); ok && faultChar(e) {
		c.Use()
	}
	if p.onKey != nil {
		if e, ok := c.Key(); ok && p.onKey(e) {
			c.Use()
		}
	}
	y := in.Min.Y
	if p.head != "" {
		c.LabelIn(image.Rect(in.Min.X, y, in.Max.X, y+t.Si(28)), p.head, ui.Style{Kind: ui.Bold, Size: ui.SizeTitle, Color: t.Text}, 0)
		y += t.Si(38)
	}
	y = drawLines(c, image.Rect(in.Min.X, y, in.Max.X, in.Max.Y), y, p.info, c.Body())
	if p.progress >= 0 {
		y += t.Si(10)
		c.Progress(image.Rect(in.Min.X, y, in.Max.X, y+t.Si(8)), p.progress, t.Blue)
		y += t.Si(16)
		if p.progText != "" {
			c.Label(in.Min.X, y, p.progText, c.Muted())
			y += c.LineH(c.Muted())
		}
	}
	if p.extra != nil {
		y += t.Si(12)
		y += p.extra(c, image.Rect(in.Min.X, y, in.Max.X, in.Max.Y))
	}
	p.need = y - left.Min.Y + left.Max.Y - in.Max.Y + t.Si(20)
	if p.big != nil {
		y += t.Si(12)
		p.big(c, image.Rect(in.Min.X, y, in.Max.X, in.Max.Y))
		return
	}
	if p.nolog || len(p.log) == 0 {
		return
	}
	y += t.Si(14)
	if in.Max.Y-y < t.Si(60) {
		return
	}
	jr := image.Rect(in.Min.X, y, in.Max.X, in.Max.Y)
	c.Panel(jr, t.Mantle)
	jin := ui.InsetXY(jr, t.Si(12), t.Si(10))
	st := ui.Style{Kind: ui.Regular, Size: ui.SizeSmall + 1, Color: t.Subtext1}
	lh := c.LineH(st)
	var rows []string
	for _, l := range p.log {
		rows = append(rows, l)
	}
	// wrapped height first, to scroll to the end
	total := 0
	for _, l := range rows {
		_, _, rest := lineMark(t, l)
		total += len(c.Wrap(rest, jin.Dx()-t.Si(24), st)) * lh
	}
	off := c.Scroll("page/log", jin, total, true)
	c.WithClip(jin, func() {
		drawLines(c, image.Rect(jin.Min.X, jin.Min.Y-off, jin.Max.X-t.Si(8), jin.Min.Y-off+total+lh), jin.Min.Y-off, rows, st)
	})
}

func (p *page) send(id string) {
	select {
	case p.acts <- id:
	default:
	}
}
