"""The desktop PCB line, whole project: one machine from gerber to an assembled double-sided board.

Built with deckkit (~/mnt/pr/pets/deckkit): nodes have names, edges join names,
every slide carries a `note` with its meaning; explain.py holds the side column for a reader.
Slides live in chapter modules (why, frame, process, control, money); this file orders them,
adds the contents and a divider before every chapter.

    deck all docs/deck/slides.py
"""
from deckkit import M, Slide

import control
import frame
import money
import process
import versions
import why
from common import bullets, photo
from explain import EXPLAIN  # noqa: F401  deckkit reads EXPLAIN from this module

TITLE = "Плата за час"
FOOTER = "pcbmill · настольная линия для печатных плат"
FOOT_CAT = 1150

CHAPTERS = [
    ("Зачем станок", "Кому и почему нужна плата за день.",
     [why.problem, why.market, why.buyers, why.offer]),
    ("Путь платы", "Что происходит с платой от гербера до пайки.",
     [why.flow, why.order, why.factory]),
    ("Две версии", "Standard на стол и Max для листа 500 × 500.",
     [versions.versions, versions.shared, versions.standard_first, versions.pricing, versions.night_batch]),
    ("Рама и оси", "Как станок двигается и держит точность 0.02 мм.",
     [frame.own_frame, frame.layout, frame.motors, frame.rails, frame.zaxis]),
    ("Стол", "Как плата держится и что лежит под ней.",
     [frame.vacuum, frame.plate, frame.plate_material]),
    ("Шпиндель и магазин", "Чем станок режет и как меняет инструмент.",
     [frame.spindle, frame.toolchange, frame.magazine, frame.rfid]),
    ("Фрезеровка", "Дорожки, отверстия и контур платы.",
     [process.millmap, process.laser, process.isolation, process.holes, process.vcut]),
    ("Проверка двумя щупами", "Как станок сам находит замыкания и обрывы.",
     [process.probes, process.probe_accuracy, process.nets, process.rejected]),
    ("Маска", "Как маска ложится и где она вскрывается.",
     [process.mask, process.windows]),
    ("Заклёпки", "Чем станок заменяет металлизацию отверстий.",
     [process.joining, process.rivet_photo, process.rivets, process.setting, process.punch]),
    ("Переворот и кассета", "Как станок переворачивает плату, находит её и берёт следующую.",
     [process.two_flips, process.flipper, process.lift, process.wedge, process.register, process.cassette,
      process.unattended]),
    ("Паста", "Как паста ложится без трафарета.",
     [process.paste, process.heightmap, process.paste_care]),
    ("Сборка и пайка", "Как детали встают на плату и паяются.",
     [process.pnp, process.tht, process.heat, process.rules]),
    ("Управление", "Электроника и программа станка.",
     [control.brain, control.mcu, control.software, control.pi_photo]),
    ("Питание и корпус", "Розетка, свет, пыль и пары.",
     [control.power, control.blackout, control.dust, control.housing, control.chip_tray]),
    ("Деньги", "Цена, себестоимость, доход и документы.",
     [money.price, money.hardware, money.new_costs, money.economics, money.consumables, money.russia,
      money.papers]),
    ("Дальше", "Риски, проверки и следующий шаг.",
     [money.risks, money.stages, money.checks, money.next_step]),
]


def title():
    s = Slide(None, None, note="title: one desktop machine takes a board from gerber to an assembled double-sided "
                               "board; photo of a milled PCB")
    s.eyebrow(M, 228, "pcbmill · весь проект · 18.10.2026", "accent")
    s.text(M, 300, "Плата за час", 76)
    s.para(M, 368, "Один настольный станок проходит весь путь: от гербера до двухсторонней платы с деталями.",
           520, 26, "muted")
    s.text(M, 640, "Цены и сроки в презентации оценки.", 15, "soft")
    photo(s, 680, 150, 536, 402, "milledpcb.jpg", "плата, выфрезерованная на настольном станке")
    return s


def contents():
    s = Slide("содержание", f"{len(CHAPTERS)} глав",
              "Презентация идёт по станку последовательно: от рынка через узлы и процесс к деньгам.",
              note="table of contents: chapters in order")
    for i, (name, _, _) in enumerate(CHAPTERS):
        x, y = M + (i // 9) * 584, 236 + (i % 9) * 44
        s.text(x, y, f"{i + 1:02d}", 22, "accent")
        s.text(x + 56, y, name, 22, "ink")
    return s


def divider(n, name, lead, funcs):
    """Chapter opener: number, name, lead and the titles of the chapter's slides."""
    titles = [f().title for f in funcs]

    def make():
        s = Slide(None, None, note=f"chapter {n}: {name}; slides: {len(funcs)}",
                  explain=f"Глава {n} из {len(CHAPTERS)}: «{name}». {lead}\n\n"
                          f"Справа перечислены слайды главы по порядку.")
        s.eyebrow(M, 228, f"глава {n:02d} из {len(CHAPTERS)}", "accent")
        s.text(M, 310, name, 54)
        s.para(M, 364, lead, 520, 22, "muted")
        bullets(s, 680, 236, titles, 536, 20)
        return s
    make.__name__ = f"ch{n:02d}"
    return make


SLIDES = [title, contents]
for _n, (_name, _lead, _funcs) in enumerate(CHAPTERS, 1):
    SLIDES.append(divider(_n, _name, _lead, _funcs))
    SLIDES.extend(_funcs)
SLIDES.append(money.credits)
