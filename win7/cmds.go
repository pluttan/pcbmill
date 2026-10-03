package main

// The menu commands but the scan and the manual mode: each runs on its own
// work page, asks in dialogs, and leaves the page on screen until Enter.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// spaced writes a word with spaces between the letters: big text for the wire check.
func spaced(s string) string { return strings.Join(strings.Split(s, ""), " ") }

// wireBig is the large contact indicator.
func wireBig(on bool, hits int, since string, w int) []string {
	bw := w
	if bw > 44 {
		bw = 44
	}
	word, col, mark := spaced("разомкнуто"), cOK, "○"
	if on {
		word, col, mark = spaced("КОНТАКТ"), cTouch, "♦"
	}
	b := boxC(thick, col, "", []string{"", center(paint(col, mark+"   "+word+"   "+mark), bw-4), ""}, bw)
	return append(b, center(fmt.Sprintf("касаний %s   последнее изменение %s",
		paint(cTitle, fmt.Sprint(hits)), paint(cTitle, since)), bw))
}

func cmdWire(o opts) error {
	p := newPage(menuTitle("1"), "Ищу МК на COM-портах...")
	m, name, err := findMCU(o.port)
	if err != nil {
		return err
	}
	p.set(func() {
		p.info = []string{paint(cOK, "МК на "+name) + ". Касайтесь зажимом фрезы: здесь должно",
			"загораться КОНТАКТ. Станок не двигается."}
		p.keys = []string{"Esc", "в меню"}
	})
	backToConsole()
	last, since, start := !m.State(), "-", m.Hits()
	for {
		if e := m.Alive(); e != nil {
			return e
		}
		if s := m.State(); s != last {
			last = s
			since = time.Now().Format("15:04:05")
			if s {
				logLine(paint(cHint, since) + "  " + paint(cTouch, "♦ КОНТАКТ"))
			} else {
				logLine(paint(cHint, since) + "  " + paint(cOK, "○ разомкнуто"))
			}
		}
		w, _ := term.Size()
		p.set(func() { p.big = wireBig(last, m.Hits()-start, since, colW(w)) })
		if k, ok := term.Key(40 * time.Millisecond); ok {
			if k.Code == KeyEsc || k.Code == KeyEnter || k.Code == KeyCtrlC || k.letter() == 'q' {
				return nil
			}
		}
	}
}

func cmdNcTest(o opts) error {
	p := newPage(menuTitle("2"),
		"Фреза не ниже 5 мм над заготовкой. Программа нажмёт в NcStudio "+paint(cKey, o.mdiKey)+" и введёт",
		"  "+paint(cTitle, "G91 G00 Z1")+"   вверх на 1 мм, потом",
		"  "+paint(cTitle, "G91 G00 Z-1")+"  обратно.",
		sep,
		paint(cHint, "Пока она печатает, мышь и клавиатуру не трогать."))
	if !confirm("Начать проверку связи?") {
		return nil
	}
	n := newNc(o)
	if err := n.Grab(); err != nil {
		return err
	}
	sayHint("G91 G00 Z1")
	if err := n.RapidZ(1); err != nil {
		return err
	}
	up := confirm("Z поднялся на 1 мм?")
	if err := n.Grab(); err != nil {
		return err
	}
	sayHint("G91 G00 Z-1")
	if err := n.RapidZ(0); err != nil {
		return err
	}
	down := confirm("Z вернулся вниз?")
	if up && down {
		sayOK("Связь с NcStudio работает.")
	} else {
		sayWarn("не сработало. Узнайте, какими клавишами в вашей NcStudio открывается окно ввода " +
			"одной строки G-кода (MDI), и запустите: pcbscan.exe -mdi ctrl+shift+f9 (свои клавиши).")
	}
	p.wait()
	return nil
}

func gridFor(tap string, step float64) (Grid, error) {
	lines, err := readLines(tap)
	if err != nil {
		return Grid{}, err
	}
	x0, x1, y0, y1, err := CutBounds(lines)
	if err != nil {
		return Grid{}, err
	}
	return GridFor(x0, x1, y0, y1, step), nil
}

func cmdZero(o opts) error {
	// Z0 goes where the map has its zero: the first point of scan.csv, whatever grid it used
	var first [2]float64
	from := "первая точка карты, scan.csv"
	if rows, e := LoadPoints(filepath.Join(exeDir(), "scan.csv")); e == nil && len(rows) > 0 {
		first = [2]float64{rows[0].X, rows[0].Y}
	} else {
		tap, err := pickTap(exeDir())
		if err != nil {
			return err
		}
		g, err := gridFor(tap, o.grid)
		if err != nil {
			return err
		}
		first = g.Points()[0]
		from = "scan.csv нет, первая точка сетки по программе"
	}
	p := newPage(menuTitle("4"), "Ищу МК на COM-портах...")
	m, _, err := findMCU(o.port)
	if err != nil {
		return err
	}
	p.set(func() {
		p.info = []string{
			"Фреза 0.1 стоит на 1-2 мм над медью. Будет одно медленное касание в " +
				paint(cZero, fmt.Sprintf("♦ X%.2f Y%.2f", first[0], first[1])) + " (" + from + "),",
			"потом нужно обнулить Z в NcStudio.",
		}
	})
	if !confirm("Начать касание?") {
		return nil
	}
	nc := newNc(o)
	if err := nc.Grab(); err != nil {
		return err
	}
	s := newScanner(o, nc, m)
	s.StepCoarse = 0.02 // the thin tip goes down in small steps
	z, err := s.TouchAt(first[0], first[1], 0)
	if err != nil {
		return err
	}
	if err := nc.MoveZ(z, 20); err != nil {
		return err
	}
	logLine(paint(cTouch, "♦ КОНТАКТ:") + " фреза касается меди.")
	message("Обнулите Z", "Фреза касается меди.", paint(cTitle, "Обнулите Z в NcStudio")+" (рабочая система координат),",
		"затем Enter здесь.")
	nc.ResetZ()
	if err := nc.Grab(); err != nil {
		return err
	}
	if err := nc.RapidZ(o.clear * 3); err != nil {
		return err
	}
	sayOK("Z0 стоит на меди в первой точке карты. Теперь пункт 5 и запуск *_level.tap.")
	p.wait()
	return nil
}

