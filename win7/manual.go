package main

// Manual mode: the operator drives NcStudio through this program, one key one
// move, with the MCU contact on screen all the time. Every key is turned into
// the same short command the text version had (u, d, x+, m x5 ...), and one
// function checks and runs them all: the safety rules live in one place.

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"time"

	"pcbscan/gfx"
	"pcbscan/ui"
)

type manual struct {
	o                            opts
	nc                           *NcStudio
	m                            Touch // nil: the operator chose to work without the MCU
	port, title                  string
	step, stepX, stepY, stepFast float64
	cmd                          *ui.Field // the command line, open after Tab
	in                           chan string
	busy                         string // the command being typed into NcStudio now
	p                            *page  // the log
}

func newManual(o opts, nc *NcStudio, m Touch, port string) *manual {
	s := &manual{o: o, nc: nc, m: m, port: port, title: o.title, step: 0.1, stepX: 1, stepY: 1, stepFast: 1}
	s.p = &page{title: menuTitle("7"), progress: -1, acts: make(chan string, 8)}
	return s
}

func (s *manual) touching() bool { return s.m != nil && s.m.State() }

// do runs one command; false when it was q. The checks are those of the text
// manual mode, unchanged: a live MCU before any move down or sideways, no
// move down and no move sideways while the tip touches the copper; going up is
// allowed always.
func (s *manual) do(raw string) bool {
	in := latin(strings.TrimSpace(raw))
	o, nc, m := s.o, s.nc, s.m
	moving := in == "u" || in == "d" || in == "uu" || in == "dd" || strings.HasPrefix(in, "g ") ||
		in == "x+" || in == "x-" || in == "y+" || in == "y-" || in == "t" || strings.HasPrefix(in, "t ") ||
		strings.HasPrefix(in, "m ")
	if moving && m != nil {
		if e := m.Alive(); e != nil {
			sayBad(fmt.Sprintf("не двигаю: %v", e))
			return true
		}
	}
	if moving || in == "lift" {
		// the command first, its result (copper found, a stop) under it
		logLine(time.Now().Format("15:04:05") + "  " + s.said(in))
		app.Do(func() { s.busy = in })
		defer app.Do(func() { s.busy = "" })
	}
	var err error
	switch {
	case in == "q":
		return false
	case in == "lift":
		// going up is the way out of trouble: done even when the MCU is silent
		sayHint(fmt.Sprintf("быстрый подъём на %.1f мм", s.stepFast))
		if err = nc.Grab(); err == nil {
			err = nc.RapidZ(nc.z + s.stepFast)
		}
	case in == "u":
		if err = nc.Grab(); err == nil {
			err = nc.RapidZ(nc.z + s.step)
		}
	case in == "d" || in == "dd":
		if s.touching() {
			sayBad("фреза касается меди: вниз нельзя, сначала вверх (пробел или PageUp)")
			return true
		}
		if err = nc.Grab(); err == nil {
			if in == "d" {
				err = nc.MoveZ(nc.z-s.step, 20)
			} else {
				err = nc.MoveZ(nc.z-s.stepFast, 600)
			}
		}
	case in == "uu":
		if err = nc.Grab(); err == nil {
			err = nc.RapidZ(nc.z + s.stepFast)
		}
	case strings.HasPrefix(in, "m "):
		var dx, dy, dz float64
		if dx, dy, dz, err = parseShift(in[2:]); err != nil {
			sayBad(err.Error())
			return true
		}
		if s.touching() && dz <= 0 { // up comes first in shift, so lifting off is allowed
			sayBad("фреза касается меди: сначала поднимите Z (m z0.5)")
			return true
		}
		if err = nc.Grab(); err == nil {
			err = shift(nc, dx, dy, dz)
		}
	case in == "t" || strings.HasPrefix(in, "t "):
		depth := o.search
		if in != "t" {
			if _, e := fmt.Sscan(in[2:], &depth); e != nil || depth <= 0 || depth > 20 {
				sayBad("глубина поиска от 0.1 до 20 мм")
				return true
			}
		}
		if m == nil {
			sayBad("без МК касание не увидеть: выйдите в меню и подключите МК")
			return true
		}
		if s.touching() {
			sayBad("фреза уже касается меди")
			return true
		}
		if err = nc.Grab(); err == nil {
			err = lowerToCopper(o, nc, m, depth)
		}
	case strings.HasPrefix(in, "sz "):
		var v float64
		if _, e := fmt.Sscan(in[3:], &v); e == nil && v > 0 && v <= 50 {
			s.stepFast = v
		} else {
			sayBad("шаг быстрого Z от 0.1 до 50 мм")
		}
		return true
	case in == "x+" || in == "x-" || in == "y+" || in == "y-":
		if s.touching() {
			sayBad("фреза касается меди: сначала вверх (пробел или PageUp), потом по X/Y")
			return true
		}
		d := s.stepX
		if in[0] == 'y' {
			d = s.stepY
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
				s.stepX = v
			} else {
				s.stepY = v
			}
		} else {
			sayBad("шаг по X/Y от 0.01 до 300 мм")
		}
		return true
	case strings.HasPrefix(in, "s "):
		var v float64
		if _, e := fmt.Sscan(in[2:], &v); e == nil && v > 0 && v <= 5 {
			s.step = v
		} else {
			sayBad("шаг от 0.001 до 5 мм")
		}
		return true
	case strings.HasPrefix(in, "g "):
		line := strings.ToUpper(strings.TrimSpace(strings.TrimSpace(raw)[2:]))
		if !confirm("Отправить в NcStudio эту строку?", line) {
			return true
		}
		if err = nc.Grab(); err == nil {
			err = nc.cmd(line, 1.0) // our Z count no longer matches after a free-form line
		}
	case in == "":
	default:
		sayBad(fmt.Sprintf("не понял \"%s\"", raw))
		return true
	}
	if err != nil {
		sayStop(err)
	}
	if m != nil {
		if e := m.Alive(); e != nil {
			sayBad(e.Error())
		}
	}
	return true
}

