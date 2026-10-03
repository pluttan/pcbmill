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
	a := strings.ToLower(prompt(q + " " + paint(cKey, "[y/n]") + ": "))
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
	var items []menuItem
	for i, t := range taps {
		items = append(items, menuItem{strconv.Itoa(i + 1), filepath.Base(t)})
	}
	show(menu("Программы рядом с pcbscan.exe", items, boxW()))
	var k int
	fmt.Sscan(prompt("номер программы: "), &k)
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
	keep, nolayout      bool
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
	s.Log = func(l string) { show([]string{l}) }
	return s
}

func cmdWire(o opts) error {
	show(rule("1 ► Проверка провода", boxW()))
	m, name, err := findMCU(o.port)
	if err != nil {
		return err
	}
	sayOK("МК найден на " + name)
	sayHint("Касайтесь зажимом фрезы; выход в меню: Enter.")
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
			t := paint(cHint, time.Now().Format("15:04:05"))
			if s {
				show([]string{t + "  " + paint(cTouch, "♦ КОНТАКТ")})
			} else {
				show([]string{t + "  " + paint(cOK, "○ разомкнуто")})
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func cmdNcTest(o opts) error {
	show(rule("2 ► Связь с NcStudio", boxW()))
	show(box(thin, "Что будет", []string{
		"Фреза не ниже 5 мм над заготовкой. Программа нажмёт в NcStudio " + paint(cKey, o.mdiKey) + " и введёт",
		"  " + paint(cTitle, "G91 G00 Z1") + "   вверх на 1 мм, потом",
		"  " + paint(cTitle, "G91 G00 Z-1") + "  обратно.",
		sep,
		paint(cHint, "Пока она печатает, мышь и клавиатуру не трогать."),
	}, boxW()))
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
		sayOK("Связь с NcStudio работает.")
	} else {
		sayWarn("не сработало. Узнайте, какими клавишами в вашей NcStudio открывается окно ввода " +
			"одной строки G-кода (MDI), и запустите: pcbscan.exe -mdi ctrl+shift+f9 (свои клавиши).")
	}
	return nil
}

func gridFor(tap string, step float64) (Grid, error) {
	lines, err := readLines(tap)
	if err != nil {
		return Grid{}, err
	}
	x0, x1, y0, y1, err := CutBounds(lines)
	if err != nil {
		return Grid{}, err
	}
	return GridFor(x0, x1, y0, y1, step), nil
}

// secondsPerPoint: about 8 MDI commands of about 1.1 s each with the predicted approach.
const secondsPerPoint = 9.0

func cmdScan(o opts) error {
	show(rule("3 ► Карта высот", boxW()))
	tap, err := pickTap(exeDir())
	if err != nil {
		return err
	}
	g, err := gridFor(tap, o.grid)
	if err != nil {
		return err
	}
	cut := rect{g.X0, g.X1, g.Y0, g.Y1}
	g, ok := editGrid(g)
	if !ok {
		return nil
	}
	pts := g.Points()
	m, name, err := findMCU(o.port)
	if err != nil {
		return err
	}
	show(box(thin, "Перед стартом", []string{
		fmt.Sprintf("МК на %s. Точек %d, примерно %.0f мин.", paint(cOK, name), len(pts), float64(len(pts))*secondsPerPoint/60),
		"В NcStudio ноль X0 Y0 стоит как для этой платы, шпиндель выключен,",
		fmt.Sprintf("фреза (любая крепкая) на 1-2 мм над медью рядом с %s.",
			paint(cZero, fmt.Sprintf("♦ X%.1f Y%.1f", pts[0][0], pts[0][1]))),
		fmt.Sprintf("Не выше 2 мм: в первой точке медь ищется на %.0f мм вниз от старта.", o.search),
		sep,
		paint(cAlarm, "▲ Сейчас ОБНУЛИТЕ Z в NcStudio на этой высоте:") + " обход ведёт Z абсолютными командами от этого нуля.",
	}, boxW()))
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
	sc.Predict = true // nc.Abs above
	start, done := time.Now(), 0
	sc.Log = func(l string) {
		done++
		left := ""
		if done < len(pts) {
			rest := time.Since(start) / time.Duration(done) * time.Duration(len(pts)-done)
			left = paint(cHint, fmt.Sprintf("  ещё ~%.0f мин", math.Ceil(rest.Minutes())))
		}
		show([]string{bar(done, len(pts), 12) + " " + l + left})
	}
	rows, err := sc.Grid(pts, func(p Point) {
		fmt.Fprintf(f, "%.3f,%.3f,%.4f\n", p.X, p.Y, p.Z)
		f.Sync()
	})
	if err != nil {
		say("\n")
		sayStop(err)
		if yes("поднять фрезу на 1 мм (NcStudio снова получит фокус)?") && nc.Grab() == nil {
			nc.RapidZ(nc.z + o.clear)
		}
		return fmt.Errorf("карта не досняна, scan.csv содержит измеренные точки")
	}
	sayHint("Повторное касание первой точки: насколько всё уехало за время обхода...")
	drift, err := sc.Recheck(rows)
	if err != nil {
		return fmt.Errorf("повторное касание не удалось, карте верить нельзя: %v", err)
	}
	fmt.Fprintf(f, "# повторное касание первой точки: сдвиг %+.4f мм\n", drift)
	hm, _ := MapFromPoints(rows)
	showMap(hm, &cut)
	tell("Сдвиг первой точки за обход: %s мм", paint(cTitle, fmt.Sprintf("%+.3f", drift)))
	if math.Abs(drift) > 0.03 {
		sayWarn("больше 0.03 мм. Ось Z уходит во время работы или станок пропускает команды: " +
			"по этой карте резать нельзя. Проверьте механику Z и повторите.")
	} else {
		sayOK("Сдвиг в норме, карте можно верить.")
	}
	sayOK("Карта записана: " + out)
	return nil
}

func cmdZero(o opts) error {
	show(rule("4 ► Z0 на меди после смены фрезы", boxW()))
	// Z0 goes where the map has its zero: the first point of scan.csv, whatever grid it used
	var first [2]float64
	if rows, e := LoadPoints(filepath.Join(exeDir(), "scan.csv")); e == nil && len(rows) > 0 {
		first = [2]float64{rows[0].X, rows[0].Y}
	} else {
		tap, err := pickTap(exeDir())
		if err != nil {
			return err
		}
		g, err := gridFor(tap, o.grid)
		if err != nil {
			return err
		}
		first = g.Points()[0]
		sayHint("scan.csv нет, беру первую точку сетки по программе")
	}
	pts := [][2]float64{first}
	m, _, err := findMCU(o.port)
	if err != nil {
		return err
	}
	show(box(thin, "Что будет", []string{
		"Фреза 0.1 стоит на 1-2 мм над медью. Будет одно медленное касание в " +
			paint(cZero, fmt.Sprintf("♦ X%.2f Y%.2f", pts[0][0], pts[0][1])) + " (первая точка карты),",
		"потом нужно обнулить Z в NcStudio.",
	}, boxW()))
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
	show(note("♦", cTouch, "КОНТАКТ:", "фреза касается меди.", boxW()))
	prompt("Обнулите Z в NcStudio (рабочая система координат), затем Enter здесь... ")
	nc.ResetZ()
	if err := nc.Grab(); err != nil {
		return err
	}
	if err := nc.RapidZ(o.clear * 3); err != nil {
		return err
	}
	sayOK("Z0 стоит на меди в первой точке карты. Теперь пункт 5 и запуск *_level.tap.")
	return nil
}

func cmdLevel(o opts) error {
	show(rule("5 ► Выровнять программу по карте", boxW()))
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
	var cut *rect
	if x0, x1, y0, y1, e := CutBounds(lines); e == nil {
		cut = &rect{x0, x1, y0, y1}
	}
	showMap(hm, cut)
	sayOK("Готово: " + filepath.Base(dst))
	return nil
}

// cmdFindCoords: can this program read the X/Y/Z NcStudio shows? Snapshot the text
// of every NcStudio control, move each axis by 1 mm, and see which texts change.
func cmdFindCoords(o opts) error {
	show(rule("7 ► Откуда читать координаты NcStudio", boxW()))
	show(box(thin, "Что будет", []string{
		"Программа ищет, откуда можно читать координаты NcStudio.",
		"Станок сдвинется: Z вверх на 1 мм, X на +1, Y на +1, и вернётся обратно.",
		paint(cHint, "Фреза должна быть поднята над платой, шпиндель стоять."),
	}, boxW()))
	if !yes("начать?") {
		return nil
	}
	out := filepath.Join(exeDir(), "ncwindows.txt")
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	snap := func(name string) (map[uintptr]WinInfo, error) {
		ws, err := NcWindows(o.title)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(f, "\r\n=== %s: %d окон\r\n", name, len(ws))
		m := map[uintptr]WinInfo{}
		for _, w := range ws {
			m[w.H] = w
			fmt.Fprintf(f, "%08x  %-28s id %-6d (%d,%d)-(%d,%d)  %q\r\n",
				w.H, w.Class, w.ID, w.Left, w.Top, w.Right, w.Bott, w.Text)
		}
		return m, nil
	}
	base, err := snap("до движения")
	if err != nil {
		return err
	}
	nc := newNc(o)
	if err := nc.Grab(); err != nil {
		return err
	}
	found := 0
	prev := base
	for _, ax := range []string{"Z", "X", "Y"} {
		if err := nc.cmd("G91 G00 "+ax+"1", 1.0); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond) // the display refreshes after the move
		cur, err := snap(ax + " +1 мм")
		if err != nil {
			return err
		}
		for h, w := range cur {
			if p, ok := prev[h]; ok && p.Text != w.Text {
				found++
				line := fmt.Sprintf("после %s+1 изменилось: %s id %d (%d,%d)  %q -> %q",
					ax, w.Class, w.ID, w.Left, w.Top, p.Text, w.Text)
				fmt.Fprintf(f, "%s\r\n", line)
				show(wrap("  "+line, boxW(), 4))
			}
		}
		prev = cur
	}
	if err := nc.Grab(); err == nil {
		nc.cmd("G91 G00 X-1 Y-1", 1.0)
		nc.cmd("G91 G00 Z-1", 1.0)
	}
	if found == 0 {
		sayWarn("ни один текст не изменился: NcStudio рисует цифры картинкой, читать придётся с экрана.")
	} else {
		sayOK("Координаты читаются как текст.")
	}
	tell("Всё записано в %s, пришлите этот файл.", out)
	return nil
}

