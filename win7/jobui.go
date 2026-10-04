package main

// The main screen: the job stage by stage, laid out like yazi. On the left
// the stages, in the middle what the chosen one holds (files to run, what to
// do), on the right the sheet with what the line is about. Right or Enter
// goes in, Left or Esc back out; Enter on a line does it. After every program
// the screen comes back here and the next stage is chosen.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pcbscan/ui"
)

type jnode struct {
	icon, title, right string
	act                func() error        // what Enter does; nil: a line to read
	about              []string            // under the picture
	ov                 func() sheetOverlay // what the picture shows
	branch             bool                // has lines inside
	mapView            bool                // the picture is the height map
	field              *formField          // a number edited right in the line
	head               bool                // a heading between lines, not chosen
	hot                bool                // an action that waits to be done
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
	// what is slow to compute is kept until the job changes
	ptsKey string
	pts    [][2]float64
	mapKey string
	hm     *HeightMap
	hmPts  []Point
	hmErr  error
	gen    int // changes after every action: the lists are built again
	// numbers edited in the lines (inline.go)
	pendS    pending
	relay    bool // a number of the programs moves the boards: Разложить
	camDirty bool // the numbers of the programs changed: Пересчитать
	edit     *ui.Field
	editID   string
	focused  string // the field the screen gave the keys to
	editF    formField
	msg      string
	cache    struct {
		gen, sel0 int
		roots     []jnode
		kids      []jnode
	}
}

// points: the points of the height map, kept while the layout stays.
func (u *jobUI) points() [][2]float64 {
	k := sheetKey(u.j, u.mirror(), 0, 0) + "/" + num(u.j.Step)
	if k != u.ptsKey {
		u.ptsKey, u.pts = k, u.j.scanPoints(u.mirror())
	}
	return u.pts
}

// jmap: the height map of the job, read again only when the file changes.
func (u *jobUI) jmap() (*HeightMap, error) {
	k := ""
	if st, err := os.Stat(u.j.mapPath()); err == nil {
		k = st.ModTime().String() + num(float64(st.Size()))
	}
	z := u.j.zero()
	k += num(z.X) + "," + num(z.Y)
	if k != u.mapKey {
		u.mapKey = k
		u.hm, u.hmErr = jobMap(u.j)
		u.hmPts, _ = LoadPoints(u.j.mapPath())
	}
	return u.hm, u.hmErr
}

// lists: the stages and the lines of the chosen one, built once per change.
func (u *jobUI) lists() ([]jnode, []jnode) {
	if u.cache.roots == nil || u.cache.gen != u.gen || u.cache.sel0 != u.sel[0] {
		u.cache.roots = u.roots()
		if u.sel[0] >= len(u.cache.roots) {
			u.sel[0] = len(u.cache.roots) - 1
		}
		u.cache.kids = nil
		if u.cache.roots[u.sel[0]].branch {
			u.cache.kids = u.kids(u.sel[0])
		}
		u.cache.gen, u.cache.sel0 = u.gen, u.sel[0]
	}
	if u.sel[1] >= len(u.cache.kids) {
		u.sel[1] = len(u.cache.kids) - 1
	}
	if u.sel[1] < 0 {
		u.sel[1] = 0
	}
	return u.cache.roots, u.cache.kids
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
	if _, err := u.jmap(); err != nil {
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
		if _, err := u.jmap(); err == nil {
			m.icon = "√"
			m.mapView = true
		} else {
			m.right = "устарела"
		}
	}
	pts := u.points()
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
		}
		out = append(out, u.boardRows()...)
		out = append(out, u.sheetRows()...)
		lay := jnode{icon: "·", title: "Разложить и посчитать", act: u.layout,
			about: []string{"Платы сами встанут на лист, потом программы всех этапов."}}
		if u.changed() {
			lay.hot, lay.right = true, "изменено"
			lay.about = append([]string{"▲ Лист, платы или сторона изменены: разложите заново."}, lay.about...)
		}
		out = append(out, lay)
		if u.camDirty && len(j.Places) > 0 && !u.changed() {
			out = append(out, jnode{icon: "·", title: "Пересчитать программы", act: func() error { return u.programs(false) },
				hot: true, right: "числа изменены", about: []string{"Программы всех этапов с новыми числами, раскладка та же."}})
		}
		out = append(out, jnode{title: "Числа программ", head: true})
		out = append(out, u.camRows()...)
		return out
	case 1:
		pts := u.points()
		ov := func() sheetOverlay { return sheetOverlay{points: pts} }
		out := []jnode{
			{title: "Шаг карты", field: &formField{label: "шаг карты", unit: "мм", lo: 3, hi: 50, step: 1, digits: 1,
				get: func() float64 { return j.Step }, set: func(v float64) { j.Step = v; j.save() }}, ov: ov,
				about: []string{fmt.Sprintf("Точек %d, ~%.0f мин.", len(pts), float64(len(pts))*secondsPerPoint/60),
					"Расстояние между точками касания; точки на картинке пересчитываются сразу."}},
			{icon: "·", title: "Снять карту высот", right: fmt.Sprintf("%d т.", len(pts)), act: u.scan, ov: ov,
				about: []string{"Обход точек над платами, первая - угол листа ♦. Потом по карте выравнивается каждый файл."}},
		}
		if hm, err := u.jmap(); err == nil {
			out = append(out, jnode{icon: "·", title: "Посмотреть карту", mapView: true, act: func() error {
				mapResult("задание / Карта высот", hm, u.hmPts, j, u.mirror(), []string{"Карта " + filepath.Base(j.mapPath())})
				return nil
			}, about: []string{"Высоты меди: поверхность, числа, таблица."}})
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
			out = append(out, jnode{icon: " ", title: "Сначала маска", about: []string{
				"Нанесите паяльную маску на платы и высушите её.",
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
			stage := s.prefix[:1]
			n.ov = func() sheetOverlay { return sheetOverlay{paths: u.pathsOf(f), marks: marks, stage: stage} }
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
