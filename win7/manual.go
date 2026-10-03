package main

// Manual mode: the operator drives NcStudio through this program one command at
// a time, with the MCU contact shown after every move.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// HotLift is what readLine returns for PageUp: lift at once, no Enter needed.
const HotLift = "\x01lift"

// Arrow keys, also without Enter: move X/Y by the current step.
const (
	HotLeft  = "\x01x-"
	HotRight = "\x01x+"
	HotUp    = "\x01y+"
	HotDown  = "\x01y-"
)

// cmdManual: the operator drives NcStudio through this program step by step,
// to see that keystrokes land and the MCU reports contact before any scan.
func cmdManual(o opts) error {
	show(rule("6 ► Ручной режим", boxW()))
	var m *MCU
	var name string
	if yes("подключить МК (показывать касание)?") {
		var err error
		if m, name, err = findMCU(o.port); err != nil {
			return err
		}
		sayOK("МК на " + name)
	}
	nc := newNc(o)
	step, stepX, stepY, stepFast := 0.1, 1.0, 1.0, 1.0
	show(box(thin, "Команды, Enter после каждой", twoCols(manualKeys(o), boxW()-4), boxW()))
	sayHint("Пока команда печатается в NcStudio, мышь и клавиатуру не трогать.")
	show(note("▲", cAlarm, "СТОП:", "щелчок мышью по любому другому окну во время движения.", boxW()))
	touching := func() bool { return m != nil && m.State() }
	for {
		say("\n")
		show(statusLine(Status{Port: name, MCU: mcuStateOf(m), Nc: ncFound(o.title), Z: nc.z, HasZ: true,
			More: []string{fmt.Sprintf("шаг Z %.3f · быстрый %.1f · X %.2f · Y %.2f", step, stepFast, stepX, stepY)}},
			boxW()))
		raw := prompt("")
		in := latin(raw)
		if strings.HasPrefix(raw, "\x01") {
			in = raw[1:] // PageUp or an arrow
		}
		moving := in == "u" || in == "d" || in == "uu" || in == "dd" || strings.HasPrefix(in, "g ") ||
			in == "x+" || in == "x-" || in == "y+" || in == "y-" || in == "t" || strings.HasPrefix(in, "t ") ||
			strings.HasPrefix(in, "m ")
		if moving && m != nil {
			if e := m.Alive(); e != nil {
				sayBad(fmt.Sprintf("не двигаю: %v", e))
				continue
			}
		}
		var err error
		switch {
		case in == "q":
			return nil
		case in == "lift":
			// going up is the way out of trouble: done even when the MCU is silent
			sayHint(fmt.Sprintf("быстрый подъём на %.1f мм", stepFast))
			if err = nc.Grab(); err == nil {
				err = nc.RapidZ(nc.z + stepFast)
			}
		case in == "u":
			if err = nc.Grab(); err == nil {
				err = nc.RapidZ(nc.z + step)
			}
		case in == "d" || in == "dd":
			if touching() {
				sayBad("фреза касается меди: вниз нельзя, сначала u")
				continue
			}
			if err = nc.Grab(); err == nil {
				if in == "d" {
					err = nc.MoveZ(nc.z-step, 20)
				} else {
					err = nc.MoveZ(nc.z-stepFast, 600)
				}
			}
		case in == "uu":
			if err = nc.Grab(); err == nil {
				err = nc.RapidZ(nc.z + stepFast)
			}
		case strings.HasPrefix(in, "m "):
			var dx, dy, dz float64
			if dx, dy, dz, err = parseShift(in[2:]); err != nil {
				sayBad(err.Error())
				continue
			}
			if touching() && dz <= 0 { // up comes first in shift, so lifting off is allowed
				sayBad("фреза касается меди: сначала поднимите Z (m z0.5)")
				continue
			}
			if err = nc.Grab(); err == nil {
				err = shift(nc, dx, dy, dz)
			}
		case in == "t" || strings.HasPrefix(in, "t "):
			depth := o.search
			if in != "t" {
				if _, e := fmt.Sscan(in[2:], &depth); e != nil || depth <= 0 || depth > 20 {
					sayBad("глубина поиска от 0.1 до 20 мм")
					continue
				}
			}
			if m == nil {
				sayBad("без МК касание не увидеть: выйдите в меню и подключите МК")
				continue
			}
			if touching() {
				sayBad("фреза уже касается меди")
				continue
			}
			if err = nc.Grab(); err == nil {
				err = lowerToCopper(o, nc, m, depth)
			}
		case strings.HasPrefix(in, "sz "):
			var v float64
			if _, e := fmt.Sscan(in[3:], &v); e == nil && v > 0 && v <= 50 {
				stepFast = v
			} else {
				sayBad("шаг быстрого Z от 0.1 до 50 мм")
			}
			continue
		case in == "x+" || in == "x-" || in == "y+" || in == "y-":
			if touching() {
				sayBad("фреза касается меди: сначала u, потом двигать по X/Y")
				continue
			}
			d := stepX
			if in[0] == 'y' {
				d = stepY
			}
			if in[1] == '-' {
				d = -d
			}
			if err = nc.Grab(); err == nil {
				if in[0] == 'x' {
					err = nc.MoveXYRel(d, 0)
				} else {
					err = nc.MoveXYRel(0, d)
				}
			}
		case strings.HasPrefix(in, "sx ") || strings.HasPrefix(in, "sy "):
			var v float64
			if _, e := fmt.Sscan(in[3:], &v); e == nil && v > 0 && v <= 300 {
				if in[1] == 'x' {
					stepX = v
				} else {
					stepY = v
				}
			} else {
				sayBad("шаг по X/Y от 0.01 до 300 мм")
			}
			continue
		case strings.HasPrefix(in, "s "):
			var v float64
			if _, e := fmt.Sscan(in[2:], &v); e == nil && v > 0 && v <= 5 {
				step = v
			} else {
				sayBad("шаг от 0.001 до 5 мм")
			}
			continue
		case strings.HasPrefix(in, "g "):
			line := strings.ToUpper(strings.TrimSpace(raw[2:]))
			if !yes("отправить в NcStudio \"" + line + "\"?") {
				continue
			}
			if err = nc.Grab(); err == nil {
				err = nc.cmd(line, 1.0) // our Z count no longer matches after a free-form line
			}
		case in == "?" || in == "":
		default:
			sayBad(fmt.Sprintf("не понял \"%s\"", raw))
			continue
		}
		if err != nil {
			sayStop(err)
		}
		if m != nil {
			if e := m.Alive(); e != nil {
				sayBad(e.Error())
			}
		}
	}
}

