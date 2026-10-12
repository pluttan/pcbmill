package main

// The scan of the height map: the sheet in 3D with the tool going from
// point to point, every measured point coloured by its height, the time left;
// the map in MATLAB's way at the end.

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pcbscan/ui"
)

// secondsPerPoint: about 8 MDI commands of about 1.1 s each with the predicted approach.
const secondsPerPoint = 9.0

// escStop passes the moves on, but before the travel to the next point (the
// tip is at travel height then) the Стоп button of the page stops the scan.
// A click on the page does not take the focus from NcStudio (the window is
// in no-activate mode while the machine works), so the button is safe.
type escStop struct {
	Machine
	p *page
}

func (e escStop) RapidXY(x, y float64) error {
	if e.p != nil {
		if id, ok := e.p.pressed(); ok && id == "stop" {
			return fail("остановлено кнопкой Стоп в окне pcbscan")
		}
	}
	return e.Machine.RapidXY(x, y)
}

// scanText: the progress under the bar: how many, the time left.
func scanText(done, all int, start time.Time) string {
	left := "считаю время"
	if done > 0 && done < all {
		rest := time.Since(start) / time.Duration(done) * time.Duration(all-done)
		left = fmt.Sprintf("осталось ~%.0f мин", math.Ceil(rest.Minutes()))
	}
	if done >= all {
		left = "все точки сняты"
	}
	return fmt.Sprintf("%d из %d   %s", done, all, left)
}

