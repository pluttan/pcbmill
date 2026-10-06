// Правила проектирования плат под наш станок: одна таблица, мм и милы.
// Сборка: make rules (typst compile docs/pcb-rules.typ docs/pcb-rules.pdf).
// Почему такие числа: docs/pcb-rules.md.

#set document(title: "Правила проектирования плат")
#set page(paper: "a4", margin: (x: 14mm, y: 12mm))
#set text(font: "PT Sans", lang: "ru", size: 10pt)
#set table(stroke: 0.4pt + luma(170), inset: (x: 6pt, y: 4.5pt))

// 1 mil = 0.0254 mm: one decimal, whole mils from 100 up
#let mil(x) = {
  let m = x / 0.0254
  if m < 100 { str(calc.round(m, digits: 1)) } else { str(int(calc.round(m))) }
}
// mm as written in the rules: 1 is 1.0, 84 stays 84
#let mm(x) = if x == calc.floor(x) and x < 10 { str(x) + ".0" } else { str(x) }
// a value: mm and mil cells; none: empty; a string: a word over both
#let cells(x) = if x == none { ([], []) } else if type(x) == str { (table.cell(colspan: 2, align: center, x),) } else if type(x) == array {
  ([#x.map(mm).join("×")], [#x.map(mil).join("×")])
} else { ([#mm(x)], [#mil(x)]) }
#let row(name, lo, rec) = (name, ..cells(lo), ..cells(rec))
#let group(name) = (table.cell(colspan: 5, fill: luma(232), text(weight: "bold", name)),)

#align(center, text(size: 15pt, weight: "bold")[Правила проектирования плат под наш станок])
#align(center, text(fill: luma(90))[один слой меди · отверстия без металлизации · 1 mil = 0.0254 мм])
#v(4pt)

#table(
  columns: (1fr, 48pt, 50pt, 62pt, 84pt),
  align: (left, right, right, right, right),
  table.header(
    table.cell(rowspan: 2, align: horizon, text(weight: "bold")[Параметр]),
    table.cell(colspan: 2, align: center, text(weight: "bold")[Минимум]),
    table.cell(colspan: 2, align: center, text(weight: "bold")[Рекомендуется]),
    [мм], [mil], [мм], [mil],
  ),
  ..group("Медь"),
  ..row([Зазор медь – медь], 0.3, 0.4),
  ..row([Зазор заливки до чужой меди], 0.4, 0.5),
  ..row([Ширина сигнальной дорожки], 0.3, 0.4),
  ..row([Ширина дорожки питания], 0.8, 1.0),
  ..row([Перемычка тепловой разгрузки], 0.4, 0.5),
  ..row([Медь до края платы], 1.0, 1.5),
  ..row([Медь до края головки винта M3], 1.0, none),
  ..group("Отверстия"),
  ..row([Диаметр отверстия], 0.8, none),
  ..row([Резисторы, диоды, светодиоды, DIP], none, 0.8),
  ..row([Штыревые разъёмы], none, 1.0),
  ..row([Клеммники, мощные выводы], none, 1.2),
  ..row([Крепёж M3], none, 3.2),
  ..row([Между краями отверстий], 0.8, 1.0),
  ..row([Шаг диаметров (округление)], 0.1, none),
  ..group("Площадки"),
  ..row([Поясок круглой площадки], 0.5, 0.6),
  ..row([Поясок овала по узкой стороне], 0.25, none),
  ..row([Овал под отверстие 1.0], none, (1.5, 2.2)),
  ..row([Окно маски шире площадки, на сторону], 0.05, 0.1),
  ..group("SMD"),
  ..row([Шаг выводов], 0.65, 1.27),
  ..row([Ширина площадки при шаге 0.65], none, 0.35),
  ..row([Пассивные корпуса], none, "0805, 1206"),
  ..row([QFN, BGA, шаг 0.5], none, "нельзя"),
  ..group("Надписи медью"),
  ..row([Высота букв], 2.0, 2.5),
  ..row([Толщина линии], 0.3, 0.4),
  ..group("Плата"),
  ..row([Слоёв меди], none, "1"),
  ..row([Переходные отверстия], none, "нет"),
  ..row([Проволочные перемычки на плату], none, "до 5"),
  ..row([Форма контура], none, "прямоугольник"),
  ..row([Плата на заготовке 100×100 мм], none, (84, 84)),
  ..row([Отступ от края заготовки], none, 8),
  ..row([Между платами на заготовке], 3, none),
  ..row([Наибольшая заготовка], none, (600, 600)),
)

#v(4pt)
#text(size: 9pt, fill: luma(90))[DRC KiCad: `docs/pcbmill.kicad_dru` рядом с платой как `<имя платы>.kicad_dru`.]