// said is a done command in words for the log.
func (s *manual) said(in string) string {
	switch in {
	case "lift", "uu":
		return fmt.Sprintf("Z вверх на %.1f, Z %+.3f", s.stepFast, s.nc.z)
	case "u":
		return fmt.Sprintf("Z вверх на %.3f, Z %+.3f", s.step, s.nc.z)
	case "d":
		return fmt.Sprintf("Z вниз на %.3f, Z %+.3f", s.step, s.nc.z)
	case "dd":
		return fmt.Sprintf("Z вниз на %.1f, Z %+.3f", s.stepFast, s.nc.z)
	case "x+", "x-":
		return fmt.Sprintf("X %c%.2f", in[1], s.stepX)
	case "y+", "y-":
		return fmt.Sprintf("Y %c%.2f", in[1], s.stepY)
	}
	return in
}

// Steps the bracket keys walk through.
var (
	zSteps    = []float64{0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1}
	xySteps   = []float64{0.1, 0.5, 1, 2, 5, 10, 20, 50}
	fastSteps = []float64{0.5, 1, 2, 5, 10, 20}
)

// nextStep is the step after cur in list, up or down; the ends stay.
func nextStep(list []float64, cur float64, dir int) float64 {
	if dir > 0 {
		for _, v := range list {
			if v > cur+1e-9 {
				return v
			}
		}
		return list[len(list)-1]
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] < cur-1e-9 {
			return list[i]
		}
	}
	return list[0]
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// command is what a key means in manual mode: a command for do, "" for none.
// Keys that only change a step are commands too (s, sx, sz), so the limits are checked once.
// command: the key as a command of the manual mode, "" for none.
func (s *manual) command(e ui.Event) string {
	if e.Kind == ui.EvKey {
		switch e.Key {
		case ui.KeyLeft:
			return "x-"
		case ui.KeyRight:
			return "x+"
		case ui.KeyUp:
			return "y+"
		case ui.KeyDown:
			return "y-"
		case ui.KeyPgUp:
			return "lift"
		case ui.KeyPgDn:
			return "d"
		case ui.KeyEsc:
			return "q"
		}
		return ""
	}
	if e.Kind != ui.EvChar {
		return ""
	}
	switch e.Rune { // the shifted keys first: latin() would map Russian capitals to brackets
	case ' ':
		return "u"
	case '+', '=':
		return "sxy+"
	case '-', '_':
		return "sxy-"
	case '*':
		return "s " + num(nextStep(zSteps, s.step, 1))
	case '/':
		return "s " + num(nextStep(zSteps, s.step, -1))
	}
	switch latin(string(e.Rune)) {
	case "q":
		return "q"
	case "t":
		return "t"
	}
	return ""
}

