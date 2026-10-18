"""Chapters 3-5: frame and axes, the table, spindle and tool magazine."""
from deckkit import M, W, Slide, row

from common import bullets, cards, photo, table


# ==========================
# ===  3. Frame and axes ===
# ==========================

def own_frame():
    s = Slide("рама", "Своя рама, а не старый 3040",
              "Китайские 3040 и 6040 годятся для макета модулей, но не для продукта.",
              note="why an own frame: used machines all differ, profile frames bend, 1605 screws give 3-5 m/min "
                   "while placement needs 20-30, they do not pass registry 719, no room for the magazine")
    items = [("diff", "Каждый б/у станок разный", "гарантию на такой станок не дать"),
             ("bend", "Рама из профиля гнётся", "точность 0.02 мм не держит"),
             ("slow", "Винт 1605: 3–5 м/мин", "сборке нужно 20–30 м/мин"),
             ("reg", "Не пройдёт в реестр 719", "нужны своя рама, плата и программа"),
             ("room", "Нет места", "под магазин, рамку переворота, фен")]
    cards(s, items, y=228, h=88, kinds={k: "drop" for k in ("diff", "bend", "slow", "reg", "room")})
    s.para(M, 576, "Макет модулей собирается на нашем текущем станке, продукт на своей раме.", W - 2 * M, 18, "ink")
    return s


def layout():
    s = Slide("рама", "Стол стоит, ездит портал",
              "Стол неподвижен, поэтому вакуум и пластина не трясутся. Голова ездит над столом на портале.",
              note="layout from above: fixed table 510x510, magazine strip along one side, gantry moves; "
                   "travel about 600 x 510 x 300 mm")
    s.node("plate", M, 228, 352, 352, "focal", "Стол", "510 × 510 мм", size=22, sub_size=16)
    s.node("mag", M + 368, 228, 72, 352, "store", "ER", size=16)
    s.text(M + 404, 604, "магазин", 14, "muted", "middle")
    s.line([(M - 16, 300), (M + 456, 300)], "teal", 4)
    s.text(M + 176, 290, "портал", 14, "teal", "middle", owner="plate")
    items = [("tx", "Ход X: около 600 мм", "стол и полоса магазина"),
             ("ty", "Ход Y: 510 мм", "стол целиком"),
             ("tz", "Ход Z: 300 мм", "переворот листа 500 × 500"),
             ("frame", "Рама около 750 × 650 мм", "оценка по ходам и порталу")]
    cards(s, items, y=228, cols=1, h=76, gap=16, size=20, sub_size=15, x=584, width=632)
    return s


def motors():
    s = Slide("рама", "Моторы, которые не теряют шаг",
              "Если мотор пропустил шаг, щупы, автосмена и дорезка промахиваются.",
              note="motion: NEMA23 steppers with encoders and external drivers that raise an alarm; "
                   "plain NEMA23 with TMC5160 is 5-8k cheaper but a missed step goes unseen")
    photo(s, M, 216, 420, 360, "motor.jpg", "шаговый мотор NEMA23")
    items = [("nema", "NEMA23 с энкодером", "мотор сам видит, где он стоит"),
             ("drv", "Внешние драйверы", "подают сигнал аварии"),
             ("cheap", "TMC5160 дешевле на 5–8 тыс ₽", "но пропуск шага не заметит")]
    cards(s, items, y=216, cols=1, h=104, gap=24, size=22, x=540, width=676, kinds={"cheap": "drop"})
    return s


def rails():
    s = Slide("рама", "Рельсы и винты",
              "Рельсы держат голову без люфта. Винт с двойной гайкой не даёт выборки.",
              note="linear guides HGR15/MGN15 on X and Y, ball screw with double nut; X and Y need 20-30 m/min "
                   "for placement, so belts or 1610 screws")
    photo(s, M, 216, 420, 360, "ballscrew.jpg", "шариковый винт с гайкой")
    items = [("guide", "Рельсы HGR15 или MGN15", "на X и Y, каретки без люфта"),
             ("xy", "X и Y: ремни или винт 1610", "сборке нужна скорость 20–30 м/мин"),
             ("nut", "Двойная гайка", "нет выборки при смене направления")]
    cards(s, items, y=216, cols=1, h=104, gap=24, size=22, x=540, width=676)
    return s


def zaxis():
    s = Slide("рама", "Ось Z на 300 мм",
              "Длинный ход нужен для переворота листа. Точность у стола при этом остаётся 0.02 мм.",
              note="Z axis 300 mm for flipping a 500x500 sheet; to keep 0.02 mm at the table: 20 mm rails, "
                   "two blocks per rail far apart, 15 mm plate, double-nut ball screw")
    items = [("rail", "Рельсы от 20 мм", "по две каретки, далеко друг от друга"),
             ("plate", "Плита Z от 15 мм", "сталь или алюминий"),
             ("screw", "Винт с двойной гайкой", "без люфта по Z"),
             ("why", "Зачем 300 мм", "подъём листа 500 × 500 на 270 мм")]
    cards(s, items, y=228, h=110, kinds={"why": "focal"})
    s.para(M, 496, "Фреза работает у стола, то есть в нижней части хода. Там шпиндель дальше всего от каретки, "
           "поэтому жёсткость мы считаем для этой точки.", W - 2 * M, 18, "ink")
    return s


# ==========================
# ===  4. Table          ===
# ==========================

