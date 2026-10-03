package main

// The main screen: the job stage by stage, laid out like yazi. On the left
// the stages, in the middle what the chosen one holds (files to run, what to
// do), on the right the sheet with what the line is about. Right or Enter
// goes in, Left or Esc back out; Enter on a line does it. After every program
// the screen comes back here and the next stage is chosen.

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type jnode struct {
	icon, title, right string
	act                func() error        // what Enter does; nil: a line to read
	about              []string            // under the picture
	ov                 func() sheetOverlay // what the picture shows
	branch             bool                // has lines inside
}

type jobUI struct {
	o      opts
	j      *Job
	c      camCfg
	lvl    int    // 0: the stages, 1: inside one
	sel    [2]int // the line chosen at each level
	quit   bool
	ovKey  string // the cache of the path drawn
	ovPath []cpath
}

func (u *jobUI) mirror() bool { return u.c.Side == sideBottom }

// stageState: the files of a stage and how many of the plain ones are done.
func (u *jobUI) stageState(prefix string) (files []string, done, plain int) {
	files = u.j.stageFiles(prefix)
	for _, f := range files {
		if strings.Contains(f, "_redo") || strings.Contains(f, "_probe") { // a redo and the mask probe are extra
			continue
		}
		plain++
		if _, ok := u.j.Done[strings.TrimSuffix(filepath.Base(f), ".tap")]; ok {
			done++
		}
	}
	return
}

// next: the first stage not done, the one the cursor goes to.
func (u *jobUI) next() int {
	if len(u.j.Places) == 0 {
		return 0
	}
	if _, err := jobMap(u.j); err != nil {
		return 1
	}
	for i, s := range stages {
		if _, done, plain := u.stageState(s.prefix); plain > 0 && done < plain {
			return 2 + i
		}
	}
	return 0
}

func (u *jobUI) roots() []jnode {
	j := u.j
	var out []jnode
	// boards
	n := jnode{title: "Платы", branch: true, icon: "○"}
	if len(j.Places) > 0 {
		n.right = fmt.Sprintf("%d шт", len(j.Places))
		if len(u.j.stageFiles("")) > 0 {
			n.icon = "√"
		}
	} else {
		n.right = "не выбраны"
	}
	n.about = []string{fmt.Sprintf("Лист %sx%s мм, отступ %s, зазор %s.", num(j.W), num(j.H), num(j.Margin), num(j.Gap)),
		"Платы, лист, числа программ; раскладка по настоящему контуру."}
	out = append(out, n)
	// height map
	m := jnode{title: "Карта высот", branch: true, icon: "○", right: "нет"}
	if rows, err := LoadPoints(j.mapPath()); err == nil && len(rows) > 0 {
		m.right = fmt.Sprintf("%d т.", len(rows))
		if _, err := jobMap(j); err == nil {
			m.icon = "√"
		} else {
			m.right = "устарела"
		}
	}
	pts := j.scanPoints(u.mirror())
	m.about = []string{fmt.Sprintf("Точки только над платами: %d, шаг %s мм, ~%.0f мин.", len(pts), num(j.Step),
		float64(len(pts))*secondsPerPoint/60), "♦ угол листа: ноль Z карты и всех смен фрезы."}
	m.ov = func() sheetOverlay { return sheetOverlay{points: pts} }
	out = append(out, m)
	for _, s := range stages {
		files, done, plain := u.stageState(s.prefix)
		st := jnode{title: s.title, branch: true, icon: "○"}
		switch {
		case plain == 0:
			st.right = "нет файлов"
		case done == plain:
			st.icon, st.right = "√", "сделано"
		default:
			st.right = fmt.Sprintf("%d из %d", done, plain)
		}
		st.about = []string{fmt.Sprintf("Файлов: %d.", len(files))}
		out = append(out, st)
	}
	out = append(out, jnode{title: "Сервис", branch: true, icon: "·", about: []string{"Провод, связь, ручной режим, эмуляция."}})
	out = append(out, jnode{title: "Выход", icon: "·", act: func() error {
		if confirm("Выйти из pcbscan?") {
			u.quit = true
		}
		return nil
	}})
	for i := range out {
		if i == u.next() && out[i].icon == "○" {
			out[i].icon = "►"
		}
	}
	return out
}

