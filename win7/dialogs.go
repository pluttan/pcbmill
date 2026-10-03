package main

// Dialogs: a card in the middle of the window over the dimmed screen. The
// work goroutine shows one and waits for the answer; the window goroutine
// draws it and sends the answer. The first button chosen is the safe one
// (Нет, Отмена): a stray Enter must not start the machine.

import (
	"image"
	"strings"

	"pcbscan/gfx"
	"pcbscan/ui"
)

type dlgKind int

const (
	dlgAsk dlgKind = iota
	dlgInfo
	dlgStop
	dlgOK
)

type dialog struct {
	title   string
	text    []string
	buttons []string
	sel     int
	cancel  int
	kind    dlgKind
	letters bool      // Д/Y and Н/N answer at once (two buttons: yes, no)
	field   *ui.Field // a typed answer (askYes)
	hint    string    // a line under the field
	check   func(s string) (ok bool, hint string)
	done    chan int
	sent    bool
}

// show waits for the answer: the index of the button.
func (d *dialog) show() int {
	if app == nil {
		return d.cancel
	}
	d.done = make(chan int, 1)
	backToApp()
	app.Push(d)
	r := <-d.done
	app.Pop(d)
	return r
}

func (d *dialog) answer(i int) {
	if !d.sent {
		d.sent = true
		d.done <- i
	}
}

func (d *dialog) Draw(c *ui.Ctx) {
	t := c.T
	pad := t.Si(24)
	w := ui.Inset(image.Rect(0, 0, c.W, c.H), t.Si(20)).Dx()
	if w > t.Si(560) {
		w = t.Si(560)
	}
	inner := w - 2*pad - t.Si(52)
	body := c.Body()
	var lines []string
	for _, l := range d.text {
		if l == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, c.Wrap(l, inner, body)...)
	}
	lh := c.LineH(body)
	title := d.title
	if d.kind == dlgStop && !strings.HasPrefix(title, "ОСТАНОВ") {
		title = "ОСТАНОВ: " + title
	}
	tst := ui.Style{Kind: ui.Bold, Size: ui.SizeTitle - 2, Color: t.Text}
	tlines := c.Wrap(title, inner, tst)
	th := len(tlines) * c.LineH(tst)
	if th < t.Si(30) {
		th = t.Si(30)
	}
	h := pad + th + t.Si(10) + len(lines)*lh + pad + t.Si(38) + pad
	if d.field != nil {
		h += t.Si(54)
	}
	if d.hint != "" {
		h += lh
	}
	if h > c.H-t.Si(20) {
		h = c.H - t.Si(20)
	}
	r := ui.Center(image.Rect(0, 0, c.W, c.H), w, h)
	c.Card(r)
	accent := t.Blue
	ic := ui.IconInfo
	switch d.kind {
	case dlgStop:
		accent, ic = t.Red, ui.IconWarn
	case dlgOK:
		accent, ic = t.Green, ui.IconCheck
	case dlgAsk:
		accent, ic = t.Lavender, ui.IconInfo
	}
	in := ui.Inset(r, pad)
	iconR := image.Rect(in.Min.X, in.Min.Y, in.Min.X+t.Si(36), in.Min.Y+t.Si(36))
	c.C.FillCircle(float64(iconR.Min.X+iconR.Dx()/2), float64(iconR.Min.Y+iconR.Dy()/2), t.S(18), gfx.Alpha(accent, 0.16))
	c.Icon(ic, ui.Inset(iconR, t.Si(8)), accent)
	tx := in.Min.X + t.Si(52)
	ty := in.Min.Y + (t.Si(36)-c.LineH(tst))/2
	if ty < in.Min.Y {
		ty = in.Min.Y
	}
	for _, l := range tlines {
		c.Label(tx, ty, l, tst)
		ty += c.LineH(tst)
	}
	y := in.Min.Y + th + t.Si(10)
	for _, l := range lines {
		c.Label(tx, y, l, body)
		y += lh
	}
	if d.field != nil {
		y += t.Si(10)
		fr := image.Rect(tx, y, in.Max.X, y+t.Si(40))
		c.SetFocus("dlg/field")
		enter, cancel := c.TextField("dlg/field", fr, d.field, ui.Style{Kind: ui.Mono, Size: ui.SizeLarge, Color: t.Text}, 0)
		if enter {
			d.submit()
		}
		if cancel {
			d.answer(d.cancel)
		}
		y += t.Si(48)
	}
	if d.hint != "" {
		c.Label(tx, y, d.hint, ui.Style{Kind: ui.Regular, Size: ui.SizeBody, Color: t.Yellow})
	}
	// the buttons, right aligned, the chosen one with a ring
	bh := t.Si(38)
	by := r.Max.Y - pad - bh
	x := in.Max.X
	for i := len(d.buttons) - 1; i >= 0; i-- {
		bw := c.Measure(d.buttons[i], c.Body()) + t.Si(40)
		if bw < t.Si(96) {
			bw = t.Si(96)
		}
		x -= bw
		br := image.Rect(x, by, x+bw, by+bh)
		kind := ui.BtnNormal
		if i == 0 && d.cancel != 0 {
			kind = ui.BtnPrimary
			if d.kind == dlgStop {
				kind = ui.BtnDanger
			}
		}
		if c.ButtonX("dlg/b"+string(rune('0'+i)), br, d.buttons[i], kind, true, i == d.sel && d.field == nil) {
			if d.field != nil && i != d.cancel {
				d.submit()
			} else {
				d.answer(i)
			}
		}
		x -= t.Si(10)
	}
	d.keys(c)
}

func (d *dialog) submit() {
	if d.check == nil {
		d.answer(0)
		return
	}
	ok, hint := d.check(d.field.String())
	switch {
	case ok:
		d.answer(0)
	case hint != "":
		d.hint = hint
		d.field.Set("")
	default:
		d.answer(d.cancel)
	}
}

