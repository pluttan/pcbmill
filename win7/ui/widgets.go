package ui

// The widgets: cards, buttons, list rows, chips, a progress bar, a text
// field and a scrolled area. Each is one call that draws and reports.

import (
	"image"
	"image/color"
	"math"
	"time"
	"unicode/utf8"

	"pcbscan/gfx"
)

// ==========================
// ===  Surfaces          ===
// ==========================

// Card: a raised panel with a soft shadow.
func (c *Ctx) Card(r image.Rectangle) {
	t := c.T
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	c.C.Shadow(x, y+t.S(2), w, h, t.S(10), t.S(14), gfx.Alpha(t.Crust, 0.55))
	c.C.FillRoundRect(x, y, w, h, t.S(10), t.Base)
	c.C.StrokeRoundRect(x+0.5, y+0.5, w-1, h-1, t.S(10), 1, gfx.Alpha(t.Surface1, 0.6))
}

// Panel: a flat area of another shade, rounded.
func (c *Ctx) Panel(r image.Rectangle, col color.NRGBA) {
	c.C.FillRoundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), c.T.S(8), col)
}

// HLine: a thin separator.
func (c *Ctx) HLine(x0, x1, y int) {
	c.C.FillRect(image.Rect(x0, y, x1, y+imax(1, c.T.Si(1))), gfx.Alpha(c.T.Surface1, 0.7))
}

// WithClip draws f limited to r.
func (c *Ctx) WithClip(r image.Rectangle, f func()) {
	old := c.C.Clip
	c.C.Clip = old.Intersect(r)
	f()
	c.C.Clip = old
}

// ==========================
// ===  Buttons           ===
// ==========================

type BtnKind int

const (
	BtnNormal BtnKind = iota
	BtnPrimary
	BtnDanger
	BtnGhost
	BtnSuccess
)

// Button: true when clicked. A disabled button is drawn but does nothing.
func (c *Ctx) Button(id string, r image.Rectangle, label string, k BtnKind) bool {
	return c.ButtonX(id, r, label, k, true, false)
}

// ButtonX: with enabled and a focus ring (the button Enter presses).
func (c *Ctx) ButtonX(id string, r image.Rectangle, label string, k BtnKind, enabled, def bool) bool {
	t := c.T
	clicked := false
	if enabled {
		clicked = c.Click(id, r)
	}
	bg, fg := t.Surface0, t.Text
	switch k {
	case BtnPrimary:
		bg, fg = t.Blue, t.Crust
	case BtnDanger:
		bg, fg = t.Red, t.Crust
	case BtnSuccess:
		bg, fg = t.Green, t.Crust
	case BtnGhost:
		bg = gfx.Alpha(t.Surface0, 0)
	}
	if !enabled {
		bg, fg = gfx.Alpha(t.Surface0, 0.5), t.Overlay0
	} else if c.Active(id) && c.Hot(id) {
		bg = gfx.Mix(bg, t.Crust, 0.18)
		if k == BtnGhost {
			bg = t.Surface1
		}
	} else if c.Hot(id) {
		bg = gfx.Mix(bg, t.Text, 0.12)
		if k == BtnGhost {
			bg = t.Surface0
		}
	}
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	rad := t.S(7)
	if def && enabled {
		c.C.StrokeRoundRect(x-t.S(2.5), y-t.S(2.5), w+t.S(5), h+t.S(5), rad+t.S(2.5), t.S(1.5), gfx.Alpha(t.Lavender, 0.85))
	}
	c.C.FillRoundRect(x, y, w, h, rad, bg)
	if k == BtnNormal && enabled {
		c.C.StrokeRoundRect(x+0.5, y+0.5, w-1, h-1, rad, 1, gfx.Alpha(t.Surface2, 0.6))
	}
	st := Style{Bold, SizeBody, fg}
	if k == BtnNormal || k == BtnGhost {
		st.Kind = Regular
	}
	c.LabelIn(r, label, st, 1)
	return clicked
}

