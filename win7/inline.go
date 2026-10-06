package main

// Numbers right in the lines of a stage: a line with a field shows − and +
// beside its value and takes a typed number, so the sheet, the counts of the
// boards, the step of the map and the numbers of the programs need no form of
// their own. The sheet and the counts wait for Разложить (a new layout drops
// the map and the marks of done work); the step of the map and the numbers of
// the programs take effect at once.

import (
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"

	"pcbscan/gfx"
	"pcbscan/ui"
)

// pending: the sheet and the boards as changed in the lines, not laid out yet.
type pending struct {
	set   bool
	sheet [4]float64 // width, height, margin, gap
	srcs  []string   // every board next to the exe and those of the job, full paths
	n     []int      // how many of each
}

func (u *jobUI) pend() *pending {
	p, j := &u.pendS, u.j
	if p.set {
		return p
	}
	p.sheet = [4]float64{j.W, j.H, j.Margin, j.Gap}
	p.srcs, p.n = nil, nil
	idx := map[string]int{}
	add := func(path string, n int) {
		if _, ok := idx[path]; ok { // already a board of the job, with its count
			return
		}
		idx[path] = len(p.srcs)
		p.srcs = append(p.srcs, path)
		p.n = append(p.n, n)
	}
	for _, s := range j.Srcs { // the boards of the job first, in their order
		add(j.srcPath(s), s.N)
	}
	for _, s := range boardSources(exeDir()) {
		add(s, 0)
	}
	p.set = true
	return p
}

// chosen: the boards the lines put on the sheet, paths next to the exe kept short.
func (p *pending) chosen() []jobSrc {
	var out []jobSrc
	for i, s := range p.srcs {
		if p.n[i] == 0 {
			continue
		}
		if rel, err := filepath.Rel(exeDir(), s); err == nil && !strings.HasPrefix(rel, "..") {
			s = rel
		}
		out = append(out, jobSrc{s, p.n[i]})
	}
	return out
}

// changed: the lines differ from the layout, Разложить is needed.
func (u *jobUI) changed() bool {
	if u.relay {
		return true
	}
	if !u.pendS.set {
		return false
	}
	p, j := u.pend(), u.j
	if p.sheet != [4]float64{j.W, j.H, j.Margin, j.Gap} {
		return true
	}
	ch := p.chosen()
	if len(ch) != len(j.Srcs) {
		return true
	}
	for i, s := range j.Srcs {
		if j.srcPath(ch[i]) != j.srcPath(s) || ch[i].N != s.N {
			return true
		}
	}
	return false
}

// boardsLeft: how many boards the lines put on the sheet.
func (u *jobUI) boardsLeft() int {
	n := 0
	for _, k := range u.pend().n {
		n += k
	}
	return n
}

// applyPend takes the lines into the job, just before a layout; a board new
// to the job is read here.
func (u *jobUI) applyPend() error {
	if !u.pendS.set {
		return nil
	}
	p, j := u.pend(), u.j
	ch := p.chosen()
	same := len(ch) == len(j.Srcs)
	for i := 0; same && i < len(ch); i++ {
		same = j.srcPath(ch[i]) == j.srcPath(j.Srcs[i])
	}
	if !same {
		newBusyPage("задание / Платы", "Читаю платы...")
		old := j.Srcs
		j.Srcs, j.boards = ch, nil
		if err := j.loadBoards(); err != nil {
			j.Srcs, j.boards = old, nil
			j.loadBoards()
			return err
		}
	}
	j.Srcs = ch
	j.W, j.H, j.Margin, j.Gap = p.sheet[0], p.sheet[1], p.sheet[2], p.sheet[3]
	u.pendS.set, u.relay = false, false
	return nil
}

// ==========================
// ===  The lines         ===
// ==========================

// sheetRows: the size of the sheet, its margin and the gap between boards.
func (u *jobUI) sheetRows() []jnode {
	mk := func(label string, i int, lo, hi, step float64, digits int, about string) jnode {
		f := formField{label: label, unit: "мм", lo: lo, hi: hi, step: step, digits: digits,
			get: func() float64 { return u.pend().sheet[i] }, set: func(v float64) { u.pend().sheet[i] = v }}
		return jnode{title: label, field: &f, about: []string{about, "Применится по кнопке Разложить и посчитать."}}
	}
	m := u.pend().sheet[2]
	return []jnode{
		{title: "Лист", head: true},
		mk("ширина X", 0, 20, 600, 5, 0, "Ширина заготовки текстолита."),
		mk("высота Y", 1, 20, 600, 5, 0, "Высота заготовки текстолита."),
		mk("отступ от края", 2, 4, 30, 1, 1, fmt.Sprintf("По краю листа плат нет; ноль Z касается в углу X%s Y%s.", num(m/2), num(m/2))),
		mk("зазор плат", 3, 0, 20, 0.5, 1, "Между контурами плат; V-резом их потом отламывают."),
	}
}

