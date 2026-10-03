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

// What each colour means on screen.
const (
	cFrame = cCyan       // frames and axes
	cTitle = cWhite      // titles, values that matter
	cKey   = cLightCyan  // keys the operator types
	cHint  = cDark       // hints
	cOK    = cLightGreen // done, fine
	cAlarm = cLightRed   // stop, warning
	cTouch = cYellow     // contact
	cZero  = cLightMagenta
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

// The selected line of a menu, a pressed button: dark text on a cyan bar.
const (
	cSelBg = cCyan
	cSelFg = cBlack
)

// hilite draws s as selected; colour marks inside s are dropped.
func hilite(s string) string { return bgInk(cSelBg) + ink(cSelFg) + strip(s) + plain }

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
		if m == '0' {
			b.WriteString("\x1b[0m")
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

var (
	thin  = frame{"┌", "┐", "└", "┘", "─", "│", "├", "┤"}
	thick = frame{"╔", "╗", "╚", "╝", "═", "║", "╟", "╢"}
)

// sep as a body line of a box draws a rule across it.
const sep = "\x00sep"

// box frames body lines, w wide in all; a title sits in the top edge. Lines too
// long for the box are wrapped, so nothing sticks out of the frame.
func box(f frame, title string, body []string, w int) []string {
	return boxC(f, cFrame, title, body, w)
}

// boxC is box with its frame in colour fc: red for a stop.
func boxC(f frame, fc int, title string, body []string, w int) []string {
	cFrame := fc
	if w < 8 {
		w = 8
	}
	inner := w - 4
	var out []string
	if title == "" {
		out = append(out, paint(cFrame, f.tl+repeat(f.h, w-2)+f.tr))
	} else {
		t, _ := cutv(title, w-6)
		out = append(out, paint(cFrame, f.tl+f.h)+" "+paint(cTitle, t)+" "+
			paint(cFrame, repeat(f.h, w-5-vlen(t))+f.tr))
	}
	for _, l := range body {
		if l == sep {
			mid := f.h
			if f.h == "═" {
				mid = "─"
			}
			out = append(out, paint(cFrame, f.ml+repeat(mid, w-2)+f.mr))
			continue
		}
		for _, p := range wrap(l, inner, 2) {
			out = append(out, paint(cFrame, f.v)+" "+padRight(p, inner)+" "+paint(cFrame, f.v))
		}
	}
	out = append(out, paint(cFrame, f.bl+repeat(f.h, w-2)+f.br))
	return out
}

// rule is a section heading: a double line with the title in it.
func rule(title string, w int) []string {
	t, _ := cutv(title, w-8)
	return []string{paint(cFrame, "═══ ") + paint(cTitle, t) + " " + paint(cFrame, repeat("═", w-5-vlen(t)))}
}

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

// bar is a progress bar n wide for done of all.
func bar(done, all, n int) string {
	k := n
	if all > 0 {
		k = done * n / all
	}
	if k > n {
		k = n
	}
	return paint(cOK, repeat("█", k)) + paint(cHint, repeat("░", n-k))
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

type Status struct {
	Port string
	MCU  mcuState
	Nc   bool
	Z    float64
	HasZ bool
	More []string // more cells: manual mode steps
}

// statusLine joins the cells with │ and breaks between cells when too long.
func statusLine(st Status, w int) []string {
	var cells []string
	port := "МК"
	if st.Port != "" {
		port = "МК " + st.Port
	}
	switch st.MCU {
	case mcuNone:
		cells = append(cells, paint(cHint, "○")+" МК: порт не открыт")
	case mcuOpen:
		cells = append(cells, paint(cOK, "○")+" "+port+": "+paint(cOK, "разомкнуто"))
	case mcuContact:
		cells = append(cells, paint(cTouch, "♦")+" "+port+": "+paint(cTouch, "КОНТАКТ"))
	case mcuDead:
		cells = append(cells, paint(cAlarm, "■")+" "+port+": "+paint(cAlarm, "НЕ ОТВЕЧАЕТ"))
	}
	if st.Nc {
		cells = append(cells, paint(cOK, "√")+" NcStudio найдена")
	} else {
		cells = append(cells, paint(cHint, "○")+" NcStudio не найдена")
	}
	if st.HasZ {
		cells = append(cells, "Z от старта "+paint(cTitle, fmt.Sprintf("%+.3f", st.Z)))
	}
	cells = append(cells, st.More...)
	div := paint(cFrame, " │ ")
	var out []string
	cur := ""
	for _, c := range cells {
		switch {
		case cur == "":
			cur = " " + c
		case vlen(cur)+3+vlen(c) <= w:
			cur += div + c
		default:
			out = append(out, cur)
			cur = " " + c
		}
	}
	out = append(out, cur)
	for i, l := range out {
		if vlen(l) > w { // one cell wider than the screen
			h, _ := cutv(l, w)
			out[i] = h + plain
		}
	}
	return out
}
