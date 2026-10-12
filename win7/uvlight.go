package main

// The UV mask cured by a torch on the spindle: a narrow spot, so the machine
// carries it over the sheet in a zigzag, rows half a spot apart, every point
// lit twice. The spindle stays off and Z does not move: the operator sets the
// height of the torch by hand.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"pcbscan/ui"
)

type uvCfg struct {
	Spot   float64 // the lit spot on the board, mm
	Feed   float64
	Passes float64
	DX, DY float64 // the centre of the spot from the axis of the spindle
}

var uv = uvCfg{Spot: 20, Feed: 20, Passes: 1}

// uvProgram: the centres of the spot, zigzag over w x h, and where the spindle
// goes for them (the spot is off the axis by DX, DY).
func uvProgram(p uvCfg, w, h float64) (lines []string, minutes float64) {
	step := p.Spot / 2
	n := int(math.Ceil(h/step-1e-9)) + 1
	lines = []string{"M5", "G90"}
	var length float64
	for k := 0; k < int(math.Max(1, p.Passes)); k++ {
		for i := 0; i < n; i++ {
			y := math.Min(h, float64(i)*step)
			x0, x1 := 0.0, w
			if i%2 == 1 {
				x0, x1 = w, 0
			}
			mv := "G1"
			if k == 0 && i == 0 {
				mv = "G0" // to the start in the air
			} else {
				length += step
			}
			lines = append(lines,
				fmt.Sprintf("%sX%sY%sF%.0f", mv, n3(x0-p.DX), n3(y-p.DY), p.Feed),
				fmt.Sprintf("G1X%sY%sF%.0f", n3(x1-p.DX), n3(y-p.DY), p.Feed))
			length += w
		}
	}
	lines = append(lines, "G0X0.000Y0.000", "M30")
	return lines, length / p.Feed
}

// uvLight: the numbers, the program, and NcStudio.
func (u *jobUI) uvLight() error {
	p := &uv
	field := func(label, unit string, lo, hi, step float64, digits int, v *float64) formField {
		return formField{label: label, unit: unit, lo: lo, hi: hi, step: step, digits: digits,
			get: func() float64 { return *v }, set: func(x float64) { *v = x }}
	}
	f := &numForm{title: "Засветка маски фонарём", ok: "Сделать файл", fields: []formField{
		field("пятно", "мм", 3, 100, 1, 0, &p.Spot),
		field("подача", "мм/мин", 5, 2000, 5, 0, &p.Feed),
		field("проходов", "", 1, 10, 1, 0, &p.Passes),
		field("фонарь от оси X", "мм", -200, 200, 1, 0, &p.DX),
		field("фонарь от оси Y", "мм", -200, 200, 1, 0, &p.DY),
	}, about: func() []string {
		_, min := uvProgram(*p, u.j.W, u.j.H)
		return []string{
			"Змейка по всему листу, ряды через половину пятна: каждая точка под светом дважды.",
			fmt.Sprintf("Точка под светом ~%.0f с, весь лист ~%.0f мин.", 2*p.Spot/p.Feed*60*math.Max(1, p.Passes), math.Ceil(min)),
			"Шпиндель выключен, Z не двигается.",
		}
	}}
	f.scene = func() ui.Scene { return newSheetScene(u.j, u.mirror(), sheetOverlay{}) }
	if !editForm(f) {
		return nil
	}
	lines, min := uvProgram(*p, u.j.W, u.j.H)
	dst := filepath.Join(exeDir(), "uv.tap")
	if err := os.WriteFile(dst, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644); err != nil {
		return err
	}
	about := []string{
		"Файл:  " + dst,
		fmt.Sprintf("Лист %sx%s мм, ~%.0f мин.", num(u.j.W), num(u.j.H), math.Ceil(min)),
		"",
		"Закрепите фонарь у шпинделя светом вниз и включите его.",
		"Поднимите Z так, чтобы пятно на плате было " + num(p.Spot) + " мм.",
		"Шпиндель не включайте. Не смотрите на свет и блики.",
		"",
		"pcbscan откроет файл в NcStudio. Запускаете вы: F9.",
	}
	if !askYes("Засветка: открыть в NcStudio?", about...) {
		return nil
	}
	machineBusy(true)
	err := newNc(u.o).Load(dst)
	machineBusy(false)
	backToApp()
	if err != nil {
		message("Засветка", "Откройте в NcStudio сами (Файл, Открыть):", dst, "и запустите F9.")
		return nil
	}
	message("Засветка", "Файл открыт в NcStudio. Запустите его там: F9.")
	return nil
}