func (u *jobUI) kids(i int) []jnode {
	j, o := u.j, u.o
	switch i {
	case 0:
		out := []jnode{
			{icon: "·", title: "Выбрать платы", act: u.pickBoards,
				about: []string{"Zip или папки из KiCad, EasyEDA, Eagle рядом с pcbscan.exe, у каждой - сколько штук."}},
			{icon: "·", title: "Лист", right: fmt.Sprintf("%sx%s, %s/%s", num(j.W), num(j.H), num(j.Margin), num(j.Gap)), act: u.sheetForm,
				about: []string{"Размер листа, отступ от края (голая медь, в углу ноль Z) и зазор между платами."}},
			{icon: "·", title: "Числа программ", act: u.numbers,
				about: []string{"Фреза, глубины, подачи, обороты, сверловка, V-рез, сторона."}},
			{icon: "·", title: "Разложить и посчитать", act: u.layout,
				about: []string{"Платы сами встанут на лист, потом программы всех этапов."}},
		}
		if j.loadBoards() == nil {
			for k, s := range j.Srcs {
				b := j.boards[k]
				out = append(out, jnode{icon: " ", title: b.name, right: fmt.Sprintf("x%d  %.0fx%.0f", s.N, b.box.X1-b.box.X0, b.box.Y1-b.box.Y0),
					about: boardFiles(b)})
			}
		}
		return out
	case 1:
		pts := j.scanPoints(u.mirror())
		ov := func() sheetOverlay { return sheetOverlay{points: pts} }
		out := []jnode{
			{icon: "·", title: "Шаг карты", right: num(j.Step) + " мм", act: u.stepForm, ov: ov},
			{icon: "·", title: "Снять карту высот", right: fmt.Sprintf("%d т.", len(pts)), act: u.scan, ov: ov,
				about: []string{"Обход точек над платами, первая - угол листа ♦. Потом по карте выравнивается каждый файл."}},
		}
		if hm, err := jobMap(j); err == nil {
			out = append(out, jnode{icon: "·", title: "Посмотреть карту", act: func() error {
				u := j.used(u.mirror())
				mapResult("pcbscan / Карта высот", hm, &u, []string{"Карта " + filepath.Base(j.mapPath())})
				return nil
			}})
		}
		z := j.zero()
		out = append(out,
			jnode{icon: "·", title: "Z0 в углу листа", act: func() error {
				err := zeroAt(o, "задание / Карта высот / Z0", [2]float64{z.X, z.Y}, "угол листа")
				if err == nil {
					delete(j.Done, "_tool") // which tool it was is not known here
					return j.save()
				}
				return err
			}, about: []string{"Касание и Z0 без программы; инструмент спросят при запуске."}},
			jnode{icon: "·", title: "Проверить провод (МК)", act: func() error { return cmdWire(o) }},
			jnode{icon: "·", title: "Проверить связь с NcStudio", act: func() error { return cmdNcTest(o) }},
			jnode{icon: "·", title: "Ручной режим", act: func() error { return cmdManual(o) }},
			jnode{icon: "·", title: "Найти координаты NcStudio", act: func() error { return cmdFindCoords(o) }},
		)
		return out
	case 2, 3, 4, 5:
		s := stages[i-2]
		var out []jnode
		if s.key == "mask" {
			z := j.zero()
			out = append(out, jnode{icon: " ", title: "Сначала маска", about: []string{
				"Нанесите паяльную маску на платы и высушите её.",
				fmt.Sprintf("Угол листа ♦ X%s Y%s оставьте без маски: там касание Z0.", num(z.X), num(z.Y)),
				"Лист со стола не снимать: карта высот та же."}})
		}
		files, _, _ := u.stageState(s.prefix)
		for _, f := range files {
			f := f
			name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "sheet_"), ".tap")
			title, notes, warn, marks := progNotes(f)
			n := jnode{icon: "○", title: name, right: toolOf(f, u.c)}
			if t, ok := j.Done[strings.TrimSuffix(filepath.Base(f), ".tap")]; ok {
				n.icon, n.right = "√", strings.ReplaceAll(t, "_", " ")
			}
			if title != "" {
				n.about = append(n.about, title+".")
			}
			n.about = append(n.about, notes...)
			for _, w := range warn {
				n.about = append(n.about, "▲ "+w)
			}
			n.ov = func() sheetOverlay { return sheetOverlay{paths: u.pathsOf(f), marks: marks} }
			n.act = func() error { return u.run(f, s.title) }
			out = append(out, n)
		}
		if len(files) > 0 {
			out = append(out, jnode{icon: "·", title: "Повторить в круге", act: func() error { return u.redo(files, s.title) },
				about: []string{"Тот же этап только внутри круга и глубже: новый файл _redo, его можно запустить сразу."}})
		} else {
			out = append(out, jnode{icon: " ", title: "файлов нет", about: []string{"Платы / Разложить и посчитать."}})
		}
		return out
	case 6:
		var out []jnode
		if simM != nil {
			out = append(out, jnode{icon: "·", title: "Эмуляция: станок и неисправности", act: func() error { return cmdSim(o) }})
		}
		out = append(out,
			jnode{icon: "·", title: "Проверить провод (МК)", act: func() error { return cmdWire(o) }},
			jnode{icon: "·", title: "Проверить связь с NcStudio", act: func() error { return cmdNcTest(o) }},
			jnode{icon: "·", title: "Ручной режим", act: func() error { return cmdManual(o) }},
			jnode{icon: "·", title: "Найти координаты NcStudio", act: func() error { return cmdFindCoords(o) }})
		return out
	}
	return nil
}