func cmdLevel(o opts) error {
	pts, err := LoadPoints(filepath.Join(exeDir(), "scan.csv"))
	if err != nil {
		return fmt.Errorf("нет scan.csv: сначала снимите карту (%v)", err)
	}
	hm, err := MapFromPoints(pts)
	if err != nil {
		return err
	}
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	lines, err := readLines(tap)
	if err != nil {
		return err
	}
	out, err := ApplyMap(lines, hm, 1.0)
	if err != nil {
		return err
	}
	dst := strings.TrimSuffix(tap, filepath.Ext(tap)) + "_level" + filepath.Ext(tap)
	if err := os.WriteFile(dst, []byte(strings.Join(out, "\r\n")+"\r\n"), 0644); err != nil {
		return err
	}
	var cut *rect
	if x0, x1, y0, y1, e := CutBounds(lines); e == nil {
		cut = &rect{x0, x1, y0, y1}
	}
	mapResult(menuTitle("5"), hm, cut, []string{
		paint(cOK, "√ Готово: "+filepath.Base(dst)),
		"Запускать в NcStudio этот файл, ноль Z ставить пунктом 4.",
	})
	return nil
}

// mapResult shows a height map with lines above it; Tab switches between the
// picture and the table of numbers.
func mapResult(title string, hm *HeightMap, cut *rect, info []string) {
	table := false
	p := &page{title: title, info: info}
	p.pic = func(w, h int) []string {
		if table {
			t := heightTable(hm, colW(w))
			if len(t) > h {
				t = append(t[:h-1], paint(cHint, "... окно мало для всей таблицы"))
			}
			return t
		}
		return fitRows(h, func(r int) []string { return heightViewRows(hm, cut, w, r) })
	}
	p.keys = []string{"Tab", "картинка / таблица", "Enter", "в меню", "Esc", "в меню"}
	cur = p
	run(p.view, func(k Key) bool {
		if k.Code == KeyTab {
			table = !table
			return false
		}
		return k.Code == KeyEnter || k.Code == KeyEsc || k.Code == KeyCtrlC
	}, 500*time.Millisecond)
}

// cmdFindCoords: can this program read the X/Y/Z NcStudio shows? Snapshot the text
// of every NcStudio control, move each axis by 1 mm, and see which texts change.
func cmdFindCoords(o opts) error {
	p := newPage(menuTitle("7"),
		"Программа ищет, откуда можно читать координаты NcStudio.",
		"Станок сдвинется: Z вверх на 1 мм, X на +1, Y на +1, и вернётся обратно.",
		paint(cHint, "Фреза должна быть поднята над платой, шпиндель стоять."))
	if !confirm("Начать поиск координат?") {
		return nil
	}
	out := filepath.Join(exeDir(), "ncwindows.txt")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	snap := func(name string) (map[uintptr]WinInfo, error) {
		ws, err := NcWindows(o.title)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(f, "\r\n=== %s: %d окон\r\n", name, len(ws))
		m := map[uintptr]WinInfo{}
		for _, w := range ws {
			m[w.H] = w
			fmt.Fprintf(f, "%08x  %-28s id %-6d (%d,%d)-(%d,%d)  %q\r\n",
				w.H, w.Class, w.ID, w.Left, w.Top, w.Right, w.Bott, w.Text)
		}
		return m, nil
	}
	base, err := snap("до движения")
	if err != nil {
		return err
	}
	nc := newNc(o)
	if err := nc.Grab(); err != nil {
		return err
	}
	found := 0
	prev := base
	for _, ax := range []string{"Z", "X", "Y"} {
		sayHint("G91 G00 " + ax + "1")
		if err := nc.cmd("G91 G00 "+ax+"1", 1.0); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond) // the display refreshes after the move
		now, err := snap(ax + " +1 мм")
		if err != nil {
			return err
		}
		for h, w := range now {
			if p, ok := prev[h]; ok && p.Text != w.Text {
				found++
				line := fmt.Sprintf("после %s+1 изменилось: %s id %d (%d,%d)  %q -> %q",
					ax, w.Class, w.ID, w.Left, w.Top, p.Text, w.Text)
				fmt.Fprintf(f, "%s\r\n", line)
				logLine(line)
			}
		}
		prev = now
	}
	if err := nc.Grab(); err == nil {
		nc.cmd("G91 G00 X-1 Y-1", 1.0)
		nc.cmd("G91 G00 Z-1", 1.0)
	}
	if found == 0 {
		sayWarn("ни один текст не изменился: NcStudio рисует цифры картинкой, читать придётся с экрана.")
	} else {
		sayOK("Координаты читаются как текст.")
	}
	logLine(fmt.Sprintf("Всё записано в %s, пришлите этот файл.", out))
	p.wait()
	return nil
}