// send gives a command to the work goroutine; while it is busy with one the
// next is dropped, so a held arrow does not pile up moves.
func (s *manual) send(c string) {
	if c == "" {
		return
	}
	select {
	case s.in <- c:
	default:
	}
}

// stepXY: the X/Y step one notch up or down.
func (s *manual) stepXY(dir int) {
	v := num(nextStep(xySteps, math.Min(s.stepX, s.stepY), dir))
	if dir > 0 {
		v = num(nextStep(xySteps, math.Max(s.stepX, s.stepY), dir))
	}
	s.do("sx " + v)
	s.do("sy " + v)
}

// onKey runs on the window goroutine: keys become commands.
func (s *manual) onKey(e ui.Event) bool {
	if s.cmd != nil {
		return false // the command line takes the keys
	}
	if e.Kind == ui.EvKey && e.Key == ui.KeyTab {
		s.cmd = ui.NewField("")
		return true
	}
	c := s.command(e)
	s.send(c)
	return c != ""
}

// panel: the state of the machine, the jog pad and the command line.
func (s *manual) panel(c *ui.Ctx, r image.Rectangle) int {
	t := c.T
	y := r.Min.Y
	// the contact, large
	label, col := "разомкнуто", t.Green
	switch {
	case s.m == nil:
		label, col = "МК не подключён: касание не видно", t.Overlay1
	case s.m.Alive() != nil:
		label, col = "МК НЕ ОТВЕЧАЕТ: двигать можно только вверх", t.Red
	case s.m.State():
		label, col = "КОНТАКТ: фреза на меди, вниз и вбок нельзя", t.Yellow
	}
	hb := t.Si(40)
	c.C.FillRoundRect(float64(r.Min.X), float64(y), float64(r.Dx()), float64(hb), t.S(10), gfx.Alpha(col, 0.15))
	c.C.FillCircle(float64(r.Min.X)+t.S(20), float64(y)+float64(hb)/2, t.S(6), col)
	c.LabelIn(image.Rect(r.Min.X+t.Si(36), y, r.Max.X, y+hb), label, ui.Style{Kind: ui.Bold, Size: ui.SizeBody, Color: col}, 0)
	y += hb + t.Si(12)
	// numbers
	mono := ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}
	c.Label(r.Min.X, y, fmt.Sprintf("Z от старта %+9.3f мм", s.nc.z), mono)
	y += c.LineH(mono) + t.Si(2)
	c.Label(r.Min.X, y, fmt.Sprintf("шаг Z %s   быстрый %s   X/Y %s / %s мм", num(s.step), num(s.stepFast), num(s.stepX), num(s.stepY)), c.Muted())
	y += c.LineH(mono) + t.Si(10)
	if s.busy != "" {
		c.Label(r.Min.X, y, "в NcStudio: "+s.busy+"   мышь и клавиатуру не трогать", ui.Style{Kind: ui.Regular, Size: ui.SizeBody, Color: t.Blue})
	} else {
		c.Label(r.Min.X, y, "СТОП во время движения: щелчок по другому окну", ui.Style{Kind: ui.Regular, Size: ui.SizeBody, Color: t.Overlay2})
	}
	y += c.LineH(mono) + t.Si(14)
	// the jog pad: arrows for X/Y, a column for Z, the steps
	b := t.Si(44)
	g := t.Si(6)
	x0 := r.Min.X
	pad := []struct {
		dx, dy int
		ic     ui.Icon
		cmd    string
		lab    string
	}{{1, 0, ui.IconNone, "y+", "Y+"}, {0, 1, ui.IconNone, "x-", "X-"}, {2, 1, ui.IconNone, "x+", "X+"}, {1, 2, ui.IconNone, "y-", "Y-"}}
	for _, k := range pad {
		br := image.Rect(x0+k.dx*(b+g), y+k.dy*(b+g), x0+k.dx*(b+g)+b, y+k.dy*(b+g)+b)
		if c.Button("man/"+k.cmd, br, k.lab, ui.BtnNormal) {
			s.send(k.cmd)
		}
	}
	zx := x0 + 3*(b+g) + t.Si(14)
	zcol := []struct{ cmd, lab string }{{"lift", "Z быстро вверх"}, {"u", "Z вверх"}, {"d", "Z вниз"}}
	for i, k := range zcol {
		br := image.Rect(zx, y+i*(b+g), zx+t.Si(150), y+i*(b+g)+b)
		if c.Button("man/"+k.cmd, br, k.lab, ui.BtnNormal) {
			s.send(k.cmd)
		}
	}
	tx := zx + t.Si(150) + g
	if r.Max.X-tx > t.Si(110) {
		if c.Button("man/t", image.Rect(tx, y, tx+t.Si(130), y+b), "До меди (T)", ui.BtnPrimary) {
			s.send("t")
		}
		if c.Button("man/cmd", image.Rect(tx, y+b+g, tx+t.Si(130), y+2*b+g), "Команда (Tab)", ui.BtnNormal) {
			s.cmd = ui.NewField("")
		}
	}
	y += 3*(b+g) + t.Si(8)
	// the steps
	steps := []struct{ lab, cmd string }{{"шаг X/Y -", "sxy-"}, {"шаг X/Y +", "sxy+"},
		{"шаг Z -", "s " + num(nextStep(zSteps, s.step, -1))}, {"шаг Z +", "s " + num(nextStep(zSteps, s.step, 1))}}
	cells := ui.SplitX(image.Rect(r.Min.X, y, r.Max.X, y+t.Si(34)), 4, g)
	for i, k := range steps {
		if c.Button("man/st"+k.lab, cells[i], k.lab, ui.BtnGhost) {
			s.send(k.cmd)
		}
	}
	y += t.Si(34) + t.Si(12)
	// the command line
	if s.cmd != nil {
		fr := image.Rect(r.Min.X, y, r.Max.X, y+t.Si(38))
		c.SetFocus("man/line")
		enter, cancel := c.TextField("man/line", fr, s.cmd, ui.Style{Kind: ui.Mono, Size: ui.SizeBody, Color: t.Text}, 0)
		if enter {
			line := strings.TrimSpace(s.cmd.String())
			s.cmd = nil
			s.send(line)
		}
		if cancel {
			s.cmd = nil
		}
		y += t.Si(42)
		y += c.Para(image.Rect(r.Min.X, y, r.Max.X, y+t.Si(200)), "m x5 y-2.5 z1   g G91 G00 X5   t 5   s 0.05   sz 2   sx 5   sy 2", c.Small())
	} else {
		y += c.Para(image.Rect(r.Min.X, y, r.Max.X, y+t.Si(200)), "Клавиши: стрелки X/Y, PageUp/PageDown Z, пробел Z вверх, T до меди, + - * / шаги, Esc выход", c.Small())
	}
	return y - r.Min.Y
}

