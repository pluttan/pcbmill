package main

// Pieces every screen uses: dialogs over the screen (Да/Нет, ОСТАНОВ), the
// list to choose from, and the work page with its log window, where messages
// go instead of scrolling the console.

import (
	"strings"
	"sync"
	"time"
)

// colW is the width of the text column: boxes, the menu, the log.
func colW(w int) int {
	if w > 78 {
		return 78
	}
	return w
}

// room is how many rows the body of a screen gets between the status line and
// the hint line.
func room(w, h int) int {
	n := h - 1
	if statusFn != nil {
		n -= len(statusLine(statusFn(), w))
	}
	if n < 3 {
		n = 3
	}
	return n
}

// ==========================
// ===  Dialogs           ===
// ==========================

type dialog struct {
	title   string
	alarm   bool // red frame: ОСТАНОВ
	text    []string
	buttons []string
	sel     int
	cancel  int // the button Esc means
}

// The answer letters on either layout: Д is L on the Latin layout, N on the
// Russian one gives т. Y on the Russian layout is н and means Нет: what is
// on the screen decides, not the key cap.
const (
	yesRunes = "yYдДlL"
	noRunes  = "nNнНтТ"
)

// key returns the chosen button and true once the dialog is answered.
func (d *dialog) key(k Key) (int, bool) {
	n := len(d.buttons)
	switch k.Code {
	case KeyLeft, KeyBackTab, KeyUp:
		if d.sel > 0 {
			d.sel--
		}
	case KeyRight, KeyDown:
		if d.sel < n-1 {
			d.sel++
		}
	case KeyTab:
		d.sel = (d.sel + 1) % n
	case KeyEnter:
		return d.sel, true
	case KeyEsc, KeyCtrlC:
		return d.cancel, true
	case KeyRune:
		if n == 2 && strings.ContainsRune(yesRunes, k.R) {
			return 0, true
		}
		if n == 2 && strings.ContainsRune(noRunes, k.R) {
			return 1, true
		}
		if n == 1 && k.R == ' ' {
			return 0, true
		}
	}
	return 0, false
}

// box: as wide as the text needs, 34 to 64 columns; short text is centred.
func (d *dialog) box(w int) []string {
	bw := blockWidth(d.text) + 8
	if bw < 34 {
		bw = 34
	}
	if bw > 64 {
		bw = 64
	}
	if bw > w {
		bw = w
	}
	var body []string
	body = append(body, "")
	for _, l := range d.text {
		if vlen(l) <= bw-4 {
			l = center(l, bw-4)
		}
		body = append(body, l)
	}
	body = append(body, "")
	var bs []string
	for i, b := range d.buttons {
		if i == d.sel {
			bs = append(bs, hilite("[ "+b+" ]"))
		} else {
			bs = append(bs, "[ "+b+" ]")
		}
	}
	body = append(body, center(strings.Join(bs, "   "), bw-4))
	title, fc := d.title, cKey
	if d.alarm {
		title, fc = paint(cAlarm, d.title), cAlarm
	}
	return boxC(thick, fc, title, body, bw)
}

func (d *dialog) hint(w int) string {
	if len(d.buttons) == 1 {
		return hints(w, "Enter", "закрыть")
	}
	return hints(w, "← →", "выбор", "Enter", "ответ", "Д / Н", "сразу", "Esc", d.buttons[d.cancel])
}

// show runs the dialog over the current screen and returns the button chosen.
func (d *dialog) show() int {
	base := backdrop
	defer func() { backdrop = base }()
	view := func(w, h int) []string {
		b := d.box(w)
		if base == nil {
			return screen(w, h, b, d.hint(w))
		}
		fr := overlay(base(w, h), b, w, h)
		fr[h-1] = center(d.hint(w), w)
		return fr
	}
	res := 0
	run(view, func(k Key) bool {
		var done bool
		res, done = d.key(k)
		return done
	}, 500*time.Millisecond)
	return res
}

// confirm asks a yes/no question; Нет is chosen at first, as an empty answer
// was "no" before: starting the machine always takes a deliberate key.
func confirm(q string, more ...string) bool {
	d := &dialog{title: "Вопрос", text: append([]string{paint(cTitle, q)}, more...),
		buttons: []string{"Да", "Нет"}, sel: 1, cancel: 1}
	return d.show() == 0
}

// alert shows a stop in red over the screen until Enter.
func alert(err error) {
	d := &dialog{title: "ОСТАНОВ", alarm: true, text: []string{err.Error()}, buttons: []string{"OK"}}
	d.show()
}

// message shows a note with one button.
func message(title string, text ...string) {
	d := &dialog{title: title, text: text, buttons: []string{"OK"}}
	d.show()
}

// ==========================
// ===  List              ===
// ==========================

// listNav is a list walked with the arrows: Enter takes, Esc goes back, a digit
// takes that line at once.
type listNav struct {
	title string
	items []string
	sel   int
}

