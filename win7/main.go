package main

// pcbscan.exe — height map for PCB milling on a Weihong NcStudio machine with
// no probe input: an MCU reports bit-to-copper contact, this program types
// short moves into NcStudio's MDI window. One file, runs on bare Windows 7.
//
// pluttan

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ==========================
// ===  MCU touch sensor  ===
// ==========================

type MCU struct {
	mu       sync.Mutex
	state    bool
	known    bool
	hits     int
	lastSeen time.Time
	dead     error
	port     io.ReadWriteCloser
}

func NewMCU(p io.ReadWriteCloser) *MCU {
	m := &MCU{port: p, lastSeen: time.Now()}
	go m.run()
	return m
}

// run asks and reads in ONE goroutine. The Windows 7 usbser driver can block a read
// until data arrives, and a write from another goroutine then waits behind it forever:
// the first answer came, then "no answer" for good. Ask, then read the reply, in turn.
func (m *MCU) run() {
	buf := make([]byte, 64)
	var line []byte
	for {
		if _, err := m.port.Write([]byte("?")); err != nil {
			m.setDead(err)
			return
		}
		got := false
		end := time.Now().Add(150 * time.Millisecond)
		for !got && time.Now().Before(end) {
			n, err := m.port.Read(buf)
			if err != nil {
				m.setDead(err)
				return
			}
			for _, c := range buf[:n] {
				if c == '\r' {
					continue
				}
				if c != '\n' {
					line = append(line, c)
					continue
				}
				m.handle(string(line))
				line = line[:0]
				got = true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (m *MCU) setDead(err error) {
	m.mu.Lock()
	m.dead = fmt.Errorf("МК отключён: %v", err)
	m.mu.Unlock()
}

func (m *MCU) handle(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch s {
	case "T1", "S1":
		if !m.state {
			m.hits++
		}
		m.state, m.known = true, true
	case "T0", "S0":
		m.state, m.known = false, true
	default:
		return
	}
	m.lastSeen = time.Now()
}

// Alive fails when the MCU has not answered for half a second.
func (m *MCU) Alive() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dead != nil {
		return m.dead
	}
	if d := time.Since(m.lastSeen); d > 500*time.Millisecond {
		return fmt.Errorf("МК не отвечает %.1f с: выдернут USB или завис", d.Seconds())
	}
	return nil
}

func (m *MCU) State() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *MCU) Hits() int   { m.mu.Lock(); defer m.mu.Unlock(); return m.hits }
func (m *MCU) Known() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.known }

var (
	mcuCache *MCU
	mcuName  string
)

func (m *MCU) Close() { m.port.Close() }

// findMCU keeps one connection for the whole session: Windows will not open a COM
// port twice, so reopening it for every menu item failed every other time.
func findMCU(want string) (*MCU, string, error) {
	if mcuCache != nil {
		if mcuCache.Alive() == nil {
			return mcuCache, mcuName, nil
		}
		mcuCache.Close()
		mcuCache = nil
		time.Sleep(500 * time.Millisecond)
	}
	ports := ComPorts()
	if want != "" {
		ports = []string{want}
	} else if mcuName != "" { // last good port first
		ports = append([]string{mcuName}, ports...)
	}
	if len(ports) == 0 {
		return nil, "", fmt.Errorf("COM-портов нет: МК не подключён или нет драйвера")
	}
	var tried []string
	seen := map[string]bool{}
	for _, name := range ports {
		if seen[name] {
			continue
		}
		seen[name] = true
		var p *Port
		var err error
		for i := 0; i < 5; i++ { // a port just closed can stay busy for a moment
			if p, err = OpenPort(name); err == nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if err != nil {
			tried = append(tried, name+": не открывается ("+err.Error()+")")
			continue
		}
		m := NewMCU(p)
		for i := 0; i < 25 && !m.Known(); i++ { // up to 2.5 s: Nano reboots on open
			time.Sleep(100 * time.Millisecond)
		}
		if m.Known() {
			mcuCache, mcuName = m, name
			return m, name, nil
		}
		m.Close()
		tried = append(tried, name+": молчит")
	}
	return nil, "", fmt.Errorf("МК не найден: %s", strings.Join(tried, "; "))
}

// ==========================
// ===  Operator dialogue ===
// ==========================

var lines = make(chan string)

func init() {
	go func() {
		for {
			lines <- readLine()
		}
	}()
}

// ask takes focus back from NcStudio first: the operator answers without clicking.
func ask(q string) string {
	backToConsole()
	say("%s", q)
	return strings.TrimSpace(<-lines)
}

// latin maps a command typed on the Russian layout to the same keys in Latin.
func latin(s string) string {
	ru := "йцукенгшщзхъфывапролджэячсмитьбю"
	en := "qwertyuiop[]asdfghjkl;'zxcvbnm,."
	er := []rune(en)
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if i := strings.IndexRune(ru, r); i >= 0 {
			b.WriteRune(er[len([]rune(ru[:i]))])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func yes(q string) bool {
	a := strings.ToLower(ask(q + " [y/n]: "))
	return a == "y" || a == "д" || a == "l" // l: 'д' typed on the latin layout
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func pickTap(dir string) (string, error) {
	var taps []string
	for _, pat := range []string{"*.tap", "*.nc", "*.TAP", "*.NC"} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, f := range m {
			if !strings.Contains(strings.ToLower(f), "_level.") {
				taps = append(taps, f)
			}
		}
	}
	sort.Strings(taps)
	taps = uniq(taps)
	if len(taps) == 0 {
		return "", fmt.Errorf("рядом с pcbscan.exe нет файлов .tap: положите программу платы в эту папку")
	}
	for i, t := range taps {
		say("  %d  %s\n", i+1, filepath.Base(t))
	}
	var k int
	fmt.Sscan(ask("номер программы: "), &k)
	if k < 1 || k > len(taps) {
		return "", fmt.Errorf("нет такого номера")
	}
	return taps[k-1], nil
}

func uniq(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || !strings.EqualFold(v, s[i-1]) {
			out = append(out, v)
		}
	}
	return out
}

// ==========================
// ===  Commands          ===
// ==========================

type opts struct {
	port, mdiKey, title string
	grid, clear, search float64
	below               float64
	keep                bool
	step, fine          float64
	rapid               float64
	settle              int
}

func newNc(o opts) *NcStudio {
	n := NewNcStudio(&WinKeys{Title: o.title, MDIKey: o.mdiKey, Delay: 5 * time.Millisecond})
	n.Rapid = o.rapid
	n.Settle = time.Duration(o.settle) * time.Millisecond
	n.Reopen = !o.keep
	return n
}

func newScanner(o opts, m Machine, t Touch) *Scanner {
	s := NewScanner(m, t)
	s.Clear, s.Search, s.StepCoarse, s.StepFine = o.clear, o.search, o.step, o.fine
	s.Below = o.below
	s.Log = func(l string) { say("%s\n", l) }
	return s
}

func cmdWire(o opts) error {
	m, name, err := findMCU(o.port)
	if err != nil {
		return err
	}
	say("МК найден на %s. Касайтесь зажимом фрезы; выход в меню: Enter.\n", name)
	last := !m.State()
	for {
		select {
		case <-lines:
			return nil
		default:
		}
		if e := m.Alive(); e != nil {
			return e
		}
		if s := m.State(); s != last {
			last = s
			if s {
				say("%s  КОНТАКТ\n", time.Now().Format("15:04:05"))
			} else {
				say("%s  разомкнуто\n", time.Now().Format("15:04:05"))
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func cmdNcTest(o opts) error {
	say("Фреза не ниже 5 мм над заготовкой. Программа нажмёт в NcStudio %s и введёт\n", o.mdiKey)
	say("  G91 G00 Z1   (вверх на 1 мм), потом G91 G00 Z-1 (обратно).\n")
	say("Пока она печатает, мышь и клавиатуру не трогать.\n")
	if !yes("начать?") {
		return nil
	}
	n := newNc(o)
	if err := n.Grab(); err != nil {
		return err
	}
	if err := n.RapidZ(1); err != nil {
		return err
	}
	up := yes("Z поднялся на 1 мм?")
	if err := n.Grab(); err != nil {
		return err
	}
	if err := n.RapidZ(0); err != nil {
		return err
	}
	down := yes("Z вернулся вниз?")
	if up && down {
		say("Связь с NcStudio работает.\n")
	} else {
		say("Не сработало. Узнайте, какими клавишами в вашей NcStudio открывается окно ввода\n")
		say("одной строки G-кода (MDI), и запустите: pcbscan.exe -mdi ctrl+shift+f9 (свои клавиши).\n")
	}
	return nil
}

func gridFor(tap string, step float64) ([][2]float64, error) {
	lines, err := readLines(tap)
	if err != nil {
		return nil, err
	}
	x0, x1, y0, y1, err := CutBounds(lines)
	if err != nil {
		return nil, err
	}
	return GridPoints(x0, x1, y0, y1, step), nil
}

func cmdScan(o opts) error {
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	pts, err := gridFor(tap, o.grid)
	if err != nil {
		return err
	}
	m, name, err := findMCU(o.port)
	if err != nil {
		return err
	}
	say("МК на %s. Точек %d, примерно %.0f мин.\n", name, len(pts), float64(len(pts))*12/60)
	say("Перед стартом: в NcStudio ноль X0 Y0 стоит как для этой платы, шпиндель выключен,\n")
	say("фреза (любая крепкая) на 1-2 мм над медью рядом с X%.1f Y%.1f.\n", pts[0][0], pts[0][1])
	say("Не выше 2 мм: в первой точке медь ищется на %.0f мм вниз от старта.\n", o.search)
	say("Сейчас ОБНУЛИТЕ Z в NcStudio на этой высоте: обход ведёт Z абсолютными командами от этого нуля.\n")
	if !yes("Z обнулён, начать?") {
		return nil
	}
	out := filepath.Join(exeDir(), "scan.csv")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# x,y,z  pcbscan, Z от высоты старта\n")
	nc := newNc(o)
	nc.Abs = true
	if err := nc.Grab(); err != nil {
		return err
	}
	sc := newScanner(o, nc, m)
	rows, err := sc.Grid(pts, func(p Point) {
		fmt.Fprintf(f, "%.3f,%.3f,%.4f\n", p.X, p.Y, p.Z)
		f.Sync()
	})
	if err != nil {
		say("\nОСТАНОВ: %v\n", err)
		if yes("поднять фрезу на 1 мм (NcStudio снова получит фокус)?") && nc.Grab() == nil {
			nc.RapidZ(nc.z + o.clear)
		}
		return fmt.Errorf("карта не досняна, scan.csv содержит измеренные точки")
	}
	say("Повторное касание первой точки: насколько всё уехало за время обхода...\n")
	drift, err := sc.Recheck(rows)
	if err != nil {
		return fmt.Errorf("повторное касание не удалось, карте верить нельзя: %v", err)
	}
	fmt.Fprintf(f, "# повторное касание первой точки: сдвиг %+.4f мм\n", drift)
	hm, _ := MapFromPoints(rows)
	say("%s", hm.Table())
	say("Сдвиг первой точки за обход: %+.3f мм\n", drift)
	if math.Abs(drift) > 0.03 {
		say("ВНИМАНИЕ: больше 0.03 мм. Ось Z уходит во время работы или станок пропускает команды:\n")
		say("по этой карте резать нельзя. Проверьте механику Z и повторите.\n")
	} else {
		say("Сдвиг в норме, карте можно верить.\n")
	}
	say("Карта записана: %s\n", out)
	return nil
}

func cmdZero(o opts) error {
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	pts, err := gridFor(tap, o.grid)
	if err != nil {
		return err
	}
	m, _, err := findMCU(o.port)
	if err != nil {
		return err
	}
	say("Фреза 0.1 стоит на 1-2 мм над медью. Будет одно медленное касание в X%.2f Y%.2f\n", pts[0][0], pts[0][1])
	say("(первая точка карты), потом нужно обнулить Z в NcStudio.\n")
	if !yes("начать?") {
		return nil
	}
	nc := newNc(o)
	if err := nc.Grab(); err != nil {
		return err
	}
	s := newScanner(o, nc, m)
	s.StepCoarse = 0.02 // the thin tip goes down in small steps
	z, err := s.TouchAt(pts[0][0], pts[0][1], 0)
	if err != nil {
		return err
	}
	if err := nc.MoveZ(z, 20); err != nil {
		return err
	}
	ask("Фреза касается меди. Обнулите Z в NcStudio (рабочая система координат), затем Enter здесь... ")
	nc.ResetZ()
	if err := nc.Grab(); err != nil {
		return err
	}
	if err := nc.RapidZ(o.clear * 3); err != nil {
		return err
	}
	say("Z0 стоит на меди в первой точке карты. Теперь пункт 5 и запуск *_level.tap.\n")
	return nil
}

func cmdLevel(o opts) error {
	pts, err := LoadPoints(filepath.Join(exeDir(), "scan.csv"))
	if err != nil {
		return fmt.Errorf("нет scan.csv: сначала снимите карту (%v)", err)
	}
	hm, err := MapFromPoints(pts)
	if err != nil {
		return err
	}
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	lines, err := readLines(tap)
	if err != nil {
		return err
	}
	out, err := ApplyMap(lines, hm, 1.0)
	if err != nil {
		return err
	}
	dst := strings.TrimSuffix(tap, filepath.Ext(tap)) + "_level" + filepath.Ext(tap)
	if err := os.WriteFile(dst, []byte(strings.Join(out, "\r\n")+"\r\n"), 0644); err != nil {
		return err
	}
	say("%s", hm.Table())
	say("Готово: %s\n", filepath.Base(dst))
	return nil
}

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
	var m *MCU
	if yes("подключить МК (показывать касание)?") {
		var name string
		var err error
		if m, name, err = findMCU(o.port); err != nil {
			return err
		}
		say("МК на %s\n", name)
	}
	nc := newNc(o)
	step, stepX, stepY, stepFast := 0.1, 1.0, 1.0, 1.0
	say("Команды (Enter после каждой):\n")
	say("  u      Z вверх на шаг        d   Z вниз на шаг (медленно)\n")
	say("  uu dd  Z быстро на шаг sz    t   опустить до меди (медленно, с касанием)\n")
	say("  x+ x-  X на шаг X            y+ y-  Y на шаг Y\n")
	say("  s 0.05 шаг по Z, мм          sz 2   шаг быстрого Z, мм\n")
	say("  sx 5   шаг по X, мм          sy 5   шаг по Y, мм\n")
	say("  t 5    искать медь не глубже 5 мм (без числа: %.1f мм)\n", o.search)
	say("  ?      есть ли касание\n")
	say("  m x5 y-2.5 z1  сдвинуть на любую величину (любые из X Y Z)\n")
	say("  g ...  любая строка G-кода как есть, например: g G91 G00 X5\n")
	say("  q      назад в меню\n")
	say("  Стрелки (без Enter)  X/Y на шаг sx/sy;  PageUp  быстро поднять Z на шаг sz\n")
	say("Пока команда печатается в NcStudio, мышь и клавиатуру не трогать.\n")
	say("Щелчок мышью по любому другому окну во время движения = стоп.\n")
	touching := func() bool { return m != nil && m.State() }
	for {
		raw := ask(fmt.Sprintf("[Z %.3f, быстрый Z %.1f, X %.2f, Y %.2f; Z %+.3f от старта] > ",
			step, stepFast, stepX, stepY, nc.z))
		in := latin(raw)
		if strings.HasPrefix(raw, "\x01") {
			in = raw[1:] // PageUp or an arrow
		}
		moving := in == "u" || in == "d" || in == "uu" || in == "dd" || strings.HasPrefix(in, "g ") ||
			in == "x+" || in == "x-" || in == "y+" || in == "y-" || in == "t" || strings.HasPrefix(in, "t ") ||
			strings.HasPrefix(in, "m ")
		if moving && m != nil {
			if e := m.Alive(); e != nil {
				say("не двигаю: %v\n", e)
				continue
			}
		}
		var err error
		switch {
		case in == "q":
			return nil
		case in == "lift":
			// going up is the way out of trouble: done even when the MCU is silent
			say("быстрый подъём на %.1f мм\n", stepFast)
			if err = nc.Grab(); err == nil {
				err = nc.RapidZ(nc.z + stepFast)
			}
		case in == "u":
			if err = nc.Grab(); err == nil {
				err = nc.RapidZ(nc.z + step)
			}
		case in == "d" || in == "dd":
			if touching() {
				say("фреза касается меди: вниз нельзя, сначала u\n")
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
				say("%v\n", err)
				continue
			}
			if touching() && dz <= 0 { // up comes first in shift, so lifting off is allowed
				say("фреза касается меди: сначала поднимите Z (m z0.5)\n")
				continue
			}
			if err = nc.Grab(); err == nil {
				err = shift(nc, dx, dy, dz)
			}
		case in == "t" || strings.HasPrefix(in, "t "):
			depth := o.search
			if in != "t" {
				if _, e := fmt.Sscan(in[2:], &depth); e != nil || depth <= 0 || depth > 20 {
					say("глубина поиска от 0.1 до 20 мм\n")
					continue
				}
			}
			if m == nil {
				say("без МК касание не увидеть: выйдите в меню и подключите МК\n")
				continue
			}
			if touching() {
				say("фреза уже касается меди\n")
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
				say("шаг быстрого Z от 0.1 до 50 мм\n")
			}
			continue
		case in == "x+" || in == "x-" || in == "y+" || in == "y-":
			if touching() {
				say("фреза касается меди: сначала u, потом двигать по X/Y\n")
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
				say("шаг по X/Y от 0.01 до 300 мм\n")
			}
			continue
		case strings.HasPrefix(in, "s "):
			var v float64
			if _, e := fmt.Sscan(in[2:], &v); e == nil && v > 0 && v <= 5 {
				step = v
			} else {
				say("шаг от 0.001 до 5 мм\n")
			}
			continue
		case strings.HasPrefix(in, "g "):
			line := strings.ToUpper(strings.TrimSpace(raw[2:]))
			if !yes("отправить в NcStudio «" + line + "»?") {
				continue
			}
			if err = nc.Grab(); err == nil {
				err = nc.cmd(line, 1.0) // our Z count no longer matches after a free-form line
			}
		case in == "?" || in == "":
		default:
			say("не понял «%s»\n", raw)
			continue
		}
		if err != nil {
			say("ОСТАНОВ: %v\n", err)
		}
		if m != nil {
			if e := m.Alive(); e != nil {
				say("  %v\n", e)
			} else if m.State() {
				say("  КОНТАКТ\n")
			} else {
				say("  разомкнуто\n")
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
			return 0, 0, 0, fmt.Errorf("не понял «%s», пример: m x5 y-2.5 z1", w)
		}
		seen[w[0]] = true
		v, e := strconv.ParseFloat(w[1:], 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, 0, 0, fmt.Errorf("не число в «%s»", w)
		}
		switch w[0] {
		case 'x':
			dx = v
		case 'y':
			dy = v
		case 'z':
			dz = v
		default:
			return 0, 0, 0, fmt.Errorf("оси только x, y, z: «%s»", w)
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
	say("медь на Z %+.3f от старта ручного режима\n", zf)
	return nil
}

func main() {
	var o opts
	flag.StringVar(&o.port, "com", "", "порт МК, например COM5 (по умолчанию ищется сам)")
	flag.StringVar(&o.mdiKey, "mdi", "ctrl+shift+f9", "как открыть окно MDI: \"ctrl+f9\" или по меню \"alt o e\"")
	flag.StringVar(&o.title, "title", "NcStudio", "часть заголовка окна NcStudio")
	flag.Float64Var(&o.grid, "grid", 10, "шаг сетки, мм")
	flag.Float64Var(&o.clear, "clear", 0.3, "подъём над медью между точками, мм")
	flag.Float64Var(&o.search, "search", 3, "на сколько вниз искать медь в первой точке, мм")
	flag.Float64Var(&o.below, "below", 0.3, "дальше: не глубже стольких мм под самой низкой медью")
	flag.Float64Var(&o.step, "step", 0.05, "шаг поиска по Z, мм")
	flag.Float64Var(&o.fine, "fine", 0.01, "шаг точного касания, мм")
	flag.BoolVar(&o.keep, "keep", false, "окно ввода команды в NcStudio не закрывается после Enter")
	flag.Float64Var(&o.rapid, "rapid", 1500, "скорость G00 в NcStudio, мм/мин")
	flag.IntVar(&o.settle, "settle", 350, "пауза на каждую команду, мс")
	flag.Parse()
	if err := ValidKeys(o.mdiKey); err != nil {
		say("%v\n", err)
		return
	}

	for {
		say("\npcbscan — карта высот для NcStudio\n")
		say("  1  проверить провод (МК)\n")
		say("  2  проверить связь с NcStudio (Z вверх-вниз на 1 мм)\n")
		say("  3  снять карту высот\n")
		say("  4  после смены фрезы: Z0 на меди\n")
		say("  5  выровнять программу по карте\n")
		say("  6  ручной режим: сами шлёте команды в NcStudio\n")
		say("  0  выход\n")
		var err error
		switch ask("> ") {
		case "1":
			err = cmdWire(o)
		case "2":
			err = cmdNcTest(o)
		case "3":
			err = cmdScan(o)
		case "4":
			err = cmdZero(o)
		case "5":
			err = cmdLevel(o)
		case "6":
			err = cmdManual(o)
		case "0":
			return
		}
		if err != nil {
			say("\nОСТАНОВ: %v\n", err)
		}
	}
}
