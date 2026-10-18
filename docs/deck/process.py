"""Chapters 6-12: milling, two-probe test, mask, rivets, flipping and registration, paste, assembly."""
from deckkit import M, W, Slide, row

from common import bullets, cards, photo, table


# ==========================
# ===  6. Milling        ===
# ==========================

def millmap():
    s = Slide("фрезеровка", "Карта высот",
              "Заготовка неровная на десятые доли миллиметра. Щуп касается меди по сетке, и фреза идёт по рельефу.",
              note="height map: the touch probe touches copper on a grid, the V-bit follows the relief at 0.1 mm depth; "
                   "the map error can reach 0.05 mm")
    row(s, ["grid", "touch", "mesh", "cut"], M, 250, 260, 110, 32, lambda i: "focal" if i == 2 else "plain",
        ["Сетка точек", "Касание", "Карта", "Фрезеровка"],
        ["по всему листу", "щуп замыкает медь", "bed_mesh в Klipper", "глубина 0.1 мм"], size=21)
    bullets(s, M, 430, [
        "Ошибка карты высот бывает до 0.05 мм. На такой ошибке канавка V-фрезы становится шире.",
        "Карта снимается заново после каждого переворота.",
    ], W - 2 * M)
    return s


def laser():
    s = Slide("фрезеровка", "Лазерный датчик расстояния",
              "Лазер снимает рельеф без касания, за один проход. Щуп даёт несколько опорных касаний.",
              note="laser distance sensor as a fast helper of the height map: a rough scan of the whole sheet, "
                   "then the probe refines; the laser also sees the mask surface, the probe does not; "
                   "industrial sensors: Panasonic HG-C1030 10 um at $440-610, Optex CD22-15 1 um at about $690; "
                   "shiny copper makes noise, so probe touches calibrate each surface")
    row(s, ["scan", "rough", "touch", "fine"], M, 250, 260, 110, 32, lambda i: "focal" if i == 3 else "plain",
        ["Лазер", "Рельеф", "Щуп", "Точная карта"],
        ["весь лист за проход", "по меди и маске", "касание на каждой поверхности", "для V-фрезы и иглы"], size=21)
    bullets(s, M, 430, [
        "Panasonic HG-C1030 даёт повторяемость 10 мкм за $440–610. Карте высот нужно 50 мкм.",
        "Блестящая медь даёт шум. Медь, маска и текстолит дают сдвиг, его снимает касание щупа.",
        "Лазер видит поверхность маски. Щупу маска мешает, лазеру нет.",
    ], W - 2 * M)
    return s


def isolation():
    s = Slide("фрезеровка", "Изоляция 0.2 мм",
              "Программа pcbscan уже режет дорожки и зазоры 0.2 мм на настоящей плате.",
              note="isolation result 09.10.2026: 0.2 mm tracks and gaps cut clean at 24000 rpm, 300 mm/min, 0.1 mm deep; "
                   "two passes with 0.3 overlap; 0.3 mm gaps recommended")
    photo(s, M, 216, 520, 388, "lifestand.png", "плата стенда «Жизнь», 90 × 90 мм")
    items = [("min", "Минимум 0.2 мм", "дорожка и зазор, тест 09.10.2026"),
             ("rec", "Советуем 0.3 мм", "запас на ошибку карты высот"),
             ("mode", "24 000 об/мин, 300 мм/мин", "глубина 0.1 мм"),
             ("pass", "Два обхода", "перекрытие 0.3, рез около 0.31 мм")]
    cards(s, items, y=216, cols=1, h=84, gap=16, size=20, sub_size=15, x=640, width=576, kinds={"min": "load"})
    return s