func cmdManual(o opts) error {
	var m Touch
	var name string
	if confirm("Подключить МК (показывать касание)?", "Без МК нельзя опустить до меди (T), а касание не видно.") {
		newPage(menuTitle("7"), "Ищу МК на COM-портах...")
		mc, n, err := findTouch(o.port)
		if err != nil {
			return err
		}
		m, name = mc, n
	}
	s := newManual(o, newNc(o), m, name)
	s.in = make(chan string, 1)
	cur = s.p
	crumb = s.p.title
	s.p.progress = -1
	s.p.head = "Ручной режим"
	s.p.scene = jobToolScene()
	s.p.extra = s.panel
	s.p.onKey = s.onKey
	s.p.btns = []pageBtn{{id: "q", label: "Выход", kind: ui.BtnNormal}}
	app.Show(s.p)
	logLine("Пока команда печатается в NcStudio, мышь и клавиатуру не трогать.")
	if name != "" {
		sayOK(mcuAt(name))
	}
	backToApp()
	defer faultKeysOn(func() bool { return s.cmd == nil })()
	for {
		select {
		case c := <-s.in:
			if !s.run(c) {
				return nil
			}
		case id := <-s.p.acts:
			if id == "q" {
				return nil
			}
		}
	}
}

// run does one command with the machine; false for q.
func (s *manual) run(c string) bool {
	switch c {
	case "sxy+":
		s.stepXY(1)
		return true
	case "sxy-":
		s.stepXY(-1)
		return true
	}
	machineBusy(true)
	more := s.do(c)
	machineBusy(false)
	backToApp()
	app.Wake()
	return more
}

// parseShift reads "x5 y-2.5 z1" (any of the three, in any order) as relative mm.
func parseShift(str string) (dx, dy, dz float64, err error) {
	f := strings.Fields(strings.ReplaceAll(str, ",", "."))
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

func mcuStateOf(m Touch) mcuState {
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