// showMap: the table picture, then the same map in numbers.
func showMap(hm *HeightMap, cut *rect) {
	say("\n")
	show(rule("Карта высот, вид сверху", boxW()))
	showMid(heightView(hm, cut, mapW()))
	say("\n")
	show(heightTable(hm, boxW()))
}

var mainMenu = []menuItem{
	{"1", "проверить провод (МК)"},
	{"2", "проверить связь с NcStudio (Z вверх-вниз на 1 мм)"},
	{sep, ""},
	{"3", paint(cTitle, "снять карту высот")},
	{"4", "после смены фрезы: Z0 на меди"},
	{"5", "выровнять программу по карте"},
	{sep, ""},
	{"6", "ручной режим: сами шлёте команды в NcStudio"},
	{"7", "найти, откуда читать координаты NcStudio"},
	{sep, ""},
	{"0", "выход"},
}

// status is what the main screen shows above the menu.
func status(o opts) Status {
	st := Status{MCU: mcuStateOf(mcuCache), Nc: ncFound(o.title)}
	if mcuCache != nil {
		st.Port = mcuName
	}
	return st
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
	flag.BoolVar(&o.nolayout, "nolayout", false, "не ставить NcStudio на левую половину экрана, а pcbscan на правую")
	flag.Parse()
	if err := ValidKeys(o.mdiKey); err != nil {
		sayStop(err)
		return
	}
	if !o.nolayout {
		layoutWindows(o.title)
	}

	for {
		say("\n")
		show(banner(boxW()))
		show(statusLine(status(o), boxW()))
		show(menu("Меню", mainMenu, boxW()))
		var err error
		switch prompt("пункт: ") {
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
		case "7":
			err = cmdFindCoords(o)
		case "0":
			return
		}
		if err != nil {
			say("\n")
			sayStop(err)
		}
	}
}