// IconButton: a square button with an icon, for tool bars.
func (c *Ctx) IconButton(id string, r image.Rectangle, ic Icon, on bool) bool {
	t := c.T
	clicked := c.Click(id, r)
	bg := gfx.Alpha(t.Surface0, 0.0)
	fg := t.Subtext0
	if on {
		bg, fg = gfx.Alpha(t.Lavender, 0.22), t.Lavender
	}
	if c.Hot(id) {
		bg = gfx.Mix(t.Surface0, t.Text, 0.06)
		fg = t.Text
	}
	c.C.FillRoundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), t.S(6), bg)
	d := imin(r.Dx(), r.Dy()) * 5 / 9
	c.Icon(ic, Center(r, d, d), fg)
	return clicked
}

// ==========================
// ===  Rows, chips, bars ===
// ==========================

// Row: a list row; true when clicked. Selected rows have a band and an
// accent bar at the left.
func (c *Ctx) Row(id string, r image.Rectangle, selected bool) bool {
	t := c.T
	clicked := c.Click(id, r)
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	switch {
	case selected:
		c.C.FillRoundRect(x, y, w, h, t.S(7), t.Surface0)
		c.C.FillRoundRect(x+t.S(3), y+h*0.22, t.S(3), h*0.56, t.S(1.5), t.Lavender)
	case c.Hot(id):
		c.C.FillRoundRect(x, y, w, h, t.S(7), gfx.Alpha(t.Surface0, 0.55))
	}
	return clicked
}

// Chip: a small rounded label with a coloured dot; returns its width.
func (c *Ctx) Chip(x, y int, s string, dot color.NRGBA, bg color.NRGBA) int {
	t := c.T
	st := Style{Regular, SizeSmall, t.Subtext1}
	h := t.Si(22)
	w := c.Measure(s, st) + t.Si(28)
	c.C.FillRoundRect(float64(x), float64(y), float64(w), float64(h), float64(h)/2, bg)
	c.C.FillCircle(float64(x)+t.S(11), float64(y)+float64(h)/2, t.S(3.5), dot)
	c.LabelIn(image.Rect(x+t.Si(19), y, x+w, y+h), s, st, 0)
	return w
}

