package main

// A form of numbers: a field per number with − and + that change it by its
// step, a number can be typed in; a switch is a row of choices. The 3D view
// beside it follows every change. Up and Down walk the fields, Left and
// Right step the value, Enter on a field takes the typed number.

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"pcbscan/gfx"
	"pcbscan/ui"
)

type formField struct {
	label, unit  string
	group        string // a heading over the field when it changes
	lo, hi, step float64
	digits       int
	choices      []string
	get          func() float64
	set          func(float64)
	show         func(v float64) string // a word for a special value ("та же V")
}

func (f formField) text() string {
	v := f.get()
	if f.choices != nil {
		i := int(v)
		if i < 0 || i >= len(f.choices) {
			return "?"
		}
		return f.choices[i]
	}
	if f.show != nil {
		if s := f.show(v); s != "" {
			return s
		}
	}
	return strconv.FormatFloat(v, 'f', f.digits, 64)
}

func (f formField) check(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("не число")
	}
	if v < f.lo-1e-9 || v > f.hi+1e-9 {
		return fmt.Errorf("%s: от %s до %s %s", f.label, num(f.lo), num(f.hi), f.unit)
	}
	if f.digits == 0 && v != math.Trunc(v) {
		return fmt.Errorf("%s: целое число", f.label)
	}
	return nil
}

// stepBy: the value moved by n steps, back onto the grid of steps.
func (f formField) stepBy(n float64) {
	v := f.get()
	if f.choices != nil {
		k := float64(len(f.choices))
		f.set(math.Mod(v+n+k, k))
		return
	}
	nv := math.Round((v+n*f.step)/f.step) * f.step
	nv = math.Round(nv*1e6) / 1e6
	nv = math.Max(f.lo, math.Min(f.hi, nv))
	f.set(nv)
}

type numForm struct {
	title   string
	crumb   string
	fields  []formField
	ok      string // the button that accepts
	extra   string // a third button (Числа по умолчанию), its action
	onExtra func()
	about   func() []string
	scene   func() ui.Scene
	pick    func(x, y float64) // a click on the sheet in the 3D view
	sel     int
	edit    *ui.Field
	focused string // the field the form gave the keys to
	msg     string
	done    chan bool
	sent    bool
}

// editForm shows the form until accepted (true) or left (false).
func editForm(f *numForm) bool {
	f.done = make(chan bool, 1)
	if f.crumb == "" {
		f.crumb = crumb + " / " + f.title
	}
	old := crumb
	crumb = f.crumb
	backToApp()
	app.Show(f)
	ok := <-f.done
	crumb = old
	return ok
}

func (f *numForm) answer(ok bool) {
	if f.edit != nil && ok {
		f.apply()
	}
	if !f.sent {
		f.sent = true
		f.done <- ok
	}
}

// apply takes the typed number into the chosen field.
func (f *numForm) apply() {
	if f.edit == nil || f.sel >= len(f.fields) {
		return
	}
	fl := f.fields[f.sel]
	s := strings.ReplaceAll(strings.TrimSpace(f.edit.String()), ",", ".")
	f.edit = nil
	if s == fl.text() {
		return
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		f.msg = fmt.Sprintf("не число: \"%s\"", s)
		return
	}
	if err := fl.check(v); err != nil {
		f.msg = err.Error()
		return
	}
	fl.set(v)
	f.msg = ""
}

func (f *numForm) choose(i int) {
	if i == f.sel {
		return
	}
	f.apply()
	f.sel = i
}

func (f *numForm) Draw(c *ui.Ctx) {
	t := c.T
	f.keys(c) // before the fields: the arrows step values, they do not move the caret
	body := ui.Inset(chrome(c, f.crumb), t.Si(16))
	var left, right image.Rectangle
	if f.scene != nil && body.Dx() > t.Si(760) {
		lw := imin(t.Si(500), body.Dx()*9/20)
		left = ui.CutLeft(&body, lw)
		ui.CutLeft(&body, t.Si(14))
		right = body
	} else if f.scene != nil {
		right = ui.CutTop(&body, body.Dy()*2/5)
		ui.CutTop(&body, t.Si(12))
		left = body
	} else {
		w := imin(t.Si(560), body.Dx())
		left = image.Rect(body.Min.X+(body.Dx()-w)/2, body.Min.Y, body.Min.X+(body.Dx()+w)/2, body.Max.Y)
	}
	if f.scene != nil {
		c.Card(right)
		hit := c.View3D("form/view", ui.Inset(right, t.Si(1)), f.scene())
		if hit.Click && f.pick != nil {
			if p, ok := hit.Cam.PickPlane(hit.X, hit.Y, hit.W, hit.H, 0); ok {
				f.edit = nil
				f.pick(p.X, p.Y)
			}
		}
	}
	c.Card(left)
	in := ui.Inset(left, t.Si(20))
	c.LabelIn(ui.CutTop(&in, t.Si(30)), f.title, ui.Style{Kind: ui.Bold, Size: ui.SizeTitle, Color: t.Text}, 0)
	ui.CutTop(&in, t.Si(8))
	// the buttons
	foot := ui.CutBottom(&in, t.Si(40))
	labels := []string{f.ok, "Отмена"}
	if f.extra != "" {
		labels = []string{f.ok, f.extra, "Отмена"}
	}
	x := foot.Max.X
	for i := len(labels) - 1; i >= 0; i-- {
		bw := c.Measure(labels[i], c.Body()) + t.Si(36)
		x -= bw
		br := image.Rect(x, foot.Min.Y, x+bw, foot.Max.Y)
		kind := ui.BtnNormal
		if i == 0 {
			kind = ui.BtnPrimary
		}
		if c.ButtonX("form/b"+labels[i], br, labels[i], kind, true, i == 0) {
			switch {
			case i == 0:
				f.answer(true)
			case labels[i] == "Отмена":
				f.answer(false)
			default:
				f.edit = nil
				if f.onExtra != nil {
					f.onExtra()
				}
			}
		}
		x -= t.Si(10)
	}
	ui.CutBottom(&in, t.Si(12))
	// the lines under the fields
	var notes []string
	if f.msg != "" {
		notes = append(notes, "■ "+f.msg)
	}
	if f.about != nil {
		notes = append(notes, f.about()...)
	}
	nh := 0
	for _, l := range notes {
		_, _, rest := lineMark(t, l)
		nh += c.ParaH(in.Dx()-t.Si(24), rest, c.Muted())
	}
	if nh > in.Dy()/3 {
		nh = in.Dy() / 3
	}
	nr := ui.CutBottom(&in, nh)
	drawLines(c, nr, nr.Min.Y, notes, c.Muted())
	ui.CutBottom(&in, t.Si(10))
	// the fields, scrolled when there are many
	rowH, headH := t.Si(42), t.Si(34)
	total := 0
	for _, fl := range f.fields {
		if fl.group != "" {
			total += headH
		}
		total += rowH
	}
	off := c.Scroll("form/fields", in, total, false)
	y := in.Min.Y - off
	c.WithClip(in, func() {
		for i, fl := range f.fields {
			if fl.group != "" {
				c.LabelIn(image.Rect(in.Min.X, y+t.Si(8), in.Max.X, y+headH), fl.group, ui.Style{Kind: ui.Bold, Size: ui.SizeSmall + 1, Color: t.Lavender}, 0)
				y += headH
			}
			rr := image.Rect(in.Min.X, y, in.Max.X-t.Si(8), y+rowH-t.Si(6))
			if i == f.sel && (rr.Min.Y < in.Min.Y || rr.Max.Y > in.Max.Y) { // keep the chosen field in sight
				c.SetScroll("form/fields", rr.Min.Y+off-in.Min.Y-in.Dy()/2)
			}
			f.drawField(c, i, fl, rr)
			y += rowH
		}
	})
}

