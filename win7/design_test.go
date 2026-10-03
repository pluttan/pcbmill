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
		// the frame: the first and the last line that are not empty
		a, b := 0, h-1
		for a < h && strings.TrimSpace(strip(fr[a])) == "" {
			a++
		}
		for b > a && strings.TrimSpace(strip(fr[b])) == "" {
			b--
		}
		first, last := strings.TrimSpace(strip(fr[a])), strings.TrimSpace(strip(fr[b]))
		if !strings.HasPrefix(first, outer.tl) || !strings.HasSuffix(first, outer.tr) ||
			!strings.HasPrefix(last, outer.bl) || !strings.HasSuffix(last, outer.br) {
			t.Fatalf("%dx%d: углы: %q / %q", w, h, first, last)
		}
		for _, l := range fr[a+1 : b] {
			p := strings.TrimSpace(strip(l))
			if !strings.HasPrefix(p, outer.v) || !strings.HasSuffix(p, outer.v) {
				t.Fatalf("%dx%d: бок рамки: %q", w, h, p)
			}
		}
		// in the middle of the window, not the whole of it
		top, bottom := a, h-1-b
		if top-bottom > 1 || bottom-top > 1 {
			t.Fatalf("%dx%d: рамка не посередине по высоте: сверху %d, снизу %d", w, h, top, bottom)
		}
		lft := len([]rune(strip(fr[a]))) - len([]rune(strings.TrimLeft(strip(fr[a]), " ")))
		rgt := w - len([]rune(strings.TrimRight(strip(fr[a]), " ")))
		if lft-rgt > 1 || rgt-lft > 1 {
			t.Fatalf("%dx%d: рамка не посередине по ширине: слева %d, справа %d", w, h, lft, rgt)
		}
		if w >= 100 && (lft == 0 || top == 0) {
			t.Fatalf("%dx%d: рамка на всё окно", w, h)
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

// In emulation the top edge says so and counts the faults; X Y Z are not there.
func TestStatusSim(t *testing.T) {
	st := Status{Port: "эмуляция", MCU: mcuOpen, Nc: true,
		Sim: &SimInfo{X: 12.5, Y: -1, Z: -1.234, Moving: true, Faults: []string{"ЗАЖИМ", "Z ПОЛЗЁТ"}}}
	e := strip(edgeStatus(st))
	if !strings.Contains(e, "сбоев 2") || strings.Contains(e, "12.500") {
		t.Fatalf("кромка: %q", e)
	}
	if strings.Contains(strip(edgeStatus(Status{MCU: mcuOpen, Nc: true, Sim: &SimInfo{}})), "сбоев") {
		t.Fatal("сбои без сбоев")
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
	// the menu has the item 9 only in emulation
	useTerm(t, 80, 25)
	fr := newMenuNav(menuItems()).view(80, 25)
	if !strings.Contains(plainFrame(fr), "9   Эмуляция") || !strings.Contains(plainFrame(fr), "1-9") {
		t.Fatal(plainFrame(fr))
	}
	centred(t, "меню эмуляции", fr, 80, "Выход")
}
