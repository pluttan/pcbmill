package ui

// One pass of drawing: the canvas, the theme, and the input of the pass.
// Widgets are drawn and asked "was I clicked" in the same call, the usual
// immediate-mode way: no widget objects to keep in sync with the data.

import (
	"image"
	"image/color"
	"math"
	"time"

	"pcbscan/gfx"
)

type input struct {
	mx, my  int
	inside  bool
	focused bool
	ev      Event // the discrete event of this pass, Kind EvNone in the shown pass
	down    [3]bool
}

// state lives across frames: which widget is pressed, has the keys, scroll
// offsets and 3D cameras by widget id.
type state struct {
	hot, active string
	focus       string
	scroll      map[string]float64
	views       map[string]*viewState
	drag        image.Point // where the press of the active widget began
	clickAt     time.Time
	clickID     string
	dbl         bool
}

func (s *state) init() {
	s.scroll = map[string]float64{}
	s.views = map[string]*viewState{}
}

func (s *state) beginFrame() { s.hot = "" }

type Ctx struct {
	App  *App
	C    *gfx.Canvas
	T    *Theme
	W, H int

	in       *input
	st       *state
	live     bool // this layer gets the input
	used     bool // the key of the pass was taken
	texts    []TextAt
	cursor   Cursor
	animate  bool
	wakeAt   time.Time // one more frame then, without a frame in between
	tooltips []string
}

// ==========================
// ===  Input             ===
// ==========================

// Mouse: the pointer in client pixels.
func (c *Ctx) Mouse() (int, int) { return c.in.mx, c.in.my }

// Over: the pointer is over r and this layer gets the input.
func (c *Ctx) Over(r image.Rectangle) bool {
	return c.live && c.in.inside && image.Pt(c.in.mx, c.in.my).In(r)
}

// Key: the key of this pass, once, for the top layer.
func (c *Ctx) Key() (Event, bool) {
	e := c.in.ev
	if !c.live || c.used || (e.Kind != EvKey && e.Kind != EvChar) {
		return Event{}, false
	}
	return e, true
}

// Use marks the key of the pass as handled.
func (c *Ctx) Use() { c.used = true }

// Pressed: the key k went down in this pass (and is taken).
func (c *Ctx) Pressed(k Key) bool {
	e, ok := c.Key()
	if ok && e.Kind == EvKey && e.Key == k {
		c.used = true
		return true
	}
	return false
}

// Wheel: notches turned over r in this pass.
func (c *Ctx) Wheel(r image.Rectangle) float64 {
	e := c.in.ev
	if e.Kind == EvWheel && c.live && image.Pt(e.X, e.Y).In(r) {
		return e.Wheel
	}
	return 0
}

// Animate asks for frames by themselves for a while (blinking, progress).
func (c *Ctx) Animate() { c.animate = true }

// WakeAt asks for one frame at t: a change that comes at a known time
// (a caret blinking) costs one frame, not twenty a second.
func (c *Ctx) WakeAt(t time.Time) {
	if c.wakeAt.IsZero() || t.Before(c.wakeAt) {
		c.wakeAt = t
	}
}

// SetCursor: the pointer for this frame.
func (c *Ctx) SetCursor(k Cursor) { c.cursor = k }

// Click handles a clickable area: hot under the mouse, active while
// pressed, true when released over it.
func (c *Ctx) Click(id string, r image.Rectangle) bool {
	e := c.in.ev
	over := c.Over(r)
	if over {
		c.st.hot = id
		c.cursor = CursorHand
	}
	switch {
	case e.Kind == EvMouseDown && e.Button == BtnLeft && over:
		c.st.active = id
		c.st.drag = image.Pt(e.X, e.Y)
	case e.Kind == EvMouseUp && e.Button == BtnLeft && c.st.active == id:
		c.st.active = ""
		if over {
			now := time.Now()
			c.st.dbl = c.st.clickID == id && now.Sub(c.st.clickAt) < 400*time.Millisecond
			c.st.clickAt, c.st.clickID = now, id
			return true
		}
	}
	return false
}

// Double: the click just reported was the second of a double click.
func (c *Ctx) Double() bool { return c.st.dbl }

// Hot / Active: for the look of a widget.
func (c *Ctx) Hot(id string) bool    { return c.st.hot == id }
func (c *Ctx) Active(id string) bool { return c.st.active == id }

