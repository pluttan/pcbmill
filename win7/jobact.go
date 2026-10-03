package main

// The actions of the job screen: boards, forms, the layout, the scan, a
// program, a redo.

import (
	"fmt"

	"math"
	"os"
	"path/filepath"
	"pcbscan/ui"
	"strings"
	"time"
)

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
	count, ok := pickCounts(srcs, count)
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
		return []string{fmt.Sprintf("Ноль Z касается в углу X%s Y%s.", num(m/2), num(m/2)),
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
	}}
	var lastStep float64
	var pts [][2]float64
	f.scene = func() ui.Scene {
		if s != lastStep || pts == nil {
			old := u.j.Step
			u.j.Step = s
			pts = u.j.scanPoints(u.mirror())
			u.j.Step, lastStep = old, s
		}
		return newSheetScene(u.j, u.mirror(), sheetOverlay{points: pts})
	}
	if !editForm(f) {
		return nil
	}
	u.j.Step = s
	return u.j.save()
}

func (u *jobUI) numbers() error {
	c := u.c
	if !editForm(camNumForm(&c)) {
		return nil
	}
	u.c = c
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
		var marks []pt
		for _, op := range ops {
			if strings.HasPrefix(op.file, "1_") {
				marks = op.marks
			}
		}
		p.scene = newSheetScene(j, u.mirror(), sheetOverlay{marks: marks})
	})
	p.wait()
	u.lvl, u.sel[0], u.sel[1] = 0, u.next(), 0
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
	_ = used
	hm, err := scanRun(u.o, "задание / Карта высот", pts, tmp, j, mirror)
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
	u.lvl, u.sel[0], u.sel[1] = 0, u.next(), 0
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
		u.lvl, u.sel[0], u.sel[1] = 0, u.next(), 0
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
		return []string{"Пути файла внутри круга, на столько глубже; новый файл рядом, с _redo в имени.",
			"Щелчок по листу ставит центр круга."}
	}}
	f.pick = func(px, py float64) {
		x = math.Max(0, math.Min(u.j.W, math.Round(px*10)/10))
		y = math.Max(0, math.Min(u.j.H, math.Round(py*10)/10))
	}
	f.scene = func() ui.Scene {
		k := int(src)
		if k < 0 || k >= len(files) {
			k = 0
		}
		digit := strings.TrimPrefix(filepath.Base(files[k]), "sheet_")[:1]
		return newSheetScene(u.j, u.mirror(), sheetOverlay{paths: u.pathsOf(files[k]), circle: &[3]float64{x, y, r}, stage: digit})
	}
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
