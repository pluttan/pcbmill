package main

// pcbscan.exe — height map for PCB milling on a Weihong NcStudio machine with
// no probe input: an MCU reports bit-to-copper contact, this program types
// short moves into NcStudio's MDI window. Runs on bare Windows 7 in its own
// window, drawn in software: no runtime, no libraries.
//
// pluttan

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func exeDir() string {
	if d := os.Getenv("PCBSCAN_DIR"); d != "" { // tests and demos: another folder
		return d
	}
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

// errBack: the operator went back from a list; not an error to show.
var errBack = errors.New("назад")

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
	keep, nolayout, sim bool
	simSpeed            float64
	step, fine          float64
	rapid               float64
	settle              int
}

func newNc(o opts) *NcStudio {
	if simM != nil {
		n := NewNcStudio(simM)
		n.Rapid, n.Sleep = o.rapid, simM.sleep
		n.Settle = time.Duration(o.settle) * time.Millisecond
		n.Reopen = !o.keep
		return n
	}
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
	s.Log = logLine
	return s
}

var (
	statusMu   sync.Mutex
	statusLast Status
	statusAt   time.Time
)

// statusCached: the wire check redraws 25 times a second, looking for the
// NcStudio window that often is pointless.
func statusCached(o opts) Status {
	if simM != nil { // nothing to look for: the emulation answers at once
		return status(o)
	}
	statusMu.Lock()
	defer statusMu.Unlock()
	if time.Since(statusAt) > 300*time.Millisecond {
		statusLast, statusAt = status(o), time.Now()
	}
	return statusLast
}

// status is what the status line of every screen shows.
func status(o opts) Status {
	if simM != nil {
		return simStatus()
	}
	var m Touch
	if mcuCache != nil { // a nil *MCU in the interface would not be nil
		m = mcuCache
	}
	st := Status{MCU: mcuStateOf(m), Nc: ncFound(o.title)}
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
	flag.BoolVar(&o.sim, "sim", runtime.GOOS != "windows", "эмуляция станка и МК: всё меню без станка (на Windows по ключу)")
	flag.Float64Var(&o.simSpeed, "simspeed", 5, "эмуляция: во сколько раз быстрее настоящего станка (1 - как есть)")
	flag.Parse()
	if o.sim {
		startSim(o)
	}
	if err := ValidKeys(o.mdiKey); err != nil {
		fatal("ОСТАНОВ: " + err.Error())
		os.Exit(2)
	}
	defer func() {
		if r := recover(); r != nil {
			fatal(fmt.Sprintf("pcbscan упал: %v", r))
			panic(r)
		}
	}()
	if err := startGUI(o); err != nil {
		fatal("ОСТАНОВ: " + err.Error())
		os.Exit(2)
	}
	if !o.nolayout && !o.sim {
		layoutWindows(o.title)
	}
	statusFn = func() Status { return statusCached(o) }
	jobMain(o)
	app.Disp.Close()
}
