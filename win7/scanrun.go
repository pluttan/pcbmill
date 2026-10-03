package main

// Menu item 3: the program, the grid form, the scan with the table filling in
// point by point, and the height map at the end.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// secondsPerPoint: about 8 MDI commands of about 1.1 s each with the predicted approach.
const secondsPerPoint = 9.0

// escStop passes the moves on, but before the travel to the next point (the
// tip is at travel height then) an Esc in the console stops the scan. Esc only
// gets here when the console has the focus, and taking the focus from
// NcStudio stops the scan anyway; this is the gentle way between points.
type escStop struct{ Machine }

func (e escStop) RapidXY(x, y float64) error {
	for {
		k, ok := term.Key(0)
		if !ok {
			break
		}
		if k.Code == KeyEsc || k.Code == KeyCtrlC {
			return fail("остановлено клавишей Esc в окне pcbscan")
		}
	}
	return e.Machine.RapidXY(x, y)
}

// scanInfo: the progress lines over the picture: the bar with its percent,
// the count and the time left, then what is being done.
func scanInfo(done, all int, start time.Time, last, phase string) []string {
	left := paint(cHint, "считаю время")
	if done > 0 && done < all {
		rest := time.Since(start) / time.Duration(done) * time.Duration(all-done)
		left = paint(cHint, "осталось ~") + val(fmt.Sprintf("%.0f", math.Ceil(rest.Minutes())), "мин")
	}
	if done >= all {
		left = paint(cOK, "все точки сняты")
	}
	out := []string{bar(done, all, 24) + "   " + val(fmt.Sprint(done), "") + paint(cHint, fmt.Sprintf(" из %d", all)) + "   " + left}
	if phase != "" {
		out = append(out, paint(cKey, phase))
	}
	if last != "" {
		out = append(out, paint(cHint, "последняя ")+last)
	}
	return out
}

func cmdScan(o opts) error {
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	g, err := gridFor(tap, o.grid)
	if err != nil {
		return err
	}
	cut := rect{g.X0, g.X1, g.Y0, g.Y1}
	g, ok := editGrid(g)
	if !ok {
		return nil
	}
	_, err = scanRun(o, menuTitle("4"), g.Points(), filepath.Join(exeDir(), "scan.csv"), &cut,
		func(w, r int) []string { return gridViewRows(g, &cut, w, r) },
		func(done []Point, blink bool, w, r int) []string { return scanView(g, &cut, done, blink, w, r) })
	return err
}

// scanRun: the scan of pts (the first is where Z is zeroed) into out, with
// before the picture of the points and during the picture of the progress;
// the height map at the end, nil when the operator said no.
func scanRun(o opts, title string, pts [][2]float64, out string, cut *rect,
	before func(w, rows int) []string, during func(done []Point, blink bool, w, rows int) []string) (*HeightMap, error) {
	p := newPage(title, "Ищу МК на COM-портах...")
	m, name, err := findTouch(o.port)
	if err != nil {
		return nil, err
	}
	p.set(func() {
		p.head = "Перед обходом"
		p.info = []string{
			fmt.Sprintf("%s. Точек %d, примерно %.0f мин.", mcuAt(name), len(pts), float64(len(pts))*secondsPerPoint/60),
			"В NcStudio ноль X0 Y0 стоит в углу листа, шпиндель выключен,",
			fmt.Sprintf("фреза (любая крепкая) на 1-2 мм над медью у первой точки ♦ X%.1f Y%.1f.", pts[0][0], pts[0][1]),
			fmt.Sprintf("Не выше 2 мм: в первой точке медь ищется на %.0f мм вниз от старта.", o.search),
			sep,
			"▲ Сейчас ОБНУЛИТЕ Z в NcStudio на этой высоте: обход ведёт Z абсолютными командами от этого нуля.",
		}
		p.pic = func(w, h int) []string { return fitRows(h, func(r int) []string { return before(w, r) }) }
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
	if err := nc.Grab(); err != nil {
		return nil, err
	}
	sc := newScanner(o, escStop{nc}, m)
	sc.Predict = true // nc.Abs above
	var done []Point
	start, last, phase, blink := time.Now(), "", "", false
	refresh := func() { p.info = scanInfo(len(done), len(pts), start, last, phase) }
	p.set(func() {
		refresh()
		p.nolog, p.head = true, "Обход"
		p.keys = []string{"Esc", "стоп между точками", "щелчок по другому окну", "стоп сразу"}
		if simM != nil { // the virtual NcStudio has no window to lose the focus
			p.keys = p.keys[:2]
		}
		p.pic = func(w, h int) []string {
			return fitRows(h, func(r int) []string { return during(done, blink, w, r) })
		}
	})
	sc.Log = func(l string) { p.set(func() { last = l; p.log = append(p.log, l); refresh() }) }
	stopTick, ticked := make(chan bool), make(chan bool)
	go func() { // blink and the clock while the main goroutine waits for NcStudio
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		defer close(ticked)
		for {
			select {
			case <-stopTick:
				return
			case <-t.C:
				p.set(func() { blink = !blink; refresh() })
			}
		}
	}()
	var once sync.Once
	stop := func() { once.Do(func() { close(stopTick); <-ticked }) } // before any dialog
	defer stop()
	stopFaults := faultKeysOn(func() bool { return true })
	defer stopFaults()
	rows, err := sc.Grid(pts, func(pt Point) {
		fmt.Fprintf(f, "%.3f,%.3f,%.4f\n", pt.X, pt.Y, pt.Z)
		f.Sync()
		p.set(func() { done = append(done, pt); refresh() })
	})
	stop()
	if err != nil {
		sayStop(err)
		if confirm(fmt.Sprintf("Поднять фрезу на %.1f мм?", o.clear), "NcStudio снова получит фокус.") && nc.Grab() == nil {
			nc.RapidZ(nc.z + o.clear)
		}
		return nil, fmt.Errorf("карта не досняна: %v. В %s измеренные точки", err, filepath.Base(out))
	}
	p.set(func() {
		phase = "Повторное касание первой точки: насколько всё уехало за время обхода..."
		refresh()
	})
	drift, err := sc.Recheck(rows)
	if err != nil {
		return nil, fmt.Errorf("повторное касание не удалось, карте верить нельзя: %v", err)
	}
	fmt.Fprintf(f, "# повторное касание первой точки: сдвиг %+.4f мм\n", drift)
	hm, _ := MapFromPoints(rows)
	info := []string{fmt.Sprintf("Сдвиг первой точки за обход: %+.3f мм", drift)}
	if math.Abs(drift) > 0.03 {
		info = append(info, "▲ ВНИМАНИЕ: больше 0.03 мм. Ось Z уходит во время работы или станок "+
			"пропускает команды: по этой карте резать нельзя. Проверьте механику Z и повторите.")
	} else {
		info = append(info, "√ Сдвиг в норме, карте можно верить.")
	}
	info = append(info, "√ Карта записана: "+filepath.Base(out))
	mapResult(title+": готово", hm, cut, info)
	if math.Abs(drift) > 0.03 {
		return nil, fmt.Errorf("сдвиг первой точки %+.3f мм: карта не годится, снимите заново", drift)
	}
	return hm, nil
}
