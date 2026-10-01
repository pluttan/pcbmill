"""Commissioning programs: confirm the machine moves the way pcbmill assumes
before the first real board. Each step checks one assumption and pauses (M0)
so a wrong direction or scale is caught at a safe height."""
import math
import os

from shapely.geometry import box
from shapely.ops import unary_union

from . import gcode, toolpath as tp
from .gcode import comment


def head(title, notes):
    return [comment(title)] + [comment(s) for s in notes] + ['G21G90G17G94']


def probe_word(cfg):
    return 'G31' if cfg['machine']['dialect'] == 'mach3' else 'G38.2'


def zero_word(cfg):
    return 'G92Z0' if cfg['machine']['dialect'] == 'mach3' else 'G10L20P1Z0'


# ==========================
# ===  Air moves         ===
# ==========================

def axes(cfg):
    out = head('00 оси: шпиндель выключен, фреза над столом не меньше чем на 15 мм', [
        'ноль X0 Y0 Z0 выставить в текущем положении',
        'после каждой паузы M0 сверить движение с комментарием и нажать старт'])
    steps = [('G0Z10', 'Z вверх на 10 мм: фреза ДОЛЖНА подняться'),
             ('G0X20', 'X плюс 20: стол или портал так, что фреза идёт ВПРАВО относительно заготовки'),
             ('G0Y20', 'Y плюс 20: фреза идёт ОТ ВАС, к дальнему краю'),
             ('G0X0Y0', 'обратно в ноль по X и Y'),
             ('G0Z0', 'Z обратно на исходную высоту')]
    for g, msg in steps:
        out += [comment(msg), g, 'M0']
    return out + ['M2']


def square(cfg):
    w, h = 80, 50
    out = head('01 масштаб: шпиндель выключен, Z0 на 5 мм над заготовкой', [
        'положить линейку вдоль X, отметить кончик фрезы в каждой паузе',
        'между паузами 1 и 2 должно быть ровно %d мм, между 2 и 3 ровно %d мм' % (w, h),
        'три обхода подряд: в последней паузе фреза должна встать точно над первой отметкой'])
    out += ['G0Z5', 'G0X0Y0', comment('пауза 1: начало'), 'M0', 'G1Z2F300',
            'G1X%dF600' % w, comment('пауза 2: прошли X'), 'M0', 'G1Y%d' % h, comment('пауза 3: прошли Y'), 'M0']
    for _ in range(3):
        out += ['G1X0', 'G1Y0', 'G1X%d' % w, 'G1Y%d' % h, 'G1X%dY0' % (w / 2), 'G1X0Y0']
    out += ['G0Z5', comment('пауза 4: фреза над исходной отметкой, расхождение = пропуск шагов'), 'M0', 'M2']
    return out


