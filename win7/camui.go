package main

// The numbers of the programs: the tool, depths, feeds, drilling, the
// V-groove, the side; kept next to the exe between runs.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ==========================
// ===  Settings          ===
// ==========================

// camField: one line of the form. choices make it a switch walked by the arrows.
type camField struct {
	key, label, unit string
	group            string // a heading over the field when it changes
	lo, hi, step     float64
	digits           int
	choices          []string
	get              func(*camCfg) float64
	put              func(*camCfg, float64)
}

var camFields = []camField{
	{key: "tip", group: "Фреза V (изоляция, маска)", label: "кончик", unit: "мм", lo: 0.02, hi: 1, step: 0.05, digits: 2,
		get: func(c *camCfg) float64 { return c.Tip }, put: func(c *camCfg, v float64) { c.Tip = v }},
	{key: "angle", label: "угол", unit: "°", lo: 10, hi: 90, step: 5,
		get: func(c *camCfg) float64 { return c.Angle }, put: func(c *camCfg, v float64) { c.Angle = v }},
	{key: "spindle", label: "обороты", unit: "об/мин", lo: 1000, hi: 30000, step: 1000,
		get: func(c *camCfg) float64 { return c.Spindle }, put: func(c *camCfg, v float64) { c.Spindle = v }},
	{key: "isodepth", group: "Изоляция дорожек", label: "глубина", unit: "мм", lo: 0.02, hi: 0.5, step: 0.01, digits: 2,
		get: func(c *camCfg) float64 { return c.IsoDepth }, put: func(c *camCfg, v float64) { c.IsoDepth = v }},
	{key: "passes", label: "обходов", lo: 1, hi: 8, step: 1,
		get: func(c *camCfg) float64 { return float64(c.Passes) }, put: func(c *camCfg, v float64) { c.Passes = int(v) }},
	{key: "isofeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.IsoFeed }, put: func(c *camCfg, v float64) { c.IsoFeed = v }},
	{key: "plunge", label: "врезание", unit: "мм/мин", lo: 10, hi: 1000, step: 10,
		get: func(c *camCfg) float64 { return c.Plunge }, put: func(c *camCfg, v float64) { c.Plunge = v }},
	{key: "isotest", label: "пробы станка", choices: []string{"нет", "да"},
		get: func(c *camCfg) float64 { return b2f(c.IsoTest) }, put: func(c *camCfg, v float64) { c.IsoTest = v != 0 }},
	{key: "maskdepth", group: "Вскрытие маски", label: "глубина", unit: "мм", lo: 0.005, hi: 0.3, step: 0.005, digits: 3,
		get: func(c *camCfg) float64 { return c.MaskDepth }, put: func(c *camCfg, v float64) { c.MaskDepth = v }},
	{key: "maskflat", label: "фреза", unit: "мм", lo: 0, hi: 3, step: 0.1, digits: 1,
		get: func(c *camCfg) float64 { return c.MaskFlat }, put: func(c *camCfg, v float64) { c.MaskFlat = v }},
	{key: "maskfeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.MaskFeed }, put: func(c *camCfg, v float64) { c.MaskFeed = v }},
	{key: "masktest", label: "проба глубины", choices: []string{"нет", "да"},
		get: func(c *camCfg) float64 { return b2f(c.MaskTest) }, put: func(c *camCfg, v float64) { c.MaskTest = v != 0 }},
	{key: "drill", group: "Сверловка", label: "как", choices: []string{"насквозь", "наколка", "оба"},
		get: func(c *camCfg) float64 { return float64(c.DrillMode) }, put: func(c *camCfg, v float64) { c.DrillMode = int(v) }},
	{key: "thick", label: "толщина платы", unit: "мм", lo: 0.2, hi: 4, step: 0.1, digits: 1,
		get: func(c *camCfg) float64 { return c.Thickness }, put: func(c *camCfg, v float64) { c.Thickness = v }},
	{key: "drillfeed", label: "врезание сверла", unit: "мм/мин", lo: 5, hi: 500, step: 5,
		get: func(c *camCfg) float64 { return c.DrillFeed }, put: func(c *camCfg, v float64) { c.DrillFeed = v }},
	{key: "holemill", label: "отверстия", choices: []string{"сверлами", "одной фрезой"},
		get: func(c *camCfg) float64 { return float64(c.HoleMill) }, put: func(c *camCfg, v float64) { c.HoleMill = int(v) }},
	{key: "holetool", label: "фреза отверстий", unit: "мм", lo: 0.3, hi: 3, step: 0.1, digits: 1,
		get: func(c *camCfg) float64 { return c.HoleTool }, put: func(c *camCfg, v float64) { c.HoleTool = v }},
	{key: "holefeed", label: "подача по кругу", unit: "мм/мин", lo: 10, hi: 1000, step: 10,
		get: func(c *camCfg) float64 { return c.HoleFeed }, put: func(c *camCfg, v float64) { c.HoleFeed = v }},
	{key: "holestep", label: "вниз за оборот", unit: "мм", lo: 0.05, hi: 1, step: 0.05, digits: 2,
		get: func(c *camCfg) float64 { return c.HoleStep }, put: func(c *camCfg, v float64) { c.HoleStep = v }},
	{key: "vdepth", group: "V-рез по контуру", label: "глубина", unit: "мм", lo: 0.1, hi: 4, step: 0.1, digits: 2,
		get: func(c *camCfg) float64 { return c.VDepth }, put: func(c *camCfg, v float64) { c.VDepth = v }},
	{key: "vstep", label: "за проход", unit: "мм", lo: 0.05, hi: 2, step: 0.05, digits: 2,
		get: func(c *camCfg) float64 { return c.VStep }, put: func(c *camCfg, v float64) { c.VStep = v }},
	{key: "vfeed", label: "подача", unit: "мм/мин", lo: 10, hi: 3000, step: 50,
		get: func(c *camCfg) float64 { return c.VFeed }, put: func(c *camCfg, v float64) { c.VFeed = v }},
	{key: "side", group: "Сторона", label: "медь сверху", choices: []string{"верх", "низ"},
		get: func(c *camCfg) float64 { return float64(c.Side) }, put: func(c *camCfg, v float64) { c.Side = int(v) }},
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

const camCfgFile = "pcbscan_cam.txt"

// loadCam: the numbers of the last run, the defaults for what is missing.
func loadCam(dir string) camCfg {
	c := defaultCam()
	b, err := os.ReadFile(filepath.Join(dir, camCfgFile))
	if err != nil {
		return c
	}
	for _, l := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		for _, f := range camFields {
			if f.key == strings.TrimSpace(k) && f.valid(x) == nil {
				f.put(&c, x)
			}
		}
	}
	return c
}

func saveCam(dir string, c camCfg) error {
	var b strings.Builder
	b.WriteString("# pcbscan: числа программ из Gerber, последний запуск\r\n")
	for _, f := range camFields {
		fmt.Fprintf(&b, "%s=%s\r\n", f.key, strconv.FormatFloat(f.get(&c), 'f', -1, 64))
	}
	return os.WriteFile(filepath.Join(dir, camCfgFile), []byte(b.String()), 0644)
}

func (f camField) valid(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("не число")
	}
	if f.choices != nil {
		if v < 0 || int(v) >= len(f.choices) || v != math.Trunc(v) {
			return fmt.Errorf("нет такого выбора")
		}
		return nil
	}
	if v < f.lo-1e-9 || v > f.hi+1e-9 {
		return fmt.Errorf("%s: от %s до %s %s", f.label, num(f.lo), num(f.hi), f.unit)
	}
	if f.digits == 0 && v != math.Trunc(v) {
		return fmt.Errorf("%s: целое число", f.label)
	}
	return nil
}

func (f camField) show(c *camCfg) string {
	v := f.get(c)
	if f.choices != nil {
		return f.choices[int(v)]
	}
	if f.key == "maskflat" && v == 0 {
		return "та же V"
	}
	return strconv.FormatFloat(v, 'f', f.digits, 64)
}

// ==========================
// ===  The form          ===
// ==========================
