package main

// The emulation on screen: the virtual machine and MCU stand in for the real
// ones in every menu item, the head bar says ЭМУЛЯЦИЯ, the status line shows
// where the virtual NcStudio has the tool, and menu item 8 or the digits 1-6 on the work screens switch
// the faults the protections are there for.

import (
	"fmt"
	"image"
	"time"

	"pcbscan/ui"
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
		return "МК виртуальный"
	}
	return "МК на " + port
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

// simPanel: the faults as switches, the tool in 3D.
func simPanel(c *ui.Ctx, r image.Rectangle) int {
	t := c.T
	y := r.Min.Y
	x, yy, z, moving := simM.WorkPos()
	state := "стоит"
	if moving {
		state = "едет"
		c.Animate()
	}
	mono := ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}
	c.Label(r.Min.X, y, fmt.Sprintf("X %8.3f   Y %8.3f   Z %+8.3f   %s, время x%g", x, yy, z, state, simM.Speed), mono)
	y += c.LineH(mono) + t.Si(14)
	c.Label(r.Min.X, y, "Неисправности: щелчок или цифра 1-6 включает и выключает", ui.Style{Kind: ui.Bold, Size: ui.SizeSmall + 1, Color: t.Lavender})
	y += t.Si(26)
	rowH := t.Si(38)
	for i := 0; i < int(fCount); i++ {
		rr := image.Rect(r.Min.X, y, r.Max.X, y+rowH-t.Si(4))
		on := simM.On(simFault(i))
		if c.Row(fmt.Sprintf("sim/%d", i), rr, false) {
			simM.Toggle(simFault(i))
		}
		// a switch: a pill with a knob
		sw := image.Rect(rr.Min.X+t.Si(8), rr.Min.Y+(rr.Dy()-t.Si(20))/2, rr.Min.X+t.Si(44), rr.Min.Y+(rr.Dy()+t.Si(20))/2)
		bg := t.Surface1
		kx := float64(sw.Min.X) + t.S(10)
		if on {
			bg = t.Red
			kx = float64(sw.Max.X) - t.S(10)
		}
		c.C.FillRoundRect(float64(sw.Min.X), float64(sw.Min.Y), float64(sw.Dx()), float64(sw.Dy()), t.S(10), bg)
		c.C.FillCircle(kx, float64(sw.Min.Y)+float64(sw.Dy())/2, t.S(7), t.Text)
		st := c.Body()
		if on {
			st.Color = t.Red
		}
		c.LabelIn(image.Rect(sw.Max.X+t.Si(12), rr.Min.Y, rr.Max.X, rr.Max.Y), fmt.Sprintf("%d  %s", simFaults[i].fkey, simFaults[i].long), st, 0)
		y += rowH
	}
	y += t.Si(8)
	bw, bh := simSize()
	y += c.Para(image.Rect(r.Min.X, y, r.Max.X, r.Max.Y),
		fmt.Sprintf("Заготовка размером с лист задания: %sx%s мм и 1 мм вокруг, перекос и бугор; вне её меди нет.", num(bw), num(bh)), c.Small())
	return y - r.Min.Y
}

// cmdSim: the virtual machine and its faults.
func cmdSim(o opts) error {
	p := newPage(menuTitle("9"))
	p.set(func() {
		p.head = "Эмуляция"
		p.extra = simPanel
		p.scene = jobToolScene()
	})
	defer faultKeysOn(func() bool { return true })()
	for {
		switch p.ask(pageBtn{id: "reset", label: "Фрезу в начало", kind: ui.BtnNormal},
			pageBtn{id: "clear", label: "Выключить все", kind: ui.BtnNormal},
			pageBtn{id: "ok", label: "Готово", kind: ui.BtnPrimary, key: ui.KeyEsc}) {
		case "reset":
			simM.Reset()
			logLine("эмуляция: фреза над X0 Y0, 1.5 мм над медью, Z обнулён")
		case "clear":
			for f := simFault(0); f < fCount; f++ {
				if simM.On(f) {
					simM.Toggle(f)
				}
			}
		default:
			return nil
		}
	}
}

// faultChar: a digit on a work screen switches a fault of the emulation.
func faultChar(e ui.Event) bool {
	if simM == nil || faultKeys == nil || !faultKeys() || e.Kind != ui.EvChar {
		return false
	}
	f, ok := faultKey(e.Rune)
	if ok {
		simM.Toggle(f)
	}
	return ok
}
