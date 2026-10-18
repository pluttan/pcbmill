"""The desktop PCB line, whole project: one machine from gerber to an assembled double-sided board.

Built with deckkit (~/mnt/pr/pets/deckkit): nodes have names, edges join names,
every slide carries a `note` with its meaning. Photos are SVG <image> from img/
(Wikimedia Commons, credits on the last slide and in img/credits.tsv).

    deck all docs/deck/slides.py
"""
import itertools

from deckkit import M, W, Slide, row
from explain import EXPLAIN  # noqa: F401  deckkit reads EXPLAIN from this module

TITLE = "Плата за час"
FOOTER = "pcbmill · настольная линия для печатных плат"
FOOT_CAT = 1150

_clip = itertools.count()


def bullets(s, x, y, items, width, size=18):
    """A vertical list with teal dots; returns the y under the last item."""
    for t in items:
        s.dot(x + 6, y + 9, 4, "teal")
        y = s.para(x + 24, y + 9, t, width - 24, size, "ink") + 10
    return y


def photo(s, x, y, w, h, name, cap=None):
    """A photo cropped into a rounded frame, the caption under it."""
    cid = f"ph{next(_clip)}"
    s.raw(lambda t: (
        f'<defs><clipPath id="{cid}"><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="12"/></clipPath></defs>'
        f'<image href="img/{name}" x="{x}" y="{y}" width="{w}" height="{h}" '
        f'preserveAspectRatio="xMidYMid slice" clip-path="url(#{cid})"/>'
        f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="12" fill="none" stroke="{t["muted"]}" stroke-width="1"/>'),
        "back")
    if cap:
        s.text(x, y + h + 20, cap, 14, "muted")


# ==========================
# ===  Why               ===
# ==========================

def title():
    s = Slide(None, None, note="title: one desktop machine takes a board from gerber to assembled; photo of a milled PCB")
    s.eyebrow(M, 228, "pcbmill · весь проект · 18.10.2026", "accent")
    s.text(M, 300, "Плата за час", 76)
    s.para(M, 368, "Один настольный станок проходит весь путь: от гербера до собранной платы.", 520, 26, "muted")
    s.text(M, 640, "Цены и сроки в презентации оценки.", 15, "soft")
    photo(s, 680, 150, 536, 402, "milledpcb.jpg", "плата, выфрезерованная на настольном станке")
    return s


def problem():
    s = Slide("зачем", "Прототип платы ждёт месяц",
              "Своё оборудование для плат в России стоит как машина или не делает и половины работы.",
              note="the pain: JLC does not ship to Russia since Feb 2026, middlemen take 20-40 days; "
                   "Carvera is ~0.95 M rub grey import; an LPKF line is several million")
    facts = [("jlc", "20–40 дней", "плата из Китая через посредника", "focal"),
             ("carvera", "0.95 млн ₽", "Makera Carvera серым импортом", "plain"),
             ("lpkf", "несколько млн ₽", "линия LPKF из нескольких машин", "plain")]
    for i, (nid, big, sub, kind) in enumerate(facts):
        s.node(nid, M + i * 392, 232, 368, 132, kind, big, sub, size=34, sub_size=17)
    s.para(M, 432, "По сообщениям форумов, JLCPCB с февраля 2026 года в Россию не возит. "
           "Кафедре, кружку или фирме электроники плата нужна сегодня, а не через месяц.", W - 2 * M, 20, "ink")
    return s


def flow():
    s = Slide("продукт", "Один станок, весь путь платы",
              "Плата не покидает стол: станок сам меняет фрезы, иглу, пуансоны и щупы.",
              note="the whole path on one table: mill and mask, rivets instead of plating, needle paste, "
                   "pick and place, hot air, test with two probes; photos of the real parts of the process")
    steps = ["mill", "mask", "rivet", "paste", "place", "test"]
    titles = ["Фрезеровка", "Маска", "Заклёпки", "Паста", "Сборка", "Проверка"]
    row(s, steps, M, 470, 168, 64, 24, lambda i: "focal" if i in (2, 5) else "plain", titles, size=18)
    for i, img in enumerate(["pcbmill.jpg", "lifestand.png", "rivets.png", "paste.jpg", "pnp.jpg", "pogo.jpg"]):
        photo(s, M + i * 192, 220, 168, 220, img)
    s.para(M, 580, "После сборки фен паяет плату. После проверки станок сам дорезает найденные перемычки.",
           W - 2 * M, 19, "ink")
    return s


def today():
    s = Slide("основа", "Что уже работает",
              "Программа pcbscan режет настоящую плату с зазорами 0.2 мм.",
              note="what exists: pcbscan with height map, touch probe, isolation of 0.2 mm gaps, V-cut, holes; "
                   "the life-stand board is the proof")
    photo(s, M, 216, 520, 388, "lifestand.png", "плата стенда «Жизнь», 90×90 мм, 444 отверстия")
    done = [("map", "Карта высот", "щуп снимает рельеф листа"),
            ("iso", "Изоляция 0.2 мм", "сплошными линиями, без обрывов"),
            ("holes", "Отверстия и V-рез", "одной фрезой 1 мм"),
            ("log", "Журнал щупа", "обрывы связи видны и лечатся сами")]
    for i, (nid, t, sub) in enumerate(done):
        s.node(nid, 640, 216 + i * 100, 576, 84, "load", t, sub, size=21, sub_size=15)
    return s


# ==========================
# ===  How               ===
# ==========================

def probes():
    s = Slide("как", "Проверка двумя щупами",
              "Два рычага на оси головы, как стрелки часов. Станок везёт ось, серво разводят иглы.",
              note="two-probe test: two 20 mm arms around one vertical axis on the head, each with a pogo needle; "
                   "the machine puts the axis at arm length from both targets; encoder servos for +-0.05 mm")
    s.node("axis", 330, 380, 56, 56, "focal", shape="circle")
    s.node("tipA", 150, 252, 44, 44, "plain", "A", size=16, shape="circle")
    s.node("tipB", 520, 300, 44, 44, "plain", "B", size=16, shape="circle")
    s.line([(335, 392), (190, 287)], "accent", 3)  # the arms go straight, as clock hands, edge to edge
    s.line([(383, 396), (522, 331)], "accent", 3)
    s.text(358, 466, "ось головы", 15, "muted", "middle")
    s.para(M, 540, "Пара точек до 40 мм в любом месте стола. Обрыв длинной цепи проверяется по звеньям.",
           600, 18, "ink")
    photo(s, 760, 216, 456, 270, "pogo.jpg", "подпружиненные иглы: кончик касается дорожки 0.2 мм")
    s.para(760, 540, "Нужна точность ±0.05 мм. Серво с энкодером дают 0.09° на шаг, это 0.03 мм на рычаге 20 мм.",
           456, 17)
    return s


def toolchange():
    s = Slide("как", "Автосмена без дорогого шпинделя",
              "У каждой фрезы своя гайка и цанга ER. Шпиндель сам откручивает и закручивает гайку.",
              note="tool change on ER nuts: the magazine holds the nut, the spindle spins back to release and "
                   "forward to take the next tool; no pneumatics, no ISO20 spindle")
    photo(s, M, 216, 300, 400, "collet.jpg", "цанга ER")
    row(s, ["down", "unscrew", "next", "screw"], 420, 300, 180, 96, 26, lambda i: "plain",
        ["Опустился", "Назад", "К гнезду", "Вперёд"],
        ["над гнездом", "гайка осталась", "новой фрезы", "до упора"], size=20)
    s.para(420, 470, "Шпиндель ISO20 с автосменой стоит 130–160 тыс ₽. Здесь автосмена добавляет 7–13 тыс ₽: "
           "магазин и гайки с цангами. Длину новой фрезы снимает щуп.", 796, 19, "ink")
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


def parts():
    s = Slide("начинка", "Из чего собран станок",
              "Известные узлы, свои платы и программа. Всё покупается в России или делается здесь.",
              note="the parts with photos: closed-loop steppers, ball screws, Raspberry Pi 5 with Klipper, "
                   "Pico or own STM32 boards on CAN")
    items = [("motor.jpg", "Шаговые моторы с энкодером"), ("ballscrew.jpg", "ШВП и рельсы"),
             ("pi5.jpg", "Raspberry Pi 5 и Klipper"), ("pico.jpg", "Свои платы по CAN")]
    for i, (img, cap) in enumerate(items):
        photo(s, M + i * 292, 216, 268, 300, img, cap)
    s.para(M, 580, "Моторы не теряют шагов: щупы, автосмена и дорезка попадают туда, куда нужно.", W - 2 * M, 18, "ink")
    return s


def power():
    s = Slide("начинка", "Одна вилка, всё внутри",
              "Станок ставится хоть в подвал: обычная розетка, никаких внешних щитов и пылесосов.",
              note="power: 48 V brushless spindle without a VFD (no leakage, no noise), one 100-240 V supply, "
                   "ride-through on power loss, built-in dust extraction; 1.3-1.6 kW")
    blocks = [("spindle", "Шпиндель 48 В", "без частотника: УЗО не срабатывает"),
              ("psu", "Блок 100–240 В", "работает при просадках сети"),
              ("hold", "Запас на 2–5 с", "свет пропал: Z вверх, место запомнено"),
              ("dust", "Пылесос и вытяжка", "встроены, с фильтрами")]
    for i, (nid, t, sub) in enumerate(blocks):
        s.node(nid, M + (i % 2) * 584, 224 + (i // 2) * 124, 560, 104, "plain", t, sub, size=22, sub_size=16)
    s.node("total", M, 488, W - 2 * M, 84, "focal", "1.3–1.6 кВт, 6–7 А", "хватит любой розетки", size=24, sub_size=16)
    return s


# ==========================
# ===  Money             ===
# ==========================

def price():
    s = Slide("деньги", "Цена против рынка",
              "Ниже набора LPKF, а умеет больше Carvera и Voltera.",
              note="price positioning: our 1.5-2.5 M rub vs LPKF line several M, Carvera 0.95 M (no paste, no test), "
                   "Voltera ~0.4-0.5 M (no milling)")
    rivals = [("ours", "Наша линия", "1.5–2.5 млн ₽", "весь путь платы", "focal"),
              ("lpkf", "Линия LPKF", "несколько млн ₽", "несколько машин", "plain"),
              ("carv", "Makera Carvera", "0.95 млн ₽", "без пасты, сборки, проверки", "plain"),
              ("volt", "Voltera V-One", "€3,850–5,400", "паста, но не фрезерует", "plain")]
    for i, (nid, name, cost, what, kind) in enumerate(rivals):
        s.node(nid, M + i * 292, 236, 268, 200, kind)
        s.text(M + i * 292 + 134, 284, name, 21, "ink", "middle", owner=nid)
        s.text(M + i * 292 + 134, 340, cost, 24, "ink", "middle", owner=nid)
        s.text(M + i * 292 + 134, 392, what, 14, "muted", "middle", owner=nid)
    s.para(M, 500, "Цены LPKF дилеры дают под заказ, поэтому для неё только оценка.", W - 2 * M, 16)
    return s


def economics():
    s = Slide("деньги", "Экономика одного станка",
              "Железо стоит в 5–7 раз меньше цены. Главные затраты в разработке.",
              note="unit economics: hardware 219-399 k rub, assembly 40-80 h, price 1.5-2.5 M; "
                   "development 3-6 M rub over 1-2 years; 10 machines a year is about 20 M rub")
    nums = [("hw", "219–399 тыс ₽", "железо одного станка"),
            ("labor", "40–80 ч", "сборка одного станка"),
            ("sale", "1.5–2.5 млн ₽", "цена продажи"),
            ("dev", "3–6 млн ₽", "разработка, 1–2 года")]
    for i, (nid, big, sub) in enumerate(nums):
        s.node(nid, M + i * 292, 236, 268, 140, "focal" if nid == "sale" else "plain", big, sub,
               size=28, sub_size=15)
    s.node("year", M, 432, W - 2 * M, 96, "load", "10 станков в год ≈ 20 млн ₽ выручки",
           "покупатели: кафедры, кванториумы, НИИ, фирмы электроники", size=24, sub_size=16)
    return s


def russia():
    s = Slide("деньги", "Сделано в России",
              "В госзакупках это может закрыть дорогу импортным станкам.",
              note="made in Russia: decree 1875 national regime covers milling machines OKPD2 28.41.2; "
                   "with two bids from registry 719, foreign bids are rejected; needs the registry entry")
    s.node("law", M, 236, 560, 132, "plain", "ПП № 1875", "национальный режим, фрезерные станки 28.41.2",
           size=24, sub_size=16)
    s.node("reg", 656, 236, 560, 132, "focal", "Реестр 719", "две заявки из реестра: импорт отклоняется",
           size=24, sub_size=16)
    s.edge("law", "reg", "accent", width=2)
    items = ["Сервис, запчасти и ремонт здесь, за дни.",
             "Доработка под заказчика: поле, головки, методичка под кружок.",
             "Условие: свои плата, программа и сборка; раму, возможно, делать в России."]
    y = 432
    for t in items:
        s.dot(M + 6, y + 9, 4, "teal")
        y = s.para(M + 24, y + 9, t, W - 2 * M - 24, 18, "ink") + 10
    return s


def next_step():
    s = Slide("дальше", "Следующий шаг",
              "Один станок делает одну плату целиком, затем показ трём организациям.",
              note="next step: full cycle on one real board, then a demo to three organisations with the price question")
    row(s, ["proto", "cycle", "demo"], M, 260, 352, 120, 48, lambda i: "focal" if i == 1 else "plain",
        ["Модули на своём станке", "Полный цикл на плате", "Показ трём организациям"],
        ["автосмена, заклёпки, игла", "от гербера до пайки", "цена 1.5–2.5 млн ₽"], size=21)
    photo(s, M, 440, 552, 176, "feeder.jpg", "ленты с деталями для сборки")
    s.para(680, 470, "Если две организации готовы купить, делаем раму, декларацию и заявку в реестр 719.",
           536, 19, "ink")
    return s


def credits():
    s = Slide("источники", "Фотографии",
              "Wikimedia Commons, свободные лицензии; рендер платы стенда свой.",
              note="photo credits from img/credits.tsv")
    rows = []
    with open(__file__.replace("slides.py", "img/credits.tsv")) as f:
        for line in f:
            _, title, author, lic = line.rstrip("\n").split("\t")
            rows.append(f"{title} · {author[:40]} · {lic}")
    for i, r in enumerate(rows):
        s.text(M + (i // 7) * 584, 228 + (i % 7) * 52, r, 13, "muted")
    return s


SLIDES = [title, problem, flow, today,
          order, probes, toolchange, rivets, setting, paste, heightmap, heat, table, flipper, register, rules,
          parts, power, price, economics, russia, checks, next_step, credits]
