package main

// The map grid before the scan: drawn over the program cut, changed by short
// commands until the operator presses Enter.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func showGrid(g Grid, prog Grid) {
	cut := rect{prog.X0, prog.X1, prog.Y0, prog.Y1}
	say("\n")
	show(rule("Сетка карты высот", boxW()))
	showMid(gridView(g, &cut, mapW()))
	show(box(thin, "Сетка", gridInfo(g, prog), boxW()))
}

func gridInfo(g, prog Grid) []string {
	n := g.NX * g.NY
	return []string{
		fmt.Sprintf("область  X %.2f .. %.2f   Y %.2f .. %.2f мм", g.X0, g.X1, g.Y0, g.Y1),
		fmt.Sprintf("точек    X %s (шаг %.2f)   Y %s (шаг %.2f)   всего %s", paint(cTitle, strconv.Itoa(g.NX)),
			g.StepX(), paint(cTitle, strconv.Itoa(g.NY)), g.StepY(), paint(cTitle, strconv.Itoa(n))),
		fmt.Sprintf("время    примерно %s мин", paint(cTitle, fmt.Sprintf("%.0f", float64(n)*secondsPerPoint/60))),
		paint(cHint, fmt.Sprintf("рез программы: X %.2f .. %.2f   Y %.2f .. %.2f", prog.X0, prog.X1, prog.Y0, prog.Y1)),
	}
}

var gridKeys = [][2]string{
	{"x 0 60", "область по X, мм"}, {"sx 5", "шаг по X, мм"},
	{"y 0 40", "область по Y, мм"}, {"sy 5", "шаг по Y, мм"},
	{"m 2", "запас за край реза"}, {"s 10", "шаг по обеим осям"},
	{"nx 8", "точек по X"}, {"ny 6", "точек по Y"},
	{"r", "как в программе"}, {"q", "отмена"},
	{"Enter", "начать обход с этой сеткой"},
}

// keepStep: the current step, or the program's when there is a single point (no step yet).
func keepStep(cur, prog float64) float64 {
	if cur >= 1 {
		return cur
	}
	if prog >= 1 {
		return prog
	}
	return 10
}

// editGrid lets the operator change the map layout before the scan; false = cancelled.
func editGrid(g Grid) (Grid, bool) {
	prog := g
	for {
		showGrid(g, prog)
		if g.X0 > prog.X0+1e-6 || g.X1 < prog.X1-1e-6 || g.Y0 > prog.Y0+1e-6 || g.Y1 < prog.Y1-1e-6 {
			sayWarn("сетка меньше реза программы, за её краем высота берётся с края карты")
		}
		show(box(thin, "Поменять сетку или Enter", twoCols(gridKeys, boxW()-4), boxW()))
		in := latin(prompt("сетка: "))
		f := strings.Fields(strings.ReplaceAll(in, ",", "."))
		if len(f) == 0 {
			return g, true
		}
		num := func(i int) (float64, bool) {
			if len(f) <= i {
				return 0, false
			}
			v, err := strconv.ParseFloat(f[i], 64)
			return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
		}
		a, okA := num(1)
		b, okB := num(2)
		ok := true
		switch f[0] {
		case "q":
			return g, false
		case "r":
			g = prog
		case "x", "y":
			if !okA || !okB || b <= a || a < -10 || b > 600 {
				ok = false
				break
			}
			if f[0] == "x" { // the step stays what it was, the count follows the new length
				sx := keepStep(g.StepX(), prog.StepX())
				g.X0, g.X1 = a, b
				g.NX = CountFor(a, b, sx)
			} else {
				sy := keepStep(g.StepY(), prog.StepY())
				g.Y0, g.Y1 = a, b
				g.NY = CountFor(a, b, sy)
			}
		case "m":
			if !okA || a < 0 || a > 20 {
				ok = false
				break
			}
			sx, sy := keepStep(g.StepX(), prog.StepX()), keepStep(g.StepY(), prog.StepY())
			g.X0, g.X1, g.Y0, g.Y1 = prog.X0-a, prog.X1+a, prog.Y0-a, prog.Y1+a
			g.NX, g.NY = CountFor(g.X0, g.X1, sx), CountFor(g.Y0, g.Y1, sy)
		case "sx", "sy", "s":
			if !okA || a < 1 || a > 200 {
				sayBad("шаг от 1 до 200 мм")
				continue
			}
			if f[0] != "sy" {
				g.NX = CountFor(g.X0, g.X1, a)
			}
			if f[0] != "sx" {
				g.NY = CountFor(g.Y0, g.Y1, a)
			}
		case "nx", "ny":
			if !okA || a < 1 || a > 100 || a != math.Trunc(a) {
				sayBad("точек от 1 до 100")
				continue
			}
			if f[0] == "nx" {
				g.NX = int(a)
			} else {
				g.NY = int(a)
			}
		default:
			ok = false
		}
		if !ok {
			sayBad(fmt.Sprintf("не понял \"%s\"", in))
		}
		if g.NX*g.NY > 400 {
			sayWarn("больше 400 точек - это больше часа, уменьшите")
		}
	}
}
