"""Chapters 15-16: money and documents, risks, open checks and the next step."""
from deckkit import M, W, Slide, row

from common import cards, photo, table


# ==========================
# ===  15. Money         ===
# ==========================

def price():
    s = Slide("деньги", "Цена против рынка",
              "Ниже набора LPKF, а умеет больше Carvera и Voltera.",
              note="price positioning: Standard about 1.5 M rub, Max above 2.5 M, LPKF line several M, "
                   "Carvera 0.95 M (no paste, no test), Voltera EUR 3,850-5,400 (no milling)")
    rivals = [("std", "Standard", "около 1.5 млн ₽", "весь путь платы", "focal"),
              ("max", "Max", "выше 2.5 млн ₽", "лист 500 × 500", "focal"),
              ("lpkf", "Линия LPKF", "несколько млн ₽", "несколько машин", "plain"),
              ("carv", "Carvera", "0.95 млн ₽", "без пасты и проверки", "plain"),
              ("volt", "Voltera", "€3,850–5,400", "не фрезерует", "plain")]
    for i, (nid, name, cost, what, kind) in enumerate(rivals):
        x = M + i * 235
        s.node(nid, x, 236, 211, 200, kind)
        s.text(x + 105, 284, name, 21, "ink", "middle", owner=nid)
        s.text(x + 105, 340, cost, 20, "ink", "middle", owner=nid)
        s.text(x + 105, 392, what, 14, "muted", "middle", owner=nid)
    s.para(M, 500, "Цены LPKF дилеры дают под заказ, поэтому для неё только оценка.", W - 2 * M, 16)
    return s


def hardware():
    s = Slide("деньги", "Железо одного станка",
              "Оценка по группам, тыс ₽. Её считали под стол 300 × 300, ближе к Standard, до решений о заклёпках и игле.",
              note="hardware estimate by group, 219-399 k rub total, computed before rivets, needle, hot air, "
                   "the flip frame and the 510 table; must be recomputed")
    left = [("Группа", "тыс ₽"), ("Механика", "74–132"), ("Шпиндель", "5–15"), ("Питание", "15–25"),
            ("Стол и вакуум", "13–25"), ("Автосмена и фрезы", "11–21"), ("Два щупа", "9–15"), ("Камеры", "8–14")]
    right = [("Группа", "тыс ₽"), ("Сборка деталей", "15–28"), ("Паста", "4–8"), ("Маска", "4–10"),
             ("Пайка и вытяжка", "8–16"), ("Электрика", "3–6"), ("Управление", "24–39"), ("Пыль и кожух", "26–45")]
    table(s, M, 236, [340, 200], left, size=18, row_h=42, accent_col=1)
    table(s, 676, 236, [340, 200], right, size=18, row_h=42, accent_col=1)
    s.node("sum", M, 584, W - 2 * M, 56, "focal", "Итого 219–399 тыс ₽, ждёт пересчёта", size=20)
    return s


def new_costs():
    s = Slide("деньги", "Что добавить в расчёт",
              "Новые узлы ещё не посчитаны. Их цены ждут поиска и опыта.",
              note="items not yet costed: punches, positive displacement dispenser, hot air head, flip frame, "
                   "Z travel 300 and the 510 table, G10 plates, RFID, laser sensor, blank cassette")
    items = [("p", "Пуансоны", "конус и плоский, расходник"),
             ("d", "Дозатор вытеснения", "мотор, поршень, шприцы"),
             ("f", "Фен на голове", "нагреватель и вентилятор"),
             ("fr", "Рамка переворота", "клинья, привод поворота"),
             ("z", "Z 300 и стол 510", "длиннее рельсы и винты"),
             ("g", "Пластины G10", "первый комплект"),
             ("rf", "RFID", "метки и считыватель"),
             ("ls", "Лазерный датчик", "расстояние до платы"),
             ("cs", "Кассета", "полки, датчики дыма")]
    cards(s, items, y=224, cols=3, h=96, size=21, sub_size=15,
          kinds={k: "load" for k in ("p", "d", "f", "fr", "z", "g", "rf", "ls", "cs")})
    return s


def economics():
    s = Slide("деньги", "Экономика одного станка",
              "Железо стоит в 4–7 раз меньше цены. Главные затраты в разработке.",
              note="unit economics: hardware 219-399 k rub, assembly 40-80 h, price Standard 1.5 M and Max 2.5+ M; "
                   "development 3-6 M rub over 1-2 years; 10 machines a year is 15-25 M rub depending on the mix")
    nums = [("hw", "219–399 тыс ₽", "железо одного станка"),
            ("labor", "40–80 ч", "сборка одного станка"),
            ("sale", "1.5 и 2.5+ млн ₽", "Standard и Max"),
            ("dev", "3–6 млн ₽", "разработка, 1–2 года")]
    for i, (nid, big, sub) in enumerate(nums):
        s.node(nid, M + i * 292, 236, 268, 140, "focal" if nid == "sale" else "plain", big, sub,
               size=28, sub_size=15)
    s.node("year", M, 432, W - 2 * M, 96, "load", "10 станков в год ≈ 15–25 млн ₽ выручки",
           "больше Max: выше выручка; покупатели через закупки", size=24, sub_size=16)
    return s