// Progress: a rounded bar filled to frac.
func (c *Ctx) Progress(r image.Rectangle, frac float64, col color.NRGBA) {
	t := c.T
	if frac < 0 || math.IsNaN(frac) {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	c.C.FillRoundRect(x, y, w, h, h/2, t.Surface0)
	if frac > 0 {
		c.C.FillRoundRect(x, y, math.Max(h, w*frac), h, h/2, col)
	}
}

// ==========================
// ===  Text field        ===
// ==========================

// Field is the text of an input box and the caret in it.
type Field struct {
	Text []rune
	Pos  int
	All  bool // the whole text is selected: typing replaces it
}

func NewField(s string) *Field { r := []rune(s); return &Field{Text: r, Pos: len(r), All: true} }

func (f *Field) String() string { return string(f.Text) }

func (f *Field) Set(s string) { f.Text = []rune(s); f.Pos = len(f.Text); f.All = true }

// Key edits the field; enter / cancel for Enter and Esc.
func (f *Field) Key(e Event) (used, enter, cancel bool) {
	if e.Kind == EvChar {
		if f.All {
			f.Text, f.Pos, f.All = nil, 0, false
		}
		if len(f.Text) < 64 {
			f.Text = append(f.Text[:f.Pos], append([]rune{e.Rune}, f.Text[f.Pos:]...)...)
			f.Pos++
		}
		return true, false, false
	}
	if e.Kind != EvKey {
		return false, false, false
	}
	all := f.All
	f.All = false
	switch e.Key {
	case KeyLeft:
		if f.Pos > 0 && !all {
			f.Pos--
		}
	case KeyRight:
		if f.Pos < len(f.Text) {
			f.Pos++
		}
	case KeyHome:
		f.Pos = 0
	case KeyEnd:
		f.Pos = len(f.Text)
	case KeyBackspace:
		if all {
			f.Text, f.Pos = nil, 0
		} else if f.Pos > 0 {
			f.Text = append(f.Text[:f.Pos-1], f.Text[f.Pos:]...)
			f.Pos--
		}
	case KeyDelete:
		if all {
			f.Text, f.Pos = nil, 0
		} else if f.Pos < len(f.Text) {
			f.Text = append(f.Text[:f.Pos], f.Text[f.Pos+1:]...)
		}
	case KeyEnter:
		return true, true, false
	case KeyEsc:
		return true, false, true
	default:
		f.All = all
		return false, false, false
	}
	return true, false, false
}

// TextField draws an input box; when it has the focus it takes the typed
// keys. Returns enter / cancel as Field.Key does.
func (c *Ctx) TextField(id string, r image.Rectangle, f *Field, st Style, align int) (enter, cancel bool) {
	t := c.T
	if c.Click(id, r) {
		c.SetFocus(id)
	}
	if c.Hot(id) {
		c.cursor = CursorText
	}
	focused := c.Focused(id)
	if focused {
		if e, ok := c.Key(); ok {
			used, en, ca := f.Key(e)
			if used {
				c.Use()
			}
			enter, cancel = en, ca
		}
	}
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	bg := t.Crust
	c.C.FillRoundRect(x, y, w, h, t.S(6), bg)
	border := gfx.Alpha(t.Surface1, 0.9)
	if focused {
		border = t.Lavender
	} else if c.Hot(id) {
		border = t.Surface2
	}
	c.C.StrokeRoundRect(x+0.5, y+0.5, w-1, h-1, t.S(6), t.S(1.2), border)
	in := InsetXY(r, t.Si(8), 0)
	fc := c.face(st)
	s := string(f.Text)
	tw := fc.Measure(s)
	tx := float64(in.Min.X)
	switch align {
	case 1:
		tx += (float64(in.Dx()) - tw) / 2
	case 2:
		tx += float64(in.Dx()) - tw
	}
	base := y + (h-(fc.Ascent+fc.Descent))/2 + fc.Ascent
	if focused && f.All && len(f.Text) > 0 {
		c.C.FillRoundRect(tx-1, base-fc.Ascent, tw+2, fc.Ascent+fc.Descent, 2, gfx.Alpha(t.Lavender, 0.35))
	}
	c.WithClip(in, func() {
		c.texts = append(c.texts, TextAt{s, in, c.live})
		c.C.Text(fc, tx, base, s, st.Color)
		if focused && !f.All && time.Now().UnixNano()/5e8%2 == 0 {
			cx := tx + fc.Measure(string(f.Text[:clampPos(f.Pos, len(f.Text))]))
			c.C.FillRect(image.Rect(int(cx), int(base-fc.Ascent), int(cx)+imax(1, t.Si(1.5)), int(base+fc.Descent)), t.Rosewater)
		}
	})
	if focused {
		c.animate = true
	}
	return enter, cancel
}

func clampPos(p, n int) int {
	if p < 0 {
		return 0
	}
	if p > n {
		return n
	}
	return p
}

// ==========================
// ===  Scrolled area     ===
// ==========================

// Scroll: the offset of a scrolled area of content height ch shown in r;
// the wheel scrolls it, a thin bar shows where it is. follow keeps it at
// the end while it is there (a log).
func (c *Ctx) Scroll(id string, r image.Rectangle, ch int, follow bool) int {
	t := c.T
	maxOff := float64(imax(0, ch-r.Dy()))
	off, seen := c.st.scroll[id]
	if !seen && follow {
		off = maxOff
	}
	atEnd := off >= maxOff-1
	if d := c.Wheel(r); d != 0 {
		off -= d * t.S(48)
	}
	if follow && atEnd && c.Wheel(r) == 0 {
		off = maxOff
	}
	if off > maxOff {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}
	c.st.scroll[id] = off
	if maxOff > 0 {
		bh := float64(r.Dy()) * float64(r.Dy()) / float64(ch)
		if bh < t.S(24) {
			bh = t.S(24)
		}
		by := float64(r.Min.Y) + (float64(r.Dy())-bh)*off/maxOff
		c.C.FillRoundRect(float64(r.Max.X)-t.S(5), by, t.S(3), bh, t.S(1.5), gfx.Alpha(t.Overlay0, 0.6))
	}
	return int(off)
}

// SetScroll moves a scrolled area (to show a selected row).
func (c *Ctx) SetScroll(id string, off int) { c.st.scroll[id] = float64(off) }

// ScrollOf: the offset of a scrolled area now.
func (c *Ctx) ScrollOf(id string) int { return int(c.st.scroll[id]) }

// runeLen: for fields of fixed width.
func runeLen(s string) int { return utf8.RuneCountInString(s) }