// boardRows: every board next to the exe, how many of it go on the sheet.
func (u *jobUI) boardRows() []jnode {
	j, p := u.j, u.pend()
	out := []jnode{{title: "Платы на листе", head: true}}
	if len(p.srcs) == 0 {
		return append(out, jnode{icon: " ", title: "рядом с pcbscan плат нет", about: []string{
			"Положите рядом с pcbscan.exe zip, который KiCad, EasyEDA или Eagle делает для завода, или папку с этими файлами."}})
	}
	loaded := map[string]*board{}
	if j.loadBoards() == nil {
		for k, s := range j.Srcs {
			loaded[j.srcPath(s)] = j.boards[k]
		}
	}
	for k, src := range p.srcs {
		k := k
		name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		about := []string{"Сколько штук положить на лист, 0 - не брать. Раскладка будет сама по кнопке Разложить и посчитать."}
		if b := loaded[src]; b != nil {
			name = b.name
			about = append([]string{fmt.Sprintf("%.0f x %.0f мм.", b.box.X1-b.box.X0, b.box.Y1-b.box.Y0)}, append(about, boardFiles(b)...)...)
		} else {
			about = append(about, filepath.Base(src)+": прочитается при раскладке.")
		}
		f := formField{label: name, unit: "шт", lo: 0, hi: 50, step: 1, digits: 0,
			get: func() float64 { return float64(u.pend().n[k]) }, set: func(v float64) { u.pend().n[k] = int(v) }}
		out = append(out, jnode{title: name, field: &f, about: about})
	}
	return out
}

// camRows: the numbers of the programs, kept in pcbscan_cam.txt as they change.
func (u *jobUI) camRows() []jnode {
	var out []jnode
	for _, cf := range camFields {
		cf := cf
		if cf.group != "" {
			out = append(out, jnode{title: cf.group, head: true})
		}
		f := formField{label: cf.label, unit: cf.unit, lo: cf.lo, hi: cf.hi, step: cf.step, digits: cf.digits, choices: cf.choices,
			get: func() float64 { return cf.get(&u.c) },
			set: func(v float64) {
				cf.put(&u.c, v)
				saveCam(exeDir(), u.c)
				u.camDirty = true
				if cf.key == "masktest" || cf.key == "isotest" || cf.key == "side" { // the probe strip and the mirror move the boards
					u.relay = true
				}
			}}
		if cf.key == "maskflat" {
			f.show = func(v float64) string {
				if v == 0 {
					return "та же V"
				}
				return ""
			}
		}
		about := []string{"Сохраняется сразу; программы - кнопкой Пересчитать программы."}
		if cf.key == "tip" || cf.key == "angle" || cf.key == "isodepth" {
			about = append([]string{fmt.Sprintf("Ширина реза V-фрезы на глубине изоляции %.3f мм.", u.c.cutWidth(u.c.IsoDepth))}, about...)
		}
		out = append(out, jnode{title: cf.group + cf.label, field: &f, about: about})
	}
	out = append(out, jnode{icon: "·", title: "Числа по умолчанию", act: func() error {
		if confirm("Вернуть числа программ по умолчанию?") {
			u.c = defaultCam()
			u.camDirty, u.relay = true, true
			return saveCam(exeDir(), u.c)
		}
		return nil
	}, about: []string{"Фреза, глубины и подачи, с которыми pcbscan пришёл."}})
	return out
}

// ==========================
// ===  Drawing and keys  ===
// ==========================