func boardFiles(b *board) []string {
	out := []string{fmt.Sprintf("%s: %.1f x %.1f мм, отверстий %d.", b.name, b.box.X1-b.box.X0, b.box.Y1-b.box.Y0, len(b.holes))}
	for _, k := range []string{lyTop, lyBottom, lyTopMask, lyBotMask, lyOutline} {
		if fs := b.files[k]; len(fs) > 0 {
			out = append(out, k+": "+filepath.Base(fs[0]))
		}
	}
	for _, n := range b.notes {
		out = append(out, "▲ "+n)
	}
	return out
}

// pathsOf: the paths of a program for the picture, read once per file.
func (u *jobUI) pathsOf(f string) []cpath {
	st, err := os.Stat(f)
	if err != nil {
		return nil
	}
	key := f + st.ModTime().String()
	if u.ovKey != key {
		u.ovKey, u.ovPath = key, nil
		if lines, err := readLines(f); err == nil {
			if op, err := readProg(lines); err == nil {
				u.ovPath = op.paths
			}
		}
	}
	return u.ovPath
}

// ==========================
// ===  The screen        ===
// ==========================

func (u *jobUI) lists() ([]jnode, []jnode) {
	roots := u.roots()
	if u.sel[0] >= len(roots) {
		u.sel[0] = len(roots) - 1
	}
	var kids []jnode
	if roots[u.sel[0]].branch {
		kids = u.kids(u.sel[0])
		if u.sel[1] >= len(kids) {
			u.sel[1] = len(kids) - 1
		}
		if u.sel[1] < 0 {
			u.sel[1] = 0
		}
	}
	return roots, kids
}

