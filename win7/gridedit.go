package main

// The map grid before the scan: a form of fields next to the table seen from
// above. Up/Down walk the fields, Left/Right change the value, Enter types a
// number in; the picture follows every change.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// The lines of the form: nine fields, then three buttons.
const (
	gfX0 = iota
	gfX1
	gfY0
	gfY1
	gfSX
	gfSY
	gfNX
	gfNY
	gfMargin
	gfStart
	gfProg
	gfCancel
	gfCount
)

var gridLabels = [gfCount]string{"X от", "X до", "Y от", "Y до", "шаг X", "шаг Y", "точек X", "точек Y",
	"запас за край", "Начать обход", "Как в программе", "Отмена"}

type gridForm struct {
	g, prog Grid
	margin  float64
	sel     int
	edit    *field // a number being typed into the selected field
	msg     string // why the last change was refused
}

func newGridForm(g Grid) *gridForm { return &gridForm{g: g, prog: g} }

// keepStep: the current step, or the program's when there is a single point (no step yet).
func keepStep(cur, prog float64) float64 {
	if cur >= 1 {
		return cur
	}
	if prog >= 1 {
		return prog
	}
	return 10
}

func (f *gridForm) value(i int) string {
	g := f.g
	switch i {
	case gfX0:
		return fmt.Sprintf("%.2f", g.X0)
	case gfX1:
		return fmt.Sprintf("%.2f", g.X1)
	case gfY0:
		return fmt.Sprintf("%.2f", g.Y0)
	case gfY1:
		return fmt.Sprintf("%.2f", g.Y1)
	case gfSX:
		return fmt.Sprintf("%.2f", g.StepX())
	case gfSY:
		return fmt.Sprintf("%.2f", g.StepY())
	case gfNX:
		return strconv.Itoa(g.NX)
	case gfNY:
		return strconv.Itoa(g.NY)
	case gfMargin:
		return mm(f.margin)
	}
	return ""
}

func unit(i int) string {
	if i == gfNX || i == gfNY {
		return ""
	}
	return "мм"
}

// set puts v into field i with the same limits and links the text commands had:
// a new edge keeps the step and changes the count, a step sets the count.
func (f *gridForm) set(i int, v float64) error {
	g, prog := &f.g, f.prog
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("не число")
	}
	switch i {
	case gfX0, gfX1, gfY0, gfY1:
		a, b := g.X0, g.X1
		if i == gfY0 || i == gfY1 {
			a, b = g.Y0, g.Y1
		}
		if i == gfX0 || i == gfY0 {
			a = v
		} else {
			b = v
		}
		if b <= a || a < -10 || b > 600 {
			return fmt.Errorf("край от -10 до 600 мм, начало меньше конца")
		}
		if i == gfX0 || i == gfX1 { // the step stays what it was, the count follows the new length
			sx := keepStep(g.StepX(), prog.StepX())
			g.X0, g.X1, g.NX = a, b, CountFor(a, b, sx)
		} else {
			sy := keepStep(g.StepY(), prog.StepY())
			g.Y0, g.Y1, g.NY = a, b, CountFor(a, b, sy)
		}
	case gfSX, gfSY:
		if v < 1 || v > 200 {
			return fmt.Errorf("шаг от 1 до 200 мм")
		}
		if i == gfSX {
			g.NX = CountFor(g.X0, g.X1, v)
		} else {
			g.NY = CountFor(g.Y0, g.Y1, v)
		}
	case gfNX, gfNY:
		if v < 1 || v > 100 || v != math.Trunc(v) {
			return fmt.Errorf("точек от 1 до 100")
		}
		if i == gfNX {
			g.NX = int(v)
		} else {
			g.NY = int(v)
		}
	case gfMargin:
		if v < 0 || v > 20 {
			return fmt.Errorf("запас от 0 до 20 мм")
		}
		sx, sy := keepStep(g.StepX(), prog.StepX()), keepStep(g.StepY(), prog.StepY())
		g.X0, g.X1, g.Y0, g.Y1 = prog.X0-v, prog.X1+v, prog.Y0-v, prog.Y1+v
		g.NX, g.NY = CountFor(g.X0, g.X1, sx), CountFor(g.Y0, g.Y1, sy)
		f.margin = v
	}
	return nil
}

