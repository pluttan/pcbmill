package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// ==========================
// ===  Grid form         ===
// ==========================

func progGrid() Grid { return GridFor(0.63, 61.26, 1.8, 46.05, 10) }

func TestGridFormStep(t *testing.T) {
	f := newGridForm(progGrid())
	for i := 0; i < gfSX; i++ {
		f.key(k(KeyDown))
	}
	if f.sel != gfSX {
		t.Fatal(f.sel)
	}
	cut := rect{0.63, 61.26, 1.8, 46.05}
	before := plainFrame(gridView(f.g, &cut, 60))
	nx := f.g.NX
	f.key(k(KeyRight)) // a bigger X step: fewer points along X
	if f.g.NX >= nx || f.g.NY != progGrid().NY {
		t.Fatalf("точек X %d -> %d, Y %d", nx, f.g.NX, f.g.NY)
	}
	if plainFrame(gridView(f.g, &cut, 60)) == before {
		t.Fatal("вид стола не изменился")
	}
	f.key(k(KeyLeft))
	if f.g.NX != nx {
		t.Fatalf("назад: %d, было %d", f.g.NX, nx)
	}
	// every Left adds a point until the step would go under 1 mm
	for i := 0; i < 200; i++ {
		f.key(k(KeyLeft))
	}
	if f.g.StepX() < 1 || f.msg == "" {
		t.Fatalf("шаг %.2f, сообщение %q", f.g.StepX(), f.msg)
	}
}

func TestGridFormFields(t *testing.T) {
	f := newGridForm(progGrid())
	// X от: Right moves the edge 1 mm, the step stays, the count follows
	f.key(k(KeyRight))
	if f.g.X0 != 1.63 {
		t.Fatal(f.g.X0)
	}
	// a number typed into "шаг Y"
	f.sel = gfSY
	for _, x := range keys(KeyEnter, KeyBackspace, KeyBackspace, KeyBackspace, KeyBackspace, KeyBackspace, "5,5", KeyEnter) {
		f.key(x)
	}
	if f.g.NY != CountFor(f.g.Y0, f.g.Y1, 5.5) || f.edit != nil {
		t.Fatalf("точек Y %d, msg %q", f.g.NY, f.msg)
	}
	// typing starts at the first digit; a bad number is refused with a message
	f.sel = gfNX
	for _, x := range keys("500", KeyEnter) {
		f.key(x)
	}
	if f.g.NX > 100 || !strings.Contains(f.msg, "от 1 до 100") {
		t.Fatal(f.g.NX, f.msg)
	}
	// Esc while typing only closes the field
	f.sel = gfNX
	if _, done := f.key(runeKey('7')); done {
		t.Fatal()
	}
	if _, done := f.key(k(KeyEsc)); done || f.edit != nil || f.g.NX > 100 {
		t.Fatal("Esc в поле закрыл форму")
	}
	// margin: the program cut plus 2 mm on every side
	f.sel = gfMargin
	f.key(k(KeyRight))
	f.key(k(KeyRight))
	p := progGrid()
	if f.g.X0 != p.X0-2 || f.g.Y1 != p.Y1+2 {
		t.Fatalf("%+v", f.g)
	}
	// "Как в программе"
	f.sel = gfProg
	f.key(k(KeyEnter))
	if f.g != p || f.margin != 0 {
		t.Fatal(f.g)
	}
}

func TestEditGridRun(t *testing.T) {
	// down past the fields to the buttons: Начать обход
	ks := []Key{k(KeyDown), k(KeyDown), k(KeyDown), k(KeyDown), k(KeyRight)} // шаг X bigger
	for i := gfSX; i < gfStart; i++ {
		ks = append(ks, k(KeyDown))
	}
	f := useTerm(t, 100, 40, append(ks, k(KeyEnter))...)
	g, ok := editGrid(progGrid())
	if !ok || g.NX >= progGrid().NX {
		t.Fatal(ok, g)
	}
	all := plainFrame(f.last())
	for _, want := range []string{"шаг X", "Начать обход", "Как в программе", "Отмена", "X, мм", "точек"} {
		if !strings.Contains(all, want) {
			t.Fatalf("нет %q:\n%s", want, all)
		}
	}
	selected(t, f.last(), "► Начать обход ◄")
	// Отмена and Esc
	useTerm(t, 80, 25, k(KeyUp), k(KeyEnter))
	if _, ok := editGrid(progGrid()); ok {
		t.Fatal("Отмена начала обход")
	}
	useTerm(t, 80, 25, k(KeyEsc))
	if _, ok := editGrid(progGrid()); ok {
		t.Fatal("Esc начал обход")
	}
}

