package main

// Menu 1: programs from Gerber. The operator drops the zip of the factory
// files (or a folder) next to the exe, picks it here, checks the numbers of
// the form, and gets one .tap per operation next to the exe: isolation,
// drilling, the mask opening, the V-groove.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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
	{key: "side", group: "Раскладка", label: "сторона", choices: []string{"верх", "низ", "обе"},
		get: func(c *camCfg) float64 { return float64(c.Side) }, put: func(c *camCfg, v float64) { c.Side = int(v) }},
	{key: "nx", label: "плат по X", lo: 1, hi: 20, step: 1,
		get: func(c *camCfg) float64 { return float64(c.CopiesX) }, put: func(c *camCfg, v float64) { c.CopiesX = int(v) }},
	{key: "ny", label: "плат по Y", lo: 1, hi: 20, step: 1,
		get: func(c *camCfg) float64 { return float64(c.CopiesY) }, put: func(c *camCfg, v float64) { c.CopiesY = int(v) }},
	{key: "gap", label: "зазор между", unit: "мм", lo: 0, hi: 30, step: 0.5, digits: 1,
		get: func(c *camCfg) float64 { return c.Gap }, put: func(c *camCfg, v float64) { c.Gap = v }},
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
	b    *board
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
	info := boardInfo(f.b, f.c)
	var body []string
	if w >= camFormW+3+30 { // the board beside the form
		pw := w - camFormW - 3
		if pw > 76 {
			pw = 76
		}
		about := f.about(pw)
		inf := wrapAll(info, pw)
		right := stack(inf, about)
		if ph := rows - len(right) - 2; ph >= 6 {
			right = stack(boardPic(f.b, f.c, nil, pw, ph), right)
		}
		body = beside(f.form(rows), right, 3)
	} else {
		about := f.about(colW(w))
		body = stack(f.form(rows-len(about)-1), about)
	}
	hint := hints(fw, "↑↓", "поле", "← →", "меньше / больше", "Enter", "ввести", "Esc", "отмена")
	if f.edit != nil {
		hint = hints(fw, "Enter", "принять", "Esc", "отменить", "← →", "по числу")
	}
	return screen(fw, h, curHead, body, hint)
}

func wrapAll(ls []string, w int) []string {
	var out []string
	for _, l := range ls {
		out = append(out, wrap(l, w, 2)...)
	}
	return out
}

// boardInfo: the size of the board and which file gave which layer.
func boardInfo(b *board, c camCfg) []string {
	bw, bh := b.box.X1-b.box.X0, b.box.Y1-b.box.Y0
	out := []string{paint(cTitle, b.name) + paint(cHint, "  ") + val(fmt.Sprintf("%.1f x %.1f", bw, bh), "мм")}
	if c.CopiesX*c.CopiesY > 1 {
		out[0] += paint(cHint, fmt.Sprintf("   %d плат: ", c.CopiesX*c.CopiesY)) +
			val(fmt.Sprintf("%.1f x %.1f", bw*float64(c.CopiesX)+c.Gap*float64(c.CopiesX-1), bh*float64(c.CopiesY)+c.Gap*float64(c.CopiesY-1)), "мм")
	}
	names := []struct{ k, s string }{{lyTop, "медь верх"}, {lyBottom, "медь низ"}, {lyTopMask, "маска верх"},
		{lyBotMask, "маска низ"}, {lyOutline, "контур"}}
	for _, n := range names {
		if fs := b.files[n.k]; len(fs) > 0 {
			out = append(out, paint(cHint, padRight(n.s, 11))+filepath.Base(fs[0]))
		}
	}
	if len(b.holes) > 0 {
		sizes := map[float64]int{}
		for _, h := range b.holes {
			sizes[drillSize(h.D)]++
		}
		var ds []float64
		for d := range sizes {
			ds = append(ds, d)
		}
		sort.Float64s(ds)
		var parts []string
		for _, d := range ds {
			parts = append(parts, fmt.Sprintf("%.1fx%d", d, sizes[d]))
		}
		out = append(out, paint(cHint, padRight("сверловка", 11))+fmt.Sprintf("%d отв.: ", len(b.holes))+strings.Join(parts, " "))
	} else {
		out = append(out, paint(cHint, padRight("сверловка", 11))+"нет файла")
	}
	for _, n := range b.notes {
		out = append(out, paint(cAlarm, "▲")+" "+n)
	}
	return out
}

