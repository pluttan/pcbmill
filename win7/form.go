package main

// A form of numbers: fields walked with the arrows, Left/Right change a value
// by its step, Enter or a digit types a number in, a switch turns by Left/Right.
// A picture beside it follows every change.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type formField struct {
	label, unit  string
	lo, hi, step float64
	digits       int
	choices      []string
	get          func() float64
	set          func(float64)
}

func (f formField) show() string {
	v := f.get()
	if f.choices != nil {
		i := int(v)
		if i < 0 || i >= len(f.choices) {
			return "?"
		}
		return f.choices[i]
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

type numForm struct {
	title  string
	fields []formField
	ok     string // the button that accepts
	sel    int
	edit   *field
	msg    string
	pic    func(w, h int) []string
	about  func() []string
}

func (f *numForm) count() int { return len(f.fields) + 2 }

// key: true, true accepted; false, true cancelled.
func (f *numForm) key(k Key) (bool, bool) {
	nf := len(f.fields)
	if f.edit != nil {
		done, cancel := f.edit.key(k)
		if cancel {
			f.edit = nil
		}
		if done {
			fl := f.fields[f.sel]
			v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.edit.String()), ",", "."), 64)
			if err != nil {
				f.msg = fmt.Sprintf("не число: \"%s\"", f.edit.String())
			} else if err = fl.check(v); err != nil {
				f.msg = err.Error()
			} else {
				fl.set(v)
				f.msg = ""
			}
			f.edit = nil
		}
		return false, false
	}
	f.msg = ""
	switch k.Code {
	case KeyUp, KeyBackTab:
		f.sel = (f.sel + f.count() - 1) % f.count()
	case KeyDown, KeyTab:
		f.sel = (f.sel + 1) % f.count()
	case KeyLeft, KeyRight:
		dir := 1.0
		if k.Code == KeyLeft {
			dir = -1
		}
		if f.sel >= nf {
			f.sel = nf + (f.sel-nf+1)%2
			break
		}
		fl := f.fields[f.sel]
		v := fl.get()
		if fl.choices != nil {
			n := float64(len(fl.choices))
			fl.set(math.Mod(v+dir+n, n))
			break
		}
		nv := math.Round(math.Round((v+dir*fl.step)/fl.step)*fl.step*1e6) / 1e6
		fl.set(math.Max(fl.lo, math.Min(fl.hi, nv)))
	case KeyEnter:
		switch f.sel - nf {
		case 0:
			return true, true
		case 1:
			return false, true
		}
		fl := f.fields[f.sel]
		if fl.choices != nil {
			fl.set(math.Mod(fl.get()+1, float64(len(fl.choices))))
		} else {
			f.edit = newField(fl.show())
		}
	case KeyEsc, KeyCtrlC:
		return false, true
	case KeyRune:
		if f.sel < nf && f.fields[f.sel].choices == nil && strings.ContainsRune("0123456789.,-", k.R) {
			f.edit = newField(string(k.R))
		}
	}
	return false, false
}

const numFormW = 46

func (f *numForm) lines() []string {
	in := inner(numFormW)
	var body []string
	for i, fl := range f.fields {
		lab := padRight("  "+fl.label, 20)
		v := fl.show()
		switch {
		case i == f.sel && f.edit != nil:
			body = append(body, lab+" "+f.edit.view(9)+" "+fl.unit)
		case i == f.sel:
			body = append(body, sel(lab+"◄"+fmt.Sprintf("%9s", v)+" ► "+fl.unit))
		default:
			body = append(body, lab+" "+fmt.Sprintf("%9s", v)+"   "+fl.unit)
		}
	}
	body = append(body, sep)
	for i, b := range []string{f.ok, "Отмена"} {
		if len(f.fields)+i == f.sel {
			body = append(body, sel(center("► "+b+" ◄", in)))
		} else {
			body = append(body, center(b, in))
		}
	}
	if f.about != nil {
		body = append(body, "")
		for _, l := range f.about() {
			body = append(body, wrap(l, in, 2)...)
		}
	}
	if f.msg != "" {
		body = append(body, "", "◄ "+f.msg)
	}
	return body
}

func (f *numForm) view(w, h int) []string {
	fw := w
	w -= 4
	rows := room(fw, h)
	form := box(thin, f.title, f.lines(), numFormW)
	body := form
	if pw := w - numFormW - 3; f.pic != nil && pw >= 24 {
		if pw > 80 {
			pw = 80
		}
		body = beside(form, f.pic(pw, rows-1), 3)
	}
	hint := hints(fw, "↑↓", "поле", "← →", "меньше / больше", "Enter", "ввести", "Esc", "отмена")
	if f.edit != nil {
		hint = hints(fw, "Enter", "принять", "Esc", "отменить")
	}
	return screen(fw, h, curHead, body, hint)
}

// editForm runs the form; true when the operator accepted the values.
func editForm(f *numForm) bool {
	if f.ok == "" {
		f.ok = "Готово"
	}
	ok := false
	run(f.view, func(k Key) bool {
		var done bool
		ok, done = f.key(k)
		return done
	}, 500*time.Millisecond)
	return ok
}