def vacuum():
    s = Slide("стол", "Вакуумная плита",
              "Плата держится без скотча и прижимов. Плита из алюминия никогда не режется.",
              note="vacuum table: aluminium plate with channels, a pump of 3-8k rub, one valve on and off; "
                   "the plate can also serve as an electrode for net tests")
    items = [("al", "Плита из алюминия", "вакуумные каналы, ровная, не режется"),
             ("pump", "Насос 3–8 тыс ₽", "и фильтр перед ним"),
             ("valve", "Один клапан", "вакуум включён или выключен"),
             ("el", "Плита как электрод", "пригодится для проверки цепей")]
    cards(s, items, y=228, h=110, kinds={"al": "focal"})
    return s


def plate():
    s = Slide("стол", "Сменная пластина как у 3D-принтера",
              "Сверху на плите лежит пластина из текстолита G10. Её режет сверло, она расходник.",
              note="replaceable G10 plate on magnets or pins, vacuum holes on a grid, changed in 5 s, "
                   "the machine resurfaces it by 0.2-0.3 mm; a consumable with our logo")
    s.node("al", M, 452, 560, 72, "store", "Плита из алюминия", "не режется", size=20, sub_size=15)
    s.node("g10", M, 344, 560, 92, "focal", "Пластина G10 с логотипом", "на магнитах или штифтах", size=22)
    s.node("pcb", M + 80, 260, 400, 68, "plain", "Плата", "держится вакуумом сквозь пластину", size=20)
    bullets(s, 680, 260, [
        "Вакуумные отверстия в пластине стоят по сетке.",
        "Пластина меняется за 5 с.",
        "Изрезанную пластину станок выравнивает фрезой на 0.2–0.3 мм.",
        "Одной пластины хватает на десятки плат.",
    ], 536)
    return s


def plate_material():
    s = Slide("стол", "Почему текстолит",
              "Сверло выходит сквозь плату, поэтому под платой нужен материал, который не жалко резать.",
              note="plate material: steel breaks the 0.7 drill, cannot be pocketed, sinks heat, blocks air; "
                   "MDF burns under hot air; G10 cuts like a board and stands soldering heat")
    items = [("steel", "Сталь", "сверло 0.7 ломается, воздух не проходит"),
             ("mdf", "МДФ", "пористый, но горит под феном"),
             ("g10", "Текстолит G10", "режется как плата, нагрев держит")]
    cards(s, items, y=248, cols=3, h=140, size=26, kinds={"steel": "drop", "mdf": "drop", "g10": "focal"})
    s.para(M, 452, "Обрезки заготовок для плат тоже подходят. Тогда пластина стоит копейки.", W - 2 * M, 18, "ink")
    return s


# ==========================
# ===  5. Spindle        ===
# ==========================

def spindle():
    s = Slide("шпиндель", "Шпиндель 48 В без частотника",
              "Бесщёточный шпиндель с драйвером убирает утечку и главный источник помех.",
              note="spindle: brushless 48 V 500 W ER11 with a driver, up to 20000 rpm; no VFD means no leakage that "
                   "trips the RCD and no ground loop that dropped the probe USB; reverse must be checked")
    items = [("p", "500 Вт, до 20 000 об/мин", "цанга ER11, воздушное охлаждение"),
             ("rcd", "Нет утечки", "обычное УЗО здания не срабатывает"),
             ("emi", "Нет помех", "частотник рвал связь USB щупа"),
             ("rev", "Реверс", "нужен для автосмены, проверить до покупки")]
    cards(s, items, y=228, h=110, kinds={"p": "focal"})
    s.para(M, 496, "20 000 об/мин вместо 24 000 означает немного меньшую подачу V-фрезы.", W - 2 * M, 18, "ink")
    return s


def toolchange():
    s = Slide("шпиндель", "Автосмена без дорогого шпинделя",
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


def magazine():
    s = Slide("шпиндель", "Что лежит в магазине",
              "Каждый инструмент станок берёт так же, как фрезу. Рамка переворота стоит отдельно.",
              note="magazine slots: V-bit 0.1, 1 mm and 5 mm mills, 0.7 drill, cone and flat punches, paste needle, "
                   "pipette; the 520 mm flip frame lives along the back wall")
    items = [("v", "V-фреза 0.1", "изоляция"), ("m1", "Фреза 1 мм", "отверстия, контур"),
             ("m5", "Фреза 5 мм", "выравнивание пластины"), ("d07", "Сверло 0.7", "под заклёпки"),
             ("cone", "Пуансон-конус", "раскрывает хвост"), ("flat", "Плоский пуансон", "прижимает край"),
             ("ndl", "Игла с пастой", "шприц 0.4 мм"), ("pip", "Пипетка", "сборка деталей")]
    cards(s, items, y=228, cols=4, h=96, gap=20, size=20, sub_size=15, kinds={"cone": "load", "flat": "load",
                                                                             "ndl": "load"})
    s.para(M, 480, "Зелёным выделены новые инструменты. Рамка переворота длиной 520 мм в магазин не входит, "
           "она стоит вдоль задней стенки.", W - 2 * M, 18, "ink")
    return s


def rfid():
    s = Slide("шпиндель", "Метки RFID на держателях",
              "Станок читает метку и знает, что держит: фрезу, сверло, иглу или пуансон.",
              note="RFID tags in every holder, a reader at the spindle; tool identity, wear count per tool, "
                   "a wrong tool stops the machine; idea taken from NestWorks C500")
    items = [("tag", "Метка в держателе", "номер и вид инструмента"),
             ("read", "Считыватель у шпинделя", "читает метку при взятии"),
             ("life", "Учёт износа", "часы фрезы, удары пуансона, остаток пасты"),
             ("guard", "Защита от ошибки", "не тот инструмент: станок стоит")]
    cards(s, items, y=228, h=110, kinds={"guard": "focal"})
    s.para(M, 496, "Так сделано у NestWorks C500. У нас меткам больше работы: в магазине лежат не только фрезы.",
           W - 2 * M, 18, "ink")
    return s
