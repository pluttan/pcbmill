package main

// The numbers of the programs: the tool, depths, feeds, drilling, the
// V-groove, the side; kept next to the exe between runs.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ==========================
// ===  Settings          ===
// ==========================

// camField: one line of the form. choices make it a switch walked by the arrows.
type camField struct {
	key, label, unit string
	group            string // a heading over the field when it changes
	lo, hi, step     float64
	digits           int
	choices          []string
	get              func(*camCfg) float64
	put              func(*camCfg, float64)
}

var camFields = []camField{
	{key: "tip", group: "Фреза V (изоляция, маска)", label: "кончик", unit: "мм", lo: 0.02, hi: 1, step: 0.05, digits: 2,
		get: func(c *camCfg) float64 { return c.Tip }, put: func(c *camCfg, v float64) { c.Tip = v }},
	{key: "angle", label: "угол", unit: "°", lo: 10, hi: 90, step: 5,
		get: func(c *camCfg) float64 { return c.Angle }, put: func(c *camCfg, v float64) { c.Angle = v }},
	{key: "spindle", label: "обороты", unit: "об/мин", lo: 1000, hi: 30000, step: 1000,
		get: func(c *camCfg) float64 { return c.Spindle }, put: func(c *camCfg, v float64) { c.Spindle = v }},
	{key: "isodepth", group: "Изоляция дорожек", label: "глубина", unit: "мм", lo: 0.02, hi: 0.5, step: 0.01, digits: 2,
		get: func(c *camCfg) float64 { return c.IsoDepth }, put: func(c *camCfg, v float64) { c.IsoDepth = v }},
	{key: "passes", label: "обходов", lo: 1, hi: 8, step: 1,
		get: func(c *camCfg) float64 { return float64(c.Passes) }, put: func(c *camCfg, v float64) { c.Passes = int(v) }},
	{key: "isofeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.IsoFeed }, put: func(c *camCfg, v float64) { c.IsoFeed = v }},
	{key: "plunge", label: "врезание", unit: "мм/мин", lo: 10, hi: 1000, step: 10,
		get: func(c *camCfg) float64 { return c.Plunge }, put: func(c *camCfg, v float64) { c.Plunge = v }},
	{key: "maskdepth", group: "Вскрытие маски", label: "глубина", unit: "мм", lo: 0.005, hi: 0.3, step: 0.005, digits: 3,
		get: func(c *camCfg) float64 { return c.MaskDepth }, put: func(c *camCfg, v float64) { c.MaskDepth = v }},
	{key: "maskflat", label: "фреза", unit: "мм", lo: 0, hi: 3, step: 0.1, digits: 1,
		get: func(c *camCfg) float64 { return c.MaskFlat }, put: func(c *camCfg, v float64) { c.MaskFlat = v }},
	{key: "maskfeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.MaskFeed }, put: func(c *camCfg, v float64) { c.MaskFeed = v }},
	{key: "masktest", label: "проба глубины", choices: []string{"нет", "да"},
		get: func(c *camCfg) float64 { return b2f(c.MaskTest) }, put: func(c *camCfg, v float64) { c.MaskTest = v != 0 }},
	{key: "drill", group: "Сверловка", label: "как", choices: []string{"насквозь", "наколка", "оба"},
		get: func(c *camCfg) float64 { return float64(c.DrillMode) }, put: func(c *camCfg, v float64) { c.DrillMode = int(v) }},
	{key: "thick", label: "толщина платы", unit: "мм", lo: 0.2, hi: 4, step: 0.1, digits: 1,
		get: func(c *camCfg) float64 { return c.Thickness }, put: func(c *camCfg, v float64) { c.Thickness = v }},
	{key: "drillfeed", label: "врезание сверла", unit: "мм/мин", lo: 5, hi: 500, step: 5,
		get: func(c *camCfg) float64 { return c.DrillFeed }, put: func(c *camCfg, v float64) { c.DrillFeed = v }},
	{key: "vdepth", group: "V-рез по контуру", label: "глубина", unit: "мм", lo: 0.1, hi: 4, step: 0.1, digits: 2,
		get: func(c *camCfg) float64 { return c.VDepth }, put: func(c *camCfg, v float64) { c.VDepth = v }},
	{key: "vstep", label: "за проход", unit: "мм", lo: 0.05, hi: 2, step: 0.05, digits: 2,
		get: func(c *camCfg) float64 { return c.VStep }, put: func(c *camCfg, v float64) { c.VStep = v }},
	{key: "vfeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.VFeed }, put: func(c *camCfg, v float64) { c.VFeed = v }},
	{key: "side", group: "Сторона", label: "медь сверху", choices: []string{"верх", "низ"},
		get: func(c *camCfg) float64 { return float64(c.Side) }, put: func(c *camCfg, v float64) { c.Side = int(v) }},
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

const camCfgFile = "pcbscan_cam.txt"

// loadCam: the numbers of the last run, the defaults for what is missing.
func loadCam(dir string) camCfg {
	c := defaultCam()
	b, err := os.ReadFile(filepath.Join(dir, camCfgFile))
	if err != nil {
		return c
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		for _, f := range camFields {
			if f.key == strings.TrimSpace(k) && f.valid(x) == nil {
				f.put(&c, x)
			}
		}
	}
	return c
}

func saveCam(dir string, c camCfg) error {
	var b strings.Builder
	b.WriteString("# pcbscan: числа программ из Gerber, последний запуск\r\n")
	for _, f := range camFields {
		fmt.Fprintf(&b, "%s=%s\r\n", f.key, strconv.FormatFloat(f.get(&c), 'f', -1, 64))
	}
	return os.WriteFile(filepath.Join(dir, camCfgFile), []byte(b.String()), 0644)
}

func (f camField) valid(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("не число")
	}
	if f.choices != nil {
		if v < 0 || int(v) >= len(f.choices) || v != math.Trunc(v) {
			return fmt.Errorf("нет такого выбора")
		}
		return nil
	}
	if v < f.lo-1e-9 || v > f.hi+1e-9 {
		return fmt.Errorf("%s: от %s до %s %s", f.label, num(f.lo), num(f.hi), f.unit)
	}
	if f.digits == 0 && v != math.Trunc(v) {
		return fmt.Errorf("%s: целое число", f.label)
	}
	return nil
}

func (f camField) show(c *camCfg) string {
	v := f.get(c)
	if f.choices != nil {
		return f.choices[int(v)]
	}
	if f.key == "maskflat" && v == 0 {
		return "та же V"
	}
	return strconv.FormatFloat(v, 'f', f.digits, 64)
}

// ==========================
// ===  The form          ===
// ==========================

const (
	cbMake = iota
	cbDefault
	cbCancel
	cbCount
)

var camButtons = [cbCount]string{"Сделать программы", "Числа по умолчанию", "Отмена"}

type camForm struct {
	c    camCfg
	sel  int // fields, then buttons
	edit *field
	msg  string
}

func (f *camForm) count() int { return len(camFields) + cbCount }

// key: true and true when the programs are to be made, false and true for back.
func (f *camForm) key(k Key) (bool, bool) {
	nf := len(camFields)
	if f.edit != nil {
		done, cancel := f.edit.key(k)
		if cancel {
			f.edit = nil
		}
		if done {
			fl := camFields[f.sel]
			v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.edit.String()), ",", "."), 64)
			if err != nil {
				f.msg = fmt.Sprintf("не число: \"%s\"", f.edit.String())
			} else if err = fl.valid(v); err != nil {
				f.msg = err.Error()
			} else {
				fl.put(&f.c, v)
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
	case KeyHome:
		f.sel = 0
	case KeyEnd:
		f.sel = nf
	case KeyLeft, KeyRight:
		dir := 1.0
		if k.Code == KeyLeft {
			dir = -1
		}
		if f.sel >= nf {
			if s := f.sel + int(dir); s >= nf && s < f.count() {
				f.sel = s
			}
			break
		}
		fl := camFields[f.sel]
		v := fl.get(&f.c)
		if fl.choices != nil {
			n := float64(len(fl.choices))
			fl.put(&f.c, math.Mod(v+dir+n, n))
			break
		}
		nv := math.Round((v+dir*fl.step)/fl.step) * fl.step // back onto the steps
		nv = math.Round(nv*1e6) / 1e6
		if nv < fl.lo {
			nv = fl.lo
		}
		if nv > fl.hi {
			nv = fl.hi
		}
		fl.put(&f.c, nv)
	case KeyEnter:
		switch f.sel - nf {
		case cbMake:
			return true, true
		case cbDefault:
			f.c = defaultCam()
		case cbCancel:
			return false, true
		default:
			if camFields[f.sel].choices != nil {
				fl := camFields[f.sel]
				fl.put(&f.c, math.Mod(fl.get(&f.c)+1, float64(len(fl.choices))))
			} else {
				f.edit = newField(camFields[f.sel].show(&f.c))
			}
		}
	case KeyEsc, KeyCtrlC:
		return false, true
	case KeyRune:
		if f.sel < nf && camFields[f.sel].choices == nil && strings.ContainsRune("0123456789.,", k.R) {
			f.edit = newField(string(k.R)) // typing starts a new number
		}
	}
	return false, false
}

const camFormW = 44

// lines: the form body, with the index of the body line of the selection.
func (f *camForm) lines() ([]string, int) {
	in := inner(camFormW)
	var body []string
	at := 0
	for i, fl := range camFields {
		if fl.group != "" {
			if i > 0 {
				body = append(body, "")
			}
			body = append(body, paint(cLightCyan, fl.group))
		}
		if i == f.sel {
			at = len(body)
		}
		lab := padRight("  "+fl.label, 18)
		v := fl.show(&f.c)
		switch {
		case i == f.sel && f.edit != nil:
			body = append(body, lab+" "+f.edit.view(9)+" "+paint(cHint, fl.unit))
		case i == f.sel:
			body = append(body, sel(lab+"◄"+fmt.Sprintf("%9s", v)+" ► "+fl.unit))
		default:
			body = append(body, lab+" "+paint(cTitle, fmt.Sprintf("%9s", v))+"   "+paint(cHint, fl.unit))
		}
	}
	body = append(body, sep)
	for i, b := range camButtons {
		if len(camFields)+i == f.sel {
			at = len(body)
			body = append(body, sel(center("► "+b+" ◄", in)))
		} else {
			body = append(body, center(paint(cTitle, b), in))
		}
	}
	return body, at
}

// form: the part of the form that fits rows lines, scrolled to the selection.
func (f *camForm) form(rows int) []string {
	body, at := f.lines()
	n := rows - 1 // the title line of the panel
	if n < 5 {
		n = 5
	}
	if len(body) > n {
		top := at - n/2
		if top < 0 {
			top = 0
		}
		if top > len(body)-n {
			top = len(body) - n
		}
		up, down := top > 0, top+n < len(body)
		body = append([]string(nil), body[top:top+n]...)
		if up {
			body[0] = center(paint(cHint, "▲ ещё выше"), inner(camFormW))
		}
		if down {
			body[n-1] = center(paint(cHint, "▼ ещё ниже"), inner(camFormW))
		}
	}
	return box(thin, "Числа программ", body, camFormW)
}

// about: what the numbers give, and what is wrong with them.
func (f *camForm) about(w int) []string {
	c := f.c
	var out []string
	add := func(s string) { out = append(out, wrap(s, w, 2)...) }
	iw := c.cutWidth(c.IsoDepth)
	add(paint(cHint, "рез изоляции ") + val(fmt.Sprintf("%.3f", iw), "мм") +
		paint(cHint, ", полоса ") + val(fmt.Sprintf("%.2f", iw+iw*(1-overlap)*float64(c.Passes-1)), "мм"))
	add(paint(cHint, "след на маске ") + val(fmt.Sprintf("%.3f", c.maskWidth(c.MaskDepth)), "мм"))
	if c.VStep > c.VDepth {
		add(paint(cAlarm, "▲") + " V-рез: за проход больше всей глубины")
	}
	if c.VDepth >= c.Thickness {
		add(paint(cAlarm, "▲") + " V-рез глубже толщины платы: прорежет стол")
	}
	if f.msg != "" {
		add(paint(cAlarm, "◄ "+f.msg))
	}
	return out
}

func (f *camForm) view(w, h int) []string {
	fw := w
	w -= 4
	rows := room(fw, h)
	about := f.about(colW(w))
	body := stack(f.form(rows-len(about)-1), about)
	hint := hints(fw, "↑↓", "поле", "← →", "меньше / больше", "Enter", "ввести", "Esc", "отмена")
	if f.edit != nil {
		hint = hints(fw, "Enter", "принять", "Esc", "отменить", "← →", "по числу")
	}
	return screen(fw, h, "задание / Платы / Числа программ", body, hint)
}

func wrapAll(ls []string, w int) []string {
	var out []string
	for _, l := range ls {
		out = append(out, wrap(l, w, 2)...)
	}
	return out
}