// key: the index chosen, -1 for back, and true once done.
func (l *listNav) key(k Key) (int, bool) {
	n := len(l.items)
	switch k.Code {
	case KeyUp:
		l.sel = (l.sel + n - 1) % n
	case KeyDown, KeyTab:
		l.sel = (l.sel + 1) % n
	case KeyHome:
		l.sel = 0
	case KeyEnd:
		l.sel = n - 1
	case KeyPgUp:
		l.sel = max0(l.sel - 10)
	case KeyPgDn:
		if l.sel += 10; l.sel >= n {
			l.sel = n - 1
		}
	case KeyEnter:
		return l.sel, true
	case KeyEsc, KeyCtrlC:
		return -1, true
	case KeyRune:
		if d := int(k.R - '0'); d >= 1 && d <= 9 && d <= n {
			l.sel = d - 1
			return l.sel, true
		}
	}
	return 0, false
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (l *listNav) view(w, h int) []string {
	bw := colW(w)
	if bw > 64 {
		bw = 64
	}
	rows := room(w, h) - 2
	if rows < 1 {
		rows = 1
	}
	top := 0
	if l.sel >= rows {
		top = l.sel - rows + 1
	}
	var body []string
	for i := top; i < len(l.items) && i < top+rows; i++ {
		num := "  "
		if i < 9 {
			num = string(rune('1'+i)) + " "
		}
		if i == l.sel {
			body = append(body, hilite(padRight(" ► "+num+"  "+l.items[i], bw-4)))
		} else {
			body = append(body, "   "+paint(cKey, num)+"  "+l.items[i])
		}
	}
	return screen(w, h, box(thin, l.title, body, bw),
		hints(w, "↑↓", "выбор", "Enter", "взять", "Esc", "назад"))
}

// pick runs a list; false when the operator went back.
func pick(title string, items []string) (int, bool) {
	l := &listNav{title: title, items: items}
	res := -1
	run(l.view, func(k Key) bool {
		var done bool
		res, done = l.key(k)
		return done
	}, 500*time.Millisecond)
	return res, res >= 0
}

// ==========================
// ===  Work page         ===
// ==========================

// page is the screen of a running command: title, what will happen, an
// optional picture, and the log of what happened, newest at the bottom.
type page struct {
	mu    sync.Mutex
	title string
	info  []string
	pic   func(w, h int) []string
	log   []string
	keys  []string // hint pairs
	big   []string // a large indicator in place of the log (wire check)

	screenFn func(w, h int) []string // a screen of its own instead of view (manual mode)
}

// cur is the page messages go to.
var cur *page

func newPage(title string, info ...string) *page {
	p := &page{title: title, info: info}
	cur = p
	backdrop = p.view
	p.draw()
	return p
}

func (p *page) draw() {
	if p.screenFn != nil {
		draw(p.screenFn)
		return
	}
	draw(p.view)
}

func (p *page) set(f func()) {
	p.mu.Lock()
	f()
	p.mu.Unlock()
	p.draw()
}

// add puts a line into the log; the screen is redrawn at once.
func (p *page) add(s string) {
	p.set(func() {
		p.log = append(p.log, s)
		if len(p.log) > 500 {
			p.log = p.log[len(p.log)-500:]
		}
	})
}

func (p *page) view(w, h int) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	bw := colW(w)
	left := room(w, h)
	blocks := [][]string{rule(p.title, bw)}
	used := 1
	if len(p.info) > 0 {
		b := box(thin, "", p.info, bw)
		blocks = append(blocks, b)
		used += len(b) + 1
	}
	if p.pic != nil {
		pw := w
		if pw > 110 {
			pw = 110
		}
		var b []string
		if ph := left - used - 1 - 4; ph >= 3 {
			b = p.pic(pw, ph)
		}
		if len(b) > 0 {
			blocks = append(blocks, b)
			used += len(b) + 1
		}
	}
	if len(p.big) > 0 {
		blocks = append(blocks, p.big)
		used += len(p.big) + 1
	}
	if n := left - used - 3; n >= 1 && len(p.log) > 0 {
		var ls []string
		for _, l := range p.log {
			ls = append(ls, wrap(l, bw-4, 2)...)
		}
		if len(ls) > n {
			ls = ls[len(ls)-n:]
		}
		for len(ls) < n && len(ls) < 8 {
			ls = append(ls, "")
		}
		blocks = append(blocks, box(thin, "Ход работы", ls, bw))
	}
	keys := p.keys
	if keys == nil {
		keys = []string{"", paint(cHint, "идёт работа: мышь и клавиатуру не трогать")}
	}
	return screen(w, h, stack(blocks...), hints(w, keys...))
}

// wait leaves the page on screen until Enter or Esc.
func (p *page) wait() {
	p.set(func() { p.keys = []string{"Enter", "в меню", "Esc", "в меню"} })
	run(p.view, func(k Key) bool {
		return k.Code == KeyEnter || k.Code == KeyEsc || k.Code == KeyCtrlC
	}, 500*time.Millisecond)
}

// The messages of a command, into the log of the current page.
func logLine(s string) {
	if cur != nil {
		cur.add(s)
	}
}

func sayStop(err error) { logLine(paint(cAlarm, "■ ОСТАНОВ:") + " " + err.Error()) }
func sayWarn(s string)  { logLine(paint(cAlarm, "▲ ВНИМАНИЕ:") + " " + s) }
func sayBad(s string)   { logLine(paint(cAlarm, "◄") + " " + s) }
func sayOK(s string)    { logLine(paint(cOK, "√ "+s)) }
func sayHint(s string)  { logLine(paint(cHint, s)) }
