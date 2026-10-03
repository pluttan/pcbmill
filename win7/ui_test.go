package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeTerm plays back keys and keeps every frame; a frame bigger than the
// window fails the test.
type fakeTerm struct {
	t      *testing.T
	w, h   int
	keys   []Key
	frames [][]string
	idle   int
	flush  int
}

func (f *fakeTerm) Size() (int, int) { return f.w, f.h }

func (f *fakeTerm) Draw(fr []string) {
	f.t.Helper()
	if len(fr) > f.h {
		f.t.Errorf("кадр %d строк > %d", len(fr), f.h)
	}
	onScreen(f.t, "кадр", fr, f.w)
	f.frames = append(f.frames, fr)
}

func (f *fakeTerm) Key(wait time.Duration) (Key, bool) {
	if len(f.keys) == 0 {
		if f.idle++; f.idle > 200 || wait < 0 {
			f.t.Fatal("клавиши кончились, а экран ещё ждёт")
		}
		return Key{}, false
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, true
}

func (f *fakeTerm) Flush() { f.flush++ }

func (f *fakeTerm) last() []string { return f.frames[len(f.frames)-1] }

// useTerm swaps in a fake console for one test.
func useTerm(t *testing.T, w, h int, keys ...Key) *fakeTerm {
	f := &fakeTerm{t: t, w: w, h: h, keys: keys}
	old, oldStatus, oldCur, oldBack := term, statusFn, cur, backdrop
	term = f
	statusFn = func() Status { return Status{Port: "COM5", MCU: mcuOpen, Nc: true} }
	t.Cleanup(func() { term, statusFn, cur, backdrop = old, oldStatus, oldCur, oldBack })
	return f
}

func k(c KeyCode) Key { return Key{Code: c} }

func runes(s string) []Key {
	var out []Key
	for _, r := range s {
		out = append(out, runeKey(r))
	}
	return out
}

func keys(ks ...interface{}) []Key {
	var out []Key
	for _, x := range ks {
		switch v := x.(type) {
		case KeyCode:
			out = append(out, k(v))
		case string:
			out = append(out, runes(v)...)
		}
	}
	return out
}

var sizes = [][2]int{{80, 25}, {100, 40}, {60, 20}}

func plainFrame(fr []string) string { return strip(strings.Join(fr, "\n")) }

// centred: the block between the first and the last non-empty body row has
// the same margins left and right, top and bottom (one cell of slack).
func centred(t *testing.T, what string, fr []string, w int, marker string) {
	t.Helper()
	top, bottom, left, right := -1, -1, w, w
	head := len(topLines("", w))
	for i, l := range fr[head : len(fr)-1] { // no head bar and status line, no key bar
		s := strings.TrimRight(strip(l), " ")
		if s == "" {
			continue
		}
		if top < 0 {
			top = i
		}
		bottom = i
		lead := len([]rune(s)) - len([]rune(strings.TrimLeft(s, " ")))
		if lead < left {
			left = lead
		}
		if r := w - len([]rune(s)); r < right {
			right = r
		}
	}
	rows := len(fr) - head - 1
	if d := top - (rows - 1 - bottom); d > 1 || d < -1 {
		t.Errorf("%s: не по центру по высоте: сверху %d, снизу %d\n%s", what, top, rows-1-bottom, plainFrame(fr))
	}
	if d := left - right; d > 1 || d < -1 {
		t.Errorf("%s: не по центру по ширине: слева %d, справа %d\n%s", what, left, right, plainFrame(fr))
	}
	if marker != "" && !strings.Contains(plainFrame(fr), marker) {
		t.Errorf("%s: нет %q", what, marker)
	}
}

// selected: the line with s is drawn on the selection bar.
func selected(t *testing.T, fr []string, s string) {
	t.Helper()
	for _, l := range fr {
		if strings.Contains(strip(l), s) {
			if !strings.Contains(l, bgInk(cSelBg)) {
				t.Errorf("%q не подсвечено: %q", s, l)
			}
			return
		}
	}
	t.Errorf("нет строки %q", s)
}

// ==========================
// ===  Keys              ===
// ==========================

func TestParseKeys(t *testing.T) {
	cases := map[string]Key{
		"\x1b[A": k(KeyUp), "\x1b[B": k(KeyDown), "\x1b[C": k(KeyRight), "\x1b[D": k(KeyLeft),
		"\x1bOA": k(KeyUp), "\x1b[1;5C": k(KeyRight), "\x1b[5~": k(KeyPgUp), "\x1b[6~": k(KeyPgDn),
		"\x1b[H": k(KeyHome), "\x1b[F": k(KeyEnd), "\x1b[1~": k(KeyHome), "\x1b[4~": k(KeyEnd),
		"\x1bOP": k(KeyF(1)), "\x1b[24~": k(KeyF(12)), "\x1b[Z": k(KeyBackTab), "\x1b[3~": k(KeyDelete),
		"\r": k(KeyEnter), "\n": k(KeyEnter), "\x7f": k(KeyBackspace), "\t": k(KeyTab),
		"д": runeKey('д'), "x": runeKey('x'),
	}
	for in, want := range cases {
		got, n := parseKey([]byte(in))
		if got != want || n != len(in) {
			t.Errorf("%q: %+v, %d байт", in, got, n)
		}
	}
	for _, part := range []string{"\x1b", "\x1b[", "\x1b[1;5", "\xd0"} {
		if _, n := parseKey([]byte(part)); n != 0 {
			t.Errorf("%q: неполная последовательность съедена", part)
		}
	}
}

func TestReadBytes(t *testing.T) {
	out := make(chan Key, 32)
	go readBytes(bytes.NewReader([]byte("\x1b[A\x1b[B\rн\x1b")), out)
	want := []Key{k(KeyUp), k(KeyDown), k(KeyEnter), runeKey('н'), k(KeyEsc), k(KeyCtrlC)}
	for i, w := range want {
		select {
		case got := <-out:
			if got != w {
				t.Fatalf("клавиша %d: %+v, ждали %+v", i, got, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("клавиша %d не пришла", i)
		}
	}
}

func TestField(t *testing.T) {
	f := newField("12.5")
	for _, x := range keys(KeyLeft, KeyLeft, KeyBackspace, "7", KeyEnd, "0", KeyHome, KeyDelete) {
		f.key(x)
	}
	if f.String() != "7.50" {
		t.Fatal(f.String())
	}
	if done, _ := f.key(k(KeyEnter)); !done {
		t.Fatal("Enter не закончил ввод")
	}
	if _, cancel := f.key(k(KeyEsc)); !cancel {
		t.Fatal("Esc не отменил")
	}
	if v := f.view(5); vlen(v) != 5 || !strings.Contains(v, bgInk(cSelBg)) {
		t.Fatalf("курсор не виден: %q", v)
	}
}

// ==========================
// ===  Main menu         ===
// ==========================

func TestMenuKeys(t *testing.T) {
	for _, c := range []struct {
		keys []Key
		want string
	}{
		{keys(KeyDown, KeyDown, KeyEnter), "3"},
		{keys(KeyUp, KeyEnter), "0"},                   // over the top edge to the bottom
		{keys(KeyUp, KeyDown, KeyDown, KeyEnter), "2"}, // and back over the bottom edge
		{keys("5"), "5"},
		{keys(KeyDown, "0"), "0"},
		{keys(KeyEsc), "0"},
		{keys(KeyEnd, KeyEnter), "0"},
		{keys(KeyDown, KeyHome, KeyEnter), "1"},
		{keys("x", KeyDown, KeyDown, KeyDown, KeyDown, KeyDown, KeyDown, KeyEnter), "7"},
	} {
		useTerm(t, 80, 25, c.keys...)
		if got := menuChoice(newMenuNav(mainMenu)); got != c.want {
			t.Errorf("%v: пункт %q, ждали %q", c.keys, got, c.want)
		}
	}
}

func TestMenuScreen(t *testing.T) {
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		m := newMenuNav(mainMenu)
		m.move(1)
		m.move(1)
		fr := m.view(sz[0], sz[1])
		if len(fr) != sz[1] {
			t.Fatalf("%v: %d строк", sz, len(fr))
		}
		onScreen(t, "меню", fr, sz[0])
		centred(t, "меню", fr, sz[0], "Снять карту высот")
		selected(t, fr, "► 3   Снять карту высот")
		all := plainFrame(fr)
		for _, want := range []string{"Выход", "Найти координаты", "Esc", "МК COM5", "сетка точек"} {
			if !strings.Contains(all, want) {
				t.Errorf("%v: нет %q\n%s", sz, want, all)
			}
		}
	}
}

// ==========================
// ===  Dialogs, list     ===
// ==========================

func TestConfirm(t *testing.T) {
	for _, c := range []struct {
		keys []Key
		want bool
	}{
		{keys(KeyEnter), false}, // Нет at first
		{keys(KeyRight, KeyEnter), false},
		{keys(KeyEsc), false},
		{keys(KeyLeft, KeyEnter), true},
		{keys(KeyTab, KeyEnter), true},
		{keys(KeyLeft, KeyEsc), false},
		{keys("д"), true}, {keys("Д"), true}, {keys("y"), true}, {keys("l"), true},
		{keys("н"), false}, {keys("n"), false}, {keys("т"), false},
		{keys("x", "?", KeyLeft, KeyEnter), true},
	} {
		f := useTerm(t, 80, 25, c.keys...)
		backdrop = newMenuNav(mainMenu).view
		if got := confirm("Z обнулён, начать обход?"); got != c.want {
			t.Errorf("%v: %v", c.keys, got)
		}
		all := plainFrame(f.frames[0])
		if !strings.Contains(all, "  Да   ") || !strings.Contains(all, "► Нет ◄") || !strings.Contains(all, "Меню") {
			t.Fatalf("диалог не поверх меню:\n%s", all)
		}
		selected(t, f.frames[0], "► Нет ◄")
	}
	for _, sz := range sizes {
		f := useTerm(t, sz[0], sz[1], k(KeyEnter))
		backdrop = newMenuNav(mainMenu).view
		alert(errors.New("остановлено: окно NcStudio потеряло фокус (щелчок мышью или другое окно)"))
		fr := f.last()
		if len(fr) != sz[1] || !strings.Contains(plainFrame(fr), "ОСТАНОВ") || !strings.Contains(plainFrame(fr), "► OK ◄") {
			t.Fatalf("%v:\n%s", sz, plainFrame(fr))
		}
		if !strings.Contains(strings.Join(fr, ""), ink(cAlarm)+outer.tl) {
			t.Error("рамка ОСТАНОВ не красная")
		}
	}
}

func TestPickList(t *testing.T) {
	items := []string{"plata.tap", "calib.tap", "test.nc"}
	useTerm(t, 80, 25, keys(KeyDown, KeyDown, KeyDown, KeyDown, KeyEnter)...)
	if i, ok := pick("Программы", items); !ok || i != 1 {
		t.Fatal(i, ok)
	}
	useTerm(t, 80, 25, keys(KeyEsc)...)
	if _, ok := pick("Программы", items); ok {
		t.Fatal("Esc не вернул назад")
	}
	f := useTerm(t, 60, 20, keys("3")...)
	if i, _ := pick("Программы", items); i != 2 {
		t.Fatal(i)
	}
	centred(t, "список", f.frames[0], 60, "plata.tap")
	selected(t, f.frames[0], "plata.tap")
	// a single program is still shown and taken with Enter
	f = useTerm(t, 80, 25, keys(KeyEnter)...)
	if i, ok := pick("Программы", items[:1]); !ok || i != 0 || len(f.frames) == 0 {
		t.Fatal(i, ok)
	}
}

func TestPage(t *testing.T) {
	for _, sz := range sizes {
		useTerm(t, sz[0], sz[1])
		p := newPage(menuTitle("2"), "Фреза не ниже 5 мм над заготовкой.", sep, "вторая строка")
		for i := 0; i < 30; i++ {
			sayOK("строка журнала номер " + string(rune('A'+i)))
		}
		sayStop(errors.New("остановлено"))
		fr := p.view(sz[0], sz[1])
		onScreen(t, "страница", fr, sz[0])
		if len(fr) != sz[1] || !strings.Contains(plainFrame(fr), "ОСТАНОВ: остановлено") {
			t.Fatalf("%v: последняя строка журнала не видна:\n%s", sz, plainFrame(fr))
		}
		if strings.Contains(plainFrame(fr), "номер A") {
			t.Errorf("%v: журнал не прокручен к концу", sz)
		}
	}
}