def spindle(cfg):
    s = cfg['machine']['spindle']
    return head('02 шпиндель: фреза в воздухе', [
        'сначала половина оборотов, потом рабочие, вращение по часовой, если смотреть сверху']) + [
        'G0Z10', 'S%dM3' % (s // 2), 'G4P5', 'S%d' % s, 'G4P5', 'M5', 'G4P3',
        comment('пауза: шпиндель должен был остановиться'), 'M0', 'M2']


# ==========================
# ===  Probe             ===
# ==========================

def probe(cfg):
    p, z0 = probe_word(cfg), zero_word(cfg)
    out = head('03 щуп: шпиндель выключен, зажим на меди, фреза на 2-3 мм над медью', [
        'ноль Z0 выставить в текущем положении (над медью)',
        'перед стартом коснуться зажимом фрезы: станок должен показать срабатывание щупа',
        'первое касание станет нулём Z, два следующих должны показать Z около 0.000',
        'разброс больше 0.02 мм: грязная медь или люфт по Z'])
    out += ['G0Z0', '%sZ-5F20' % p, z0, 'G0Z1']
    for _ in range(2):
        out += ['%sZ-2F20' % p, 'G0Z1']
    out += ['G0Z3', comment('Z0 теперь на поверхности меди; не сбрасывать до 04 и 05'), 'M2']
    return out


# ==========================
# ===  Test cuts         ===
# ==========================

def ladder(cfg):
    tool = cfg['tools'][cfg['isolation']['tool']]
    m = cfg['machine']
    depths = [0.02 * k for k in range(1, 9)]
    notes = ['шпиндель включится, Z0 на меди (после 03), кусок заготовки не жалко',
             'восемь линий по 15 мм, снизу вверх глубина 0.02 … 0.16 через 0.02',
             'замерить ширину каждой под лупой; ожидаемая ширина:']
    notes += ['  глубина %.2f: %.3f мм' % (d, tp.cut_width(tool, d)) for d in depths]
    notes += ['линии, которые не прорезали медь насквозь, показывают, где медь кончается',
              'если левый и правый концы прорезаны по-разному, заготовка лежит с наклоном']
    out = head('04 лесенка глубин', notes)
    out += ['G0Z%s' % gcode.n(m['safe_z']), 'G0X0Y0S%dM3' % m['spindle'], 'G4P%s' % m['spinup']]
    for i, d in enumerate(depths):
        y = 2.0 * i
        out += ['G0Z1', 'G0X0Y%s' % gcode.n(y), 'G1Z%sF%.1f' % (gcode.n(-d), tool['plunge']),
                'G1X15F%.1f' % tool['feed']]
    return out + ['G0Z%s' % gcode.n(m['safe_z']), 'G0X0Y0', 'M5', 'M30']


def gaps(cfg):
    """Pairs of copper strips with shrinking gaps, isolated exactly like a real board."""
    gaps = [0.5, 0.4, 0.3, 0.25, 0.2, 0.15]
    shapes, x = [], 20.0
    for g in gaps:
        shapes += [box(x, 0, x + 1.0, 8), box(x + 1.0 + g, 0, x + 2.0 + g, 8)]
        x += 2.0 + g + 2.5
    copper = unary_union(shapes)
    outline = box(18, -2, x, 10)
    c = cfg['isolation']
    tool = cfg['tools'][c['tool']]
    op = tp.isolation(copper, outline, tool, c)
    op.paths = tp.order(op.paths, (20, 0))
    op.notes.insert(0, 'пары полосок 1 мм с зазорами %s мм, справа от лесенки (X от 20)'
                    % ', '.join('%g' % g for g in gaps))
    op.notes.insert(1, 'зазор прорезан, если тестер не звонит между полосками пары')
    return gcode.emit(op, cfg['machine'], tool, '05 зазоры').splitlines()


PROGRAMS = [
    ('00_axes.nc', axes, 'test'),
    ('01_square.nc', square, 'test'),
    ('02_spindle.nc', spindle, 'test'),
    ('03_probe.nc', probe, 'probe'),
    ('04_ladder.nc', ladder, 'iso'),
    ('05_gaps.nc', gaps, 'iso'),
]


CHECKLIST = """Пробный запуск станка: pcbmill selftest
Контроллер в профиле: {dialect}. Сначала убедиться, что это он.

0. Какой контроллер
   GRBL: в консоли отправителя (Candle, UGS, bCNC) команда $I печатает версию [VER:1.1…].
   Тогда же $$ покажет настройки:
     $100 $101 $102  шагов на мм по X Y Z (масштаб; проверяется программой 01)
     $3              инверсия направлений (проверяется программой 00)
     $130 $131 $132  ход осей, мм: вписать в machine.toml → limits
     $110 $111 $112  максимальные скорости, мм/мин: меньшее из них → max_feed
     $30             максимальные обороты → max_spindle
   Mach3 или LinuxCNC: поменять dialect в machine.toml и заново pcbmill selftest.
   Щуп в GRBL: команда ? при касании зажимом фрезы показывает Pn:P.

1. 00_axes.nc     оси и направления, без шпинделя, в воздухе
2. 01_square.nc   масштаб 80×50 мм линейкой и пропуск шагов, без шпинделя
3. 02_spindle.nc  шпиндель: пуск, обороты, остановка
4. 03_probe.nc    щуп: три касания в одной точке, разброс не больше 0.02 мм.
                  После него Z0 стоит на меди.
5. 04_ladder.nc   лесенка глубин: реальная ширина реза против ожидаемой
6. 05_gaps.nc     пары дорожек с зазорами 0.5…0.15 мм: какой зазор станок реально режет

Каждая программа останавливается паузой M0 там, где надо посмотреть.
Не то направление или не тот размер: стоп, программу дальше не запускать,
исправить настройку контроллера или профиль и начать с 00.

Перед любой своей программой: pcbmill check файл.tap
"""


def write_all(out_dir, cfg, guard):
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, 'CHECKLIST.txt'), 'w') as f:
        f.write(CHECKLIST.format(dialect=cfg['machine']['dialect']))
    for name, fn, kind in PROGRAMS:
        path = os.path.join(out_dir, name)
        with open(path, 'w') as f:
            f.write('\n'.join(fn(cfg)) + '\n')
        guard(path, cfg, kind)
        title = next(l for l in open(path) if l.startswith('('))
        print('  %-14s %s' % (name, title.strip()[1:-1]))
    print('порядок: 00 → 05, каждая следующая только если предыдущая прошла как написано')
    print('что смотреть на каждом шаге: %s' % os.path.join(out_dir, 'CHECKLIST.txt'))