// ==========================
// ===  Board picture     ===
// ==========================

// boardPic: the board seen from above as on the table, w x h cells: copper in
// shades by how much of a cell it covers, the edge in dots, holes, and marks.
func boardPic(b *board, c camCfg, marks []pt, w, h int) []string {
	side, mirror := lyTop, false
	if c.Side == sideBottom || b.copper[lyTop] == nil {
		side, mirror = lyBottom, true
	}
	bw, bh := b.box.X1-b.box.X0, b.box.Y1-b.box.Y0
	if bw <= 0 || bh <= 0 || w < 8 || h < 4 {
		return nil
	}
	sx := math.Min(float64(w-2)/bw, 2*float64(h-2)/bh) // cells per mm across; a cell is twice as high
	cols, rows := int(math.Ceil(bw*sx))+1, int(math.Ceil(bh*sx/2))+1
	// two samples across and four up a cell
	px := 1 / (2 * sx)
	r := newRaster(rect{b.box.X0, b.box.X0 + float64(cols)*2*px, b.box.Y0, b.box.Y0 + float64(rows)*4*px}, px)
	if g := b.copper[side]; g != nil {
		for _, s := range g.shapes {
			r.draw(s)
		}
	}
	cells := make([][]string, rows)
	for j := range cells {
		cells[j] = make([]string, cols)
		for i := range cells[j] {
			n := 0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if r.at(2*i+dx, 4*j+dy) {
						n++
					}
				}
			}
			s := " "
			switch {
			case n >= 7:
				s = "█"
			case n >= 5:
				s = "▓"
			case n >= 3:
				s = "▒"
			case n >= 1:
				s = "░"
			}
			if n > 0 {
				s = paint(cBrown, s)
			}
			cells[j][i] = s
		}
	}
	p := place{b.box, mirror, 0, 0}
	cell := func(q pt) (int, int, bool) { // a table point to its cell, row 0 on top
		i := int(q.X * sx)
		j := rows - 1 - int(q.Y*sx/2)
		return i, j, i >= 0 && j >= 0 && i < cols && j < rows
	}
	for _, l := range b.outline {
		for k := 1; k < len(l); k++ {
			a, e := p.at(l[k-1]), p.at(l[k])
			n := int(math.Hypot(e.X-a.X, e.Y-a.Y)*sx*2) + 1
			for s := 0; s <= n; s++ {
				t := float64(s) / float64(n)
				if i, j, ok := cell(pt{a.X + (e.X-a.X)*t, a.Y + (e.Y-a.Y)*t}); ok && strip(cells[j][i]) == " " {
					cells[j][i] = paint(cGray, "·")
				}
			}
		}
	}
	for _, hl := range b.holes {
		if hl.D*sx < 0.8 {
			continue // smaller than a cell: the pad round it says enough
		}
		if i, j, ok := cell(p.at(hl.P)); ok {
			cells[j][i] = paint(cWhite, "○")
		}
	}
	for _, m := range marks {
		if i, j, ok := cell(m); ok {
			cells[j][i] = paint(cAlarm, "■")
		}
	}
	out := make([]string, rows)
	for j := range cells {
		out[j] = strings.Join(cells[j], "")
	}
	return centerBlock(out)
}

// ==========================
// ===  The command       ===
// ==========================

func cmdGerber(o opts) error {
	srcs := boardSources(exeDir())
	if len(srcs) == 0 {
		message("Нет плат",
			"Рядом с pcbscan.exe нет файлов платы.",
			"Положите туда zip, который KiCad, EasyEDA или Eagle делает для завода (герберы и сверловка), или папку с этими файлами.")
		return nil
	}
	var names []string
	for _, s := range srcs {
		n := filepath.Base(s)
		if st, err := os.Stat(s); err == nil && st.IsDir() {
			n += string(filepath.Separator)
		}
		names = append(names, n)
	}
	i, ok := pick("Платы рядом с pcbscan.exe", names)
	if !ok {
		return errBack
	}
	return gerberFrom(srcs[i])
}