def holes():
    s = Slide("фрезеровка", "Отверстия",
              "Фреза 1 мм режет любое отверстие от 1 мм по кругу. Под заклёпки нужно сверло 0.7 мм.",
              note="holes: the 1 mm mill cuts any hole from 1 mm by circles (M3 included), a 0.7 drill for 0.4/0.6 rivets; "
                   "today a hole drifts up to 0.2 mm, rivets need 0.02")
    items = [("mill", "Фреза 1 мм", "отверстия от 1 мм, M3, контур платы"),
             ("drill", "Сверло 0.7 мм", "под заклёпку 0.4 / 0.6"),
             ("now", "Сейчас уход до 0.2 мм", "сверло гнётся, наколка неточна"),
             ("need", "Нужно 0.02 мм", "иначе заклёпка выпадет при перевороте")]
    cards(s, items, y=228, h=110, kinds={"now": "drop", "need": "focal"})
    s.para(M, 496, "Поясок площадки сейчас 0.5 мм из-за ухода отверстия. На точном станке площадка сократится "
           "примерно до 1.2 мм.", W - 2 * M, 18, "ink")
    return s


def vcut():
    s = Slide("фрезеровка", "V-рез и борта",
              "Плата отламывается по канавке V-фрезы. Вокруг платы остаётся жертвенный борт 5 мм.",
              note="V-cut must run to the board edges to break cleanly; a 5 mm sacrificial border holds the flip "
                   "wedges and the fiducial holes")
    items = [("v", "V-рез до краёв", "иначе плата ломается неровно"),
             ("b", "Борт 5 мм", "под клинья рамки переворота"),
             ("f", "Реперы в борту", "три отверстия насквозь")]
    cards(s, items, y=248, cols=3, h=140, size=24, kinds={"b": "focal"})
    s.para(M, 452, "Борт отламывают последним, после сборки и проверки платы.", W - 2 * M, 18, "ink")
    return s


# ==========================
# ===  7. Two probes     ===
# ==========================

def probes():
    s = Slide("проверка", "Проверка двумя щупами",
              "Два рычага на оси головы, как стрелки часов. Станок везёт ось, серво разводят иглы.",
              note="two-probe test: two 20 mm arms around one vertical axis on the head, each with a pogo needle; "
                   "the machine puts the axis at arm length from both targets; encoder servos for +-0.05 mm")
    s.node("axis", 330, 380, 56, 56, "focal", shape="circle")
    s.node("tipA", 150, 252, 44, 44, "plain", "A", size=16, shape="circle")
    s.node("tipB", 520, 300, 44, 44, "plain", "B", size=16, shape="circle")
    s.line([(335, 392), (190, 287)], "accent", 3)
    s.line([(383, 396), (522, 331)], "accent", 3)
    s.text(358, 466, "ось головы", 15, "muted", "middle")
    s.para(M, 540, "Пара точек до 40 мм в любом месте стола. Обрыв длинной цепи станок проверяет по звеньям.",
           600, 18, "ink")
    photo(s, 760, 216, 456, 270, "pogo.jpg", "подпружиненные иглы: кончик касается дорожки 0.2 мм")
    s.para(760, 540, "Иглы наклонены друг к другу на 10–15°, поэтому острия сходятся через зазор 0.2 мм.",
           456, 17)
    return s


def probe_accuracy():
    s = Slide("проверка", "Точность ±0.05 мм",
              "Дорожка 0.2 мм требует попасть иглой в её середину. Это 0.14° на рычаге 20 мм.",
              note="accuracy: model servos have ~1 deg dead zone (0.35 mm) and do not fit; Dynamixel XL330 gives 0.09 deg, "
                   "0.03 mm on the arm; calibration on a test board to +-0.03-0.05 mm")
    items = [("model", "Модельные серво", "мёртвая зона 1°, это 0.35 мм"),
             ("xl", "Dynamixel XL330", "0.09° на шаг, 0.03 мм на рычаге"),
             ("step", "Мотор с редуктором", "и энкодер AS5600, запасной путь"),
             ("cal", "Калибровка", "по площадкам пробной платы")]
    cards(s, items, y=228, h=110, kinds={"model": "drop", "xl": "focal"})
    s.para(M, 496, "Цель макета: 100 касаний из 100 по центру дорожки 0.2 мм.", W - 2 * M, 18, "ink")
    return s


