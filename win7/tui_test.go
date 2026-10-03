package main

import (
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// cp866 is every character the Windows 7 console raster font can show with code
// page 866: the glyphs it draws for 01..1F and 7F, ASCII, then 80..FF.
var cp866 = func() map[rune]bool {
	set := map[rune]bool{}
	glyphs := "☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼" + "⌂"
	for c := rune(0x20); c < 0x7F; c++ {
		set[c] = true
	}
	high := "АБВГДЕЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ" +
		"абвгдежзийклмноп" +
		"░▒▓│┤╡╢╖╕╣║╗╝╜╛┐" +
		"└┴┬├─┼╞╟╚╔╩╦╠═╬╧" +
		"╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" +
		"рстуфхцчшщъыьэюя" +
		"ЁёЄєЇїЎў°∙·√№¤■\u00a0"
	if n := len([]rune(high)); n != 128 {
		panic("cp866 table: " + strconv.Itoa(n))
	}
	for _, r := range glyphs + high {
		set[r] = true
	}
	return set
}()

// onScreen fails on a line wider than w or a character the console cannot draw.
func onScreen(t *testing.T, what string, lines []string, w int) {
	t.Helper()
	for i, l := range lines {
		if n := vlen(l); n > w {
			t.Errorf("%s, строка %d: ширина %d > %d: %q", what, i, n, w, strip(l))
		}
		for _, r := range strip(l) {
			if !cp866[r] {
				t.Errorf("%s, строка %d: символа %q (U+%04X) нет в cp866: %q", what, i, r, r, strip(l))
			}
		}
		if strings.ContainsAny(strip(l), "\n\r\t\x00") {
			t.Errorf("%s, строка %d: управляющий символ: %q", what, i, l)
		}
	}
}

func TestMarks(t *testing.T) {
	s := "a" + paint(cAlarm, "ОСТАНОВ") + " b"
	if vlen(s) != 10 || strip(s) != "aОСТАНОВ b" {
		t.Fatal(vlen(s), strip(s))
	}
	h, rest := cutv(s, 3)
	if strip(h) != "aОС" || strip(rest) != "ТАНОВ b" {
		t.Fatal(strip(h), strip(rest))
	}
}

func TestWrap(t *testing.T) {
	s := "Перед стартом: в NcStudio ноль X0 Y0 стоит как для " + paint(cTitle, "этой платы,") +
		" шпиндель выключен, фреза на 1-2 мм над медью. Оченьдлинноесловобезпробеловкотороенепомещается"
	for _, w := range []int{10, 20, 37, 78} {
		ls := wrap(s, w, 2)
		onScreen(t, "wrap", ls, w)
		joined := strings.Join(strings.Fields(strip(strings.Join(ls, " "))), "")
		if joined != strings.Join(strings.Fields(strip(s)), "") {
			t.Fatalf("w %d: текст потерян: %q", w, joined)
		}
	}
}

// closed: every line as wide as the box, corners and sides in place.
func closed(t *testing.T, what string, ls []string, f frame, w int) {
	t.Helper()
	onScreen(t, what, ls, w)
	for i, l := range ls {
		r := []rune(strip(l))
		if len(r) != w {
			t.Fatalf("%s, строка %d: ширина %d, ждали %d: %q", what, i, len(r), w, string(r))
		}
		first, last := string(r[0]), string(r[len(r)-1])
		switch {
		case i == 0 && (first != f.tl || last != f.tr):
			t.Fatalf("%s: верх не замкнут: %q", what, string(r))
		case i == len(ls)-1 && (first != f.bl || last != f.br):
			t.Fatalf("%s: низ не замкнут: %q", what, string(r))
		case i > 0 && i < len(ls)-1 && !(first == f.v && last == f.v) && !(first == f.ml && last == f.mr):
			t.Fatalf("%s, строка %d: бок не замкнут: %q", what, i, string(r))
		}
	}
}

func TestFrames(t *testing.T) {
	long := "Сейчас ОБНУЛИТЕ Z в NcStudio на этой высоте: обход ведёт Z абсолютными командами от этого нуля."
	for _, w := range []int{30, 40, 60, 78, 110} {
		closed(t, "box", box(thin, "Перед стартом", []string{long, sep, "", paint(cHint, "коротко")}, w), thin, w)
		closed(t, "box без заголовка", box(thin, "", []string{"x"}, w), thin, w)
		closed(t, "logo", logo(w, false), thick, w)
		closed(t, "ручной", box(thin, "Команды", twoCols(newManual(opts{search: 3}, nil, nil, "").keys(), w-4), w), thin, w)
		closed(t, "сетка", newGridForm(GridFor(0.63, 61.26, 1.8, 46.05, 10)).form(), thin, formW)
		onScreen(t, "rule", rule("3 ► Карта высот", w), w)
		onScreen(t, "note", note("■", cAlarm, "ОСТАНОВ:", long, w), w)
		onScreen(t, "bar", []string{bar(3, 7, 12), bar(9, 7, 12), bar(0, 0, 12)}, 12)
	}
	// the manual-mode keys fit two columns on an 80-column console
	if l := twoCols(newManual(opts{search: 3}, nil, nil, "").keys(), 74); len(l) != 5 {
		t.Fatalf("ручной режим не в две колонки: %d строк", len(l))
	}
}

func TestStatusLine(t *testing.T) {
	for _, w := range []int{30, 50, 78} {
		for _, st := range []Status{
			{},
			{Port: "COM5", MCU: mcuOpen, Nc: true},
			{Port: "COM12", MCU: mcuContact, Nc: true, Z: -1.2345, HasZ: true,
				More: []string{"шаг Z 0.100 · быстрый 1.0 · X 1.00 · Y 1.00"}},
			{Port: "COM3", MCU: mcuDead, Z: 12, HasZ: true},
		} {
			onScreen(t, "статус", statusLine(st, w), w)
		}
	}
	all := strip(strings.Join(statusLine(Status{Port: "COM5", MCU: mcuContact, Nc: true, HasZ: true}, 78), "\n"))
	for _, want := range []string{"COM5", "КОНТАКТ", "NcStudio найдена", "Z от старта +0.000"} {
		if !strings.Contains(all, want) {
			t.Errorf("в статусе нет %q: %q", want, all)
		}
	}
	if s := strip(strings.Join(statusLine(Status{Port: "COM5", MCU: mcuDead}, 78), "")); !strings.Contains(s, "НЕ ОТВЕЧАЕТ") {
		t.Error(s)
	}
}

func testMap(t *testing.T, slope bool) *HeightMap {
	t.Helper()
	var pts []Point
	for _, p := range GridFor(0, 60, 0, 40, 10).Points() {
		z := -0.4
		if slope {
			z += 0.003*p[0] - 0.002*p[1]
		}
		pts = append(pts, Point{p[0], p[1], z})
	}
	h, err := MapFromPoints(pts)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func has(ls []string, s string) bool { return strings.Contains(strip(strings.Join(ls, "\n")), s) }

func TestGridView(t *testing.T) {
	cut := rect{0.63, 61.26, 1.8, 46.05}
	for _, w := range []int{50, 79, 110} {
		for _, g := range []Grid{
			GridFor(cut.X0, cut.X1, cut.Y0, cut.Y1, 10),
			GridFor(cut.X0-2, cut.X1+2, cut.Y0-2, cut.Y1+2, 5),
			{10, 30, 10, 30, 3, 3}, // smaller than the cut
			{5, 5, 5, 5, 1, 1},     // one point
			{0, 100, 3, 3, 11, 1},  // one row
		} {
			ls := gridView(g, &cut, w)
			onScreen(t, "сетка", ls, w)
			if !has(ls, "♦") || !has(ls, "первая точка") || !has(ls, "X, мм") || !has(ls, "Y, мм") {
				t.Fatalf("нет отметки нуля или осей:\n%s", strip(strings.Join(ls, "\n")))
			}
			if g.NX*g.NY > 1 && !has(ls, "■") {
				t.Fatal("нет точек")
			}
			if !has(ls, "рез программы") || !has(ls, "──") {
				t.Fatal("нет рамки реза")
			}
		}
		onScreen(t, "сетка без реза", gridView(GridFor(0, 60, 0, 40, 10), nil, w), w)
	}
	// the zero is where the snake starts: bottom left, on the lowest labelled row
	ls := gridView(GridFor(0, 60, 0, 40, 10), nil, 79)
	for i, l := range ls {
		if strings.Contains(l, "♦") && strings.Contains(l, "┤") {
			if !strings.HasPrefix(strings.TrimSpace(strip(l)), "0 ┤") || !strings.Contains(strip(ls[i+2]), "└") {
				t.Fatalf("ноль не внизу слева:\n%s", strip(strings.Join(ls, "\n")))
			}
		}
	}
}

func TestHeightView(t *testing.T) {
	h := testMap(t, true)
	for _, w := range []int{50, 79, 110} {
		for _, cut := range []*rect{nil, {0, 60, 0, 40}, {5, 50, 5, 30}} {
			ls := heightView(h, cut, w)
			onScreen(t, "карта", ls, w)
			for _, want := range []string{"♦", "░", "▒", "▓", "█", "мин -0.080", "макс +0.180", "перепад 0.260", "X, мм", "Y, мм"} {
				if !has(ls, want) {
					t.Fatalf("w %d: на карте нет %q:\n%s", w, want, strip(strings.Join(ls, "\n")))
				}
			}
			onScreen(t, "таблица", heightTable(h, w), w)
		}
	}
	// the slope goes up along X and down along Y: the bottom row runs from light to dark
	ls := heightView(h, nil, 79)
	row := ""
	for _, l := range ls {
		if strings.Contains(l, "♦") && strings.Contains(l, "┤") {
			row = strip(l)
		}
	}
	if !strings.Contains(row, "♦▒") || !strings.HasSuffix(strings.TrimRight(row, " "), "█") {
		t.Fatalf("уклон не виден: %q", row)
	}
	if ls := heightView(testMap(t, false), nil, 79); !has(ls, "одной высоте") {
		t.Fatal(strip(strings.Join(ls, "\n")))
	}
	// 13 columns do not fit 78: the table goes in two blocks, nothing lost
	var pts []Point
	for _, p := range GridFor(0, 60, 0, 10, 5).Points() {
		pts = append(pts, Point{p[0], p[1], p[0] / 1000})
	}
	wide, _ := MapFromPoints(pts)
	tb := heightTable(wide, 78)
	onScreen(t, "широкая таблица", tb, 78)
	if n := strings.Count(strip(strings.Join(tb, "\n")), "Y\\X"); n != 2 {
		t.Fatalf("блоков %d:\n%s", n, strip(strings.Join(tb, "\n")))
	}
}

// Every string the program can print is in cp866: string literals of all sources.
func TestSourcesCP866(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fs := token.NewFileSet()
		var s scanner.Scanner
		s.Init(fs.AddFile(f, -1, len(src)), src, nil, 0)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok != token.STRING && tok != token.CHAR {
				continue
			}
			v, err := strconv.Unquote(lit)
			if tok == token.CHAR {
				v, err = lit[1:len(lit)-1], nil
			}
			if err != nil {
				continue
			}
			for _, r := range v {
				if r >= 0x20 && !cp866[r] && r != 0x7F {
					t.Errorf("%s: символа %q (U+%04X) нет в cp866: %s", fs.Position(pos), r, r, lit)
				}
			}
		}
	}
}
