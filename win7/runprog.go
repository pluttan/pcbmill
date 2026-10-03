package main

// One program of the job on the machine: the tool, Z0 in the corner of the
// sheet, the program levelled by the height map, a typed yes, the program
// opened in NcStudio by this program. The operator starts it (F9) and says
// here when the machine has finished.

import (
	"fmt"

	"math"
	"os"
	"path/filepath"
	"pcbscan/ui"
	"strings"
	"time"
)

// progPaths: the paths of a program for the 3D view.
func progPaths(lines []string) []cpath {
	op, err := readProg(lines)
	if err != nil {
		return nil
	}
	return op.paths
}

// progStage: the digit of the stage a program belongs to ("1" .. "4").
func progStage(file string) string {
	b := strings.TrimPrefix(filepath.Base(file), "sheet_")
	if b == "" {
		return ""
	}
	return b[:1]
}

// toolOf: what a program of the job wants in the spindle, by its name.
func toolOf(file string, c camCfg) string {
	base := strings.TrimSuffix(filepath.Base(file), ".tap")
	if i := strings.Index(base, "_redo"); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimPrefix(base, "sheet_")
	vbit := fmt.Sprintf("V-фреза %s/%s°", num(c.Tip), num(c.Angle))
	switch {
	case strings.HasPrefix(base, "1_"), base == "3_drill_spot":
		return vbit
	case strings.HasPrefix(base, "2_"):
		if c.MaskFlat > 0 {
			return "плоская фреза " + num(c.MaskFlat)
		}
		return vbit
	case strings.HasPrefix(base, "3_drill_"):
		return "сверло " + strings.TrimPrefix(base, "3_drill_")
	case strings.HasPrefix(base, "4_"):
		return "V-резец для V-реза"
	}
	return "?"
}

// progInfo: the depth, the box of the cut and the time of a program.
func progInfo(lines []string) (depth float64, cut rect, minutes float64, err error) {
	x0, x1, y0, y1, err := CutBounds(lines)
	if err != nil {
		return 0, rect{}, 0, err
	}
	op, err := readProg(lines)
	if err != nil {
		return 0, rect{}, 0, err
	}
	for _, p := range op.paths {
		for _, q := range p.pts {
			depth = math.Max(depth, -q.Z)
		}
	}
	_, minutes = op.minutes()
	return depth, rect{x0, x1, y0, y1}, minutes, nil
}

// jobMap: the height map of the job, and an error when it is missing or
// was taken with another zero than the corner of this sheet.
func jobMap(j *Job) (*HeightMap, error) {
	rows, err := LoadPoints(j.mapPath())
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("карты высот нет: сначала этап \"Карта высот\"")
	}
	z := j.zero()
	if math.Hypot(rows[0].X-z.X, rows[0].Y-z.Y) > 0.01 {
		return nil, fmt.Errorf("карта снята с нулём в X%.1f Y%.1f, а угол листа сейчас X%.1f Y%.1f: снимите карту заново",
			rows[0].X, rows[0].Y, z.X, z.Y)
	}
	return MapFromPoints(rows)
}

