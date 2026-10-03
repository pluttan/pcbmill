package main

// The full screen: every screen is a function from its state to the lines of
// one frame, and this layer puts the frame on the console in place, without
// scrolling. Only the lines that changed since the last frame are written.

import (
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Term is the console as the screens see it. The real one is conTerm; tests
// use a fake that records frames and plays back keys.
type Term interface {
	Size() (w, h int)                   // usable columns and rows
	Draw(frame []string)                // lines no wider than w, at most h of them
	Key(wait time.Duration) (Key, bool) // wait < 0: until a key comes; false: none in time
	Flush()                             // drop keys pressed so far
}

var term Term = &conTerm{}

// ==========================
// ===  The real console  ===
// ==========================

type conTerm struct {
	mu      sync.Mutex
	keys    chan Key
	last    []string
	lw, lh  int
	started bool
}

func (t *conTerm) start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return
	}
	t.started = true
	t.keys = make(chan Key, 64)
	conStart(t.keys)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		t.stop()
		os.Exit(130)
	}()
}

// stop gives the console back as it was: cursor, colours, terminal mode.
func (t *conTerm) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started {
		return
	}
	t.started = false
	conStop()
}

// Size: one column less than the window, a line as wide as the console moves
// the cursor to the next line and the last one would scroll the screen.
func (t *conTerm) Size() (int, int) {
	w, h := conSize()
	if w < 20 {
		w = 20
	}
	if h < 8 {
		h = 8
	}
	return w - 1, h
}

func (t *conTerm) Draw(frame []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started {
		return
	}
	w, h := t.Size()
	frame = fit(frame, w, h)
	full := w != t.lw || h != t.lh || len(t.last) != len(frame)
	if full {
		conClear()
	}
	for i, l := range frame {
		if full || l != t.last[i] {
			conLine(i, l)
		}
	}
	conDone()
	t.last, t.lw, t.lh = frame, w, h
}

func (t *conTerm) Key(wait time.Duration) (Key, bool) {
	var tm <-chan time.Time
	switch {
	case wait == 0:
		select {
		case k := <-t.keys:
			return k, true
		default:
			return Key{}, false
		}
	case wait > 0:
		tm = time.After(wait)
	}
	select {
	case k := <-t.keys:
		if k.Code == KeyResize {
			t.mu.Lock()
			t.last = nil
			t.mu.Unlock()
		}
		return k, true
	case <-tm:
		return Key{}, false
	}
}

func (t *conTerm) Flush() {
	conFlushInput()
	for {
		select {
		case <-t.keys:
		default:
			return
		}
	}
}

// fit makes the frame exactly h lines of exactly w columns: longer lines are
// cut, and every line ends in the plain colour so nothing bleeds into the next.
func fit(frame []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		l := ""
		if i < len(frame) {
			l = frame[i]
		}
		if vlen(l) > w {
			l, _ = cutv(l, w)
		}
		out[i] = l + plain + spaces(w-vlen(l))
	}
	return out
}

// ==========================
// ===  Screen layout     ===
// ==========================

// statusFn gives the status line of every screen; nil in tests.
var statusFn func() Status

func blockWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		if n := vlen(l); n > w {
			w = n
		}
	}
	return w
}

// screen lays a frame out: the status line on top, the key hints at the
// bottom, the body centred both ways in between. A body too tall loses its
// last lines, one too wide is cut: the frame never exceeds w x h.
func screen(w, h int, body []string, hint string) []string {
	var top []string
	if statusFn != nil {
		for _, l := range statusLine(statusFn(), w) {
			top = append(top, center(l, w))
		}
	}
	bottom := []string{}
	if hint != "" {
		bottom = append(bottom, center(hint, w))
	}
	room := h - len(top) - len(bottom)
	if room < 1 {
		top, room = nil, h-len(bottom)
	}
	if len(body) > room {
		body = body[:room]
	}
	bw := blockWidth(body)
	if bw > w {
		bw = w
	}
	left := spaces((w - bw) / 2)
	out := append([]string(nil), top...)
	pad := (room - len(body)) / 2
	for i := 0; i < pad; i++ {
		out = append(out, "")
	}
	for _, l := range body {
		if vlen(l) > w {
			l, _ = cutv(l, w)
			l += plain
		}
		out = append(out, left+l)
	}
	for len(out) < h-len(bottom) {
		out = append(out, "")
	}
	return append(out, bottom...)
}

// hints: "key what" pairs joined in one line; the last pairs go when too long.
func hints(w int, pairs ...string) string {
	for n := len(pairs) / 2; n > 0; n-- {
		var parts []string
		for i := 0; i < n; i++ {
			parts = append(parts, paint(cKey, pairs[2*i])+" "+paint(cHint, pairs[2*i+1]))
		}
		s := strings.Join(parts, "   ")
		if vlen(s) <= w {
			return s
		}
	}
	return ""
}

// stack joins blocks top to bottom, each centred on the widest, with an empty
// line between them.
func stack(blocks ...[]string) []string {
	var all []string
	for _, b := range blocks {
		all = append(all, b...)
	}
	w := blockWidth(all)
	var out []string
	for i, b := range blocks {
		if len(b) == 0 {
			continue
		}
		if i > 0 && len(out) > 0 {
			out = append(out, "")
		}
		bw := blockWidth(b)
		for _, l := range b {
			out = append(out, spaces((w-bw)/2)+l)
		}
	}
	return out
}

// beside puts two blocks next to each other with gap columns between them.
func beside(a, b []string, gap int) []string {
	aw := blockWidth(a)
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]string, n)
	for i := range out {
		l := ""
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			l = padRight(l, aw+gap) + b[i]
		}
		out[i] = l
	}
	return out
}

// inkAt is the colour in force at visible column n of s, as marks.
func inkAt(s string, n int) string {
	fg, bgm := "", ""
	k := 0
	for i := 0; i < len(s) && k < n; {
		if s[i] == esc && i+1 < len(s) {
			switch m := s[i+1]; {
			case m == '0':
				fg, bgm = "", ""
			case m >= 'a':
				bgm = s[i : i+2]
			default:
				fg = s[i : i+2]
			}
			i += 2
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		k++
	}
	return fg + bgm
}

// overlay draws a box over the middle of a frame: dialogs keep the screen
// they were opened from visible around them.
func overlay(base, box []string, w, h int) []string {
	out := fit(base, w, h)
	bw := blockWidth(box)
	x, y := (w-bw)/2, (h-len(box))/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	for i, l := range box {
		r := y + i
		if r >= h {
			break
		}
		l = padRight(l, bw)
		head, rest := cutv(out[r], x)
		_, tail := cutv(rest, bw)
		out[r] = head + plain + l + plain + inkAt(out[r], x+bw) + tail
	}
	return out
}

// ==========================
// ===  Running a screen  ===
// ==========================

// backdrop is the screen under a dialog: the last screen run or page opened.
var backdrop func(w, h int) []string

func draw(view func(w, h int) []string) {
	w, h := term.Size()
	term.Draw(view(w, h))
}

// run shows view and hands every key to handle until handle says done. With
// tick > 0 the screen is redrawn that often while no key comes (status line).
// The focus comes back from NcStudio first: the operator answers without a click.
func run(view func(w, h int) []string, handle func(Key) bool, tick time.Duration) {
	backToConsole()
	backdrop = view
	wait := time.Duration(-1)
	if tick > 0 {
		wait = tick
	}
	for {
		draw(view)
		k, ok := term.Key(wait)
		if !ok || k.Code == KeyResize {
			continue
		}
		if handle(k) {
			return
		}
	}
}
