package main

import (
	"strings"
	"testing"
)

// withSim turns the emulation on for one test.
func withSim(t *testing.T) *simMachine {
	old := simM
	simM = newSimMachine(simSurface)
	simM.Reset()
	t.Cleanup(func() { simM = old })
	return simM
}

// The bars at the top and the bottom take the whole width, keys are buttons.
func TestBars(t *testing.T) {
	for _, w := range []int{40, 59, 79, 99} {
		h := headBar("3  Снять карту высот", w)
		if vlen(h) != w || !strings.HasPrefix(h, bgInk(cBarBg)) {
			t.Fatalf("w %d: шапка %d: %q", w, vlen(h), strip(h))
		}
		k := hints(w, "Enter", "открыть", "Esc", "назад")
		if vlen(k) != w || !strings.Contains(k, bgInk(cBlack)+ink(cWhite)+" Enter ") {
			t.Fatalf("w %d: подсказки %d: %q", w, vlen(k), k)
		}
		onScreen(t, "полосы", []string{h, k}, w)
	}
	if strings.Contains(strip(headBar("x", 79)), "ЭМУЛЯЦИЯ") {
		t.Fatal("метка эмуляции без эмуляции")
	}
	withSim(t)
	if h := headBar("x", 79); !strings.Contains(strip(h), "ЭМУЛЯЦИЯ") || !strings.Contains(h, bgInk(cYellow)) {
		t.Fatalf("нет метки эмуляции: %q", h)
	}
}

// A selected line is a bar across the whole inside of its panel, margins too.
func TestSelectionBar(t *testing.T) {
	b := box(thin, "Меню", []string{"  1   один", sel("► 2   два")}, 40)
	line := b[2]
	in := strings.TrimSuffix(strings.TrimPrefix(line, paint(cFrame, "│")), paint(cFrame, "│"))
	if !strings.HasPrefix(in, bgInk(cSelBg)) || vlen(in) != 38 || strings.Count(in, "\x1b") != 3 {
		t.Fatalf("полоса не во всю ширину: %q", line)
	}
	if !strings.HasPrefix(strip(in), "  ► 2   два") {
		t.Fatalf("отступ не тот: %q", strip(in))
	}
}

// A dialog dims the screen under it and drops a shadow right and down.
func TestOverlayShadow(t *testing.T) {
	useTerm(t, 80, 25)
	base := newMenuNav(mainMenu).view(80, 25)
	d := &dialog{title: "Вопрос", text: []string{"Начать?"}, buttons: []string{"Да", "Нет"}, sel: 1}
	b := d.box(80)
	fr := overlay(base, b, 80, 25)
	if len(fr) != 25 {
		t.Fatal(len(fr))
	}
	onScreen(t, "диалог", fr, 80)
	top := -1
	for i, l := range fr {
		if strings.Contains(l, "╔") {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("нет диалога")
	}
	right := []rune(strip(fr[top+1]))
	x := strings.Index(strip(fr[top]), "╗")
	x = len([]rune(strip(fr[top])[:x]))
	if string(right[x+1:x+3]) != "░░" {
		t.Fatalf("нет тени справа: %q", strip(fr[top+1]))
	}
	if !strings.Contains(strip(fr[top+len(b)]), repeat("░", 20)) {
		t.Fatalf("нет тени снизу: %q", strip(fr[top+len(b)]))
	}
	// under the dialog the menu is grey: no colour but the dim one outside the box
	for _, l := range fr[1 : top-1] {
		for i := 0; i+1 < len(l); i++ {
			if l[i] == esc && l[i+1] != '0' && l[i+1] != byte('A'+cDark) {
				t.Fatalf("фон не приглушён: %q", l)
			}
		}
	}
}

func TestStatusSim(t *testing.T) {
	st := Status{Port: "эмуляция", MCU: mcuOpen, Nc: true,
		Sim: &SimInfo{X: 12.5, Y: -1, Z: -1.234, Moving: true, Faults: []string{"ЗАЖИМ", "Z ПОЛЗЁТ"}}}
	for _, w := range []int{40, 59, 79} {
		ls := statusLine(st, w)
		onScreen(t, "статус эмуляции", ls, w)
		all := strip(strings.Join(ls, "\n"))
		for _, want := range []string{"X  12.500", "Y  -1.000", "Z  -1.234", "едет", " ЗАЖИМ ", " Z ПОЛЗЁТ "} {
			if !strings.Contains(all, want) {
				t.Errorf("w %d: нет %q:\n%s", w, want, all)
			}
		}
	}
	if l := statusLine(Status{MCU: mcuOpen, Nc: true, Sim: &SimInfo{}}, 59); len(l) != 2 {
		t.Errorf("при 60 колонках статус эмуляции не в две строки: %q", l)
	}
}

func TestSimPanel(t *testing.T) {
	sm := withSim(t)
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		fr := (&simPanel{}).view(sz[0], sz[1])
		if len(fr) != sz[1] {
			t.Fatal(len(fr))
		}
		onScreen(t, "эмуляция", fr, sz[0])
		centred(t, "эмуляция", fr, sz[0], "Неисправности")
		selected(t, fr, "МК отвалился")
	}
	p := &simPanel{}
	p.key(k(KeyEnter))
	if !sm.On(fDead) {
		t.Fatal("Enter не включил неисправность")
	}
	p.key(runeKey('4'))
	if !sm.On(fSkip) || p.sel != int(fSkip) {
		t.Fatal("цифра не включила")
	}
	p.key(runeKey('8')) // all off
	for f := simFault(0); f < fCount; f++ {
		if sm.On(f) {
			t.Fatalf("%s осталась", simFaults[f].short)
		}
	}
	if f, ok := faultKey(runeKey('2')); !ok || f != fClip {
		t.Fatal("2")
	}
	// the menu has the item 8 only in emulation
	useTerm(t, 80, 25)
	fr := newMenuNav(menuItems()).view(80, 25)
	if !strings.Contains(plainFrame(fr), "8   Эмуляция") || !strings.Contains(plainFrame(fr), "1-8") {
		t.Fatal(plainFrame(fr))
	}
	centred(t, "меню эмуляции", fr, 80, "Выход")
}