// row: one line of a list, w wide: the icon, the title, the note on the right.
func row(n jnode, w int, on bool) string {
	left := " " + n.icon + " " + n.title
	right := n.right
	if right != "" {
		right = " " + right + " "
	}
	if vlen(left)+vlen(right) > w {
		if r := w - vlen(left); r > 4 {
			right, _ = cutv(right, r-1)
			right += " "
		} else {
			right = ""
		}
	}
	if vlen(left)+vlen(right) > w {
		left, _ = cutv(left, w-vlen(right))
	}
	l := left + spaces(w-vlen(left)-vlen(right)) + right
	if on {
		return hilite(l)
	}
	return l
}

func list(ns []jnode, sel, w, h int, on bool) []string {
	out := make([]string, 0, h)
	top := 0
	if sel >= h {
		top = sel - h + 1
	}
	for i := top; i < len(ns) && len(out) < h; i++ {
		out = append(out, row(ns[i], w, on && i == sel))
	}
	for len(out) < h {
		out = append(out, spaces(w))
	}
	return out
}

func (u *jobUI) view(w, h int) []string {
	roots, kids := u.lists()
	cur := roots[u.sel[0]]
	if u.lvl == 1 && len(kids) > 0 {
		cur = kids[u.sel[1]]
	}
	rows := room(w, h)
	if rows > 30 { // the frame stays in the middle of a big window
		rows = 30
	}
	tw := w - 4
	if tw > 116 {
		tw = 116
	}
	// panes: the stages, the lines of the stage, the preview
	sw, kw := 22, 34
	pw := tw - sw - kw - 2
	if pw < 28 { // narrow: no stage column inside, then no preview
		sw = 0
		pw = tw - kw - 1
		if pw < 24 {
			pw, kw = 0, tw
		}
	}
	var stagesCol, kidsCol []string
	if u.lvl == 0 { // the stages alone: the picture gets the room of the middle column
		if pw > 0 {
			pw += kw + 1
		}
		kw2 := kw
		if sw > 0 {
			kw2 = sw + 8
			pw -= 8
		}
		stagesCol = list(roots, u.sel[0], kw2, rows, true)
	} else {
		if sw > 0 {
			stagesCol = list(roots, u.sel[0], sw, rows, false)
		}
		kidsCol = list(kids, u.sel[1], kw, rows, true)
	}
	var prev []string
	if pw > 0 {
		var ov sheetOverlay
		if cur.ov != nil {
			ov = cur.ov()
		}
		var about []string
		for _, l := range cur.about {
			about = append(about, wrap(l, pw, 2)...)
		}
		ph := rows - len(about) - 1
		if ph > 4 && len(u.j.Places) > 0 {
			prev = stack(sheetPic(u.j, u.mirror(), ov, pw, ph), about)
		} else {
			prev = about
		}
		for len(prev) < rows {
			prev = append(prev, "")
		}
		if len(prev) > rows {
			prev = prev[:rows]
		}
	}
	bar := make([]string, rows)
	for i := range bar {
		bar[i] = "│"
	}
	var body []string
	switch {
	case u.lvl == 0 && pw > 0:
		body = beside(beside(stagesCol, bar, 0), prev, 1)
	case u.lvl == 0:
		body = stagesCol
	case sw > 0 && pw > 0:
		body = beside(beside(beside(beside(stagesCol, bar, 0), kidsCol, 0), bar, 0), prev, 1)
	case pw > 0:
		body = beside(beside(kidsCol, bar, 0), prev, 1)
	default:
		body = kidsCol
	}
	title := "задание / " + roots[u.sel[0]].title
	if u.lvl == 0 {
		title = "задание"
	}
	hint := hints(w, "↑↓", "выбор", "→ Enter", "открыть", "← Esc", "назад")
	if u.lvl == 0 {
		hint = hints(w, "↑↓", "выбор", "→ Enter", "открыть", "Esc", "выход")
	}
	return screen(w, h, title, body, hint)
}

