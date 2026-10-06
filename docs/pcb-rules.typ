// Правила проектирования плат под станок: одна таблица, мм и милы.
// Числа в pcb-rules.json (из него же страница на сервере, tools/pcb_rules_html.py).
// Сборка: make rules. Почему такие числа: docs/pcb-rules.md.

#set document(title: "Правила проектирования плат под станок")
#set page(paper: "a4", margin: (x: 14mm, y: 12mm))
#set text(font: "GOST Type BU", lang: "ru", size: 10.5pt) // docs/fonts, make rules
#set table(stroke: 0.4pt + luma(170), inset: (x: 5pt, y: 4pt))

#let data = json("pcb-rules.json")

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

#align(center, text(size: 17pt)[Правила проектирования плат под станок])
#v(4pt)

#table(
  columns: (1fr, 128pt, 30pt, 34pt, 50pt, 66pt),
  align: (left + horizon, left + horizon, right + horizon, right + horizon, right + horizon, right + horizon),
  table.header(
    table.cell(rowspan: 2, align: horizon, text(weight: "bold")[Параметр]),
    table.cell(rowspan: 2, align: horizon, text(weight: "bold")[В KiCad]),
    table.cell(colspan: 2, align: center, text(weight: "bold")[Минимум]),
    table.cell(colspan: 2, align: center, text(weight: "bold")[Рекомендуется]),
    [мм], [mil], [мм], [mil],
  ),
  ..data.map(g => (
    (table.cell(colspan: 6, fill: luma(232), text(weight: "bold", g.group)),),
    g.rows.map(r => (r.name, text(size: 9pt, fill: luma(100), r.kicad), ..cells(r.min), ..cells(r.rec))),
  )).flatten(),
)

#v(4pt)
#link("https://157.22.189.111:8445/")[https://157.22.189.111:8445/]