func (f *numForm) drawField(c *ui.Ctx, i int, fl formField, r image.Rectangle) {
	t := c.T
	on := i == f.sel
	if on {
		c.C.FillRoundRect(float64(r.Min.X)-t.S(6), float64(r.Min.Y)-t.S(2), float64(r.Dx())+t.S(12), float64(r.Dy())+t.S(4), t.S(8), gfx.Alpha(t.Surface0, 0.6))
	}
	lw := r.Dx() * 2 / 5
	lab := c.Body()
	if !on {
		lab.Color = t.Subtext1
	}
	c.LabelIn(image.Rect(r.Min.X+t.Si(4), r.Min.Y, r.Min.X+lw, r.Max.Y), fl.label, lab, 0)
	box := image.Rect(r.Min.X+lw, r.Min.Y, r.Max.X, r.Max.Y)
	id := fmt.Sprintf("form/f%d", i)
	if fl.choices != nil {
		cells := ui.SplitX(box, len(fl.choices), t.Si(4))
		cur := int(fl.get())
		for k, ch := range fl.choices {
			kind := ui.BtnGhost
			if k == cur {
				kind = ui.BtnPrimary
			}
			if c.Button(fmt.Sprintf("%s/%d", id, k), cells[k], ch, kind) {
				f.choose(i)
				fl.set(float64(k))
			}
		}
		return
	}
	bs := box.Dy()
	minus := ui.CutLeft(&box, bs)
	plus := ui.CutRight(&box, bs)
	unitW := 0
	if fl.unit != "" {
		unitW = c.Measure(fl.unit, c.Small()) + t.Si(10)
	}
	unit := ui.CutRight(&box, unitW)
	box = ui.InsetXY(box, t.Si(6), 0)
	if c.IconButton(id+"/-", minus, ui.IconMinus, false) {
		f.choose(i)
		fl.stepBy(-1)
	}
	if c.IconButton(id+"/+", plus, ui.IconPlus, false) {
		f.choose(i)
		fl.stepBy(1)
	}
	if fl.unit != "" {
		c.LabelIn(unit, fl.unit, c.Small(), 0)
	}
	if on && f.edit == nil {
		f.edit = ui.NewField(fl.text())
	}
	fd := f.edit
	if !on || fd == nil {
		fd = ui.NewField(fl.text())
	}
	if c.Focus() == id && !on && f.focused != id { // a click into another field
		f.choose(i)
	}
	if on {
		c.SetFocus(id)
		f.focused = id
	}
	enter, cancel := c.TextField(id, box, fd, ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}, 2)
	if on && enter {
		f.apply()
	}
	if on && cancel {
		f.edit = nil
	}
}

func (f *numForm) keys(c *ui.Ctx) {
	e, ok := c.Key()
	if !ok || e.Kind != ui.EvKey {
		return
	}
	n := len(f.fields)
	switch e.Key {
	case ui.KeyUp:
		f.choose((f.sel + n - 1) % n)
	case ui.KeyDown, ui.KeyTab:
		f.choose((f.sel + 1) % n)
	case ui.KeyLeft, ui.KeyRight:
		if f.edit != nil && !f.edit.All {
			return // typing: the arrows move the caret
		}
		d := 1.0
		if e.Key == ui.KeyLeft {
			d = -1
		}
		f.edit = nil
		f.fields[f.sel].stepBy(d)
	case ui.KeyPgUp, ui.KeyPgDn:
		return
	case ui.KeyEnter:
		if f.edit != nil && f.edit.String() != f.fields[f.sel].text() {
			f.apply()
		} else {
			f.answer(true)
		}
	case ui.KeyEsc:
		f.answer(false)
	default:
		return
	}
	c.Use()
}