// key: the action chosen, when Enter fell on one.
func (u *jobUI) key(k Key) (func() error, bool) {
	roots, kids := u.lists()
	n := len(roots)
	if u.lvl == 1 {
		n = len(kids)
	}
	move := func(d int) {
		if n > 0 {
			u.sel[u.lvl] = (u.sel[u.lvl] + d + n) % n
		}
	}
	switch k.Code {
	case KeyUp:
		move(-1)
	case KeyDown, KeyTab:
		move(1)
	case KeyHome:
		u.sel[u.lvl] = 0
	case KeyEnd:
		u.sel[u.lvl] = n - 1
	case KeyRight, KeyEnter:
		if u.lvl == 0 {
			r := roots[u.sel[0]]
			if r.branch {
				u.lvl, u.sel[1] = 1, 0
				return nil, false
			}
			if k.Code == KeyEnter && r.act != nil {
				return r.act, true
			}
			return nil, false
		}
		if len(kids) > 0 && kids[u.sel[1]].act != nil && k.Code == KeyEnter {
			return kids[u.sel[1]].act, true
		}
	case KeyLeft, KeyEsc:
		if u.lvl == 1 {
			u.lvl = 0
			return nil, false
		}
		if k.Code == KeyEsc {
			return roots[len(roots)-1].act, true // Выход
		}
	case KeyCtrlC:
		u.quit = true
		return nil, true
	}
	return nil, false
}

// jobMain is the program: the job screen until Выход.
func jobMain(o opts) {
	j, err := loadJob(filepath.Join(exeDir(), "job"))
	if err != nil {
		alert(err)
		j = newJob(filepath.Join(exeDir(), "job"))
	}
	u := &jobUI{o: o, j: j, c: loadCam(exeDir())}
	if err := j.loadBoards(); err != nil {
		alert(fmt.Errorf("платы задания не читаются: %v", err))
		j.Srcs, j.Places, j.boards = nil, nil, nil
	}
	u.sel[0] = u.next()
	for !u.quit {
		cur, curHead = nil, ""
		var act func() error
		run(u.view, func(k Key) bool {
			a, done := u.key(k)
			act = a
			return done
		}, 500*time.Millisecond)
		if act == nil {
			continue
		}
		roots := u.roots()
		curHead = "задание / " + roots[u.sel[0]].title
		if err := act(); err != nil && !errors.Is(err, errBack) {
			alert(err)
		}
	}
}

// ==========================
// ===  Actions           ===
// ==========================

