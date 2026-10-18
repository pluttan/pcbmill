"""Chapters 1-2: why the machine is needed and the path of a board through it."""
from deckkit import M, W, Slide, row

from common import cards, photo, table


# ==========================
# ===  1. Why            ===
# ==========================

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


def market():
    s = Slide("зачем", "Что продают сейчас",
              "Станок «всё в одном» есть только у Makera. В России он идёт серым импортом.",
              note="market table: Carvera Air 538k, Carvera 949k, Russian desktop with ATC from 1.2M, "
                   "desktop without ATC 170-280k without PCB software, LPKF E44 1.5-3M estimate, Voltera EUR 3,850-5,400")
    rows = [("Станок", "Цена в России", "Чего не хватает"),
            ("Makera Carvera Air", "около 538 тыс ₽", "смена фрез ручная, поле 300 × 200 мм"),
            ("Makera Carvera", "около 949 тыс ₽", "нет пасты, сборки и проверки цепей"),
            ("Российские с автосменой", "от 1.2 млн ₽", "нет программы и щупа под платы"),
            ("Российские без автосмены", "170–280 тыс ₽", "станок, а не комплект для плат"),
            ("LPKF ProtoMat E44", "1.5–3 млн ₽, оценка", "только фрезеровка, смена ручная"),
            ("Voltera V-One", "€3,850–5,400", "не фрезерует, дорожки чернилами")]
    table(s, M, 250, [340, 300, 512], rows, size=18, row_h=46, accent_col=1)
    return s


def buyers():
    s = Slide("зачем", "Кто покупает",
              "Покупатели платят за плату сегодня. Большая часть покупает через госзакупки.",
              note="segments: STEM clubs and schools, university departments, small electronics firms, makers, "
                   "owners of 3040/6040 who want a kit")
    items = [("kvant", "Кванториумы и школы", "станок для кружка с методичкой"),
             ("dept", "Кафедры вузов", "прототипы для лабораторных и курсовых"),
             ("firm", "Фирмы электроники", "плата за день вместо месяца"),
             ("maker", "Мейкеры", "смотрят на Carvera Air, ищут дешевле"),
             ("own", "Владельцы 3040 и 6040", "комплект для своего станка")]
    cards(s, items, y=228, h=88, kinds={"dept": "focal", "kvant": "focal"})
    s.para(M, 576, "Закупку до 600 тыс ₽ проводят у единственного поставщика, 44-ФЗ ст. 93.",
           W - 2 * M, 17)
    return s


def offer():
    s = Slide("зачем", "Что мы предлагаем",
              "Одна настольная машина вместо линии из нескольких станков.",
              note="the offer: the whole board path on one machine, made in Russia, 1.5-2.5 M rub estimate")
    items = [("path", "Весь путь платы", "фрезеровка, проверка, маска, заклёпки, паста, сборка, пайка"),
             ("ru", "Сделано в России", "сервис за дни, доработка под заказчика, реестр 719"),
             ("cost", "1.5–2.5 млн ₽", "оценка; линия LPKF стоит в разы дороже")]
    cards(s, items, y=236, cols=1, h=100, gap=24, size=24, kinds={"path": "focal"})
    return s


# ==========================
# ===  2. Board path     ===
# ==========================

def flow():
    s = Slide("путь платы", "Один станок, весь путь платы",
              "Плата не покидает стол: станок сам меняет фрезы, иглу, пуансоны и щупы.",
              note="the whole path on one table: mill and mask, rivets instead of plating, needle paste, "
                   "pick and place, hot air, test with two probes; photos of the real parts of the process")
    steps = ["mill", "mask", "rivet", "paste", "place", "test"]
    titles = ["Фрезеровка", "Маска", "Заклёпки", "Паста", "Сборка", "Проверка"]
    row(s, steps, M, 470, 168, 64, 24, lambda i: "focal" if i in (2, 5) else "plain", titles, size=18)
    for i, img in enumerate(["pcbmill.jpg", "lifestand.png", "rivets.png", "paste.jpg", "pnp.jpg", "pogo.jpg"]):
        photo(s, M + i * 192, 220, 168, 220, img)
    s.para(M, 580, "После сборки фен паяет плату. После проверки станок сам дорезает перемычки, которые нашёл.",
           W - 2 * M, 19, "ink")
    return s


def order():
    s = Slide("путь платы", "Весь цикл платы",
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


def factory():
    s = Slide("путь платы", "Что заменяет завод",
              "Каждую заводскую операцию станок делает своим способом.",
              note="factory step vs our machine: plating -> rivets, stencil printer -> needle, reflow oven -> hot air, "
                   "flying probe tester -> two probes on the head, panel router -> V-cut")
    rows = [("На заводе", "На нашем станке"),
            ("Металлизация отверстий в ванне", "медные заклёпки 0.4 / 0.6 мм"),
            ("Трафаретный принтер пасты", "игла 0.4 мм по карте высот"),
            ("Печь оплавления", "фен на голове, участок за участком"),
            ("Тестер с летающими щупами", "два щупа на оси головы"),
            ("Травление меди", "фрезеровка V-фрезой по карте высот"),
            ("Скрайбер панели", "V-рез до краёв платы")]
    table(s, M, 250, [576, 576], rows, size=19, row_h=50, accent_col=1)
    return s
