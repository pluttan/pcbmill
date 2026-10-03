package main

// The main screen: the logo, the menu in the middle of the window, a line
// about the chosen item under it, the status line on top.

import "time"

// menuEntry: key "" is a rule between groups.
type menuEntry struct{ key, text, about string }

var mainMenu = []menuEntry{
	{"1", "Проверить провод (МК)",
		"Касайтесь зажимом фрезы: на экране КОНТАКТ или разомкнуто и счётчик касаний. Станок не двигается."},
	{"2", "Проверить связь с NcStudio",
		"Программа откроет окно ввода команды NcStudio, поднимет Z на 1 мм и опустит обратно."},
	{"", "", ""},
	{"3", "Снять карту высот",
		"Выбор программы, сетка точек над её резом, обход с касаниями меди. Результат scan.csv."},
	{"4", "Z0 на меди после смены фрезы",
		"Одно медленное касание в первой точке карты, затем обнулить Z в NcStudio."},
	{"5", "Выровнять программу по карте",
		"Из платы.tap получится платы_level.tap по карте scan.csv; на экране карта высот."},
	{"", "", ""},
	{"6", "Ручной режим",
		"Станок двигается стрелками и клавишами, на экране касание и Z. Проверка перед обходом."},
	{"7", "Найти координаты NcStudio",
		"Ищет, откуда читать X, Y, Z NcStudio: станок сдвинется на 1 мм по осям и вернётся."},
	{"", "", ""},
	{"0", "Выход", "Закрыть pcbscan. NcStudio остаётся как есть."},
}

// simEntry is the menu item of the emulation, there only when it is on.
var simEntry = menuEntry{"8", "Эмуляция: станок и неисправности",
	"Виртуальный станок: где фреза, и неисправности для проверки защит (их же включают F2-F7)."}

// menuItems is the main menu; in emulation one more item at the end.
func menuItems() []menuEntry {
	if simM == nil {
		return mainMenu
	}
	items := append([]menuEntry(nil), mainMenu[:len(mainMenu)-1]...)
	return append(items, simEntry, menuEntry{"", "", ""}, mainMenu[len(mainMenu)-1])
}

// menuNav walks the items with the arrows; rules are skipped, the ends wrap.
type menuNav struct {
	items []menuEntry
	sel   int
}

func newMenuNav(items []menuEntry) *menuNav {
	m := &menuNav{items: items}
	for m.items[m.sel].key == "" {
		m.sel++
	}
	return m
}

func (m *menuNav) move(d int) {
	n := len(m.items)
	for i := 0; i < n; i++ {
		m.sel = (m.sel + d + n) % n
		if m.items[m.sel].key != "" {
			return
		}
	}
}

// key returns the key of the item chosen; "quit" for Ctrl+C from a pipe.
func (m *menuNav) key(k Key) (string, bool) {
	switch k.Code {
	case KeyUp, KeyBackTab:
		m.move(-1)
	case KeyDown, KeyTab:
		m.move(1)
	case KeyHome:
		m.sel = len(m.items) - 1
		m.move(1)
	case KeyEnd:
		m.sel = 0
		m.move(-1)
	case KeyEnter:
		return m.items[m.sel].key, true
	case KeyEsc:
		return "0", true
	case KeyCtrlC:
		return "quit", true
	case KeyRune:
		for i, it := range m.items {
			if it.key != "" && it.key == string(k.R) {
				m.sel = i
				return it.key, true
			}
		}
	}
	return "", false
}

// logo is the name over the menu; the short one for low windows.
func logo(w int, short bool) []string {
	if short {
		return []string{center(paint(cTitle, "pcbscan")+paint(cHint, "  карта высот для NcStudio"), w)}
	}
	return []string{
		center(paint(cTitle, "p c b s c a n"), w),
		center(paint(cHint, "карта высот платы для NcStudio"), w),
	}
}

func (m *menuNav) view(w, h int) []string {
	bw := colW(w)
	if bw > 60 {
		bw = 60
	}
	rows := room(w, h)
	var body []string
	for _, rules := range []bool{true, false} {
		var lines []string
		for i, it := range m.items {
			if it.key == "" {
				if rules {
					lines = append(lines, sep)
				}
				continue
			}
			if i == m.sel {
				lines = append(lines, sel("► "+it.key+"   "+it.text))
			} else {
				lines = append(lines, "  "+paint(cKey, it.key)+"   "+it.text)
			}
		}
		about := wrap(m.items[m.sel].about, bw-4, 0)
		if len(about) > 2 {
			about = about[:2]
		}
		for len(about) < 2 {
			about = append(about, "")
		}
		for i := range about {
			about[i] = center(paint(cHint, about[i]), bw)
		}
		menu := box(thin, "Меню", lines, bw)
		for _, short := range []bool{false, true} {
			body = stack(logo(bw, short), menu, about)
			if len(body) <= rows-2 { // a free line above and under at least
				return screen(w, h, "главное меню", body, menuHint(w, len(m.items)))
			}
		}
		if body = stack(menu, about); len(body) <= rows {
			return screen(w, h, "главное меню", body, menuHint(w, len(m.items)))
		}
	}
	return screen(w, h, "главное меню", body, menuHint(w, len(m.items)))
}

func menuHint(w, n int) string {
	last := "7"
	if n > 0 && simM != nil {
		last = "8"
	}
	return hints(w, "↑↓", "выбор", "Enter", "открыть", "1-"+last, "сразу", "Esc", "выход")
}

// menuChoice runs the main menu until an item is chosen; the status line is
// refreshed twice a second meanwhile.
func menuChoice(m *menuNav) string {
	res := ""
	run(m.view, func(k Key) bool {
		var done bool
		res, done = m.key(k)
		return done
	}, 500*time.Millisecond)
	return res
}

// menuTitle is the heading of a command's page: "3 ► Снять карту высот".
func menuTitle(key string) string {
	for _, it := range menuItems() {
		if it.key == key {
			return key + "  " + it.text
		}
	}
	return key
}
