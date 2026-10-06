"""The PCB rules as a web page in the look of the Quartus mirror page.

python3 tools/pcb_rules_html.py docs/pcb-rules.json OUT_DIR
Writes OUT_DIR/index.html; the PDF and the KiCad rules go next to it (make rules).
"""

import html
import json
import sys

STYLE = """body { max-width: 760px; margin: 16px auto; padding: 0 12px; }
table { border-collapse: collapse; }
td, th { border: 1px solid #999; padding: 2px 8px; text-align: left; }
td.n { text-align: right; }
td.k { color: #666; font-size: 90%; }
td.g { background: #e8e8e8; font-weight: bold; }
td.w { text-align: center; }"""


def mil(x):
    m = x / 0.0254
    return f"{m:.1f}" if m < 100 else f"{round(m)}"


def mm(x):
    return f"{x:.1f}" if float(x).is_integer() and x < 10 else f"{x:g}"


def cells(x):
    """Two cells (mm, mil) of a value; a word spans both."""
    if x is None:
        return "<td></td><td></td>"
    if isinstance(x, str):
        return f'<td class="w" colspan="2">{html.escape(x)}</td>'
    if isinstance(x, list):
        return f'<td class="n">{"×".join(map(mm, x))}</td><td class="n">{"×".join(map(mil, x))}</td>'
    return f'<td class="n">{mm(x)}</td><td class="n">{mil(x)}</td>'


def page(groups):
    rows = []
    for g in groups:
        rows.append(f'<tr><td class="g" colspan="6">{html.escape(g["group"])}</td></tr>')
        for r in g["rows"]:
            rows.append(f'<tr><td>{html.escape(r["name"])}</td><td class="k">{html.escape(r["kicad"])}</td>'
                        f'{cells(r["min"])}{cells(r["rec"])}</tr>')
    body = "\n".join(rows)
    return f"""<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<title>Правила проектирования плат под станок</title>
<style>
{STYLE}
</style>
</head>
<body>
<h2>Правила проектирования плат под станок</h2>

<p>Для печати: <a href="pcb-rules.pdf">pcb-rules.pdf</a>.<br>
Правила DRC для KiCad: <a href="pcbmill.kicad_dru">pcbmill.kicad_dru</a>,
положить рядом с платой под именем <tt>&lt;имя платы&gt;.kicad_dru</tt>.</p>

<table>
<tr><th rowspan="2">Параметр</th><th rowspan="2">В KiCad</th><th colspan="2">Минимум</th><th colspan="2">Рекомендуется</th></tr>
<tr><th>мм</th><th>mil</th><th>мм</th><th>mil</th></tr>
{body}
</table>

</body>
</html>
"""


def main():
    src, out = sys.argv[1], sys.argv[2]
    groups = json.load(open(src, encoding="utf-8"))
    with open(f"{out}/index.html", "w", encoding="utf-8") as f:
        f.write(page(groups))


if __name__ == "__main__":
    main()