func TestGridFormScreen(t *testing.T) {
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		f := newGridForm(progGrid())
		f.sel = gfSX
		fr := f.view(sz[0], sz[1])
		if len(fr) != sz[1] {
			t.Fatal(len(fr))
		}
		onScreen(t, "сетка", fr, sz[0])
		centred(t, "сетка", fr, sz[0], "Сетка карты высот")
		selected(t, fr, "шаг X")
		if sz[0] >= 80 && !strings.Contains(plainFrame(fr), "■") {
			t.Errorf("%v: нет вида стола:\n%s", sz, plainFrame(fr))
		}
		f.edit = newField("12")
		onScreen(t, "сетка, ввод", f.view(sz[0], sz[1]), sz[0])
	}
}

// ==========================
// ===  Manual mode       ===
// ==========================

// deadTouch is an MCU that stopped answering.
type deadTouch struct{ touch bool }

func (d deadTouch) Alive() error { return errors.New("МК не отвечает 0.7 с") }
func (d deadTouch) State() bool  { return d.touch }
func (d deadTouch) Hits() int    { return 0 }

func manualOn(t *testing.T, m Touch, sm *simMachine, nc *NcStudio, ks ...Key) *manual {
	useTerm(t, 80, 25, ks...)
	s := newManual(opts{search: 3, step: 0.05, fine: 0.01, clear: 0.3, below: 0.3}, nc, m, "COM5")
	cur = s.p
	return s
}

func sent(sm *simMachine) string { return strings.Join(sm.lines, " | ") }

func TestManualTouchingBlocks(t *testing.T) {
	sm, nc, _ := setup()
	s := manualOn(t, sm, sm, nc)
	sm.state = true // the tip is on the copper
	for _, c := range []KeyCode{KeyLeft, KeyRight, KeyUp, KeyDown, KeyPgDn} {
		s.key(k(c))
	}
	for _, r := range "-t" {
		s.key(runeKey(r))
	}
	if len(sm.lines) != 0 {
		t.Fatalf("при касании ушли команды: %s", sent(sm))
	}
	if !strings.Contains(plainFrame(s.p.log), "касается меди") {
		t.Fatalf("нет объяснения в журнале: %q", s.p.log)
	}
	s.key(k(KeyPgUp)) // up is the way out: allowed
	if len(sm.lines) != 1 || !strings.Contains(sm.lines[0], "G91 G00 Z1.0000") {
		t.Fatalf("PageUp при касании: %s", sent(sm))
	}
	sm.state = true
	s.key(runeKey(' '))
	if len(sm.lines) != 2 || !strings.Contains(sm.lines[1], "G91 G00 Z0.1000") {
		t.Fatalf("пробел при касании: %s", sent(sm))
	}
}

func TestManualDeadMCU(t *testing.T) {
	sm, nc, _ := setup()
	s := manualOn(t, deadTouch{}, sm, nc)
	for _, c := range []KeyCode{KeyLeft, KeyRight, KeyUp, KeyDown, KeyPgDn} {
		s.key(k(c))
	}
	s.key(runeKey(' '))
	if len(sm.lines) != 0 {
		t.Fatalf("без живого МК ушли команды: %s", sent(sm))
	}
	if !strings.Contains(plainFrame(s.p.log), "не двигаю") {
		t.Fatal(s.p.log)
	}
	s.key(k(KeyPgUp))
	if len(sm.lines) != 1 {
		t.Fatalf("PageUp без МК должен поднимать: %s", sent(sm))
	}
	if !strings.Contains(plainFrame(s.view(80, 25)), "НЕ ОТВЕЧАЕТ") {
		t.Fatal("на экране не видно, что МК молчит")
	}
}

