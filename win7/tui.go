package main

// Text screens: frames, menu, status line, messages. The drawing functions only
// return lines, say prints them. Only characters of the cp866 console font are
// used: the raster font of a Windows 7 console has no other glyphs.

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Console colours, the 16 of SetConsoleTextAttribute.
const (
	cBlack = iota
	cBlue
	cGreen
	cCyan
	cRed
	cMagenta
	cBrown
	cGray
	cDark
	cLightBlue
	cLightGreen
	cLightCyan
	cLightRed
	cLightMagenta
	cYellow
	cWhite
)

// What each colour means on screen. One rule for all screens: plain text in
// the console's own grey, values white, units and hints dark grey, panel
// frames dark grey with white titles, red only for a stop, yellow only for
// contact, green only for "fine".
const (
	cFrame = cWhite        // the outer frame and the dialogs
	cAxis  = cDark         // map axes
	cTitle = cWhite        // titles, values that matter
	cKey   = cLightCyan    // keys named inside a text
	cHint  = cDark         // hints, units
	cOK    = cLightGreen   // done, fine
	cAlarm = cLightRed     // stop, warning
	cTouch = cYellow       // contact
	cZero  = cLightMagenta // the first point, zero of Z
	cCut   = cCyan         // the program cut on the table
)

// A colour mark inside a string is ESC and one byte: 'A'+colour, or '0' for the
// colour the console had at start. say turns marks into console attributes on
// Windows and drops them elsewhere; widths never count them.
const (
	esc   = '\x1b'
	plain = "\x1b0"
)

func ink(c int) string { return string([]byte{esc, byte('A' + c)}) }

func paint(c int, s string) string { return ink(c) + s + plain }

// A background mark is ESC and 'a'+colour; '0' resets it too.
func bgInk(c int) string { return string([]byte{esc, byte('a' + c)}) }

// on draws s in colour fg on background bg; marks inside s are dropped.
func on(fg, bg int, s string) string { return bgInk(bg) + ink(fg) + strip(s) + plain }

// The selected line of a menu, a chosen button: black on a cyan bar. The bars
// at the top and bottom of the screen: black on grey.
const (
	cSelBg = cCyan
	cSelFg = cBlack
	cBarBg = cGray
	cBarFg = cBlack
)

// hilite draws s as selected; colour marks inside s are dropped.
func hilite(s string) string { return on(cSelFg, cSelBg, s) }

// badge is a state in a coloured box: " ОК ", " КОНТАКТ ", " НЕТ СВЯЗИ ".
func badge(fg, bg int, s string) string { return on(fg, bg, " "+s+" ") }

func okBadge(s string) string    { return badge(cBlack, cLightGreen, s) }
func touchBadge(s string) string { return badge(cBlack, cYellow, s) }
func alarmBadge(s string) string { return badge(cWhite, cRed, s) }
func offBadge(s string) string   { return badge(cWhite, cDark, s) }

// ansiOf turns the colour marks into terminal escape codes.
func ansiOf(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != esc || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		m := s[i]
		if m == '0' { // the plain colour is white
			b.WriteString("\x1b[0;97m")
			continue
		}
		base, c := 30, int(m-'A')
		if m >= 'a' {
			base, c = 40, int(m-'a')
		}
		if c < 0 || c > 15 {
			continue
		}
		// console order is blue-green-red, ANSI is red-green-blue
		n := [8]int{0, 4, 2, 6, 1, 5, 3, 7}[c&7] + base
		if c >= 8 {
			n += 60
		}
		fmt.Fprintf(&b, "\x1b[%dm", n)
	}
	return b.String()
}

// vlen is the width on screen: runes without the colour marks.
func vlen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == esc {
			i += 2
			continue
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		n++
	}
	return n
}