// Focus: the widget that gets the typed text.
func (c *Ctx) Focus() string          { return c.st.focus }
func (c *Ctx) SetFocus(id string)     { c.st.focus = id }
func (c *Ctx) Focused(id string) bool { return c.st.focus == id }

func (c *Ctx) finish() {
	if c.in.ev.Kind == EvMouseUp {
		c.st.active = ""
	}
}

// ==========================
// ===  Text              ===
// ==========================

type Style struct {
	Kind  FontKind
	Size  float64
	Color color.NRGBA
}

func (c *Ctx) face(s Style) *gfx.Face {
	if s.Size == 0 {
		s.Size = SizeBody
	}
	return c.T.Face(s.Kind, s.Size)
}

// Body, Muted, Head, MonoS: the styles used everywhere.
func (c *Ctx) Body() Style  { return Style{Regular, SizeBody, c.T.Text} }
func (c *Ctx) Muted() Style { return Style{Regular, SizeBody, c.T.Subtext0} }
func (c *Ctx) Small() Style { return Style{Regular, SizeSmall, c.T.Overlay2} }
func (c *Ctx) Head() Style  { return Style{Bold, SizeLarge, c.T.Text} }
func (c *Ctx) MonoS() Style { return Style{Mono, SizeBody, c.T.Text} }

// Label draws s with its top at y; returns the width.
func (c *Ctx) Label(x, y int, s string, st Style) int {
	f := c.face(st)
	end := c.C.Text(f, float64(x), float64(y)+f.Ascent, s, st.Color)
	c.texts = append(c.texts, TextAt{s, image.Rect(x, y, int(end+0.5), y+int(f.LineHeight+0.5)), c.live})
	return int(end+0.5) - x
}

// LabelIn draws s in r, cut with an ellipsis; align 0 left, 1 centre, 2 right;
// vertically centred.
func (c *Ctx) LabelIn(r image.Rectangle, s string, st Style, align int) {
	f := c.face(st)
	s = f.Fit(s, float64(r.Dx()))
	w := f.Measure(s)
	x := float64(r.Min.X)
	switch align {
	case 1:
		x += (float64(r.Dx()) - w) / 2
	case 2:
		x += float64(r.Dx()) - w
	}
	y := float64(r.Min.Y) + (float64(r.Dy())-(f.Ascent+f.Descent))/2 + f.Ascent
	c.texts = append(c.texts, TextAt{s, image.Rect(int(x), r.Min.Y, int(x+w), r.Max.Y), c.live})
	c.C.Text(f, x, y, s, st.Color)
}

// TextAt: a string drawn in the last frame and where, for tests.
type TextAt struct {
	S    string
	R    image.Rectangle
	Live bool // on the top layer (clickable)
}

// Measure: the width of s in the style.
func (c *Ctx) Measure(s string, st Style) int { return int(math.Ceil(c.face(st).Measure(s))) }

// LineH: the line height of the style.
func (c *Ctx) LineH(st Style) int { return int(c.face(st).LineHeight + 0.5) }

// Wrap breaks s into lines no wider than w.
func (c *Ctx) Wrap(s string, w int, st Style) []string {
	f := c.face(st)
	var out []string
	for _, para := range splitLines(s) {
		words := splitWords(para)
		line := ""
		for _, wd := range words {
			try := wd
			if line != "" {
				try = line + " " + wd
			}
			if line != "" && f.Measure(try) > float64(w) {
				out = append(out, line)
				line = wd
				continue
			}
			line = try
		}
		out = append(out, line)
	}
	return out
}

// Para draws wrapped text from the top of r; returns the height used.
func (c *Ctx) Para(r image.Rectangle, s string, st Style) int {
	lh := c.LineH(st)
	y := r.Min.Y
	for _, l := range c.Wrap(s, r.Dx(), st) {
		if y+lh > r.Max.Y+lh/2 {
			break
		}
		c.Label(r.Min.X, y, l, st)
		y += lh
	}
	return y - r.Min.Y
}

// ParaH: the height Para would take.
func (c *Ctx) ParaH(w int, s string, st Style) int { return len(c.Wrap(s, w, st)) * c.LineH(st) }

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func splitWords(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		if r == ' ' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}