// nudge is Left/Right on a field: 1 mm for an edge or the margin, 0.5 mm for a
// step, one point for a count. A step change always changes the count by at
// least one point, or the arrow would seem dead.
func (f *gridForm) nudge(i, dir int) error {
	d := float64(dir)
	g := f.g
	switch i {
	case gfX0:
		return f.set(i, g.X0+d)
	case gfX1:
		return f.set(i, g.X1+d)
	case gfY0:
		return f.set(i, g.Y0+d)
	case gfY1:
		return f.set(i, g.Y1+d)
	case gfMargin:
		return f.set(i, f.margin+d)
	case gfNX:
		return f.set(i, float64(g.NX+dir))
	case gfNY:
		return f.set(i, float64(g.NY+dir))
	case gfSX, gfSY:
		lo, hi, n, cur := g.X0, g.X1, g.NX, g.StepX()
		if i == gfSY {
			lo, hi, n, cur = g.Y0, g.Y1, g.NY, g.StepY()
		}
		if n < 2 {
			cur = hi - lo
		}
		want := cur + 0.5*d
		if want < 1 {
			return fmt.Errorf("шаг от 1 до 200 мм")
		}
		k := CountFor(lo, hi, want)
		if k == n {
			k = n - dir // a bigger step is fewer points
		}
		if k < 1 || k > 100 || (k > 1 && (hi-lo)/float64(k-1) < 1) {
			return fmt.Errorf("шаг от 1 до 200 мм")
		}
		if i == gfSX {
			f.g.NX = k
		} else {
			f.g.NY = k
		}
	}
	return nil
}

// key: the result (true: scan with f.g) and true once the form is closed.
func (f *gridForm) key(k Key) (bool, bool) {
	if f.edit != nil {
		done, cancel := f.edit.key(k)
		if cancel {
			f.edit = nil
		}
		if done {
			v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.edit.String()), ",", "."), 64)
			if err != nil {
				f.msg = fmt.Sprintf("не число: \"%s\"", f.edit.String())
			} else if err = f.set(f.sel, v); err != nil {
				f.msg = err.Error()
			} else {
				f.msg = ""
			}
			f.edit = nil
		}
		return false, false
	}
	f.msg = ""
	switch k.Code {
	case KeyUp, KeyBackTab:
		f.sel = (f.sel + gfCount - 1) % gfCount
	case KeyDown, KeyTab:
		f.sel = (f.sel + 1) % gfCount
	case KeyLeft, KeyRight:
		dir := 1
		if k.Code == KeyLeft {
			dir = -1
		}
		if f.sel >= gfStart { // buttons: Left/Right walk them too
			if s := f.sel + dir; s >= gfStart && s < gfCount {
				f.sel = s
			}
			break
		}
		if err := f.nudge(f.sel, dir); err != nil {
			f.msg = err.Error()
		}
	case KeyEnter:
		switch f.sel {
		case gfStart:
			return true, true
		case gfCancel:
			return false, true
		case gfProg:
			f.g, f.margin = f.prog, 0
		default:
			f.edit = newField(f.value(f.sel))
		}
	case KeyEsc, KeyCtrlC:
		return false, true
	case KeyRune:
		if f.sel < gfStart && strings.ContainsRune("0123456789-.,", k.R) {
			f.edit = newField(string(k.R)) // typing starts a new number
		}
	}
	return false, false
}

const formW = 36

// form: the fields with their values right-aligned and the units in grey, the
// chosen one on the selection bar with arrows round its value; the buttons
// under a rule.
func (f *gridForm) form() []string { return f.formOf(true) }