// drawField: the line of a number; the chosen one takes the keys.
func (u *jobUI) drawField(c *ui.Ctx, i int, k jnode, r image.Rectangle, on bool) {
	t := c.T
	fl := *k.field
	lw := r.Dx() * 2 / 5
	if fl.choices != nil {
		lw = r.Dx() / 3 // the choices want the room
	}
	lab := c.Body()
	if !on {
		lab.Color = t.Subtext1
	}
	c.LabelIn(image.Rect(r.Min.X+t.Si(14), r.Min.Y, r.Min.X+lw, r.Max.Y), fl.label, lab, 0)
	box := image.Rect(r.Min.X+lw, r.Min.Y+t.Si(5), r.Max.X-t.Si(4), r.Max.Y-t.Si(5))
	id := fmt.Sprintf("kidf/%d/%s", i, k.title)
	choose := func() {
		if u.sel[1] != i {
			u.msg = ""
			u.applyEdit()
			u.sel[1] = i
		}
		u.lvl = 1
	}
	if fl.choices != nil {
		cells := ui.SplitX(box, len(fl.choices), t.Si(4))
		cur := int(fl.get())
		for n, ch := range fl.choices {
			kind := ui.BtnGhost
			if n == cur {
				kind = ui.BtnPrimary
			}
			if c.Button(fmt.Sprintf("%s/%d", id, n), cells[n], ch, kind) {
				choose()
				fl.set(float64(n))
				u.gen++
			}
		}
		return
	}
	bs := box.Dy()
	minus := ui.CutLeft(&box, bs)
	plus := ui.CutRight(&box, bs)
	unitW := 0
	if fl.unit != "" {
		unitW = c.Measure(fl.unit, c.Small()) + t.Si(8)
	}
	unit := ui.CutRight(&box, unitW)
	box = ui.InsetXY(box, t.Si(4), 0)
	if c.IconButton(id+"/-", minus, ui.IconMinus, false) {
		choose()
		u.edit = nil
		fl.stepBy(-1)
		u.gen++
	}
	if c.IconButton(id+"/+", plus, ui.IconPlus, false) {
		choose()
		u.edit = nil
		fl.stepBy(1)
		u.gen++
	}
	if fl.unit != "" {
		c.LabelIn(ui.InsetXY(unit, t.Si(4), 0), fl.unit, c.Small(), 0)
	}
	mine := on && u.lvl == 1
	if mine && (u.edit == nil || u.editID != id) {
		u.edit, u.editID, u.editF = ui.NewField(fl.text()), id, fl
	}
	fd := u.edit
	if !mine {
		fd = ui.NewField(fl.text())
	}
	if c.Focus() == id && !mine {
		if u.focused == id { // left over from the line chosen before: it must not take the keys
			c.SetFocus("")
			u.focused = ""
		} else { // a click into the field of another line
			choose()
		}
	}
	if mine {
		c.SetFocus(id)
		u.focused = id
	}
	enter, cancel := c.TextField(id, box, fd, ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}, 2)
	if mine && enter {
		u.applyEdit()
	}
	if mine && cancel {
		if strings.TrimSpace(u.edit.String()) == fl.text() {
			u.lvl = 0 // nothing typed: Esc goes back as on any line
		}
		u.edit = nil
	}
}

// applyEdit takes the typed number of the chosen line.
func (u *jobUI) applyEdit() {
	if u.edit == nil {
		return
	}
	fl := u.editF
	s := strings.ReplaceAll(strings.TrimSpace(u.edit.String()), ",", ".")
	u.edit = nil
	if s == fl.text() || fl.choices != nil {
		return
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		u.msg = fmt.Sprintf("не число: \"%s\"", s)
		return
	}
	if err := fl.check(v); err != nil {
		u.msg = err.Error()
		return
	}
	fl.set(v)
	u.msg = ""
	u.gen++
}

// fieldKeys: on a line with a number Left and Right step it (before the
// field, which would move its caret).
func (u *jobUI) fieldKeys(c *ui.Ctx, kids []jnode) {
	if u.lvl != 1 || u.sel[1] >= len(kids) || kids[u.sel[1]].field == nil {
		return
	}
	e, ok := c.Key()
	if !ok || e.Kind != ui.EvKey || (e.Key != ui.KeyLeft && e.Key != ui.KeyRight) {
		return
	}
	fl := kids[u.sel[1]].field
	d := 1.0
	if e.Key == ui.KeyLeft {
		d = -1
	}
	u.edit = nil
	fl.stepBy(d)
	u.gen++
	c.Use()
}

// drawHead: a heading between the lines.
func drawHead(c *ui.Ctx, r image.Rectangle, s string) {
	t := c.T
	c.LabelIn(image.Rect(r.Min.X+t.Si(6), r.Min.Y+t.Si(10), r.Max.X, r.Max.Y), s, ui.Style{Kind: ui.Bold, Size: ui.SizeSmall + 1, Color: t.Lavender}, 0)
	c.C.FillRect(image.Rect(r.Min.X+t.Si(6), r.Max.Y-t.Si(4), r.Max.X, r.Max.Y-t.Si(4)+imax(1, t.Si(1))), gfx.Alpha(t.Surface1, 0.5))
}