// pickBoards: the boards next to the exe with how many of each.
func (u *jobUI) pickBoards() error {
	srcs := boardSources(exeDir())
	if len(srcs) == 0 {
		message("Нет плат", "Рядом с pcbscan.exe нет файлов плат.",
			"Положите туда zip, который KiCad, EasyEDA или Eagle делает для завода, или папку с этими файлами.")
		return nil
	}
	count := make([]int, len(srcs))
	for i, s := range srcs {
		for _, js := range u.j.Srcs {
			if u.j.srcPath(js) == s {
				count[i] = js.N
			}
		}
	}
	sel := 0
	view := func(w, h int) []string {
		var ls []string
		bw := colW(w)
		for i, s := range srcs {
			name := filepath.Base(s)
			l := fmt.Sprintf(" %s %-*s %3s ", map[bool]string{true: "■", false: "○"}[count[i] > 0], bw-14, name, "")
			if count[i] > 0 {
				l = fmt.Sprintf(" ■ %-*s x%-3d", bw-14, name, count[i])
			}
			if i == sel {
				l = hilite(l)
			}
			ls = append(ls, l)
		}
		ls = append(ls, "", "Сколько штук: ← → или цифра; 0 - не брать.")
		return screen(w, h, "задание / Платы / Выбрать", box(thin, "Платы рядом с pcbscan.exe", ls, bw),
			hints(w, "↑↓", "выбор", "← →", "штук", "Enter", "готово", "Esc", "отмена"))
	}
	ok := false
	run(view, func(k Key) bool {
		switch k.Code {
		case KeyUp:
			sel = (sel + len(srcs) - 1) % len(srcs)
		case KeyDown, KeyTab:
			sel = (sel + 1) % len(srcs)
		case KeyRight:
			if count[sel] < 50 {
				count[sel]++
			}
		case KeyLeft:
			if count[sel] > 0 {
				count[sel]--
			}
		case KeyRune:
			if k.R == ' ' {
				if count[sel] > 0 {
					count[sel] = 0
				} else {
					count[sel] = 1
				}
			} else if k.R >= '0' && k.R <= '9' {
				count[sel] = int(k.R - '0')
			}
		case KeyEnter:
			ok = true
			return true
		case KeyEsc, KeyCtrlC:
			return true
		}
		return false
	}, 500*time.Millisecond)
	if !ok {
		return nil
	}
	var chosen []jobSrc
	for i, s := range srcs {
		if count[i] > 0 {
			p := s
			if rel, err := filepath.Rel(exeDir(), s); err == nil && !strings.HasPrefix(rel, "..") {
				p = rel
			}
			chosen = append(chosen, jobSrc{p, count[i]})
		}
	}
	if len(chosen) == 0 {
		message("Платы", "Ни одной платы не выбрано.")
		return nil
	}
	newPage("задание / Платы", "Читаю платы...")
	j := u.j
	old := j.Srcs
	j.Srcs, j.boards = chosen, nil
	if err := j.loadBoards(); err != nil {
		j.Srcs, j.boards = old, nil
		j.loadBoards()
		return err
	}
	return u.layout()
}

func (u *jobUI) sheetForm() error {
	j := u.j
	w, h, m, g := j.W, j.H, j.Margin, j.Gap
	f := &numForm{title: "Лист", ok: "Разложить", fields: []formField{
		{label: "ширина X", unit: "мм", lo: 20, hi: 600, step: 5, digits: 0, get: func() float64 { return w }, set: func(v float64) { w = v }},
		{label: "высота Y", unit: "мм", lo: 20, hi: 600, step: 5, digits: 0, get: func() float64 { return h }, set: func(v float64) { h = v }},
		{label: "отступ от края", unit: "мм", lo: 4, hi: 30, step: 1, digits: 1, get: func() float64 { return m }, set: func(v float64) { m = v }},
		{label: "зазор плат", unit: "мм", lo: 0, hi: 20, step: 0.5, digits: 1, get: func() float64 { return g }, set: func(v float64) { g = v }},
	}, about: func() []string {
		return []string{fmt.Sprintf("Ноль Z касается в углу X%s Y%s: медь там без маски.", num(m/2), num(m/2)),
			"Зазор - между контурами плат; V-резом их потом отламывают."}
	}}
	if !editForm(f) {
		return nil
	}
	j.W, j.H, j.Margin, j.Gap = w, h, m, g
	return u.layout()
}

func (u *jobUI) stepForm() error {
	s := u.j.Step
	f := &numForm{title: "Шаг карты высот", fields: []formField{
		{label: "шаг", unit: "мм", lo: 3, hi: 50, step: 1, digits: 1, get: func() float64 { return s }, set: func(v float64) { s = v }},
	}, about: func() []string {
		old := u.j.Step
		u.j.Step = s
		n := len(u.j.scanPoints(u.mirror()))
		u.j.Step = old
		return []string{fmt.Sprintf("точек %d, ~%.0f мин", n, float64(n)*secondsPerPoint/60)}
	}, pic: func(w, h int) []string {
		old := u.j.Step
		u.j.Step = s
		defer func() { u.j.Step = old }()
		return sheetPic(u.j, u.mirror(), sheetOverlay{points: u.j.scanPoints(u.mirror())}, w, h)
	}}
	if !editForm(f) {
		return nil
	}
	u.j.Step = s
	return u.j.save()
}

