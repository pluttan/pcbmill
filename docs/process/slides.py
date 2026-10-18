"""How the desktop PCB line makes a double-sided board: rivets, needle paste, two flips by the head.

Built with deckkit (~/mnt/pr/pets/deckkit): nodes have names, edges join names,
every slide carries a `note` with its meaning; explain.py holds the side column for a reader.

    deck all docs/process/slides.py
"""
from deckkit import M, W, Slide, row
from explain import EXPLAIN  # noqa: F401  deckkit reads EXPLAIN from this module

TITLE = "Двухсторонняя плата без завода"
FOOTER = "pcbmill · двухсторонняя плата"
FOOT_CAT = 1150


def bullets(s, x, y, items, width, size=18):
    """A vertical list with teal dots; returns the y under the last item."""
    for t in items:
        s.dot(x + 6, y + 9, 4, "teal")
        y = s.para(x + 24, y + 9, t, width - 24, size, "ink") + 10
    return y


def rivet_section(s, x, y):
    """Decorative cross-section: board with copper on both sides and a flared rivet in the hole."""
    s.raw(lambda t: (
        f'<rect x="{x}" y="{y}" width="460" height="64" rx="4" fill="none" stroke="{t["soft"]}" stroke-width="1.5"/>'
        f'<line x1="{x}" y1="{y - 3}" x2="{x + 200}" y2="{y - 3}" stroke="{t["accent"]}" stroke-width="5"/>'
        f'<line x1="{x + 260}" y1="{y - 3}" x2="{x + 460}" y2="{y - 3}" stroke="{t["accent"]}" stroke-width="5"/>'
        f'<line x1="{x}" y1="{y + 67}" x2="{x + 200}" y2="{y + 67}" stroke="{t["accent"]}" stroke-width="5"/>'
        f'<line x1="{x + 260}" y1="{y + 67}" x2="{x + 460}" y2="{y + 67}" stroke="{t["accent"]}" stroke-width="5"/>'
        f'<polyline points="{x + 170},{y - 12} {x + 214},{y - 12} {x + 214},{y + 70} {x + 180},{y + 84}" '
        f'fill="none" stroke="{t["teal"]}" stroke-width="5" stroke-linejoin="round"/>'
        f'<polyline points="{x + 290},{y - 12} {x + 246},{y - 12} {x + 246},{y + 70} {x + 280},{y + 84}" '
        f'fill="none" stroke="{t["teal"]}" stroke-width="5" stroke-linejoin="round"/>'), "back")
    s.text(x + 230, y + 122, "заклёпка в отверстии: буртик сверху, раскрытый хвост снизу", 14, "muted", "middle")


def title():
    s = Slide(None, None, note="title: a double-sided board on our own machine: rivets instead of plating, "
                               "needle paste, two flips by the head")
    s.eyebrow(M, 228, "pcbmill · устройство станка · 18.10.2026", "accent")
    s.text(M, 300, "Двухсторонняя плата", 64)
    s.text(M, 370, "без завода", 64)
    s.para(M, 430, "Заклёпки вместо металлизации, паста иглой, два переворота головой станка.", 520, 24, "muted")
    s.text(M, 640, "Числа в презентации оценки, кроме отмеченных цен.", 15, "soft")
    rivet_section(s, 720, 330)
    return s


def order():
    s = Slide("порядок", "Весь цикл платы",
              "Заклёпки встают после обеих сторон. После пасты плату больше не переворачивают.",
              note="the full order: side A, flip 1, side B, rivets, flip 2, height map on pads, paste, "
                   "flaring into the paste, placement, hot air")
    top = ["sidea", "flip1", "sideb", "rivets"]
    row(s, top, M, 236, 260, 104, 32, lambda i: "focal" if i % 2 else ("load" if i == 3 else "plain"),
        ["Сторона A", "Переворот 1", "Сторона B", "Заклёпки"],
        ["карта, изоляция, маска", "рамкой с головы", "карта, изоляция, маска", "сверловка и вставка"], size=21)
    low = ["flip2", "map", "paste", "flare", "place", "heat"]
    row(s, low, M, 436, 167, 104, 30, lambda i: "focal" if i == 0 else "plain",
        ["Переворот 2", "Карта", "Паста", "Развальцовка", "Сборка", "Фен"],
        ["хвосты вверх", "по площадкам", "игла 0.4 мм", "в пасту", "SMD и ноги", "сверху"], size=18)
    s.edge("rivets", "flip2", via=[(1070, 388), (148, 388)])
    s.para(M, 584, "Детали ставятся на сторону A. Сторона B получает только дорожки и маску.", W - 2 * M, 18, "ink")
    return s