// runProg runs one program of a stage; true when the operator said it is done.
func runProg(o opts, j *Job, c camCfg, file, stage string) (bool, error) {
	title := "задание / " + stage + " / " + filepath.Base(file)
	hm, err := jobMap(j)
	if err != nil {
		return false, err
	}
	lines, err := readLines(file)
	if err != nil {
		return false, err
	}
	depth, cut, minutes, err := progInfo(lines)
	if err != nil {
		return false, fmt.Errorf("%s: %v", filepath.Base(file), err)
	}
	if cut.X0 < -0.5 || cut.Y0 < -0.5 || cut.X1 > j.W+0.5 || cut.Y1 > j.H+0.5 {
		return false, fmt.Errorf("%s режет X %.1f..%.1f Y %.1f..%.1f, а лист %sx%s: не запускаю",
			filepath.Base(file), cut.X0, cut.X1, cut.Y0, cut.Y1, num(j.W), num(j.H))
	}
	// the tool and Z0: after the map and after every change of tool
	tool, had := toolOf(file, c), j.Done["_tool"]
	z := j.zero()
	corner := fmt.Sprintf("♦ X%s Y%s (угол листа)", num(z.X), num(z.Y))
	needZero := had != tool
	newPage(title)
	if needZero {
		was := "неизвестно (после карты высот Z0 ставится заново)"
		if had != "" {
			was = had
		}
		d := &dialog{title: "Инструмент", text: []string{
			"Нужен: " + tool, "Сейчас по записи: " + was, "",
			"Поставьте его, опустите на 1-2 мм над медью у " + corner + ".",
			"Дальше касание и Z0 там."},
			buttons: []string{"Поставил, Z0", "Отмена"}, sel: 0, cancel: 1}
		if d.show() != 0 {
			return false, nil
		}
	} else {
		d := &dialog{title: "Инструмент", text: []string{"В шпинделе " + tool + ", Z0 после неё стоит."},
			buttons: []string{"Дальше", "Сделать Z0 заново", "Отмена"}, cancel: 2}
		switch d.show() {
		case 1:
			needZero = true
		case 2:
			return false, nil
		}
	}
	if needZero {
		delete(j.Done, "_tool") // until the touch is through, the zero is not known
		if err := j.save(); err != nil {
			return false, err
		}
		if err := zeroAt(o, title, [2]float64{z.X, z.Y}, "угол листа, ноль карты"); err != nil {
			if err == errBack {
				return false, nil
			}
			return false, err
		}
		j.Done["_tool"] = tool
		if err := j.save(); err != nil {
			return false, err
		}
	}
	// the program over the map, next to it
	out, err := ApplyMap(lines, hm, 1.0)
	if err != nil {
		return false, err
	}
	dst := strings.TrimSuffix(file, ".tap") + "_level.tap"
	if err := os.WriteFile(dst, []byte(strings.Join(out, "\r\n")+"\r\n"), 0644); err != nil {
		return false, err
	}
	about := []string{
		"Файл:  " + filepath.Base(dst),
		"Инструмент:  " + tool + ", Z0 в углу листа",
		fmt.Sprintf("Глубина до %.3f мм, рез X %.1f..%.1f Y %.1f..%.1f, ~%.0f мин", depth, cut.X0, cut.X1, cut.Y0, cut.Y1, math.Ceil(minutes)),
		"",
		"pcbscan откроет файл в NcStudio. Запускаете вы: F9.",
	}
	if !askYes("Открыть в NcStudio?", about...) {
		return false, nil
	}
	p := newPage(title, about[:3]...)
	sc := newSheetScene(j, c.Side == sideBottom, sheetOverlay{paths: progPaths(lines), stage: progStage(file)})
	sc.tool = true
	p.set(func() { p.head = "Открываю в NcStudio"; p.scene = sc })
	nc := newNc(o)
	sayHint("открываю " + filepath.Base(dst) + " в NcStudio (Ctrl+O)...")
	loaded := true
	machineBusy(true)
	if err := nc.Load(dst); err != nil {
		loaded = false
		sayWarn(err.Error())
	}
	machineBusy(false)
	backToApp()
	todo := "► Файл открыт в NcStudio. Запустите его там: F9."
	if !loaded {
		todo = "▲ Откройте в NcStudio сами (Файл, Открыть): " + dst + " и запустите F9."
	}
	start := time.Now()
	p.set(func() {
		p.head = "Станок работает"
		p.info = append(append([]string(nil), about[:3]...), "", todo,
			"Когда станок закончит и поднимет фрезу - кнопка ниже.")
	})
	done := false
	for !done {
		id := p.ask(pageBtn{id: "done", label: "Станок закончил", kind: ui.BtnSuccess, key: ui.KeyEnter},
			pageBtn{id: "back", label: "Не закончено", kind: ui.BtnNormal, key: ui.KeyEsc})
		if id != "done" {
			return false, nil
		}
		done = confirm("Станок закончил " + filepath.Base(dst) + "?")
	}
	j.mark(strings.TrimSuffix(filepath.Base(file), ".tap"))
	if err := j.save(); err != nil {
		return false, err
	}
	logLine(fmt.Sprintf("√ %s сделан за %s", filepath.Base(file), time.Since(start).Round(time.Second)))
	return true, nil
}