def consumables():
    s = Slide("деньги", "Доход после продажи",
              "Станок покупают один раз. Расходники покупают каждый месяц.",
              note="consumables: G10 plates with our logo, rivets, paste syringes, mills and drills, copper blanks, training")
    items = [("plate", "Пластины G10", "с логотипом, десятки плат на одну"),
             ("riv", "Заклёпки", "сотни на плату"),
             ("paste", "Шприцы пасты", "тип 4–5 для дозирования"),
             ("mill", "Фрезы и свёрла", "V-фреза, 1 мм, 0.7 мм"),
             ("blank", "Заготовки", "текстолит с медью"),
             ("learn", "Обучение", "методичка под кружок")]
    cards(s, items, y=224, h=96, size=22, kinds={"plate": "focal"})
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
    items = [("svc", "Сервис здесь", "запчасти и ремонт за дни"),
             ("cust", "Под заказчика", "поле, головки, методичка"),
             ("cond", "Условие", "свои плата, программа, сборка")]
    cards(s, items, y=420, cols=3, h=104, size=21, kinds={"cond": "focal"})
    return s


def papers():
    s = Slide("деньги", "Документы",
              "Без декларации станок не продать в тендер. Без реестра нет преимущества в закупках.",
              note="paperwork: EAEU declaration under TR CU 010/004/020 30-80k, registry 719 conclusion from Minpromtorg, "
                   "sole proprietor on 6% tax; total 0.2-0.5 M rub")
    items = [("decl", "Декларация ТР ТС", "010, 004, 020; 30–80 тыс ₽"),
             ("reg", "Заключение для реестра 719", "Минпромторг, операции в России"),
             ("ip", "ИП на УСН 6 %", "самозанятым перепродавать нельзя"),
             ("all", "Всего 0.2–0.5 млн ₽", "декларация, реестр, документация")]
    cards(s, items, y=228, h=110, kinds={"all": "focal"})
    return s


# ==========================
# ===  16. Next          ===
# ==========================

def risks():
    s = Slide("дальше", "Риски",
              "Каждый модуль сам по себе отдельный продукт. Это главный риск всей затеи.",
              note="risks: paste and assembly took Voltera and OpenPnP years; dust vs cleanliness; registry 719 demands; "
                   "Makera may enter Russia; the market is narrow; support 3-5 h per machine in year one")
    items = [("mod", "Каждый модуль продукт", "паста и сборка: годы работы Voltera"),
             ("dust", "Пыль и чистота", "в одном корпусе"),
             ("reg", "Реестр 719", "требования к операциям в России"),
             ("mk", "Makera придёт в Россию", "довод «дешевле» ослабнет"),
             ("narrow", "Узкий рынок", "станок покупают один раз"),
             ("sup", "Поддержка", "3–5 ч на станок в первый год")]
    cards(s, items, y=224, h=96, size=22, kinds={"mod": "focal"})
    return s


def stages():
    s = Slide("дальше", "Продавать ступенями",
              "Модули выходят по очереди. Каждая ступень продаётся сама.",
              note="sell in stages: first milling, vacuum, tool change and net test; then mask and rivets; "
                   "paste and assembly as expensive modules last")
    row(s, ["s1", "s2", "s3"], M, 260, 352, 150, 48, lambda i: "focal" if i == 0 else "plain",
        ["Ступень 1", "Ступень 2", "Ступень 3"],
        ["фрезеровка, вакуум, автосмена, щупы", "маска и заклёпки", "паста, сборка, пайка"], size=24)
    s.para(M, 480, "Первая ступень уже закрывает главную боль: двухсторонняя плата за день без завода.",
           W - 2 * M, 19, "ink")
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
    cards(s, items, y=224, h=96, size=22, kinds={"q1": "focal"})
    return s


def next_step():
    s = Slide("дальше", "Следующий шаг",
              "Один станок делает одну плату целиком, затем показ трём организациям.",
              note="next step: full cycle on one real board, then a demo to three organisations with the price question")
    row(s, ["proto", "cycle", "demo"], M, 260, 352, 120, 48, lambda i: "focal" if i == 1 else "plain",
        ["Модули на своём станке", "Полный цикл на плате", "Показ трём организациям"],
        ["автосмена, заклёпки, игла", "от гербера до пайки", "вопрос о цене двух версий"], size=21)
    photo(s, M, 440, 552, 176, "feeder.jpg", "ленты с деталями для сборки")
    s.para(680, 470, "Если две организации готовы купить, делаем раму, декларацию и заявку в реестр 719.",
           536, 19, "ink")
    return s


def credits():
    s = Slide("источники", "Фотографии",
              "Wikimedia Commons, свободные лицензии; рендер платы стенда свой.",
              note="photo credits from img/credits.tsv")
    rows = []
    with open(__file__.replace("money.py", "img/credits.tsv")) as f:
        for line in f:
            _, title, author, lic = line.rstrip("\n").split("\t")
            rows.append(f"{title[:48]} · {author[:32]} · {lic}")
    for i, r in enumerate(rows):
        s.text(M + (i // 7) * 584, 228 + (i % 7) * 52, r, 13, "muted")
    return s