func (d *dialog) keys(c *ui.Ctx) {
	e, ok := c.Key()
	if !ok {
		return
	}
	n := len(d.buttons)
	if e.Kind == ui.EvChar {
		if d.letters && d.field == nil {
			switch strings.ToLower(string(e.Rune)) {
			case "д", "y", "l": // l: Д on the Latin layout
				c.Use()
				d.answer(0)
			case "н", "n", "т": // т: N on the Russian layout
				c.Use()
				d.answer(1)
			}
		}
		return
	}
	switch e.Key {
	case ui.KeyLeft:
		d.sel = (d.sel + n - 1) % n
	case ui.KeyRight, ui.KeyTab:
		d.sel = (d.sel + 1) % n
	case ui.KeyEnter:
		if d.field != nil {
			d.submit()
		} else {
			d.answer(d.sel)
		}
	case ui.KeyEsc:
		d.answer(d.cancel)
	default:
		return
	}
	c.Use()
}

// ==========================
// ===  The usual ones    ===
// ==========================

// confirm: Да or Нет, Нет chosen at first.
func confirm(q string, more ...string) bool {
	d := &dialog{title: q, text: more, buttons: []string{"Да", "Нет"}, sel: 1, cancel: 1, letters: true}
	return d.show() == 0
}

// askYes: the operator types yes before a program goes to NcStudio. Only
// yes in any case is a yes; the same keys on the Russian layout get a hint.
func askYes(title string, text ...string) bool {
	d := &dialog{title: title, text: append(append([]string(nil), text...), "", "Напечатайте yes и нажмите Enter."),
		buttons: []string{"Открыть в NcStudio", "Отмена"}, cancel: 1, field: ui.NewField("")}
	d.check = func(s string) (bool, string) {
		s = strings.TrimSpace(strings.ToLower(s))
		switch s {
		case "yes":
			return true, ""
		case "нуы":
			return false, "Русская раскладка: переключите на английскую и напечатайте yes."
		case "":
			return false, "Нужно напечатать yes."
		}
		return false, ""
	}
	return d.show() == 0
}

// alert: an error stops the work; OK to close.
func alert(err error) {
	d := &dialog{title: "ОСТАНОВ", text: []string{err.Error()}, buttons: []string{"OK"}, kind: dlgStop}
	d.show()
}

// message: something to read, OK to close.
func message(title string, text ...string) {
	kind := dlgInfo
	if strings.Contains(title, "готов") || strings.Contains(title, "сделано") {
		kind = dlgOK
	}
	d := &dialog{title: title, text: text, buttons: []string{"OK"}, kind: kind}
	d.show()
}

// ==========================
// ===  A list to pick    ===
// ==========================

type pickDlg struct {
	title string
	items []string
	sel   int
	done  chan int
	sent  bool
}

// pick: one of the items, or false for back.
func pick(title string, items []string) (int, bool) {
	if app == nil || len(items) == 0 {
		return 0, false
	}
	p := &pickDlg{title: title, items: items, done: make(chan int, 1)}
	backToApp()
	app.Push(p)
	i := <-p.done
	app.Pop(p)
	return i, i >= 0
}

func (p *pickDlg) answer(i int) {
	if !p.sent {
		p.sent = true
		p.done <- i
	}
}

func (p *pickDlg) Draw(c *ui.Ctx) {
	t := c.T
	rowH := t.Si(36)
	w := ui.Inset(image.Rect(0, 0, c.W, c.H), t.Si(20)).Dx()
	if w > t.Si(520) {
		w = t.Si(520)
	}
	h := t.Si(24)*2 + t.Si(40) + len(p.items)*rowH + t.Si(54)
	if h > c.H-t.Si(40) {
		h = c.H - t.Si(40)
	}
	r := ui.Center(image.Rect(0, 0, c.W, c.H), w, h)
	c.Card(r)
	in := ui.Inset(r, t.Si(24))
	c.LabelIn(ui.CutTop(&in, t.Si(36)), p.title, ui.Style{Kind: ui.Bold, Size: ui.SizeTitle - 2, Color: t.Text}, 0)
	foot := ui.CutBottom(&in, t.Si(46))
	list := in
	off := c.Scroll("pick", list, len(p.items)*rowH, false)
	c.WithClip(list, func() {
		for i, s := range p.items {
			rr := image.Rect(list.Min.X, list.Min.Y+i*rowH-off, list.Max.X, list.Min.Y+(i+1)*rowH-off)
			if c.Row("pick/"+s, rr, i == p.sel) {
				p.sel = i
				if c.Double() {
					p.answer(i)
				}
			}
			c.LabelIn(ui.InsetXY(rr, t.Si(14), 0), s, c.Body(), 0)
		}
	})
	bs := ui.SplitX(image.Rect(foot.Max.X-t.Si(250), foot.Max.Y-t.Si(38), foot.Max.X, foot.Max.Y), 2, t.Si(10))
	if c.Button("pick/ok", bs[0], "Выбрать", ui.BtnPrimary) {
		p.answer(p.sel)
	}
	if c.Button("pick/no", bs[1], "Назад", ui.BtnNormal) {
		p.answer(-1)
	}
	if e, ok := c.Key(); ok && e.Kind == ui.EvKey {
		switch e.Key {
		case ui.KeyUp:
			p.sel = (p.sel + len(p.items) - 1) % len(p.items)
		case ui.KeyDown, ui.KeyTab:
			p.sel = (p.sel + 1) % len(p.items)
		case ui.KeyEnter:
			p.answer(p.sel)
		case ui.KeyEsc:
			p.answer(-1)
		}
		c.Use()
	}
}