def rivets():
    s = Slide("заклёпки", "Заклёпки вместо металлизации",
              "Медная трубка с буртиком соединяет стороны. Китайские заклёпки стоят меньше рубля.",
              note="rivet sizes: 0.4/0.6 for vias (hole 0.7), M0.9 fits the 1 mm mill hole, 1.0+ inner for headers; "
                   "prices: China 0.25-0.4 rub, Europe 5-7 rub per piece")
    sizes = [("small", "0.4 / 0.6 мм", "самая мелкая, отверстие 0.7 мм", "focal"),
             ("mid", "M0.9", "под отверстие 1.0 мм от фрезы 1 мм", "plain"),
             ("big", "от 1.0 мм внутри", "ноги гребёнок 2.54 и разъёмов", "plain")]
    for i, (nid, big, sub, kind) in enumerate(sizes):
        s.node(nid, M + i * 392, 232, 368, 120, kind, big, sub, size=30, sub_size=16)
    prices = [("cn", "0.25–0.4 ₽", "за штуку: 1688, Ozon, JD", "load"),
              ("eu", "5–7 ₽", "за штуку: Fortex, wegstr", "plain"),
              ("rule", "+0.1 мм", "отверстие шире заклёпки", "plain")]
    for i, (nid, big, sub, kind) in enumerate(prices):
        s.node(nid, M + i * 392, 392, 368, 120, kind, big, sub, size=30, sub_size=16)
    s.para(M, 560, "Цены найдены 16.10.2026 и у продавцов не подтверждены. Заклёпок мельче 0.4 мм в продаже нет.",
           W - 2 * M, 17)
    return s


def setting():
    s = Slide("заклёпки", "Как станок ставит заклёпку",
              "Хвост раскрывается сверху, прямо в пасту. Шпиндель не крутится, давит ось Z.",
              note="setting a rivet: press-fit from side B, flip 2 brings the tail up, paste, cone punch flares, "
                   "flat punch presses the rim; the flange below rests on the plate as the anvil")
    row(s, ["insert", "turn", "dot", "cone", "flat"], M, 250, 208, 120, 28,
        lambda i: "focal" if i in (3, 4) else "plain",
        ["Вставка", "Переворот 2", "Паста", "Конус", "Плоский"],
        ["со стороны B, с натягом", "хвост вверх", "вокруг хвоста", "раскрывает хвост", "прижимает край"], size=21)
    bullets(s, M, 430, [
        "Отверстие режется впритык к заклёпке, с точностью 0.02 мм. Иначе заклёпка выпадет при перевороте.",
        "Буртик снизу лежит на пластине и служит упором при развальцовке.",
        "Пуансоны лежат в магазине как фрезы. Пасту с них снимает станция чистки.",
    ], W - 2 * M)
    return s


