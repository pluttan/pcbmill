package main

// The service commands: the wire check, the NcStudio check, one touch for
// Z0, the search for the coordinates in the NcStudio windows. Each runs on
// its own page, asks in dialogs and leaves the page until Готово.

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"pcbscan/gfx"
	"pcbscan/ui"
)

// wireBig: the large contact indicator with the count of touches.
func wireBig(touching bool, hits int, since string) func(c *ui.Ctx, r image.Rectangle) {
	return func(c *ui.Ctx, r image.Rectangle) {
		t := c.T
		h := imin(r.Dy(), t.Si(220))
		box := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h)
		col, word, ic := t.Green, "разомкнуто", ui.IconRing
		if touching {
			col, word, ic = t.Yellow, "КОНТАКТ", ui.IconZero
		}
		c.C.FillRoundRect(float64(box.Min.X), float64(box.Min.Y), float64(box.Dx()), float64(box.Dy()), t.S(14), gfx.Alpha(col, 0.14))
		c.C.StrokeRoundRect(float64(box.Min.X)+0.5, float64(box.Min.Y)+0.5, float64(box.Dx())-1, float64(box.Dy())-1, t.S(14), t.S(2), gfx.Alpha(col, 0.7))
		c.Icon(ic, image.Rect(box.Min.X+box.Dx()/2-t.Si(28), box.Min.Y+t.Si(26), box.Min.X+box.Dx()/2+t.Si(28), box.Min.Y+t.Si(82)), col)
		c.LabelIn(image.Rect(box.Min.X, box.Min.Y+t.Si(96), box.Max.X, box.Min.Y+t.Si(140)), word, ui.Style{Kind: ui.Bold, Size: ui.SizeHuge, Color: col}, 1)
		c.LabelIn(image.Rect(box.Min.X, box.Min.Y+t.Si(150), box.Max.X, box.Min.Y+t.Si(180)),
			fmt.Sprintf("касаний %d   последнее изменение %s", hits, since), c.Muted(), 1)
	}
}