// gerberFrom: the form and the programs for one board, also when its zip was
// dropped onto the exe.
func gerberFrom(path string) error {
	newPage(menuTitle("1"), "Читаю "+filepath.Base(path)+"...")
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	f := &camForm{b: b, c: loadCam(exeDir())}
	ok := false
	run(f.view, func(k Key) bool {
		var done bool
		ok, done = f.key(k)
		return done
	}, 500*time.Millisecond)
	if !ok {
		return nil
	}
	c := f.c
	saveCam(exeDir(), c)
	newPage(menuTitle("1"), "Считаю программы платы "+paint(cTitle, b.name)+". Большая плата - до минуты.")
	t0 := time.Now()
	ops := camRun(b, c, func(s string) { sayHint(s) })
	files, err := camWrite(exeDir(), b.name, ops)
	if err != nil {
		return err
	}
	info, marks := camSummary(ops, files, c)
	info = append(info, "", fmt.Sprintf("Готово за %.0f с. Порядок: карта высот по 1_iso (пункт 4), выровнять по ней "+
		"(пункт 6) каждую программу, резать 1, сверлить 2, нанести маску и вскрыть 3, последним 4.", time.Since(t0).Seconds()))
	camResult(info, func(w, h int) []string { return boardPic(b, c, marks, w, h) })
	return nil
}

// camSummary: a line per program, the drills in one; the warnings under them.
func camSummary(ops []*camOp, files []string, c camCfg) ([]string, []pt) {
	var info []string
	var marks []pt
	var drills []string
	drillMin := 0.0
	drillAt := -1
	for k, op := range ops {
		_, min := op.minutes()
		name := filepath.Base(files[k])
		if strings.HasPrefix(op.file, "2_drill_") && op.file != "2_drill_spot" {
			if drillAt < 0 {
				drillAt = len(info)
				info = append(info, "")
			}
			drills = append(drills, strings.TrimPrefix(op.file, "2_drill_"))
			drillMin += min
			continue
		}
		info = append(info, paint(cOK, "√ ")+paint(cTitle, name)+paint(cHint, fmt.Sprintf("  %s, ~%.0f мин", op.title, math.Ceil(min))))
		for _, wn := range op.warn {
			info = append(info, "  "+paint(cAlarm, "▲ ")+wn)
		}
		if strings.HasPrefix(op.file, "1_iso") && (c.Side != sideBoth || op.file == "1_iso_top") {
			marks = append(marks, op.marks...)
		}
	}
	if drillAt >= 0 {
		base := strings.TrimSuffix(filepath.Base(files[0]), filepath.Ext(files[0]))
		if i := strings.Index(base, "_1_"); i >= 0 {
			base = base[:i]
		} else if i := strings.Index(base, "_2_"); i >= 0 {
			base = base[:i]
		}
		info[drillAt] = paint(cOK, "√ ") + paint(cTitle, base+"_2_drill_*.tap") +
			paint(cHint, fmt.Sprintf("  свёрла %s мм, ~%.0f мин", strings.Join(drills, " "), math.Ceil(drillMin)))
	}
	return info, marks
}

// camResult: the list of programs, scrolled with the arrows when it is long,
// the board beside it when the window is wide.
func camResult(info []string, pic func(w, h int) []string) {
	top := 0
	view := func(w, h int) []string {
		bw := colW(w)
		rows := room(w, h)
		var ls []string
		for _, l := range info {
			ls = append(ls, wrap(l, inner(bw), 4)...)
		}
		n := rows - 1
		if top > len(ls)-n {
			top = len(ls) - n
		}
		if top < 0 {
			top = 0
		}
		vis := ls
		if len(ls) > n {
			vis = append([]string(nil), ls[top:top+n]...)
			if top > 0 {
				vis[0] = center(paint(cHint, "▲ ещё выше"), inner(bw))
			}
			if top+n < len(ls) {
				vis[n-1] = center(paint(cHint, "▼ ещё ниже"), inner(bw))
			}
		}
		body := box(thin, "Программы рядом с pcbscan.exe", vis, bw)
		if pw := w - 4 - bw - 3; pw >= 30 {
			if pw > 70 {
				pw = 70
			}
			body = beside(body, pic(pw, rows), 3)
		}
		keys := []string{"Enter", "в меню", "Esc", "в меню"}
		if len(ls) > n {
			keys = append([]string{"↑↓", "листать"}, keys...)
		}
		return screen(w, h, curHead, body, hints(w, keys...))
	}
	run(view, func(k Key) bool {
		switch k.Code {
		case KeyUp:
			top--
		case KeyDown:
			top++
		case KeyPgUp:
			top -= 10
		case KeyPgDn:
			top += 10
		case KeyEnter, KeyEsc, KeyCtrlC:
			return true
		}
		return false
	}, 500*time.Millisecond)
}
