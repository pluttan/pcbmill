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

// layout puts the boards on the sheet again and makes the programs; a map
// and marks of done work of the old layout go, after a question.
func (u *jobUI) layout() error {
	j := u.j
	if u.boardsLeft() == 0 {
		message("Платы", "Ни одной платы на листе.", "В строках Платы на листе поставьте, сколько штук каждой.")
		return nil
	}
	_, mapErr := os.Stat(j.mapPath())
	if mapErr == nil || len(j.Done) > 0 {
		if !confirm("Разложить заново?", "Карта высот и отметки о сделанном старой раскладки не подойдут и будут сброшены.") {
			return nil
		}
	}
	if err := u.applyPend(); err != nil {
		return err
	}
	return u.programs(true)
}

// programs: the layout (when asked) and the programs of every stage.
func (u *jobUI) programs(relayout bool) error {
	j := u.j
	p := newBusyPage("задание / Платы", "Раскладываю и считаю программы...")
	turned := false
	if relayout {
		if u.c.Side == sideTop && j.bottomOnly() { // B.Cu only: milled face up it must be mirrored
			u.c.Side, turned = sideBottom, true
			saveCam(exeDir(), u.c)
		}
		if err := j.layout(u.mirror(), u.c.MaskTest, u.c.IsoTest); err != nil {
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
	u.camDirty = false
	ops, err := j.cam(u.c, func(s string) { sayHint(s) })
	if err != nil {
		return err
	}
	if err := j.writeProgs(ops); err != nil {
		return err
	}
	j.markCam(u.c)
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
	if turned {
		info = append(info, "", "▲ Медь у плат только на нижнем слое (B.Cu): сторона переключена на «низ», программы зеркальные.")
	}
	info = append(info, "", "Дальше: Карта высот.")
	sc := newSheetScene(j, u.mirror(), sheetOverlay{})
	p.set(func() {
		p.scene = sc
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
	if err := u.freshProgs(); err != nil {
		return err
	}
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
	if err := u.freshProgs(); err != nil {
		return err
	}
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

// camVersion goes up when the programs come out different for the same
// numbers (2: the V-groove to the edge of the sheet): the next stage rewrites them.
const camVersion = 2

// camSig: the numbers the programs on disk were written with.
func camSig(c camCfg) string { return fmt.Sprintf("v%d %+v", camVersion, c) }

func (j *Job) camSigPath() string { return filepath.Join(j.Dir, "cam.sig") }

func (j *Job) markCam(c camCfg) { os.WriteFile(j.camSigPath(), []byte(camSig(c)), 0o644) }

// freshProgs: the numbers changed after the programs were written (the speed
// set after the map, or a restart lost the Пересчитать line): write them again
// with the same layout before a stage runs. The map and the marks stay.
func (u *jobUI) freshProgs() error {
	j := u.j
	if b, err := os.ReadFile(j.camSigPath()); err == nil && string(b) == camSig(u.c) && !u.camDirty {
		return nil
	}
	if u.relay {
		return fmt.Errorf("числа меняют раскладку: сначала Платы / Разложить и посчитать")
	}
	sayHint("Числа программ изменились: пересчитываю...")
	ops, err := j.cam(u.c, func(s string) { sayHint(s) })
	if err != nil {
		return err
	}
	if err := j.writeProgs(ops); err != nil {
		return err
	}
	j.markCam(u.c)
	u.camDirty = false
	return nil
}
