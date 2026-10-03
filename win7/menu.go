package main

// The service commands under the job: their names for the head of their pages.

// menuEntry: key "" is a rule between groups.
type menuEntry struct{ key, text, about string }

var mainMenu = []menuEntry{
	{"2", "Проверить провод (МК)",
		"Касайтесь зажимом фрезы: на экране КОНТАКТ или разомкнуто и счётчик касаний. Станок не двигается."},
	{"3", "Проверить связь с NcStudio",
		"Программа откроет окно ввода команды NcStudio, поднимет Z на 1 мм и опустит обратно."},
	{"", "", ""},
	{"4", "Снять карту высот",
		"Выбор программы, сетка точек над её резом, обход с касаниями меди. Результат scan.csv."},
	{"5", "Z0 на меди после смены фрезы",
		"Одно медленное касание в первой точке карты, затем обнулить Z в NcStudio."},
	{"6", "Выровнять программу по карте",
		"Из платы.tap получится платы_level.tap по карте scan.csv; на экране карта высот."},
	{"", "", ""},
	{"7", "Ручной режим",
		"Станок двигается стрелками и клавишами, на экране касание и Z. Проверка перед обходом."},
	{"8", "Найти координаты NcStudio",
		"Ищет, откуда читать X, Y, Z NcStudio: станок сдвинется на 1 мм по осям и вернётся."},
	{"", "", ""},
	{"0", "Выход", "Закрыть pcbscan. NcStudio остаётся как есть."},
}

// simEntry is the menu item of the emulation, there only when it is on.
var simEntry = menuEntry{"9", "Эмуляция: станок и неисправности",
	"Виртуальный станок: где фреза, и неисправности для проверки защит (на экранах работы их же включают цифры 1-6)."}

// menuItems is the main menu; in emulation one more item at the end.
func menuItems() []menuEntry {
	if simM == nil {
		return mainMenu
	}
	items := append([]menuEntry(nil), mainMenu[:len(mainMenu)-1]...)
	return append(items, simEntry, menuEntry{"", "", ""}, mainMenu[len(mainMenu)-1])
}

// menuTitle is the heading of a command's page: "сервис / Ручной режим".
func menuTitle(key string) string {
	for _, it := range menuItems() {
		if it.key == key {
			return "сервис / " + it.text
		}
	}
	return key
}
