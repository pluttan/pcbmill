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

// banner is the head of the main screen.
func banner(w int) []string {
	return box(thick, "", []string{
		center(paint(cTitle, "pcbscan")+paint(cFrame, "  ►  ")+"карта высот платы для NcStudio", w-4),
		center(paint(cHint, "касание фрезы о медь · МК на COM · команды в окно NcStudio"), w-4),
	}, w)
}

type menuItem struct{ key, text string }

// menu: a sep item draws a rule, a key in capitals makes the line stand out.
func menu(title string, items []menuItem, w int) []string {
	var body []string
	for _, it := range items {
		if it.key == sep {
			body = append(body, sep)
			continue
		}
		body = append(body, " "+paint(cKey, padRight(it.key, 2))+paint(cFrame, "► ")+it.text)
	}
	return box(thin, title, body, w)
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

// ==========================
// ===  Printing          ===
// ==========================

// page is the usable console width: a line as wide as the console makes it
// wrap the cursor and leaves an empty line under it.
func page() int {
	w := conWidth() - 1
	if w < 40 {
		w = 40
	}
	return w
}

// boxW is the width of the text column: boxes, messages, the menu.
func boxW() int {
	if p := page(); p < 78 {
		return p
	}
	return 78
}

// mapW is the width for the table picture: wider consoles get a bigger map.
func mapW() int {
	if p := page(); p < 110 {
		return p
	}
	return 110
}

// margin centres the text column on the console.
func margin() string { return spaces((page() - boxW()) / 2) }

// show prints lines in the text column.
func show(lines []string) {
	m := margin()
	for _, l := range lines {
		say("%s%s\n", m, l)
	}
}

// showMid prints a block centred on the console.
func showMid(lines []string) {
	w := 0
	for _, l := range lines {
		if vlen(l) > w {
			w = vlen(l)
		}
	}
	m := spaces((page() - w) / 2)
	for _, l := range lines {
		say("%s%s\n", m, l)
	}
}

// tell prints a paragraph in the text column, wrapped to it.
func tell(f string, a ...interface{}) {
	for _, l := range strings.Split(strings.TrimRight(fmt.Sprintf(f, a...), "\n"), "\n") {
		show(wrap(l, boxW(), 2))
	}
}

func sayStop(err error) { show(note("■", cAlarm, "ОСТАНОВ:", err.Error(), boxW())) }
func sayWarn(s string)  { show(note("▲", cAlarm, "ВНИМАНИЕ:", s, boxW())) }
func sayBad(s string)   { show(note("◄", cAlarm, "", s, boxW())) }
func sayOK(s string)    { show(note("√", cOK, "", paint(cOK, s), boxW())) }
func sayHint(s string)  { show(wrap(paint(cHint, s), boxW(), 2)) }

// prompt asks in the text column with a marker in front.
func prompt(q string) string { return ask(margin() + paint(cKey, "► ") + q) }