// formOf: with rules false the form is two lines lower, for low windows.
func (f *gridForm) formOf(rules bool) []string {
	in := inner(formW)
	var body []string
	for i := 0; i < gfCount; i++ {
		if rules && (i == gfSX || i == gfStart) {
			body = append(body, sep)
		}
		if i >= gfStart {
			if i == f.sel {
				body = append(body, sel(center("► "+gridLabels[i]+" ◄", in)))
			} else {
				body = append(body, center(paint(cTitle, gridLabels[i]), in))
			}
			continue
		}
		lab := padRight(gridLabels[i], 14)
		switch {
		case i == f.sel && f.edit != nil:
			body = append(body, lab+" "+f.edit.view(9)+" "+paint(cHint, unit(i)))
		case i == f.sel:
			body = append(body, sel(lab+"◄"+fmt.Sprintf("%8s", f.value(i))+" ► "+unit(i)))
		default:
			body = append(body, lab+" "+paint(cTitle, fmt.Sprintf("%8s", f.value(i)))+"   "+paint(cHint, unit(i)))
		}
	}
	return box(thin, "Сетка карты высот", body, formW)
}

// summary: points, steps, time, and what is wrong with the grid.
func (f *gridForm) summary(w int) []string {
	g := f.g
	n := g.NX * g.NY
	out := []string{
		paint(cHint, "точек ") + paint(cTitle, fmt.Sprintf("%d x %d = %d", g.NX, g.NY, n)) +
			paint(cHint, "   шаг ") + val(fmt.Sprintf("%.2f x %.2f", g.StepX(), g.StepY()), "мм") +
			paint(cHint, "   ~") + val(fmt.Sprintf("%.0f", float64(n)*secondsPerPoint/60), "мин"),
	}
	p := f.prog
	if g.X0 > p.X0+1e-6 || g.X1 < p.X1-1e-6 || g.Y0 > p.Y0+1e-6 || g.Y1 < p.Y1-1e-6 {
		out = append(out, paint(cAlarm, "▲")+" сетка меньше реза: за её краем высота берётся с края карты")
	}
	if n > 400 {
		out = append(out, paint(cAlarm, "▲")+" больше 400 точек - это больше часа, уменьшите")
	}
	if f.msg != "" {
		out = append(out, paint(cAlarm, "◄ "+f.msg))
	}
	var lines []string
	for _, l := range out {
		lines = append(lines, wrap(l, w, 2)...)
	}
	return lines
}

func (f *gridForm) view(w, h int) []string {
	cut := rect{f.prog.X0, f.prog.X1, f.prog.Y0, f.prog.Y1}
	rows := room(w, h)
	form := f.form()
	sum := f.summary(colW(w))
	var body []string
	if w >= formW+3+34 { // the table next to the form
		pw := w - formW - 3
		if pw > 76 {
			pw = 76
		}
		pic := fitRows(rows-len(sum)-4, func(r int) []string { return gridViewRows(f.g, &cut, pw, r) })
		body = stack(beside(form, pic, 3), sum)
	} else { // under it, when there is room
		if body = stack(form, sum); len(body) > rows {
			body = stackGap(0, f.formOf(false), sum)
		}
		if pic := fitRows(rows-len(body)-1, func(r int) []string { return gridViewRows(f.g, &cut, w, r) }); pic != nil {
			body = stack(form, sum, pic)
		}
	}
	hint := hints(w, "↑↓", "поле", "← →", "меньше / больше", "Enter", "ввести число", "Esc", "отмена")
	if f.edit != nil {
		hint = hints(w, "Enter", "принять", "Esc", "отменить", "← →", "по числу")
	}
	return screen(w, h, curHead, body, hint)
}

// editGrid lets the operator change the map layout before the scan; false = cancelled.
func editGrid(g Grid) (Grid, bool) {
	f := newGridForm(g)
	ok := false
	run(f.view, func(k Key) bool {
		var done bool
		ok, done = f.key(k)
		return done
	}, 500*time.Millisecond)
	return f.g, ok
}