def nets():
    s = Slide("проверка", "Замыкания и обрывы",
              "Станок сам находит перемычку, дорезает зазор и проверяет снова.",
              note="test loop: isolation, check, re-cut, check again, then drilling; shorts by point pairs across gaps "
                   "0.3-3 mm apart, opens by chain links under 40 mm, threshold about 10 Ohm")
    row(s, ["iso", "chk", "recut", "chk2"], M, 236, 260, 104, 32, lambda i: "focal" if i == 2 else "plain",
        ["Изоляция", "Проверка", "Дорезка", "Проверка"],
        ["V-фреза", "две иглы", "зазор найденной пары", "ещё раз"], size=21)
    bullets(s, M, 400, [
        "Замыкание: иглы встают по обе стороны зазора, на расстоянии 0.3–3 мм.",
        "Обрыв: цепь делится на звенья короче 40 мм, станок прозванивает каждое звено.",
        "Порог около 10 Ом. Звонит ESP32 щупа.",
    ], W - 2 * M)
    return s


def rejected():
    s = Slide("проверка", "От чего отказались",
              "Два простых способа не дают точности для дорожки 0.2 мм.",
              note="rejected: an electromagnet puck shifts 0.1-0.3 mm on release; one probe measuring capacitance sees "
                   "0.2-1 pF of a net against tens of pF of its own wire")
    items = [("puck", "Шайба на электромагните", "при отпускании сдвиг 0.1–0.3 мм"),
             ("cap", "Ёмкость одним щупом", "0.2–1 пФ цепи против десятков пФ провода")]
    cards(s, items, y=248, h=140, size=24, kinds={"puck": "drop", "cap": "drop"})
    s.node("two", M, 432, W - 2 * M, 96, "focal", "Два щупа с серво", "решают и замыкания, и обрывы", size=24)
    return s


# ==========================
# ===  8. Mask           ===
# ==========================

def mask():
    s = Slide("маска", "Паяльная маска",
              "Жидкая маска раскатывается валиком, УФ засвечивает её, фреза вскрывает окна над площадками.",
              note="solder mask: liquid UV mask rolled on, a 405 nm module exposes it, the mill opens pad windows; "
                   "the mask also glues tracks to the base, so they lift less under the iron")
    row(s, ["roll", "uv", "win"], M, 236, 368, 110, 24, lambda i: "focal" if i == 1 else "plain",
        ["Нанесение", "Засветка 405 нм", "Окна фрезой"],
        ["валиком по всей плате", "УФ-модуль на голове", "по слою F_Mask"], size=22)
    bullets(s, M, 410, [
        "Маска приклеивает дорожки к основе сверху. С ней медь меньше отходит от паяльника.",
        "Глубину вскрытия окон подбирают по пробной полоске маски на листе.",
    ], W - 2 * M)
    return s


def windows():
    s = Slide("маска", "Окна в маске",
              "Окно шире площадки, чтобы маска не легла на медь. Над заклёпкой окно шире буртика.",
              note="mask windows: pad plus 0.05-0.1 mm per side, windows under 0.18 mm opened by one poke, "
                   "over a rivet at least 1.0 mm so the flange lies on bare copper")
    items = [("pad", "Площадка + 0.05–0.1 мм", "с каждой стороны"),
             ("rivet", "Над заклёпкой от 1.0 мм", "буртик 0.9 мм ложится на медь"),
             ("poke", "Окна уже 0.18 мм", "одним уколом посередине")]
    cards(s, items, y=248, cols=3, h=140, size=22, kinds={"rivet": "load"})
    return s


# ==========================
# ===  9. Rivets         ===
# ==========================