func TestManualMoves(t *testing.T) {
	sm, nc, _ := setup()
	s := manualOn(t, sm, sm, nc)
	sm.z = 5 // well above the copper
	for _, c := range []KeyCode{KeyLeft, KeyUp, KeyRight, KeyDown, KeyPgDn} {
		s.key(k(c))
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
	s.key(runeKey('*'))
	s.key(runeKey('*'))
	s.key(runeKey('+'))
	if s.step != 0.5 || s.stepX != 2 || s.stepY != 2 {
		t.Fatal(s.step, s.stepX, s.stepY)
	}
	s.key(runeKey('-'))
	if s.stepX != 1 {
		t.Fatal(s.stepX)
	}
	if s.flushCount() == 0 {
		t.Fatal("после движения очередь клавиш не очищена")
	}
	// the command line: Tab, a shift, Enter; up first, then sideways
	n := len(sm.lines)
	for _, x := range keys(KeyTab, "m x5 z1", KeyEnter) {
		s.key(x)
	}
	if len(sm.lines) != n+2 || !strings.Contains(sm.lines[n], "Z1.0000") || !strings.Contains(sm.lines[n+1], "X5.0000") {
		t.Fatal(sent(sm))
	}
	// Esc closes the command line, not the mode
	if !s.key(k(KeyTab)) || !s.key(k(KeyEsc)) || s.cmd != nil {
		t.Fatal("Esc в строке команды")
	}
	if s.key(k(KeyEsc)) {
		t.Fatal("Esc не вышел из ручного режима")
	}
}

func TestManualGcodeConfirm(t *testing.T) {
	sm, nc, _ := setup()
	s := manualOn(t, sm, sm, nc)
	sm.z = 5
	f := term.(*fakeTerm)
	f.keys = keys(KeyEnter) // the dialog: Нет is chosen at first
	for _, x := range keys(KeyTab, "g g0 x1", KeyEnter) {
		s.key(x)
	}
	if len(sm.lines) != 0 {
		t.Fatalf("без подтверждения ушло: %s", sent(sm))
	}
	f.keys = keys("д")
	for _, x := range keys(KeyTab, "g g0 x1", KeyEnter) {
		s.key(x)
	}
	if len(sm.lines) != 1 || sm.lines[0] != "G0 X1" {
		t.Fatal(sent(sm))
	}
}

func (s *manual) flushCount() int { return term.(*fakeTerm).flush }

func TestManualScreen(t *testing.T) {
	for _, sz := range sizes {
		sm, nc, _ := setup()
		s := manualOn(t, sm, sm, nc)
		term.(*fakeTerm).w, term.(*fakeTerm).h = sz[0], sz[1]
		for i := 0; i < 12; i++ {
			sayOK("движение номер " + string(rune('A'+i)))
		}
		fr := s.view(sz[0], sz[1])
		onScreen(t, "ручной", fr, sz[0])
		all := plainFrame(fr)
		for _, want := range []string{"разомкнуто", "Z от старта", "номер L"} {
			if !strings.Contains(all, want) {
				t.Errorf("%v: нет %q\n%s", sz, want, all)
			}
		}
		if sz[1] >= 25 && !strings.Contains(all, "PageUp") {
			t.Errorf("%v: нет схемы клавиш\n%s", sz, all)
		}
		sm.state = true
		s.cmd = newField("m x5")
		fr = s.view(sz[0], sz[1])
		onScreen(t, "ручной, команда", fr, sz[0])
		if !strings.Contains(plainFrame(fr), "КОНТАКТ") || !strings.Contains(plainFrame(fr), ": m x5") {
			t.Errorf("%v:\n%s", sz, plainFrame(fr))
		}
	}
}

// ==========================
// ===  Scan and map      ===
// ==========================

func TestScanScreen(t *testing.T) {
	g := progGrid()
	cut := rect{g.X0, g.X1, g.Y0, g.Y1}
	var done []Point
	for i, p := range g.Points()[:10] {
		done = append(done, Point{p[0], p[1], float64(i%4) * 0.01})
	}
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		p := newPage(menuTitle("3"))
		p.info = scanInfo(len(done), len(g.Points()), time.Now().Add(-time.Minute), "10/40 X 1 Y 2 Z 0.03", "")
		p.nolog = true
		p.keys = []string{"Esc", "стоп между точками"}
		p.pic = func(w, h int) []string {
			return fitRows(h, func(r int) []string { return scanView(g, &cut, done, true, w, r) })
		}
		sayHint("  1/40  X   0.63 Y   1.80  Z  0.0000")
		fr := p.view(sz[0], sz[1])
		onScreen(t, "обход", fr, sz[0])
		all := plainFrame(fr)
		for _, want := range []string{"10 из 48", "осталось ~", "Esc", "10/40"} {
			if !strings.Contains(all, want) {
				t.Errorf("%v: нет %q\n%s", sz, want, all)
			}
		}
		if sz[1] >= 25 && (!strings.Contains(all, "■") || !strings.Contains(all, "·") || !strings.Contains(all, "▓")) {
			t.Errorf("%v: на карте нет точек:\n%s", sz, all)
		}
	}
}

func TestMapResult(t *testing.T) {
	h := testMap(t, true)
	for _, sz := range sizes {
		f := useTerm(t, sz[0], sz[1], k(KeyTab), k(KeyEnter))
		mapResult(menuTitle("3"), h, &rect{0, 60, 0, 40}, []string{"Сдвиг первой точки за обход: +0.041 мм",
			paint(cAlarm, "▲ ВНИМАНИЕ:") + " больше 0.03 мм."})
		pic, table := plainFrame(f.frames[0]), plainFrame(f.last())
		if !strings.Contains(pic, "ВНИМАНИЕ") || !strings.Contains(table, "Y\\X") {
			t.Fatalf("%v:\n%s\n%s", sz, pic, table)
		}
		if sz[1] >= 25 && !strings.Contains(pic, "█") {
			t.Errorf("%v: нет карты:\n%s", sz, pic)
		}
	}
}
