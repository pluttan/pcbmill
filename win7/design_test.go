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

// The whole program is one frame the size of the window: the screen in the top
// edge, the keys in the bottom edge, nothing outside it.
func TestOuterFrame(t *testing.T) {
	for _, sz := range [][2]int{{40, 12}, {59, 20}, {80, 25}, {100, 40}} {
		w, h := sz[0], sz[1]
		hint := hints(w, "Enter", "открыть", "Esc", "назад")
		fr := screen(w, h, "Снять карту высот", []string{"тело"}, hint)
		onScreen(t, "рамка", fr, w)
		if len(fr) != h {
			t.Fatalf("%dx%d: строк %d", w, h, len(fr))
		}
		first, last := strip(fr[0]), strip(fr[h-1])
		if !strings.HasPrefix(first, outer.tl) || !strings.HasSuffix(first, outer.tr) ||
			!strings.HasPrefix(last, outer.bl) || !strings.HasSuffix(last, outer.br) {
			t.Fatalf("%dx%d: углы: %q / %q", w, h, first, last)
		}
		for _, l := range fr[1 : h-1] {
			p := strip(l)
			if !strings.HasPrefix(p, outer.v) || !strings.HasSuffix(p, outer.v) {
				t.Fatalf("%dx%d: бок рамки: %q", w, h, p)
			}
		}
		if !strings.Contains(first, "pcbscan") || !strings.Contains(last, "Enter") {
			t.Fatalf("%dx%d: кромки: %q / %q", w, h, first, last)
		}
		if strings.Contains(strings.Join(fr, ""), bgInk(cBarBg)) {
			t.Fatalf("%dx%d: остались полосы", w, h)
		}
	}
	if strings.Contains(strip(frameTop("x", 79)), "ЭМУЛЯЦИЯ") {
		t.Fatal("метка эмуляции без эмуляции")
	}
	withSim(t)
	if h := frameTop("x", 79); !strings.Contains(strip(h), "ЭМУЛЯЦИЯ") || vlen(h) != 79 {
		t.Fatalf("нет метки эмуляции: %q", strip(h))
	}
}

// A selected line is a bar across the whole panel, margins too.
func TestSelectionBar(t *testing.T) {
	b := box(thin, "Меню", []string{"  1   один", sel("► 2   два")}, 40)
	line := b[2]
	in := strings.TrimSuffix(strings.TrimPrefix(line, " "), " ")
	if !strings.HasPrefix(in, bgInk(cSelBg)) || vlen(in) != 38 {
		t.Fatalf("полоса не во всю ширину: %q", line)
	}
	if !strings.HasPrefix(strip(in), "  ► 2   два") {
		t.Fatalf("отступ не тот: %q", strip(in))
	}
	if strings.ContainsAny(strip(strings.Join(b, "")), "┌┐└┘│") {
		t.Fatalf("у панели внутри общей рамки своя рамка: %q", b)
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
		if strings.Contains(strip(l), outer.tl) && i > 0 {
			top = i
		}
	}
	if top < 0 {
		t.Fatal("нет диалога")
	}
	right := []rune(strip(fr[top+1]))
	x := strings.LastIndex(strip(fr[top]), outer.tr)
	x = len([]rune(strip(fr[top])[:x]))
	if string(right[x+1:x+3]) != "░░" {
		t.Fatalf("нет тени справа: %q", strip(fr[top+1]))
	}
	if !strings.Contains(strip(fr[top+len(b)]), repeat("░", 20)) {
		t.Fatalf("нет тени снизу: %q", strip(fr[top+len(b)]))
	}
	// under the dialog the menu is grey: no colour but the dim one (and the
	// white outer frame at the sides) outside the box
	for _, l := range fr[1 : top-1] {
		for i := 0; i+1 < len(l); i++ {
			if l[i] == esc && l[i+1] != '0' && l[i+1] != byte('A'+cDark) && l[i+1] != byte('A'+cFrame) {
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