def joining():
    s = Slide("заклёпки", "Как соединить две стороны",
              "Без металлизации отверстий двухсторонняя плата не работает. Есть четыре способа.",
              note="ways to join sides at home: wire (vias only), soldering legs on both sides (not everywhere), "
                   "electroplating (bath, acid, unstable), rivets (chosen)")
    items = [("wire", "Проволока", "только переходы, паять руками"),
             ("both", "Пайка с двух сторон", "под разъёмами невозможна"),
             ("galv", "Гальваника", "ванна, кислота, итог нестабилен"),
             ("riv", "Заклёпки", "станок ставит их сам")]
    cards(s, items, y=236, h=120, size=24,
          kinds={"wire": "drop", "both": "drop", "galv": "drop", "riv": "focal"})
    return s


def rivet_photo():
    s = Slide("заклёпки", "Медная заклёпка",
              "Трубка с буртиком входит в отверстие. Хвост с другой стороны раскрывается и зажимает плату.",
              note="what a PCB rivet is: a copper tube with a flange; the hole is 0.1 mm wider than its outer diameter; "
                   "the smallest is 0.4/0.6 mm, Voltera warns it is hard to handle by hand, a machine does not care")
    photo(s, M, 216, 560, 384, "rivets.png", "пустотелые заклёпки для отверстий плат")
    bullets(s, 680, 236, [
        "Отверстие делают на 0.1 мм шире заклёпки.",
        "Самая мелкая заклёпка 0.4 / 0.6 мм.",
        "Руками её брать трудно. Станку размер не мешает.",
        "Заклёпка зажимает площадку механически, поэтому площадка не отходит от паяльника.",
    ], 536)
    return s