def paste():
    s = Slide("паста", "Паста иглой 0.4 мм",
              "Трафарет не нужен. Мотор давит поршень шприца на точный объём.",
              note="needle paste: 0.4 mm needle, dot 0.6-0.8 mm, type 4-5 dispensing paste, gap 0.2-0.4 mm from "
                   "the height map, positive displacement dispenser, cleaning station, fridge")
    facts = [("needle", "Точка 0.6–0.8 мм", "0603, 0805, SOIC 1.27 точками"),
             ("line", "Линия пасты", "0402 и шаг 0.65 мм: вдоль ряда площадок"),
             ("kind", "Паста тип 4–5", "для дозирования, металла 85–88 %"),
             ("gap", "Зазор 0.2–0.4 мм", "игла над площадкой по карте высот")]
    for i, (nid, t, sub) in enumerate(facts):
        s.node(nid, M + (i % 2) * 584, 228 + (i // 2) * 124, 560, 104, "plain", t, sub, size=22, sub_size=16)
    s.node("disp", M, 488, 560, 96, "focal", "Дозатор вытеснения", "как у Voltera: шаговый мотор и поршень",
           size=22, sub_size=16)
    s.node("care", 648, 488, 560, 96, "plain", "Чистка и холод", "станция чистки иглы, шприц при 2–10 °C",
           size=22, sub_size=16)
    return s


def heightmap():
    s = Slide("паста", "Карта высот после заклёпок",
              "Щуп чувствует только металл. Поэтому карта снимается там, куда ляжет паста.",
              note="height map after rivets: the touch probe cannot feel through the mask, so it measures pads in "
                   "mask windows and rivet flanges; the two test probes do it, one point per 5-10 mm")
    s.node("pads", M, 232, 420, 96, "plain", "Площадки в окнах", "голая медь, точка нанесения пасты", size=21)
    s.node("tops", M, 352, 420, 96, "plain", "Края заклёпок", "металл, высота над платой известна", size=21)
    s.node("mask", M, 472, 420, 96, "drop", "Маска", "щуп сквозь неё не чувствует", size=21)
    s.node("map", 760, 292, 456, 156, "focal", "Новая карта высот", "щупы проверки, точка на 5–10 мм",
           size=26, sub_size=16)
    s.edge("pads", "map", "accent", width=2)
    s.edge("tops", "map", "accent", width=2)
    return s


def heat():
    s = Slide("пайка", "Только фен сверху",
              "Нагрев стола даёт больше трудностей, чем пользы. Мелкая медная заклёпка прогревается сверху сама.",
              note="reflow by hot air only: a 20-30 mm spot moved over zones, no heated table; large copper needs "
                   "thermal reliefs and a slow ramp; the textolite plate stands the heat")
    s.node("fen", M, 236, 560, 120, "focal", "Фен на голове", "пятно 20–30 мм, участок за участком",
           size=24, sub_size=16)
    s.node("bed", 656, 236, 560, 120, "drop", "Нагрев стола", "не нужен", size=24, sub_size=16)
    bullets(s, M, 412, [
        "Земляная заливка уводит тепло. Нужны тепловые разгрузки и медленный подъём температуры.",
        "Стык двух участков нагрева программа ставит мимо деталей.",
        "Пластина под платой из текстолита выдерживает нагрев пайки.",
    ], W - 2 * M)
    return s


def table():
    s = Slide("стол", "Стол 510×510 и магазин",
              "Плита стола не режется. Сверху лежит сменная пластина, её станок режет и выравнивает сам.",
              note="table: aluminium vacuum plate 510x510 that is never cut, a G10 replaceable plate on top "
                   "(consumable with our logo), tool magazine strip 60-80 mm, travel about 600 x 510 x 300 mm")
    s.node("plate", M, 228, 352, 352, "focal", "Пластина G10", "510 × 510 мм", size=22, sub_size=16)
    s.node("mag", M + 368, 228, 72, 352, "store", "ER", size=16)
    s.text(M + 404, 604, "магазин", 14, "muted", "middle")
    items = [("base", "Плита из алюминия", "вакуумные каналы, не режется"),
             ("cons", "Пластина как расходник", "с логотипом, меняется за 5 с"),
             ("face", "Выравнивание фрезой", "станок снимает 0.2–0.3 мм сам"),
             ("travel", "Ход 600 × 510 × 300 мм", "X, Y и Z; Z для переворота")]
    for i, (nid, t, sub) in enumerate(items):
        s.node(nid, 584, 228 + i * 92, 632, 76, "load" if nid == "cons" else "plain", t, sub, size=20, sub_size=15)
    return s


def flipper():
    s = Slide("переворот", "Переворот рамкой с головы",
              "Два клина входят под борта платы на 5 мм. Рамка держит лист с двух концов, как вертел.",
              note="flipper: a U-frame on the head with two wedges 5 x 100 x 1 mm under the 5 mm sacrificial borders; "
                   "one vacuum valve; lift by half the board width plus 20 mm, up to 270 mm for 500x500")
    row(s, ["wedge", "vacoff", "lift", "rotate", "lower"], M, 236, 208, 120, 28,
        lambda i: "focal" if i == 3 else "plain",
        ["Клинья", "Вакуум выкл", "Подъём", "Поворот 180°", "Опускание"],
        ["под борта, вакуум вкл", "одним клапаном", "на половину ширины", "вокруг оси рамки", "клинья выходят"],
        size=20)
    facts = [("frame", "Рамка 520 мм", "клинья 5 × 100 × 1 мм"),
             ("border", "Борт 5 мм", "жертвенный, в нём реперы"),
             ("height", "До 270 мм", "подъём только для 500 × 500")]
    for i, (nid, t, sub) in enumerate(facts):
        s.node(nid, M + i * 392, 420, 368, 104, "plain", t, sub, size=22, sub_size=16)
    s.para(M, 576, "Рамка не помещается в магазин и стоит вдоль задней стенки.", W - 2 * M, 17)
    return s


def register():
    s = Slide("привязка", "Из любого положения платы",
              "Координаты задания хранятся относительно платы. После каждого шага станок находит её заново.",
              note="registration: the camera finds the blank, three asymmetric through fiducial holes in the border, "
                   "camera and probe refine to 0.01-0.02 mm, the program is recomputed for the new position")
    row(s, ["find", "fid", "fine", "recalc"], M, 236, 260, 120, 32, lambda i: "focal" if i == 1 else "plain",
        ["Камера", "Три репера", "Уточнение", "Пересчёт"],
        ["находит края листа", "насквозь, несимметрично", "камера и щуп, 0.02 мм", "программа под плату"], size=21)
    s.text(M, 432, "Когда станок находит плату заново", 20, "ink")
    bullets(s, M, 456, [
        "после каждого переворота: реперы видны с обеих сторон;",
        "после развальцовки: пуансон может сдвинуть плату;",
        "после долгой работы: от нагрева плата и стол меняют размер.",
    ], W - 2 * M)
    return s


def rules():
    s = Slide("разводка", "Правила для платы",
              "Станок требует от разводки пять вещей.",
              note="board design rules for the line: 5 mm border, parts on side A, 2 mm pads with teardrops, "
                   "mask window wider than the rivet flange, two rivet sizes")
    items = [("rb", "Борт 5 мм", "под клинья и реперы"),
             ("ra", "Детали на стороне A", "паста и сборка идут там"),
             ("rp", "Площадки от 2 мм", "teardrop у входа дорожки"),
             ("rw", "Окно маски от 1.0 мм", "шире буртика заклёпки"),
             ("rr", "Две заклёпки", "0.4 / 0.6 и от 1.0 мм внутри")]
    for i, (nid, t, sub) in enumerate(items):
        s.node(nid, M + (i % 2) * 584, 224 + (i // 2) * 116, 560, 96, "plain", t, sub, size=22, sub_size=16)
    return s


def checks():
    s = Slide("дальше", "Что проверить опытом",
              "Схема держится на шести числах, которых пока нет.",
              note="open questions to test: M0.9 inner diameter, flaring force, board on flanges without pockets, "
                   "paste storage, hot air on large copper, Z 300 mm stiffness")
    items = [("q1", "Диаметр M0.9 внутри", "набор с Ozon, штангенциркуль"),
             ("q2", "Сила развальцовки", "от неё винт Z и рама"),
             ("q3", "Плата на буртиках", "без карманов, зазор 0.1 мм"),
             ("q4", "Паста без холода", "или холодная камера внутри"),
             ("q5", "Фен на заливке", "профиль на большой меди"),
             ("q6", "Жёсткость Z 300 мм", "0.02 мм у стола")]
    for i, (nid, t, sub) in enumerate(items):
        s.node(nid, M + (i % 2) * 584, 224 + (i // 2) * 116, 560, 96, "focal" if i == 0 else "plain", t, sub,
               size=22, sub_size=16)
    return s


SLIDES = [title, order, rivets, setting, paste, heightmap, heat, table, flipper, register, rules, checks]
