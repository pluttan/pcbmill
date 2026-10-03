package main

// The emulation on screen: the virtual machine and MCU stand in for the real
// ones in every menu item, the head bar says ЭМУЛЯЦИЯ, the status line shows
// where the virtual NcStudio has the tool, and menu item 8 or the digits 1-6 on the work screens switch
// the faults the protections are there for.

import (
	"fmt"
	"time"
)

// faultKeys says whether the digits switch faults on the screen now: on the
// scan, manual and wire screens, not where digits pick items or are typed.
var faultKeys func() bool

func faultKeysOn(on func() bool) func() {
	faultKeys = on
	return func() { faultKeys = nil }
}

// simM is the virtual machine and MCU when the program runs in emulation.
var simM *simMachine

// startSim: the blank under the tool, 1.5 mm over the copper at X0 Y0 and Z
// zeroed there, like an operator leaves it before a scan.
func startSim(o opts) {
	simM = newSimMachine(simSurface)
	simM.Speed = o.simSpeed
	if simM.Speed <= 0 {
		simM.Speed = 1
	}
	simM.Rapid = o.rapid
	simM.rnd.Seed(time.Now().UnixNano())
	simM.Reset()
	ncWindows = simWindows
	keyHook = func(k Key) bool {
		if faultKeys == nil || !faultKeys() {
			return false
		}
		f, ok := faultKey(k)
		if ok {
			simM.Toggle(f)
		}
		return ok
	}
}

// findTouch is the MCU of the work: the virtual one in emulation.
func findTouch(port string) (Touch, string, error) {
	if simM != nil {
		return simM, "эмуляция", nil
	}
	m, name, err := findMCU(port)
	if err != nil {
		return nil, "", err
	}
	return m, name, nil
}

// mcuAt says where the MCU is: "МК на COM5", or that it is the virtual one.
func mcuAt(port string) string {
	if simM != nil {
		return "МК " + paint(cTitle, "виртуальный")
	}
	return "МК на " + paint(cTitle, port)
}

func simStatus() Status {
	x, y, z, moving := simM.WorkPos()
	return Status{Port: "эмуляция", MCU: mcuStateOf(simM), Nc: true,
		Sim: &SimInfo{X: x, Y: y, Z: z, Moving: moving, Faults: simM.Faults()}}
}

// The lines of the emulation panel after the faults.
const (
	simReset = int(fCount) + iota
	simClear
	simLines
)

type simPanel struct{ sel int }

func (p *simPanel) key(k Key) bool {
	switch k.Code {
	case KeyUp, KeyBackTab:
		p.sel = (p.sel + simLines - 1) % simLines
	case KeyDown, KeyTab:
		p.sel = (p.sel + 1) % simLines
	case KeyEnter:
		p.do(p.sel)
	case KeyEsc, KeyCtrlC:
		return true
	case KeyRune:
		if k.R == ' ' {
			p.do(p.sel)
		} else if d := int(k.R - '1'); d >= 0 && d < simLines {
			p.sel = d
			p.do(d)
		}
	}
	return false
}

func (p *simPanel) do(i int) {
	switch {
	case i < int(fCount):
		simM.Toggle(simFault(i))
	case i == simReset:
		simM.Reset()
		logLine("эмуляция: фреза над X0 Y0, 1.5 мм над медью, Z обнулён")
	case i == simClear:
		for f := simFault(0); f < fCount; f++ {
			if simM.On(f) {
				simM.Toggle(f)
			}
		}
	}
}

func (p *simPanel) view(w, h int) []string {
	bw := colW(w)
	if bw > 64 {
		bw = 64
	}
	x, y, z, moving := simM.WorkPos()
	state := paint(cHint, "стоит")
	if moving {
		state = paint(cTouch, "едет")
	}
	axis := func(name string, v float64, more string) string {
		return paint(cHint, name+" ") + paint(cTitle, fmt.Sprintf("%9.3f", v)) + paint(cHint, " мм") + "      " + more
	}
	nc := box(thin, "NcStudio, эмуляция", []string{
		axis("X", x, paint(cHint, "ход    ")+state),
		axis("Y", y, paint(cHint, "МК     ")+mcuBadge(mcuStateOf(simM))),
		axis("Z", z, paint(cHint, "время  ")+paint(cTitle, fmt.Sprintf("x%g", simM.Speed))),
	}, bw)
	var lines []string
	for i := 0; i < simLines; i++ {
		var t string
		switch {
		case i < int(fCount):
			mark := "[ ]"
			if simM.On(simFault(i)) {
				mark = "[√]"
			}
			t = fmt.Sprintf("%d  %s  %s", simFaults[i].fkey, mark, simFaults[i].long)
		case i == simReset:
			lines = append(lines, sep)
			t = "   Фрезу в начало: X0 Y0, 1.5 мм над медью"
		case i == simClear:
			t = "   Выключить все неисправности"
		}
		if i == p.sel {
			lines = append(lines, sel(t))
		} else if i < int(fCount) && simM.On(simFault(i)) {
			lines = append(lines, paint(cAlarm, t))
		} else {
			lines = append(lines, t)
		}
	}
	faults := box(thin, "Неисправности", lines, bw)
	var about []string
	for _, l := range wrap("Заготовка 100x100 мм от X-2 Y-2, перекос и бугор; вне её меди нет.", bw, 0) {
		about = append(about, center(paint(cHint, l), bw))
	}
	body := stack(nc, faults, about)
	if len(body) > room(w, h) { // the status line shows X Y Z too
		body = stack(faults, about)
	}
	if len(body) > room(w, h) {
		body = faults
	}
	return screen(w, h, menuTitle("9"), body,
		hints(w, "↑↓", "выбор", "Enter", "вкл / выкл", "1-6", "сразу", "Esc", "в меню"))
}

// cmdSim is menu item 8: the virtual machine and its faults.
func cmdSim(o opts) error {
	p := &simPanel{}
	run(p.view, p.key, 200*time.Millisecond)
	return nil
}