func (u *jobUI) numbers() error {
	f := &camForm{c: u.c}
	ok := false
	run(f.view, func(k Key) bool {
		var done bool
		ok, done = f.key(k)
		return done
	}, 500*time.Millisecond)
	if !ok {
		return nil
	}
	u.c = f.c
	if err := saveCam(exeDir(), u.c); err != nil {
		return err
	}
	if len(u.j.Places) > 0 && confirm("Пересчитать программы с новыми числами?") {
		return u.programs(false)
	}
	return nil
}

// layout puts the boards on the sheet again and makes the programs; a map
// and marks of done work of the old layout go, after a question.
func (u *jobUI) layout() error {
	j := u.j
	if len(j.Srcs) == 0 {
		message("Платы", "Сначала выберите платы.")
		return nil
	}
	_, mapErr := os.Stat(j.mapPath())
	if mapErr == nil || len(j.Done) > 0 {
		if !confirm("Разложить заново?", "Карта высот и отметки о сделанном старой раскладки не подойдут и будут сброшены.") {
			return nil
		}
	}
	return u.programs(true)
}

// programs: the layout (when asked) and the programs of every stage.
func (u *jobUI) programs(relayout bool) error {
	j := u.j
	p := newPage("задание / Платы", "Раскладываю и считаю программы...")
	if relayout {
		if err := j.layout(u.mirror(), u.c.MaskTest); err != nil {
			return err
		}
		if _, err := os.Stat(j.mapPath()); err == nil {
			os.Rename(j.mapPath(), filepath.Join(j.Dir, "scan_old.csv"))
		}
		j.Done = map[string]string{}
	}
	if err := j.save(); err != nil {
		return err
	}
	t0 := time.Now()
	ops, err := j.cam(u.c, func(s string) { sayHint(s) })
	if err != nil {
		return err
	}
	if err := j.writeProgs(ops); err != nil {
		return err
	}
	for k := range j.Done { // the programs changed: what was done with the old ones is forgotten
		if strings.HasPrefix(k, "sheet_") {
			delete(j.Done, k)
		}
	}
	if err := j.save(); err != nil {
		return err
	}
	var info []string
	used := j.used(u.mirror())
	info = append(info, fmt.Sprintf("Плат %d на листе %sx%s, заняли X %.0f..%.0f Y %.0f..%.0f. Посчитано за %.0f с.",
		len(j.Places), num(j.W), num(j.H), used.X0, used.X1, used.Y0, used.Y1, time.Since(t0).Seconds()))
	for _, op := range ops {
		_, min := op.minutes()
		info = append(info, fmt.Sprintf("√ %s  %s, ~%.0f мин", filepath.Base(j.progPath(op.file)), op.tool, math.Ceil(min)))
		for _, w := range op.warn {
			info = append(info, "  ▲ "+w)
		}
	}
	info = append(info, "", "Дальше: Карта высот.")
	p.set(func() {
		p.head = "Программы листа"
		p.info = info
		p.nolog = true
		p.pic = func(w, h int) []string {
			var marks []pt
			for _, op := range ops {
				if strings.HasPrefix(op.file, "1_") {
					marks = op.marks
				}
			}
			return sheetPic(j, u.mirror(), sheetOverlay{marks: marks}, w, h)
		}
	})
	p.wait()
	u.lvl, u.sel[0] = 0, u.next()
	return nil
}