// scanRun: the scan of pts (the first is where Z is zeroed) into out; the
// height map at the end, nil when the operator said no.
func scanRun(o opts, title string, pts [][2]float64, out string, j *Job, mirror bool) (*HeightMap, error) {
	p := newPage(title, "Ищу МК на COM-портах...")
	m, name, err := findTouch(o.port)
	if err != nil {
		return nil, err
	}
	pts = cornersFirst(pts) // a wrong zero or a loose wire shows in five points
	sc := newSheetScene(j, mirror, sheetOverlay{points: pts})
	sc.tool = true
	p.set(func() {
		p.head = "Перед обходом"
		p.info = []string{
			fmt.Sprintf("%s. Точек %d, примерно %.0f мин.", mcuAt(name), len(pts), float64(len(pts))*secondsPerPoint/60),
			"В NcStudio ноль X0 Y0 стоит в углу листа, шпиндель выключен,",
			fmt.Sprintf("фреза (любая крепкая) на 1-2 мм над медью у первой точки ♦ X%.1f Y%.1f.", pts[0][0], pts[0][1]),
			fmt.Sprintf("Не выше 2 мм: в первой точке медь ищется на %.0f мм вниз от старта.", o.search),
			"",
			"▲ Сейчас ОБНУЛИТЕ Z в NcStudio на этой высоте: обход ведёт Z абсолютными командами от этого нуля.",
		}
		p.scene = sc
	})
	if !confirm("Z обнулён, начать обход?") {
		return nil, nil
	}
	if simM != nil {
		simM.ZeroZ() // what the operator has done in NcStudio
		logLine("эмуляция: Z обнулён в NcStudio на высоте старта")
	}
	f, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fmt.Fprintf(f, "# x,y,z  pcbscan, Z от высоты старта\n")
	nc := newNc(o)
	nc.Abs = true
	machineBusy(true)
	defer machineBusy(false)
	if err := nc.Grab(); err != nil {
		return nil, err
	}
	s := newScanner(o, escStop{nc, p}, newReTouch(m, o.port, p))
	s.Predict = true // nc.Abs above
	if len(pts) > 5 {
		s.Far = 5 // the zero and the corners first (cornersFirst)
	}
	start := time.Now()
	p.set(func() {
		p.head, p.info = "Обход", []string{"Мышь и клавиатуру не трогать: pcbscan печатает в NcStudio.",
			"Стоп останавливает между точками, фреза поднята. Щелчок по другому окну - сразу."}
		p.progress, p.progText = 0, scanText(0, len(pts), start)
		p.btns = []pageBtn{{id: "stop", label: "Стоп", kind: ui.BtnDanger}}
	})
	p.drain()
	s.Log = func(l string) { logLine(l) }
	stopTick, ticked := make(chan bool), make(chan bool)
	go func() { // the clock while the work goroutine waits for NcStudio
		t := time.NewTicker(time.Second)
		defer t.Stop()
		defer close(ticked)
		for {
			select {
			case <-stopTick:
				return
			case <-t.C:
				p.set(func() { p.progText = scanText(len(sc.done), len(pts), start) })
			}
		}
	}()
	var once sync.Once
	stop := func() { once.Do(func() { close(stopTick); <-ticked }) }
	defer stop()
	stopFaults := faultKeysOn(func() bool { return true })
	defer stopFaults()
	var got []Point
	cornersStop := false
	rows, err := s.Grid(pts, func(pt Point) {
		fmt.Fprintf(f, "%.3f,%.3f,%.4f\n", pt.X, pt.Y, pt.Z)
		f.Sync()
		got = append(got, pt)
		if len(got) == 5 && len(pts) > 5 { // the zero and the four corners: a look before the rest
			lines, odd := cornerReport(got)
			machineBusy(false)
			d := &dialog{title: "Углы сняты: продолжить обход?", text: append(lines, "",
				fmt.Sprintf("Дальше ещё %d точек.", len(pts)-5)),
				buttons: []string{"Продолжить обход", "Стоп"}, cancel: 1}
			if odd {
				d.sel = 1
			}
			if d.show() != 0 {
				cornersStop = true
				p.send("stop") // the scan stops before the next travel
			}
			machineBusy(true)
			nc.Grab()
		}
		if len(sc.done) == 0 {
			setCopperZ(pt.Z)
		}
		p.set(func() {
			sc.done = append(sc.done, pt)
			p.progress = float64(len(sc.done)) / float64(len(pts))
			p.progText = scanText(len(sc.done), len(pts), start)
		})
	})
	stop()
	p.buttons()
	if err != nil {
		machineBusy(false)
		sayStop(err)
		if confirm(fmt.Sprintf("Поднять фрезу на %.1f мм?", o.clear), "NcStudio снова получит фокус.") && nc.Grab() == nil {
			nc.RapidZ(nc.z + o.clear)
		}
		if cornersStop {
			return nil, nil
		}
		return nil, scanHelp(err, len(rows) <= 1, filepath.Base(out))
	}
	p.set(func() {
		p.progText = "Повторное касание первой точки: насколько всё уехало за время обхода..."
	})
	drift, err := s.Recheck(rows)
	if err != nil {
		return nil, fmt.Errorf("повторное касание не удалось, карте верить нельзя: %v", err)
	}
	machineBusy(false)
	fmt.Fprintf(f, "# повторное касание первой точки: сдвиг %+.4f мм\n", drift)
	hm, _ := MapFromPoints(rows)
	info := []string{fmt.Sprintf("Сдвиг первой точки за обход: %+.3f мм", drift)}
	if s.Stuck > 0 {
		info = append(info, fmt.Sprintf("▲ Контакт не размыкался после подъёма %d раз: фреза поднималась выше, точки сняты заново. "+
			"Если таких много, проверьте провод касания и Wi-Fi щупа.", s.Stuck))
	}
	if math.Abs(drift) > 0.03 {
		info = append(info, "▲ ВНИМАНИЕ: больше 0.03 мм. Ось Z уходит во время работы или станок "+
			"пропускает команды: по этой карте резать нельзя. Проверьте механику Z и повторите.")
	} else {
		info = append(info, "√ Сдвиг в норме, карте можно верить.")
	}
	info = append(info, "√ Карта записана: "+filepath.Base(out))
	mapResult(title+": готово", hm, rows, j, mirror, info)
	if math.Abs(drift) > 0.03 {
		return nil, fmt.Errorf("сдвиг первой точки %+.3f мм: карта не годится, снимите заново", drift)
	}
	return hm, nil
}

// mapResult shows a height map: the surface in 3D and, on a button, the
// table of numbers.
func mapResult(title string, hm *HeightMap, pts []Point, j *Job, mirror bool, info []string) {
	p := newPage(title)
	ms := newMapScene(hm, pts, j, mirror)
	info = append(append([]string(nil), info...), "",
		fmt.Sprintf("Точек %d, сетка %d x %d.", len(pts), len(hm.xs), len(hm.ys)),
		fmt.Sprintf("Медь от %+.3f до %+.3f мм от первой точки, перепад %.3f мм.", ms.lo, ms.hi, ms.hi-ms.lo),
		"Левая кнопка мыши вращает, правая двигает, колесо приближает.")
	table := false
	p.set(func() {
		p.head, p.info, p.nolog, p.scene = "Карта высот", info, true, ms
		p.extra = func(c *ui.Ctx, r image.Rectangle) int {
			if !table {
				return 0
			}
			return drawHeightTable(c, r, hm)
		}
	})
	for {
		label := "Таблица чисел"
		if table {
			label = "Скрыть таблицу"
		}
		id := p.ask(pageBtn{id: "table", label: label, kind: ui.BtnNormal, key: ui.KeyTab},
			pageBtn{id: "ok", label: "Готово", kind: ui.BtnPrimary, key: ui.KeyEnter})
		if id != "table" {
			return
		}
		p.set(func() { table = !table })
	}
}