// strip drops the colour marks.
func strip(s string) string {
	if strings.IndexByte(s, esc) < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == esc {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

func padRight(s string, w int) string { return s + spaces(w-vlen(s)) }

func center(s string, w int) string {
	l := (w - vlen(s)) / 2
	return padRight(spaces(l)+s, w)
}

// cutv splits s after n visible runes, colour marks kept with the head.
func cutv(s string, n int) (string, string) {
	k := 0
	for i := 0; i < len(s); {
		if s[i] == esc {
			i += 2
			continue
		}
		if k == n {
			return s[:i], s[i:]
		}
		_, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		k++
	}
	return s, ""
}

// wrap breaks s at spaces into lines of at most w; the lines after the first get
// hang spaces in front. A word longer than a line is cut.
func wrap(s string, w, hang int) []string {
	if vlen(s) <= w {
		return []string{s}
	}
	if hang >= w {
		hang = 0
	}
	lead := len(s) - len(strings.TrimLeft(s, " "))
	var out []string
	cur, n := s[:lead], lead
	fresh := true // nothing but indent on the current line
	for _, wd := range strings.Fields(s[lead:]) {
		for {
			l := vlen(wd)
			gap := 1
			if fresh {
				gap = 0
			}
			if n+gap+l <= w {
				if !fresh {
					cur += " "
				}
				cur, n, fresh = cur+wd, n+gap+l, false
				break
			}
			if !fresh {
				out = append(out, cur)
				cur, n, fresh = spaces(hang), hang, true
				continue
			}
			head, tail := cutv(wd, w-n) // the word alone does not fit
			out = append(out, cur+head)
			cur, n, wd = spaces(hang), hang, tail
		}
	}
	if !fresh {
		out = append(out, cur)
	}
	return out
}

// ==========================
// ===  Frames            ===
// ==========================

type frame struct{ tl, tr, bl, br, h, v, ml, mr string }

// One frame round the whole program (outer, also the dialogs over it); the
// panels inside have none (thin: a title line and an indented body).
var (
	thin  = frame{" ", " ", " ", " ", " ", " ", " ", " "}
	outer = frame{"┌", "┐", "└", "┘", "─", "│", "├", "┤"} // rounded on terminals with the glyphs
)

// pad is the inner margin of every panel and dialog, left and right.
const pad = 2

// sep as a body line of a box draws a rule across it.
const sep = "\x00sep"

// selMark starts a body line drawn as the selection bar across the whole
// inside of the box, margins included: sel(text).
const selMark = "\x00sel"

func sel(s string) string { return selMark + s }

// box frames body lines, w wide in all; a title sits in the top edge. Lines too
// long for the box are wrapped, so nothing sticks out of the frame.
func box(f frame, title string, body []string, w int) []string {
	return boxC(f, cFrame, title, body, w)
}

// boxC is box with its frame in colour fc: red for a stop. A title with
// colour marks of its own (a badge) is put in as it is.
func boxC(f frame, fc int, title string, body []string, w int) []string {
	if w < 2*pad+6 {
		w = 2*pad + 6
	}
	inner := w - 2 - 2*pad
	if f == thin {
		return panel(title, body, w, inner)
	}
	var out []string
	if title == "" {
		out = append(out, paint(fc, f.tl+repeat(f.h, w-2)+f.tr))
	} else {
		t, _ := cutv(title, w-6)
		if strings.IndexByte(t, esc) < 0 {
			t = paint(cTitle, " "+t+" ")
		}
		out = append(out, paint(fc, f.tl+f.h)+t+paint(fc, repeat(f.h, w-3-vlen(t))+f.tr))
	}
	side := paint(fc, f.v)
	for _, l := range body {
		switch {
		case l == sep:
			out = append(out, paint(fc, f.ml+repeat(f.h, w-2)+f.mr))
		case strings.HasPrefix(l, selMark):
			t, _ := cutv(l[len(selMark):], inner)
			out = append(out, side+hilite(spaces(pad)+padRight(strip(t), inner)+spaces(pad))+side)
		default:
			for _, p := range wrap(l, inner, 2) {
				out = append(out, side+spaces(pad)+padRight(p, inner)+spaces(pad)+side)
			}
		}
	}
	out = append(out, paint(fc, f.bl+repeat(f.h, w-2)+f.br))
	return out
}

// panel is a box without a frame, inside the outer one: the title in colour
// over the body, a thin grey rule for sep, the selection bar as wide as a box.
func panel(title string, body []string, w, inner int) []string {
	var out []string
	if title != "" {
		t, _ := cutv(strip(title), w-2*pad)
		out = append(out, spaces(pad)+paint(cLightCyan, t)+spaces(w-pad-vlen(t)))
	}
	for _, l := range body {
		switch {
		case l == sep:
			out = append(out, spaces(pad)+paint(cHint, repeat("─", w-2*pad))+spaces(pad))
		case strings.HasPrefix(l, selMark):
			t, _ := cutv(l[len(selMark):], inner)
			out = append(out, " "+hilite(spaces(pad)+padRight(strip(t), inner)+spaces(pad))+" ")
		default:
			for _, p := range wrap(l, inner, 2) {
				out = append(out, " "+spaces(pad)+padRight(p, inner)+spaces(pad)+" ")
			}
		}
	}
	return out
}

// inner is the text width of a box w wide.
func inner(w int) int { return w - 2 - 2*pad }

// twoCols lays "key  what it does" pairs out in two columns, row by row, or in
// one when a pair does not fit half the width.
func twoCols(items [][2]string, w int) []string {
	half := (w - 2) / 2
	keyW := [2]int{}
	for i, it := range items {
		if l := vlen(it[0]); l > keyW[i%2] {
			keyW[i%2] = l
		}
	}
	cell := func(i int) string {
		return paint(cKey, padRight(items[i][0], keyW[i%2])) + "  " + items[i][1]
	}
	fits := true
	for i := range items {
		if vlen(cell(i)) > half {
			fits = false
		}
	}
	var out []string
	if !fits {
		k := keyW[0]
		if keyW[1] > k {
			k = keyW[1]
		}
		for _, it := range items {
			out = append(out, wrap(paint(cKey, padRight(it[0], k))+"  "+it[1], w, k+2)...)
		}
		return out
	}
	for i := 0; i < len(items); i += 2 {
		l := padRight(cell(i), half)
		if i+1 < len(items) {
			l += "  " + cell(i+1)
		}
		out = append(out, l)
	}
	return out
}

// note is a one-paragraph message with a mark and a coloured label in front.
func note(mark string, c int, label, text string, w int) []string {
	head := paint(c, mark+" "+label)
	if label == "" {
		head = paint(c, mark)
	}
	return wrap(head+" "+text, w, 2)
}

// bar is a progress bar n cells wide and the percent after it: always n+5 wide.
func bar(done, all, n int) string {
	k, pct := n, 100
	if all > 0 {
		k, pct = done*n/all, done*100/all
	}
	if k > n {
		k, pct = n, 100
	}
	return paint(cOK, repeat("█", k)) + paint(cHint, repeat("░", n-k)) + paint(cTitle, fmt.Sprintf(" %3d%%", pct))
}

// val is a number in white with its unit in grey after it.
func val(v, unit string) string {
	if unit == "" {
		return paint(cTitle, v)
	}
	return paint(cTitle, v) + " " + paint(cHint, unit)
}

// ==========================
// ===  Status line       ===
// ==========================

type mcuState int

const (
	mcuNone mcuState = iota // port not opened yet
	mcuOpen
	mcuContact
	mcuDead
)

// SimInfo is the virtual machine in the status line: where NcStudio would
// show the tool, and the faults switched on.
type SimInfo struct {
	X, Y, Z float64
	Moving  bool
	Faults  []string
}

type Status struct {
	Port string
	MCU  mcuState
	Nc   bool
	Z    float64
	HasZ bool
	More []string // more cells: manual mode steps
	Sim  *SimInfo
}

// mcuBadge is the MCU state alone: ОК, КОНТАКТ, НЕТ СВЯЗИ.
func mcuBadge(s mcuState) string {
	switch s {
	case mcuOpen:
		return okBadge("ОК")
	case mcuContact:
		return touchBadge("КОНТАКТ")
	case mcuDead:
		return alarmBadge("НЕТ СВЯЗИ")
	}
	return offBadge("НЕ ОТКРЫТ")
}

// flow joins cells with three spaces and breaks between cells when too long.
func flow(cells []string, w int) []string {
	var out []string
	cur := ""
	for _, c := range cells {
		switch {
		case cur == "":
			cur = " " + c
		case vlen(cur)+3+vlen(c) <= w:
			cur += "   " + c
		default:
			out = append(out, cur)
			cur = " " + c
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	for i, l := range out {
		if vlen(l) > w { // one cell wider than the screen
			h, _ := cutv(l, w)
			out[i] = h + plain
		}
	}
	return out
}

// edgeStatus is the state of the work for the top edge of the frame, short:
// a coloured mark before МК and NcStudio (green fine, yellow contact, red no
// answer, grey not open / not found), the faults switched on in emulation.
func edgeStatus(st Status) string {
	mc := cDark
	switch st.MCU {
	case mcuOpen:
		mc = cLightGreen
	case mcuContact:
		mc = cYellow
	case mcuDead:
		mc = cLightRed
	}
	nc := cDark
	if st.Nc {
		nc = cLightGreen
	}
	s := paint(mc, "■") + " МК   " + paint(nc, "■") + " NcStudio"
	if st.Sim != nil && len(st.Sim.Faults) > 0 {
		s += "   " + paint(cAlarm, fmt.Sprintf("▲ сбоев %d", len(st.Sim.Faults)))
	}
	return s
}
