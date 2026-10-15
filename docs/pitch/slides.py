"""Pitch of the desktop PCB line: one machine from gerber to an assembled board.

Built with deckkit (~/mnt/pr/pets/deckkit): nodes have names, edges join names,
every slide carries a `note` with its meaning. Photos are SVG <image> from img/
(Wikimedia Commons, credits on the last slide and in img/credits.tsv).

    deck all docs/pitch/slides.py
"""
import itertools

from deckkit import M, W, Slide, row

TITLE = "Плата за час"
FOOTER = "pcbmill · настольная линия для печатных плат"
FOOT_CAT = 1150

_clip = itertools.count()


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
    s.eyebrow(M, 228, "pcbmill · идея продукта · 15.10.2026", "accent")
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
              "Плата не покидает стол: станок сам меняет фрезы и головки.",
              note="the whole path on one table: mill, test with two probes, re-cut, mask, paste through a stencil, "
                   "pick and place, reflow; photos of the real parts of the process")
    steps = ["mill", "test", "mask", "paste", "place", "reflow"]
    titles = ["Фрезеровка", "Проверка", "Маска", "Паста", "Сборка", "Оплавление"]
    row(s, steps, M, 470, 168, 64, 24, lambda i: "focal" if i == 1 else "plain", titles, size=18)
    for i, img in enumerate(["pcbmill.jpg", "pogo.jpg", "lifestand.png", "paste.jpg", "pnp.jpg", "reflow.jpg"]):
        photo(s, M + i * 192, 220, 168, 220, img)
    s.para(M, 580, "После проверки станок сам дорезает найденные перемычки и проверяет плату снова.", W - 2 * M, 19, "ink")
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
        ["автосмена, щупы, трафарет", "от гербера до пайки", "цена 1.5–2.5 млн ₽"], size=21)
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


SLIDES = [title, problem, flow, today, probes, toolchange, parts, power, price, economics, russia, next_step, credits]