// parseShift reads "x5 y-2.5 z1" (any of the three, in any order) as relative mm.
func parseShift(s string) (dx, dy, dz float64, err error) {
	f := strings.Fields(strings.ReplaceAll(s, ",", "."))
	if len(f) == 0 {
		return 0, 0, 0, fmt.Errorf("пример: m x5 y-2.5 z1")
	}
	seen := map[byte]bool{}
	for _, w := range f {
		if len(w) < 2 || seen[w[0]] {
			return 0, 0, 0, fmt.Errorf("не понял \"%s\", пример: m x5 y-2.5 z1", w)
		}
		seen[w[0]] = true
		v, e := strconv.ParseFloat(w[1:], 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, 0, 0, fmt.Errorf("не число в \"%s\"", w)
		}
		switch w[0] {
		case 'x':
			dx = v
		case 'y':
			dy = v
		case 'z':
			dz = v
		default:
			return 0, 0, 0, fmt.Errorf("оси только x, y, z: \"%s\"", w)
		}
	}
	if math.Abs(dx) > 600 || math.Abs(dy) > 600 || math.Abs(dz) > 50 {
		return 0, 0, 0, fmt.Errorf("слишком далеко: X/Y до 600 мм, Z до 50 мм")
	}
	return dx, dy, dz, nil
}

// shift moves so the tool never sweeps sideways low: up first, then X/Y, then down.
// Going down is G01 at 600 mm/min, as dd.
func shift(nc *NcStudio, dx, dy, dz float64) error {
	if dz > 0 {
		if err := nc.RapidZ(nc.z + dz); err != nil {
			return err
		}
	}
	if dx != 0 || dy != 0 {
		if err := nc.MoveXYRel(dx, dy); err != nil {
			return err
		}
	}
	if dz < 0 {
		return nc.MoveZ(nc.z+dz, 600)
	}
	return nil
}

// lowerToCopper goes down in short steps until the MCU reports contact, then
// backs off and finds it again with fine steps; the tool stays touching the copper.
func lowerToCopper(o opts, nc *NcStudio, m Touch, depth float64) error {
	s := newScanner(o, nc, m)
	s.Log = func(string) {}
	z, ok, err := s.stepDown(depth, s.StepCoarse, s.FeedCoarse)
	if err != nil {
		return err
	}
	if !ok {
		return fail("медь не найдена на %.2f мм ниже: зажим не на меди или нужно глубже (t 5)", depth)
	}
	if err := s.liftUntilOpen(z, s.StepCoarse); err != nil {
		return err
	}
	zf, ok, err := s.stepDown(s.StepCoarse*3, s.StepFine, s.FeedFine)
	if err != nil {
		return err
	}
	if !ok {
		return fail("повторное касание не случилось")
	}
	sayOK(fmt.Sprintf("медь на Z %+.3f от старта ручного режима", zf))
	return nil
}

// manualKeys: the left column has the long keys, the right one the long texts.
func manualKeys(o opts) [][2]string {
	return [][2]string{
		{"u", "Z вверх на шаг"}, {"d", "Z вниз на шаг медленно"},
		{"uu", "Z быстро вверх на sz"}, {"dd", "Z быстро вниз на sz"},
		{"t", fmt.Sprintf("до меди, не глубже %.1f", o.search)}, {"t 5", "до меди, не глубже 5"},
		{"x+ x-", "X на шаг sx"}, {"y+ y-", "Y на шаг sy"},
		{"s 0.05", "шаг по Z, мм"}, {"sz 2", "шаг быстрого Z, мм"},
		{"sx 5", "шаг по X, мм"}, {"sy 5", "шаг по Y, мм"},
		{"m x5 y-2 z1", "сдвиг по X Y Z"}, {"g G91 X5", "строка G-кода как есть"},
		{"?", "есть ли касание"}, {"q", "назад в меню"},
		{"← → ↑ ↓", "X/Y на шаг, без Enter"}, {"PageUp", "Z вверх на sz без Enter"},
	}
}

func mcuStateOf(m *MCU) mcuState {
	switch {
	case m == nil:
		return mcuNone
	case m.Alive() != nil:
		return mcuDead
	case m.State():
		return mcuContact
	}
	return mcuOpen
}
