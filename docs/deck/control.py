"""Chapters 13-14: control electronics and software, power, dust and the housing."""
from deckkit import M, W, Slide

from common import bullets, cards, photo


# ==========================
# ===  13. Control       ===
# ==========================

def brain():
    s = Slide("управление", "Raspberry Pi 5 и Klipper",
              "Pi считает и показывает, платы на столе и голове двигают моторы. Связь идёт по CAN.",
              note="control architecture: screen -> Raspberry Pi 5 with Klipper -> CAN -> table board (drivers X Y Z) "
                   "and head board (TMC2209 for pipette Z and rotation, Dynamixel servos, probe input)")
    s.node("screen", M, 236, 300, 72, "plain", "Экран 7–10\"", "KlipperScreen", size=20, sub_size=15)
    s.node("pi", M, 352, 300, 132, "focal", "Raspberry Pi 5", "Klipper, карта высот, задания", size=24)
    s.node("tboard", 470, 252, 320, 100, "load", "Плата стола", "STM32G0B1, по CAN", size=21)
    s.node("hboard", 470, 452, 320, 100, "load", "Плата головы", "STM32G0B1, по CAN", size=21)
    s.node("drv", 900, 252, 316, 100, "plain", "Драйверы X, Y, Z", "NEMA23 с энкодером", size=20)
    s.node("tmc", 900, 412, 316, 56, "plain", "TMC2209: пипетка", size=17)
    s.node("dyn", 900, 480, 316, 56, "plain", "Dynamixel: щупы", size=17)
    s.node("probe", 900, 548, 316, 56, "plain", "Щуп касания", size=17)
    s.edge("pi", "screen")
    s.edge("pi", "tboard", "accent", width=2)
    s.edge("pi", "hboard", "accent", width=2)
    s.edge("tboard", "drv")
    s.edge("hboard", "tmc", mid=845)
    s.edge("hboard", "dyn", mid=845)
    s.edge("hboard", "probe", mid=845)
    return s


def mcu():
    s = Slide("управление", "Зачем микроконтроллер",
              "Pi не выдаёт шаги и не ловит касание с точностью до микросекунд. Это делают платы.",
              note="why MCU boards: step timing and touch capture need microseconds; if the Pi hangs the MCU stops the "
                   "machine; CAN loss stops Klipper; FDCAN resists noise; Pico for the prototype, STM32 for the series")
    items = [("time", "Микросекунды", "шаги моторов и момент касания"),
             ("stop", "Зависла Pi", "плата сама останавливает станок"),
             ("can", "Обрыв CAN", "Klipper останавливает станок"),
             ("proto", "Pico для макета", "STM32G0B1 с FDCAN для серии")]
    cards(s, items, y=228, h=110, kinds={"stop": "focal"})
    s.para(M, 496, "Отечественный микроконтроллер дал бы баллы реестра 719, но Klipper его не поддерживает.",
           W - 2 * M, 18, "ink")
    return s


def software():
    s = Slide("управление", "Программа",
              "Логика pcbscan переходит в модули Klipper. Pi заново считает программу обработки под положение платы.",
              note="software: pcbscan logic into Klipper modules; height map as bed_mesh, sheet zero by SET_GCODE_OFFSET, "
                   "tool length macro, Dynamixel module, programs recomputed after each registration; offline updates")
    items = [("mesh", "Карта высот", "bed_mesh в Klipper"),
             ("zero", "Ноль листа", "SET_GCODE_OFFSET"),
             ("len", "Длина фрезы", "макрос после замера щупом"),
             ("dyn", "Модуль Dynamixel", "щупы проверки"),
             ("re", "Пересчёт задания", "после каждой привязки платы"),
             ("off", "Работа без сети", "обновления с флешки")]
    cards(s, items, y=224, h=96, size=22, kinds={"re": "focal"})
    return s


def pi_photo():
    s = Slide("управление", "Свои платы",
              "Покупные платы принтеров избыточны: внешние драйверы и так стоят отдельно.",
              note="own boards: printer boards with built-in TMC at 4-8k are overkill, own STM32G0B1 board 1.5-3k; "
                   "photos of Raspberry Pi 5 and Pico")
    photo(s, M, 216, 552, 330, "pi5.jpg", "Raspberry Pi 5")
    photo(s, 664, 216, 552, 330, "pico.jpg", "Raspberry Pi Pico для макета")
    s.para(M, 600, "Своя плата на STM32G0B1 стоит 1.5–3 тыс ₽. Плата принтера с драйверами TMC стоит 4–8 тыс ₽.",
           W - 2 * M, 17, "ink")
    return s


# ==========================
# ===  14. Power         ===
# ==========================

def power():
    s = Slide("питание", "Одна вилка, всё внутри",
              "Станок ставится хоть в подвал: обычная розетка, никаких внешних щитов и пылесосов.",
              note="power: 48 V brushless spindle without a VFD (no leakage, no noise), one 100-240 V supply, "
                   "ride-through on power loss, built-in dust extraction; 1.3-1.6 kW")
    blocks = [("spindle", "Шпиндель 48 В", "без частотника: УЗО не срабатывает"),
              ("psu", "Блок 100–240 В", "работает при просадках сети"),
              ("hold", "Запас на 2–5 с", "свет пропал: Z вверх, место запомнено"),
              ("dust", "Пылесос и вытяжка", "встроены, с фильтрами")]
    cards(s, blocks, y=224, h=104, size=22)
    s.node("total", M, 488, W - 2 * M, 84, "focal", "1.3–1.6 кВт, 6–7 А", "хватит любой розетки", size=24, sub_size=16)
    return s


def blackout():
    s = Slide("питание", "Если пропал свет",
              "Запас энергии в блоке питания даёт станку 2–5 с, чтобы уйти из платы.",
              note="power loss: energy reserve 2-5 s lifts Z, stops the spindle, saves the position; after power returns "
                   "the job continues; a lamp warns when there is no ground")
    s.node("loss", M, 260, 260, 104, "drop", "Свет пропал", size=22)
    s.node("up", 360, 260, 260, 104, "plain", "Z вверх", "фреза выходит из платы", size=22)
    s.node("save", 656, 260, 260, 104, "plain", "Место запомнено", "шпиндель остановлен", size=22)
    s.node("cont", 952, 260, 264, 104, "focal", "Продолжить", "после включения", size=22)
    s.edge("loss", "up")
    s.edge("up", "save")
    s.edge("save", "cont")
    bullets(s, M, 430, [
        "Лампа «нет заземления» предупреждает. Утечка мала, поэтому станок безопасен и без заземления.",
        "Блок 48 В с активным ККМ, плавный пуск, сетевой фильтр, контактор под аварийной кнопкой.",
    ], W - 2 * M)
    return s


def dust():
    s = Slide("питание", "Пыль, пары и корпус",
              "Фрезеровка пылит, а пасте и сборке нужна чистота. Корпус разделяет эти задачи.",
              note="dust and fumes: built-in brushless vacuum with HEPA, flux fume extraction with carbon filter, "
                   "closed electronics bay with filter, lacquered boards, humidity sensor, noise insulation, light inside")
    items = [("vac", "Пылесос HEPA", "встроен, бесщёточный"),
             ("fume", "Вытяжка флюса", "угольный фильтр"),
             ("bay", "Отсек электроники", "закрыт, с фильтром, платы под лаком"),
             ("hum", "Датчик влажности", "предупреждает о конденсате"),
             ("noise", "Шумоизоляция", "кожух со светом"),
             ("risk", "Пыль против чистоты", "главный риск всего состава")]
    cards(s, items, y=224, h=96, size=22, kinds={"risk": "focal"})
    return s
