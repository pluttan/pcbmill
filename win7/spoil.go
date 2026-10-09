package main

// The pocket in the spoil board: a seat for the blank, cut once with a flat
// end mill (the 8 mm key-seat cutter). Its corner is the corner of the sheet,
// so a blank pushed into it lies where every program expects it. The board is
// acrylic: the probe does not feel it, Z0 is set by hand on its top.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"pcbscan/ui"
)

// pocketCfg: the numbers of the pocket, kept for the session.
type pocketCfg struct {
	W, H      float64 // the pocket, from the corner of the sheet
	Depth     float64
	Tool      float64 // diameter of the cutter
	Step      float64 // down per pass
	Over      float64 // across between rings
	Feed      float64
	Plunge    float64
	Spindle   float64
	CornersSq float64 // 1: the corners cut out for a square blank
}

var pocket = pocketCfg{Depth: 1.0, Tool: 8, Step: 0.4, Over: 3.5, Feed: 800, Plunge: 150, Spindle: 15000, CornersSq: 1}

// pocketOp: per depth one path, from the middle out in rings against the
// turn of the cutter (climb), the last ring on the walls; at the corners of
// the last ring a step out along the diagonal, so a square corner of the
// blank fits (a dog-bone).
func pocketOp(p pocketCfg) (*camOp, error) {
	r := p.Tool / 2
	if p.W < p.Tool || p.H < p.Tool {
		return nil, fmt.Errorf("карман %sx%s мм уже фрезы %s мм", num(p.W), num(p.H), num(p.Tool))
	}
	if p.Over <= 0 || p.Over > p.Tool || p.Step <= 0 || p.Depth <= 0 {
		return nil, fmt.Errorf("шаг вбок до %s мм, шаг вниз и глубина больше нуля", num(p.Tool))
	}
	x0, x1, y0, y1 := r, p.W-r, r, p.H-r // the centre of the cutter on the walls
	n := int(math.Ceil(math.Min(x1-x0, y1-y0) / 2 / p.Over))
	bone := r - r/math.Sqrt2
	ring := func(k int, z float64) []p3 {
		d := float64(k) * p.Over
		a0, a1, b0, b1 := math.Min(x0+d, (x0+x1)/2), math.Max(x1-d, (x0+x1)/2), math.Min(y0+d, (y0+y1)/2), math.Max(y1-d, (y0+y1)/2)
		corners := [][2]float64{{a0, b0}, {a1, b0}, {a1, b1}, {a0, b1}, {a0, b0}}
		sx := []float64{-1, 1, 1, -1, -1}
		sy := []float64{-1, -1, 1, 1, -1}
		var out []p3
		for i, c := range corners {
			out = append(out, p3{c[0], c[1], z})
			if k == 0 && p.CornersSq > 0 && i < 4 {
				out = append(out, p3{c[0] + sx[i]*bone, c[1] + sy[i]*bone, z}, p3{c[0], c[1], z})
			}
		}
		return out
	}
	op := &camOp{file: "pocket", title: "Карман в жертвенной пластине", plunge: p.Plunge, feed: p.Feed, spindle: p.Spindle,
		tool: fmt.Sprintf("фреза %s мм", num(p.Tool))}
	for _, z := range levels(p.Depth, p.Step) {
		var pts []p3
		for k := n; k >= 0; k-- {
			pts = append(pts, ring(k, -z)...)
		}
		op.paths = append(op.paths, cpath{pts: pts})
	}
	op.notes = append(op.notes, fmt.Sprintf("карман X0..%s Y0..%s, глубина %s за %d прох., фреза %s мм",
		num(p.W), num(p.H), num(p.Depth), len(op.paths), num(p.Tool)))
	return op, nil
}

// pocket: the numbers, the program, and NcStudio.
func (u *jobUI) pocket() error {
	p := &pocket
	if p.W == 0 {
		p.W, p.H = u.j.W+0.4, u.j.H+0.4 // the blank goes in with a little play
	}
	field := func(label, unit string, lo, hi, step float64, digits int, v *float64) formField {
		return formField{label: label, unit: unit, lo: lo, hi: hi, step: step, digits: digits,
			get: func() float64 { return *v }, set: func(x float64) { *v = x }}
	}
	f := &numForm{title: "Карман в жертвенной пластине", ok: "Сделать файл", fields: []formField{
		field("ширина X", "мм", 10, 600, 1, 1, &p.W),
		field("высота Y", "мм", 10, 600, 1, 1, &p.H),
		field("глубина", "мм", 0.1, 10, 0.1, 2, &p.Depth),
		field("фреза", "мм", 1, 20, 0.5, 1, &p.Tool),
		field("за проход", "мм", 0.05, 3, 0.05, 2, &p.Step),
		field("шаг вбок", "мм", 0.5, 20, 0.5, 1, &p.Over),
		field("подача", "мм/мин", 50, 3000, 50, 0, &p.Feed),
		field("врезание", "мм/мин", 20, 1000, 10, 0, &p.Plunge),
		field("обороты", "об/мин", 3000, 30000, 1000, 0, &p.Spindle),
		{label: "углы", choices: []string{"скруглены", "под квадрат"}, get: func() float64 { return p.CornersSq },
			set: func(x float64) { p.CornersSq = x }},
	}, about: func() []string {
		out := []string{"Угол кармана в нуле X Y, там же угол листа: заготовку прижмите к нему.",
			"Оргстекло: тонкие слои, быстрая подача, выдувайте стружку."}
		if op, err := pocketOp(*p); err == nil {
			_, min := op.minutes()
			out = append(out, fmt.Sprintf("Проходов %d, ~%.0f мин.", len(op.paths), math.Ceil(min)))
		} else {
			out = append(out, "▲ "+err.Error())
		}
		return out
	}}
	f.scene = func() ui.Scene {
		op, _ := pocketOp(*p)
		var paths []cpath
		if op != nil {
			paths = op.paths[len(op.paths)-1:]
		}
		return newSheetScene(u.j, false, sheetOverlay{paths: paths, stage: "4"})
	}
	if !editForm(f) {
		return nil
	}
	op, err := pocketOp(*p)
	if err != nil {
		return err
	}
	dst := filepath.Join(exeDir(), "pocket.tap")
	if err := os.WriteFile(dst, []byte(strings.Join(op.gcode(), "\r\n")+"\r\n"), 0o644); err != nil {
		return err
	}
	_, min := op.minutes()
	about := []string{
		"Файл:  " + dst,
		fmt.Sprintf("Фреза %s мм, глубина %s мм, ~%.0f мин.", num(p.Tool), num(p.Depth), math.Ceil(min)),
		"",
		"Ноль X Y: угол листа, как для плат.",
		"Ноль Z: верх пластины. Щуп оргстекло не чувствует:",
		"опустите фрезу до листа бумаги на пластине и обнулите Z в NcStudio.",
		"",
		"pcbscan откроет файл в NcStudio. Запускаете вы: F9.",
	}
	if !askYes("Карман: открыть в NcStudio?", about...) {
		return nil
	}
	machineBusy(true)
	err = newNc(u.o).Load(dst)
	machineBusy(false)
	backToApp()
	if err != nil {
		message("Карман", "Откройте в NcStudio сами (Файл, Открыть):", dst, "и запустите F9.")
		return nil
	}
	message("Карман", "Файл открыт в NcStudio. Запустите его там: F9.")
	return nil
}
