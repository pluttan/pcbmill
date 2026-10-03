package main

// Manual mode: the operator drives NcStudio through this program, one key one
// move, with the MCU contact on screen all the time. Every key is turned into
// the same short command the text version had (u, d, x+, m x5 ...), and one
// function checks and runs them all: the safety rules live in one place.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type manual struct {
	o                            opts
	nc                           *NcStudio
	m                            Touch // nil: the operator chose to work without the MCU
	port, title                  string
	step, stepX, stepY, stepFast float64
	cmd                          *field // the command line, open after : or Tab
	busy                         string // the command being typed into NcStudio now
	p                            *page  // the log
}

func newManual(o opts, nc *NcStudio, m Touch, port string) *manual {
	s := &manual{o: o, nc: nc, m: m, port: port, title: o.title, step: 0.1, stepX: 1, stepY: 1, stepFast: 1}
	s.p = &page{title: menuTitle("6")}
	s.p.screenFn = s.view
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
		logLine(paint(cHint, time.Now().Format("15:04:05")) + "  " + s.said(in))
		s.busy = in
		draw(s.view) // show what is being sent before the keys go out
		defer func() { s.busy = "" }()
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
		if !confirm("Отправить в NcStudio эту строку?", paint(cKey, line)) {
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
func (s *manual) command(k Key) string {
	switch k.Code {
	case KeyLeft:
		return "x-"
	case KeyRight:
		return "x+"
	case KeyUp:
		return "y+"
	case KeyDown:
		return "y-"
	case KeyPgUp:
		return "lift"
	case KeyPgDn:
		return "d"
	case KeyEsc, KeyCtrlC:
		return "q"
	case KeyRune:
		switch k.R { // the shifted keys first: latin() would map Russian capitals to brackets
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
		switch k.letter() {
		case 'q':
			return "q"
		case 't':
			return "t"
		}
	}
	return ""
}

// key handles one key; false when the operator leaves manual mode.
func (s *manual) key(k Key) bool {
	if s.cmd != nil {
		done, cancel := s.cmd.key(k)
		if cancel {
			s.cmd = nil
		}
		if done {
			line := s.cmd.String()
			s.cmd = nil
			if strings.TrimSpace(line) != "" {
				return s.run(line)
			}
		}
		return true
	}
	if k.Code == KeyTab {
		s.cmd = newField("")
		return true
	}
	c := s.command(k)
	switch c {
	case "":
		return true
	case "sxy-", "sxy+":
		dir := 1
		if c == "sxy-" {
			dir = -1
		}
		v := num(nextStep(xySteps, math.Min(s.stepX, s.stepY), dir))
		if dir > 0 {
			v = num(nextStep(xySteps, math.Max(s.stepX, s.stepY), dir))
		}
		s.run("sx " + v)
		return s.run("sy " + v)
	}
	return s.run(c)
}

// run does a command, then gives the focus back to the console and drops the
// keys pressed meanwhile: a held arrow must not queue up moves.
func (s *manual) run(c string) bool {
	more := s.do(c)
	backToConsole()
	term.Flush()
	return more
}

// contact is the MCU state as a badge with a word on what it means.
func (s *manual) contact() string {
	switch {
	case s.m == nil:
		return offBadge("МК НЕ ПОДКЛЮЧЁН") + paint(cHint, "  касание не видно")
	case s.m.Alive() != nil:
		return alarmBadge("МК НЕ ОТВЕЧАЕТ") + paint(cHint, "  двигать можно только вверх")
	case s.m.State():
		return touchBadge("♦ КОНТАКТ") + paint(cHint, "  фреза на меди: вниз и вбок нельзя")
	}
	return okBadge("○ разомкнуто")
}

func (s *manual) keys() [][2]string {
	return [][2]string{
		{"← → ↑ ↓", fmt.Sprintf("X/Y на шаг %s / %s", num(s.stepX), num(s.stepY))},
		{"PageUp", fmt.Sprintf("Z быстро вверх на %s", num(s.stepFast))},
		{"PageDown", fmt.Sprintf("Z вниз на %s медленно", num(s.step))},
		{"Пробел", fmt.Sprintf("Z вверх на %s", num(s.step))},
		{"T", fmt.Sprintf("вниз до меди, не глубже %s", num(s.o.search))},
		{"+ -", "шаг X/Y больше / меньше"},
		{"* /", "шаг Z больше / меньше"},
		{"Tab", "строка команды: m, g, t, sz"},
		{"Esc Q", "назад в меню"},
	}
}

func (s *manual) view(w, h int) []string {
	bw := colW(w)
	st := []string{s.contact()}
	for _, l := range flow([]string{
		paint(cHint, "Z от старта") + paint(cTitle, fmt.Sprintf("%+9.3f", s.nc.z)) + paint(cHint, " мм"),
		paint(cHint, "шаг Z ") + val(num(s.step), ""),
		paint(cHint, "быстрый ") + val(num(s.stepFast), ""),
		paint(cHint, "X/Y ") + val(num(s.stepX)+" / "+num(s.stepY), "мм"),
	}, inner(bw)+1) {
		st = append(st, l[1:]) // flow starts every line with a space
	}
	if s.busy != "" {
		st = append(st, paint(cKey, "► в NcStudio: "+s.busy)+paint(cHint, "  мышь и клавиатуру не трогать"))
	} else {
		st = append(st, paint(cAlarm, "▲ СТОП:")+paint(cHint, " щелчок мышью по другому окну во время движения"))
	}
	state := box(thin, "Станок", st, bw)
	keys := box(thin, "Клавиши, без Enter", twoCols(s.keys(), inner(bw)), bw)
	var cmd []string
	if s.cmd != nil {
		cmd = box(thin, "Команда", []string{paint(cKey, ": ") + s.cmd.view(inner(bw)-2)}, bw)
	}
	used := len(state) + len(keys) + len(cmd) + 3
	rows := room(w, h)
	n := rows - used - 4 // the journal frame and a free line above and under
	if n > 8 {
		n = 8
	}
	s.p.mu.Lock()
	var ls []string
	for _, l := range s.p.log {
		ls = append(ls, wrap(l, inner(bw), 2)...)
	}
	s.p.mu.Unlock()
	if n < 1 {
		n = 1
	}
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	for len(ls) < n {
		ls = append(ls, "")
	}
	journal := box(thin, "Журнал", ls, bw)
	body := stack(state, keys, journal, cmd)
	if len(body) > rows { // a low window: the key list goes first
		body = stack(state, journal, cmd)
	}
	hint := hints(w, "Esc", "в меню", "Tab", "команда", "PageUp", "вверх")
	if s.cmd != nil {
		hint = hints(w, "Enter", "выполнить", "Esc", "закрыть строку")
	}
	return screen(w, h, menuTitle("6"), body, hint)
}

// cmdManual: the operator drives NcStudio through this program step by step,
// to see that keystrokes land and the MCU reports contact before any scan.
func cmdManual(o opts) error {
	var m Touch
	var name string
	if confirm("Подключить МК (показывать касание)?", "Без МК нельзя опустить до меди (T), а касание не видно.") {
		newPage(menuTitle("6"), "Ищу МК на COM-портах...")
		mc, n, err := findTouch(o.port)
		if err != nil {
			return err
		}
		m, name = mc, n
	}
	s := newManual(o, newNc(o), m, name)
	cur = s.p
	s.p.add(paint(cHint, "Пока команда печатается в NcStudio, мышь и клавиатуру не трогать."))
	if name != "" {
		sayOK(strip(mcuAt(name)))
	}
	term.Flush()
	defer faultKeysOn(func() bool { return s.cmd == nil })() // digits go to an open command line
	run(s.view, s.keyDone, 200*time.Millisecond)
	return nil
}

func (s *manual) keyDone(k Key) bool { return !s.key(k) }

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
