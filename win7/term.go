package main

// The full screen: every screen is a function from its state to the lines of
// one frame, and this layer puts the frame on the console in place, without
// scrolling. Only the lines that changed since the last frame are written.

import (
	"os"
	"os/signal"
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

// keyHook sees every key of the console before the screens do; true: the key
// was used up (the fault keys of the emulation work on any screen, in a scan too).
var keyHook func(Key) bool

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
	raw := make(chan Key, 64)
	conStart(raw)
	go func() {
		for k := range raw {
			if keyHook != nil && keyHook(k) {
				select { // an empty key wakes the screen up to show the change
				case t.keys <- Key{}:
				default:
				}
				continue
			}
			t.keys <- k
		}
	}()
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

// The whole program sits in one frame the size of the window: the program and
// the screen in its top edge, the keys in its bottom edge, the status lines
// at the top inside, the body centred in the rest. Rounded corners where the
// font has them (outer, set per platform).

// frameTop is the top edge: "pcbscan · screen" on the left, ЭМУЛЯЦИЯ on the right.
func frameTop(title string, w int) string {
	f := outer
	left, right := topParts(title)
	n := w - 4 - vlen(right)
	if n < 0 {
		n, right = w-4, ""
	}
	if vlen(left) > n {
		left, _ = cutv(left, n)
		left += plain
	}
	return paint(cFrame, f.tl+f.h) + left + paint(cFrame, repeat(f.h, n-vlen(left))) + right + paint(cFrame, f.h+f.tr)
}

// topWidth is how wide the frame must be for its top edge to show it all.
func topWidth(title string) int {
	left, right := topParts(title)
	return vlen(left) + vlen(right) + 6
}

// topParts: the program and the screen on the left; the state of the МК and
// NcStudio and the emulation mark on the right.
func topParts(title string) (string, string) {
	left := paint(cLightCyan, " pcbscan ")
	if title != "" {
		left += paint(cHint, "· ") + paint(cTitle, title+" ")
	}
	right := ""
	if statusFn != nil {
		right = " " + edgeStatus(statusFn()) + " "
	}
	if simM != nil {
		if right == "" {
			right = " "
		} else {
			right += "  "
		}
		right += paint(cYellow, "ЭМУЛЯЦИЯ") + " "
	}
	if right != "" {
		right = paint(cFrame, outer.h) + right
	}
	return left, right
}

// frameBottom is the bottom edge with the keys of the screen set into it.
func frameBottom(hint string, w int) string { return bottomEdge(cFrame, hint, w) }

// bottomEdge is a bottom edge in colour fc with the keys set into it.
func bottomEdge(fc int, hint string, w int) string {
	f := outer
	if vlen(hint) > w-6 {
		hint, _ = cutv(hint, w-6)
		hint += plain
	}
	if hint != "" {
		hint = " " + hint + " "
	}
	return paint(fc, f.bl+f.h) + hint + paint(fc, repeat(f.h, w-3-vlen(hint))+f.br)
}

// frameMin is the narrowest the frame gets, so screens do not jump in width
// from one to the next more than their content needs.
const frameMin = 64

// screen lays a frame out: the frame as big as its content (the status lines,
// the body, an empty line of air at the top and the bottom), in the middle of
// the window. A body too tall loses its last lines, one too wide is cut: the
// frame never exceeds w x h.
func screen(w, h int, title string, body []string, hint string) []string {
	if w < 12 || h < 4 { // no room for a frame: the bare body
		return fit(body, w, h)
	}
	room := h - 2 - 2 // the edges and the air lines
	if len(body) > room {
		body = body[:room]
	}
	// the frame: wide enough for the widest line with a margin, the title with
	// the state in the top edge, and the keys in the bottom one
	fw := blockWidth(body) + 6
	if n := vlen(hint) + 8; n > fw {
		fw = n
	}
	if n := topWidth(title); n > fw {
		fw = n
	}
	if fw < frameMin {
		fw = frameMin
	}
	if fw > w {
		fw = w
	}
	iw := fw - 2
	bw := blockWidth(body)
	if bw > iw {
		bw = iw
	}
	left := spaces((iw - bw) / 2)
	in := []string{""}
	for _, l := range body {
		in = append(in, left+l)
	}
	if len(in)+2 < h {
		in = append(in, "")
	}
	side := paint(cFrame, outer.v)
	box := []string{frameTop(title, fw)}
	for _, l := range in {
		if vlen(l) > iw {
			l, _ = cutv(l, iw)
		}
		box = append(box, side+l+plain+spaces(iw-vlen(l))+side)
	}
	box = append(box, frameBottom(hint, fw))
	// in the middle of the window
	x, y := (w-fw)/2, (h-len(box))/2
	out := make([]string, 0, h)
	for i := 0; i < y; i++ {
		out = append(out, "")
	}
	for _, l := range box {
		out = append(out, spaces(x)+l)
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

// hints are the keys of a screen for the bottom edge: the key white, what it
// does grey, "·" between; the last pairs go when the edge is too short. A pair
// with no key is a note.
func hints(w int, pairs ...string) string {
	for n := len(pairs) / 2; n > 0; n-- {
		s := ""
		for i := 0; i < n; i++ {
			if i > 0 {
				s += "   "
			}
			if pairs[2*i] != "" {
				s += paint(cTitle, pairs[2*i]) + " "
			}
			s += paint(cHint, pairs[2*i+1])
		}
		if vlen(s) <= w-8 {
			return s
		}
	}
	return ""
}

// stack joins blocks top to bottom, each centred on the widest, with an empty
// line between them.
func stack(blocks ...[]string) []string { return stackGap(1, blocks...) }

// stackGap is stack with gap empty lines between the blocks.
func stackGap(gap int, blocks ...[]string) []string {
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
		for k := 0; i > 0 && len(out) > 0 && k < gap; k++ {
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

// putAt draws s over line l from visible column x on; the colour after it is
// what l had there.
func putAt(l string, x int, s string) string {
	head, rest := cutv(l, x)
	_, tail := cutv(rest, vlen(s))
	return head + plain + s + plain + inkAt(l, x+vlen(s)) + tail
}

// overlay draws a dialog over the middle of a frame: the screen it was opened
// from stays around it, dimmed, and the dialog drops a shadow on it, two
// cells to the right and one down (a cell is twice as high as it is wide).
func overlay(base, box []string, w, h int) []string {
	out := fit(base, w, h)
	for r := range out { // the screen under the dialog dimmed, frame and all
		out[r] = paint(cDark, strip(out[r]))
	}
	bw := blockWidth(box)
	x, y := (w-bw-2)/2, (h-len(box)-1)/2
	if x < 0 {
		x = 0
	}
	if y < 1 {
		y = 1
	}
	shade := paint(cDark, "░░")
	for i, l := range box {
		r := y + i
		if r >= h {
			break
		}
		out[r] = putAt(out[r], x, padRight(l, bw))
		if i > 0 && x+bw+2 <= w {
			out[r] = putAt(out[r], x+bw, shade)
		}
	}
	if r := y + len(box); r < h && x+bw+2 <= w {
		out[r] = putAt(out[r], x+2, paint(cDark, repeat("░", bw)))
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
		if !ok || k.Code == KeyResize || k.Code == KeyNone {
			continue
		}
		if handle(k) {
			return
		}
	}
}
