"""Chapter 3: two versions of the machine, Standard (300x300, cube housing) and Max (510x510)."""
from deckkit import M, W, Slide

from common import cards, table


def versions():
    s = Slide("две версии", "Standard и Max",
              "Standard ставится на стол в кубическом корпусе. Max берёт лист 500 × 500 мм целиком.",
              note="two versions side by side: table, flip lift, Z travel, flip frame, housing, mass, blank, "
                   "rival, buyer; sizes are estimates")
    rows = [("", "Standard", "Max"),
            ("Стол", "300 × 300 мм", "510 × 510 мм"),
            ("Подъём при перевороте", "до 170 мм", "до 270 мм"),
            ("Ход Z", "около 200 мм", "300 мм"),
            ("Рамка переворота", "около 320 мм", "около 520 мм"),
            ("Корпус", "куб 650 × 600 × 750 мм, на стол", "900 × 800 × 900 мм, на тумбе"),
            ("Масса", "около 100 кг", "от 150 кг"),
            ("С кем сравнивают", "Carvera, 949 тыс ₽", "линия LPKF, миллионы ₽"),
            ("Покупатель", "кванториумы, школы, фирмы", "кафедры, НИИ, панели плат")]
    table(s, M, 240, [320, 416, 416], rows, size=17, row_h=44)
    return s


def shared():
    s = Slide("две версии", "Одна голова на две рамы",
              "Голова, магазин, электроника и программа у версий общие. Разные только рама, стол и корпус.",
              note="what is shared between the versions (head, magazine, boards, Pi, software, all modules) "
                   "and what differs (frame, table, housing); the second version costs a frame, not a machine")
    same = [("head", "Голова", "шпиндель, щупы, игла, пипетка, фен"),
            ("mag", "Магазин", "те же гнёзда и инструменты"),
            ("el", "Электроника", "Pi 5, платы по CAN"),
            ("sw", "Программа", "одна для обеих версий")]
    cards(s, same, y=228, cols=1, h=76, gap=16, size=20, sub_size=15, width=560,
          kinds={k: "load" for k in ("head", "mag", "el", "sw")})
    diff = [("frm", "Рама и портал", "длина рельсов и винтов"),
            ("tbl", "Стол и пластина", "300 или 510 мм"),
            ("box", "Корпус", "куб на стол или шкаф на тумбе"),
            ("flip", "Рамка переворота", "320 или 520 мм")]
    cards(s, diff, y=228, cols=1, h=76, gap=16, size=20, sub_size=15, x=656, width=560)
    return s


def standard_first():
    s = Slide("две версии", "Standard выходит первым",
              "Почти любой прототип помещается на 300 × 300 мм. Малый станок проще сделать точным.",
              note="why Standard first: a 100x160 board fits; a short Z is stiffer; the cube goes on a desk and is "
                   "easy to ship and show; the prototype is cheaper")
    items = [("fit", "Плата 100 × 160 мм", "и почти любой прототип помещаются"),
             ("stiff", "Короткая ось Z", "точность 0.02 мм держать проще"),
             ("show", "Куб на стол", "помещается в машину, его возят на показ"),
             ("cheap", "Макет дешевле", "короче рельсы, меньше насос")]
    cards(s, items, y=228, h=110, kinds={"fit": "focal"})
    s.para(M, 496, "Max выходит вторым: та же голова на большой раме.", W - 2 * M, 18, "ink")
    return s


def pricing():
    s = Slide("две версии", "Цена по пользе, а не по железу",
              "Стол почти не меняет себестоимость. Поэтому Max стоит заметно дороже Standard.",
              note="pricing direction: Standard about 1.5 M rub or lower, near Carvera at 949k; Max clearly above 2.5 M; "
                   "the hardware difference is tens of thousands of rubles; numbers to be checked at demos")
    s.node("std", M, 236, 560, 150, "plain", "Standard: около 1.5 млн ₽", "или ниже, ближе к Carvera",
           size=26, sub_size=17)
    s.node("max", 656, 236, 560, 150, "focal", "Max: выше 2.5 млн ₽", "замена части линии LPKF",
           size=26, sub_size=17)
    s.para(M, 440, "Разница в железе составляет десятки тысяч рублей: длинные рельсы, большой портал, корпус, тумба. "
           "Max сложнее возить и обслуживать, это тоже довод за высокую цену.", W - 2 * M, 18, "ink")
    s.text(M, 560, "Это направление, а не расчёт. Цены проверит вопрос о цене при показе.", 16, "soft")
    return s