func cmdWire(o opts) error {
	p := newPage(menuTitle("2"), "Ищу МК на COM-портах...")
	m, name, err := findTouch(o.port)
	if err != nil {
		return err
	}
	p.set(func() {
		p.head = "Проверка провода"
		p.info = []string{mcuAt(name) + ". Касайтесь зажимом фрезы: здесь должно загораться КОНТАКТ. Станок не двигается."}
		p.btns = []pageBtn{{id: "ok", label: "Готово", kind: ui.BtnPrimary, key: ui.KeyEnter}}
	})
	backToApp()
	defer faultKeysOn(func() bool { return true })()
	last, since, start := !m.State(), "-", m.Hits()
	p.drain()
	for {
		if e := m.Alive(); e != nil {
			return e
		}
		if s := m.State(); s != last {
			last = s
			since = time.Now().Format("15:04:05")
			if s {
				logLine("♦ " + since + "  КОНТАКТ")
			} else {
				logLine(since + "  разомкнуто")
			}
		}
		touching, hits, at := last, m.Hits()-start, since
		p.set(func() {
			p.extra = func(c *ui.Ctx, r image.Rectangle) int {
				wireBig(touching, hits, at)(c, r)
				return imin(r.Dy(), c.T.Si(232))
			}
		})
		if _, ok := p.pressed(); ok {
			return nil
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func cmdNcTest(o opts) error {
	p := newPage(menuTitle("3"),
		"Фреза не ниже 5 мм над заготовкой. Программа нажмёт в NcStudio "+o.mdiKey+" и введёт",
		"G91 G00 Z1 - вверх на 1 мм, потом G91 G00 Z-1 - обратно.",
		"",
		"Пока она печатает, мышь и клавиатуру не трогать.")
	p.set(func() { p.head = "Проверка связи с NcStudio"; p.scene = toolScene() })
	if !confirm("Начать проверку связи?") {
		return nil
	}
	n := newNc(o)
	machineBusy(true)
	if err := n.Grab(); err != nil {
		machineBusy(false)
		return err
	}
	sayHint("G91 G00 Z1")
	err := n.RapidZ(1)
	machineBusy(false)
	if err != nil {
		return err
	}
	up := confirm("Z поднялся на 1 мм?")
	machineBusy(true)
	if err := n.Grab(); err != nil {
		machineBusy(false)
		return err
	}
	sayHint("G91 G00 Z-1")
	err = n.RapidZ(0)
	machineBusy(false)
	if err != nil {
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

// zeroAt: one slow touch of the copper at the point, then the operator zeroes
// Z in NcStudio there and the tool goes up. errBack: the operator said no,
// or pressed Отмена on the way; the tool goes back up then.
func zeroAt(o opts, title string, at [2]float64, why string) error {
	p := newPage(title, "Ищу МК на COM-портах...")
	m, _, err := findTouch(o.port)
	if err != nil {
		return err
	}
	p.set(func() {
		p.head = "Ноль Z"
		p.info = []string{
			fmt.Sprintf("Фреза стоит над медью не выше %s мм, можно касаться меди. Будет медленное касание в ", num(zeroSearch)) +
				fmt.Sprintf("♦ X%.2f Y%.2f", at[0], at[1]) + " (" + why + "),",
			"потом нужно обнулить Z в NcStudio.",
		}
		p.scene = jobToolScene()
	})
	if !confirm("Начать касание?") {
		return errBack
	}
	nc := newNc(o)
	machineBusy(true)
	defer machineBusy(false)
	if err := nc.Grab(); err != nil {
		return err
	}
	p.buttons(pageBtn{id: "cancel", label: "Отмена", kind: ui.BtnDanger, key: ui.KeyEsc})
	defer p.buttons()
	s := newScanner(o, zeroStop{nc, p}, m)
	s.Search = zeroSearch
	s.StepCoarse = 0.05 // the thin tip: small steps, the fine touch follows
	_, _, z0 := nc.Pos()
	back := func() error { // up to where it started, the zero not taken
		nc.Grab()
		nc.RapidZ(z0)
		return errBack
	}
	if m.State() { // set down on the copper: up first, or the touch sees nothing
		if err := s.liftUntilOpenN(z0, 0.1, 20); err != nil {
			if errors.Is(err, errZeroStop) {
				return back()
			}
			return err
		}
		_, _, z0 = nc.Pos()
	}
	z, err := s.TouchAt(at[0], at[1], z0)
	if errors.Is(err, errZeroStop) {
		return back()
	}
	if err != nil {
		return err
	}
	if err := nc.MoveZ(z, 20); err != nil {
		return err
	}
	setCopperZ(z)
	machineBusy(false)
	p.buttons()
	logLine("♦ КОНТАКТ: фреза касается меди.")
	d := &dialog{title: "Обнулите Z в NcStudio", kind: dlgAsk, text: []string{
		"Фреза касается меди.",
		"Обнулите Z в NcStudio (рабочая система координат) и вернитесь сюда.",
		"Без обнуления программа уйдёт на неверную глубину.",
		"Отмена поднимет фрезу, ноль не будет записан."},
		buttons: []string{"Z обнулён", "Отмена"}, sel: 0, cancel: 1}
	if d.show() != 0 {
		setCopperZ(0)
		machineBusy(true)
		if err := nc.Grab(); err != nil {
			return err
		}
		nc.RapidZ(z + o.clear*3)
		return errBack
	}
	if simM != nil {
		simM.ZeroZ() // what the operator does in NcStudio
		logLine("эмуляция: Z обнулён в NcStudio на этой высоте")
	}
	nc.ResetZ()
	setCopperZ(0)
	machineBusy(true)
	if err := nc.Grab(); err != nil {
		return err
	}
	return nc.RapidZ(o.clear * 3)
}

// zeroSearch: how high over the copper the touch for Z0 may start, mm.
const zeroSearch = 6.0

var errZeroStop = errors.New("Z0 отменён")

// zeroStop: the Отмена button of the page stops the touch before the next
// step; a click on the page does not take the focus from NcStudio.
type zeroStop struct {
	Machine
	p *page
}

func (z zeroStop) stopped() bool {
	id, ok := z.p.pressed()
	return ok && id == "cancel"
}

func (z zeroStop) MoveZ(v, feed float64) error {
	if z.stopped() {
		return errZeroStop
	}
	return z.Machine.MoveZ(v, feed)
}

func (z zeroStop) RapidXY(x, y float64) error {
	if z.stopped() {
		return errZeroStop
	}
	return z.Machine.RapidXY(x, y)
}

// ncWindows lists the controls of NcStudio; the emulation has its own.
var ncWindows = NcWindows

// cmdFindCoords: can this program read the X/Y/Z NcStudio shows? Snapshot the text
// of every NcStudio control, move each axis by 1 mm, and see which texts change.
func cmdFindCoords(o opts) error {
	p := newPage(menuTitle("8"),
		"Программа ищет, откуда можно читать координаты NcStudio.",
		"Станок сдвинется: Z вверх на 1 мм, X на +1, Y на +1, и вернётся обратно.",
		"Фреза должна быть поднята над платой, шпиндель стоять.")
	p.set(func() { p.head = "Координаты NcStudio"; p.scene = toolScene() })
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
		ws, err := ncWindows(o.title)
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
	machineBusy(true)
	if err := nc.Grab(); err != nil {
		machineBusy(false)
		return err
	}
	found := 0
	prev := base
	for _, ax := range []string{"Z", "X", "Y"} {
		sayHint("G91 G00 " + ax + "1")
		if err := nc.cmd("G91 G00 "+ax+"1", 1.0); err != nil {
			machineBusy(false)
			return err
		}
		time.Sleep(500 * time.Millisecond) // the display refreshes after the move
		now, err := snap(ax + " +1 мм")
		if err != nil {
			machineBusy(false)
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
	machineBusy(false)
	if found == 0 {
		sayWarn("ни один текст не изменился: NcStudio рисует цифры картинкой, читать придётся с экрана.")
	} else {
		sayOK("Координаты читаются как текст.")
	}
	logLine(fmt.Sprintf("Всё записано в %s, пришлите этот файл.", out))
	p.wait()
	return nil
}

// toolScene: the sheet of the job with the tool on it, for the pages that
// move the machine outside the job.
func toolScene() ui.Scene { return jobToolScene() }

func jobToolScene() ui.Scene {
	j := newJob("")
	mirror := false
	if theJob != nil {
		j, mirror = theJob.j, theJob.mirror()
	}
	s := newSheetScene(j, mirror, sheetOverlay{})
	s.tool = true
	return s
}

// menuTitle: the heading of a service page.
func menuTitle(k string) string {
	switch k {
	case "2":
		return "сервис / Проверить провод"
	case "3":
		return "сервис / Связь с NcStudio"
	case "7":
		return "сервис / Ручной режим"
	case "8":
		return "сервис / Координаты NcStudio"
	case "9":
		return "сервис / Эмуляция"
	}
	return "сервис"
}