func (u *jobUI) scan() error {
	j := u.j
	pts := j.scanPoints(u.mirror())
	if len(pts) < 2 {
		message("Карта высот", "Сначала платы: Платы / Разложить и посчитать.")
		return nil
	}
	tmp := filepath.Join(j.Dir, "scan.new.csv")
	used := j.used(u.mirror())
	mirror := u.mirror()
	hm, err := scanRun(u.o, "задание / Карта высот", pts, tmp, &used,
		func(w, r int) []string { return sheetPic(j, mirror, sheetOverlay{points: pts}, w, r) },
		func(done []Point, blink bool, w, r int) []string {
			left := make([][2]float64, 0, len(pts))
			for _, q := range pts[len(done):] {
				left = append(left, q)
			}
			var at []pt
			if len(done) > 0 && blink {
				at = []pt{{done[len(done)-1].X, done[len(done)-1].Y}}
			}
			return sheetPic(j, mirror, sheetOverlay{points: left, marks: at}, w, r)
		})
	if err != nil || hm == nil {
		return err
	}
	if err := os.Rename(tmp, j.mapPath()); err != nil {
		return err
	}
	delete(j.Done, "_tool") // NcStudio Z is the start height now: every program wants a Z0 first
	j.mark("map")
	if err := j.save(); err != nil {
		return err
	}
	message("Карта высот готова", "Дальше: Фрезеровка.", "Каждый файл выравнивается по карте перед запуском.")
	u.lvl, u.sel[0] = 0, u.next()
	return nil
}

// run: one program, then back to the screen with the next stage chosen.
func (u *jobUI) run(f, stage string) error {
	done, err := runProg(u.o, u.j, u.c, f, stage)
	if err != nil || !done {
		return err
	}
	prefix := strings.SplitN(strings.TrimPrefix(filepath.Base(f), "sheet_"), "_", 2)[0] + "_"
	_, d, plain := u.stageState(prefix)
	if d == plain {
		message(stage+": сделано", stage+" сделана. Ожидаю дальше.")
		u.lvl, u.sel[0] = 0, u.next()
	} else {
		message(stage, filepath.Base(f)+" сделан. Ожидаю дальше: следующий файл этапа.")
		u.sel[1]++
	}
	return nil
}

// redo: a stage again inside a circle, deeper.
func (u *jobUI) redo(files []string, stage string) error {
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "sheet_"), ".tap"))
	}
	used := u.j.used(u.mirror())
	src := 0.0
	for i, f := range files { // the main file of the stage, not the mask probe
		if !strings.Contains(f, "_probe") {
			src = float64(i)
			break
		}
	}
	x, y, r, d := math.Round((used.X0+used.X1)/2), math.Round((used.Y0+used.Y1)/2), 10.0, 0.05
	f := &numForm{title: "Повторить в круге", ok: "Сделать файл", fields: []formField{
		{label: "файл", choices: names, get: func() float64 { return src }, set: func(v float64) { src = v }},
		{label: "центр X", unit: "мм", lo: 0, hi: u.j.W, step: 1, digits: 1, get: func() float64 { return x }, set: func(v float64) { x = v }},
		{label: "центр Y", unit: "мм", lo: 0, hi: u.j.H, step: 1, digits: 1, get: func() float64 { return y }, set: func(v float64) { y = v }},
		{label: "радиус", unit: "мм", lo: 1, hi: 500, step: 1, digits: 1, get: func() float64 { return r }, set: func(v float64) { r = v }},
		{label: "глубже на", unit: "мм", lo: 0, hi: 0.5, step: 0.01, digits: 2, get: func() float64 { return d }, set: func(v float64) { d = v }},
	}, about: func() []string {
		return []string{"Пути файла внутри круга, на столько глубже; новый файл рядом, с _redo в имени."}
	}, pic: func(w, h int) []string {
		return sheetPic(u.j, u.mirror(), sheetOverlay{paths: u.pathsOf(files[int(src)]), circle: &[3]float64{x, y, r}}, w, h)
	}}
	if !editForm(f) {
		return nil
	}
	dst, n, err := redoProg(files[int(src)], pt{x, y}, r, d)
	if err != nil {
		return err
	}
	if !confirm(fmt.Sprintf("Файл %s: %d путей. Запустить сейчас?", filepath.Base(dst), n)) {
		return nil
	}
	_, err = runProg(u.o, u.j, u.c, dst, stage)
	return err
}