def rivets():
    s = Slide("заклёпки", "Размеры и цены заклёпок",
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


def punch():
    s = Slide("заклёпки", "Пуансон",
              "Пуансон состоит из стального стержня с фигурным концом. Им давят на заклёпку, чтобы раскрыть её.",
              note="punches: a cone opens the tube end outwards, a flat punch presses the opened rim onto the pad; "
                   "both live in the magazine; the cone tip wears first and is a spare part")
    items = [("cone", "Конический пуансон", "входит в трубку и раскрывает край"),
             ("flat", "Плоский пуансон", "прижимает край к площадке"),
             ("wear", "Износ", "кончик конуса 0.4 мм меняют как расходник"),
             ("force", "Сила", "даёт ось Z, число даст опыт")]
    cards(s, items, y=228, h=110, kinds={"cone": "focal", "flat": "focal"})
    return s


# ==========================
# ===  10. Flip          ===
# ==========================

def two_flips():
    s = Slide("переворот", "Зачем два переворота",
              "Первый переворот открывает сторону B. Второй поднимает хвосты заклёпок к пуансону.",
              note="flip 1 after side A to work side B; flip 2 after rivets so the tails face up for paste and flaring; "
                   "after paste the board is never flipped")
    row(s, ["a", "f1", "b", "r", "f2", "p"], M, 260, 167, 110, 30,
        lambda i: "focal" if i in (1, 4) else "plain",
        ["Сторона A", "Переворот 1", "Сторона B", "Заклёпки", "Переворот 2", "Паста"],
        ["дорожки, маска", "B вверх", "дорожки, маска", "буртик на B", "хвосты вверх", "больше не вертим"], size=18)
    s.para(M, 460, "После пасты плату переворачивать нельзя: паста размажется о стол.", W - 2 * M, 19, "ink")
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
    facts = [("frame", "Рамка 320 или 520 мм", "клинья 5 × 100 × 1 мм"),
             ("border", "Борт 5 мм", "жертвенный, в нём реперы"),
             ("height", "До 170 или 270 мм", "подъём для Standard и Max")]
    for i, (nid, t, sub) in enumerate(facts):
        s.node(nid, M + i * 392, 420, 368, 104, "plain", t, sub, size=22, sub_size=16)
    s.para(M, 576, "Рамка Standard может поместиться в магазин. Рамка Max стоит вдоль задней стенки.", W - 2 * M, 17)
    return s


def lift():
    s = Slide("переворот", "Высота подъёма",
              "Лист вращается вокруг своей средней линии. Подъём равен половине ширины плюс запас 20 мм.",
              note="lift height by board width: 100 -> 70 mm, 200 -> 120, 300 -> 170, 500 -> 270; "
                   "Standard Z 200 covers 300 mm boards, Max Z 300 covers the 500 sheet")
    rows = [("Ширина платы", "Подъём"), ("100 мм", "70 мм"), ("200 мм", "120 мм"),
            ("300 мм", "170 мм"), ("500 мм", "270 мм")]
    table(s, M, 260, [320, 240], rows, size=22, row_h=56, accent_col=1)
    s.node("zs", 700, 260, 516, 96, "plain", "Standard: Z 200 мм", "платы до 300 мм", size=24, sub_size=16)
    s.node("zm", 700, 376, 516, 96, "focal", "Max: Z 300 мм", "лист 500 × 500 мм", size=24, sub_size=16)
    s.para(700, 512, "Программа считает подъём под каждую плату. Малые платы переворачиваются быстро.", 516, 18, "ink")
    return s


def wedge():
    s = Slide("переворот", "Клин 1 мм при включённом вакууме",
              "Вакуум держит плату сильнее, чем клин её толкает. Плата не сдвигается, а чуть приподнимается.",
              note="wedge: polished steel or PTFE coated, 1 mm thick, enters with vacuum on; a 2 mm wedge bends copper "
                   "and mask twice as much; the board may still shift and the camera finds it")
    items = [("thin", "Клин 1 мм", "изгиб платы вдвое меньше, чем при 2 мм"),
             ("slick", "Полировка и фторопласт", "меньше трения о медь"),
             ("vac", "Вакуум включён", "плата не уезжает от клина"),
             ("shift", "Сдвиг допустим", "камера найдёт плату заново")]
    cards(s, items, y=228, h=110, kinds={"thin": "focal"})
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


# ==========================
# ===  11. Paste         ===
# ==========================

def paste():
    s = Slide("паста", "Паста иглой 0.4 мм",
              "Трафарет не нужен. Мотор давит поршень шприца на точный объём.",
              note="needle paste: 0.4 mm needle, dot 0.6-0.8 mm, type 4-5 dispensing paste, gap 0.2-0.4 mm from "
                   "the height map, positive displacement dispenser, cleaning station, fridge")
    facts = [("needle", "Точка 0.6–0.8 мм", "0603, 0805, SOIC 1.27 точками"),
             ("line", "Линия пасты", "0402 и шаг 0.65 мм: вдоль ряда площадок"),
             ("kind", "Паста тип 4–5", "для дозирования, металла 85–88 %"),
             ("gap", "Зазор 0.2–0.4 мм", "игла над площадкой по карте высот")]
    cards(s, facts, y=228, h=104, size=22)
    s.node("disp", M, 488, 564, 96, "focal", "Дозатор вытеснения", "как у Voltera: шаговый мотор и поршень",
           size=22, sub_size=16)
    s.node("care", 652, 488, 564, 96, "plain", "Скорость", "2–5 точек в секунду, 500 площадок за 2–4 мин",
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


def paste_care():
    s = Slide("паста", "Хранение и чистка",
              "Паста сохнет на игле и портится в тепле. Станок для подвала должен это учесть.",
              note="paste care: syringe in a fridge at 2-10 C, 4 h to warm up, about 8 h of life on the board; "
                   "a needle cleaning station drops the first dot; open question: cold box inside or paste without cold")
    items = [("cold", "Хранение 2–10 °C", "шприц в холодильнике"),
             ("warm", "Прогрев 4 ч", "перед работой"),
             ("clean", "Станция чистки", "протирает иглу, сбрасывает первую каплю"),
             ("open", "Холод внутри станка", "или паста без холода: выбрать")]
    cards(s, items, y=228, h=110, kinds={"open": "focal"})
    return s


# ==========================
# ===  12. Assembly      ===
# ==========================

def pnp():
    s = Slide("сборка", "Вакуумная пипетка",
              "Пипетка берёт деталь с ленты, нижняя камера опознаёт корпус, голова ставит деталь на пасту.",
              note="pick and place: a pipette with its own small Z and rotation on TMC2209, a bottom camera, "
                   "tape strips instead of feeders, OpenPnP software")
    photo(s, M, 216, 520, 388, "pnp.jpg", "голова установщика деталей")
    items = [("pip", "Пипетка с поворотом", "малая Z и поворот от TMC2209"),
             ("cam", "Нижняя камера", "опознаёт корпус и поворот"),
             ("tape", "Полоски ленты", "вместо дорогих питателей"),
             ("sw", "OpenPnP", "готовая программа сборки")]
    cards(s, items, y=216, cols=1, h=84, gap=16, size=20, sub_size=15, x=640, width=576, kinds={"cam": "focal"})
    return s


def tht():
    s = Slide("сборка", "Детали с ногами",
              "Ноги вставляются в заклёпки. Паста внутри заклёпки паяет их в том же нагреве.",
              note="through-hole parts: legs go into rivets with paste inside (pin-in-paste), soldered by the same hot air; "
                   "header pins 0.64 square need a rivet of 1.0 mm inner or more")
    items = [("pip", "Паста в заклёпке", "на заводах это зовут pin-in-paste"),
             ("leg", "Нога в заклёпку", "после развальцовки"),
             ("hdr", "Гребёнка 2.54", "штырь 0.64 мм, заклёпка от 1.0 мм"),
             ("one", "Один нагрев", "SMD и ноги паяются вместе")]
    cards(s, items, y=228, h=110, kinds={"one": "focal"})
    return s


def heat():
    s = Slide("сборка", "Пайка феном сверху",
              "Нагрев стола даёт больше трудностей, чем пользы. Мелкая медная заклёпка прогревается сверху сама.",
              note="reflow by hot air only: a 20-30 mm spot moved over zones, no heated table; large copper needs "
                   "thermal reliefs and a slow ramp; the textolite plate stands the heat")
    s.node("fen", M, 236, 564, 120, "focal", "Фен на голове", "пятно 20–30 мм, участок за участком",
           size=24, sub_size=16)
    s.node("bed", 652, 236, 564, 120, "drop", "Нагрев стола", "не нужен", size=24, sub_size=16)
    bullets(s, M, 412, [
        "Земляная заливка уводит тепло. Нужны тепловые разгрузки и медленный подъём температуры.",
        "Стык двух участков нагрева программа ставит мимо деталей.",
        "Пластина под платой из текстолита выдерживает нагрев пайки.",
    ], W - 2 * M)
    return s


def rules():
    s = Slide("разводка", "Правила для платы",
              "Станок требует от разводки шесть вещей. Программа проверяет их до начала работы.",
              note="board design rules for the line: 5 mm border, parts on side A, 2 mm pads with teardrops, "
                   "mask window wider than the rivet flange, two rivet sizes, 0.3 mm gaps")
    items = [("rb", "Борт 5 мм", "под клинья и реперы"),
             ("ra", "Детали на стороне A", "паста и сборка идут там"),
             ("rp", "Площадки от 2 мм", "teardrop у входа дорожки"),
             ("rw", "Окно маски от 1.0 мм", "шире буртика заклёпки"),
             ("rr", "Две заклёпки", "0.4 / 0.6 и от 1.0 мм внутри"),
             ("rg", "Зазор 0.3 мм", "0.2 только там, где иначе не пройти")]
    cards(s, items, y=224, h=96, size=22)
    return s
