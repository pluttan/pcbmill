package main

// The safety of the manual mode: no move down or sideways while the tip is on
// the copper or the MCU is silent, up always allowed, G-code only after a yes.

import (
	"errors"
	"strings"
	"testing"

	"pcbscan/ui"
)

// deadTouch is an MCU that stopped answering.
type deadTouch struct{ touch bool }

func (d deadTouch) Alive() error { return errors.New("МК не отвечает 0.7 с") }
func (d deadTouch) State() bool  { return d.touch }
func (d deadTouch) Hits() int    { return 0 }

// manualOn: the manual page on the screen, as cmdManual shows it.
func manualOn(t *testing.T, m Touch, nc *NcStudio) (*manual, *ui.Headless) {
	d := testApp(t)
	s := newManual(opts{search: 3, step: 0.05, fine: 0.01, clear: 0.3, below: 0.3}, nc, m, "COM5")
	s.in = make(chan string, 1)
	cur = s.p
	t.Cleanup(func() { cur = nil })
	s.p.head = "Ручной режим"
	s.p.extra = s.panel
	s.p.onKey = s.onKey
	app.Show(s.p)
	return s, d
}

// press: a key the way the window turns it into a command, done at once.
func (s *manual) press(e ui.Event) bool {
	if c := s.command(e); c != "" {
		return s.run(c)
	}
	return true
}

func key(k ui.Key) ui.Event { return ui.Event{Kind: ui.EvKey, Key: k} }
func char(r rune) ui.Event  { return ui.Event{Kind: ui.EvChar, Rune: r} }

var sideAndDown = []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyPgDn}

func sent(sm *simMachine) string { return strings.Join(sm.lines, " | ") }

func logText(p *page) string {
	var b strings.Builder
	app.Do(func() { b.WriteString(strings.Join(p.log, "\n")) })
	return b.String()
}

func TestManualTouchingBlocks(t *testing.T) {
	sm, nc, _ := setup()
	s, _ := manualOn(t, sm, nc)
	sm.state = true // the tip is on the copper
	for _, k := range sideAndDown {
		s.press(key(k))
	}
	s.press(char('t'))
	if len(sm.lines) != 0 {
		t.Fatalf("при касании ушли команды: %s", sent(sm))
	}
	if !strings.Contains(logText(s.p), "касается меди") {
		t.Fatalf("нет объяснения в журнале: %q", logText(s.p))
	}
	app.Wake()
	waitText(t, "КОНТАКТ")
	s.press(key(ui.KeyPgUp)) // up is the way out: allowed
	if len(sm.lines) != 1 || !strings.Contains(sm.lines[0], "G91 G00 Z1.0000") {
		t.Fatalf("PageUp при касании: %s", sent(sm))
	}
	sm.state = true
	s.press(char(' '))
	if len(sm.lines) != 2 || !strings.Contains(sm.lines[1], "G91 G00 Z0.1000") {
		t.Fatalf("пробел при касании: %s", sent(sm))
	}
}

func TestManualDeadMCU(t *testing.T) {
	sm, nc, _ := setup()
	s, _ := manualOn(t, deadTouch{}, nc)
	for _, k := range sideAndDown {
		s.press(key(k))
	}
	s.press(char(' '))
	if len(sm.lines) != 0 {
		t.Fatalf("без живого МК ушли команды: %s", sent(sm))
	}
	if !strings.Contains(logText(s.p), "не двигаю") {
		t.Fatal(logText(s.p))
	}
	s.press(key(ui.KeyPgUp))
	if len(sm.lines) != 1 {
		t.Fatalf("PageUp без МК должен поднимать: %s", sent(sm))
	}
	waitText(t, "МК НЕ ОТВЕЧАЕТ")
}

func TestManualMoves(t *testing.T) {
	sm, nc, _ := setup()
	s, d := manualOn(t, sm, nc)
	sm.z = 5 // well above the copper
	for _, k := range []ui.Key{ui.KeyLeft, ui.KeyUp, ui.KeyRight, ui.KeyDown, ui.KeyPgDn} {
		s.press(key(k))
	}
	want := []string{"X-1.0000 Y0.0000", "X0.0000 Y1.0000", "X1.0000 Y0.0000", "X0.0000 Y-1.0000", "G01 Z-0.1000 F20"}
	if len(sm.lines) != len(want) {
		t.Fatal(sent(sm))
	}
	for i, w := range want {
		if !strings.Contains(sm.lines[i], w) {
			t.Fatalf("%d: %q, ждали %q", i, sm.lines[i], w)
		}
	}
	// steps by keys: * / for Z, + - for X/Y (the same keys on both layouts)
	for _, r := range "**+" {
		s.press(char(r))
	}
	if s.step != 0.5 || s.stepX != 2 || s.stepY != 2 {
		t.Fatal(s.step, s.stepX, s.stepY)
	}
	s.press(char('-'))
	if s.stepX != 1 {
		t.Fatal(s.stepX)
	}
	// the command line in the window: Tab, a shift, Enter; up first, then sideways
	n := len(sm.lines)
	sendKeys(d, ui.KeyTab, "m x5 z1", ui.KeyEnter)
	s.run(<-s.in)
	if len(sm.lines) != n+2 || !strings.Contains(sm.lines[n], "Z1.0000") || !strings.Contains(sm.lines[n+1], "X5.0000") {
		t.Fatal(sent(sm))
	}
	// Esc closes the command line, not the mode
	sendKeys(d, ui.KeyTab, ui.KeyEsc)
	app.Do(func() {})
	if s.cmd != nil || len(s.in) != 0 {
		t.Fatal("Esc в строке команды")
	}
	if s.press(key(ui.KeyEsc)) {
		t.Fatal("Esc не вышел из ручного режима")
	}
}

func TestManualGcodeConfirm(t *testing.T) {
	sm, nc, _ := setup()
	s, d := manualOn(t, sm, nc)
	sm.z = 5
	ask := func(answer interface{}) {
		done := make(chan bool)
		go func() { s.run("g g0 x1"); close(done) }()
		waitText(t, "Да")
		sendKeys(d, answer)
		<-done
	}
	ask(ui.KeyEnter) // Нет is chosen at first
	if len(sm.lines) != 0 {
		t.Fatalf("без подтверждения ушло: %s", sent(sm))
	}
	ask("д")
	if len(sm.lines) != 1 || sm.lines[0] != "G0 X1" {
		t.Fatal(sent(sm))
	}
}
