"""Command line: cam (gerbers -> .tap), probe/map/apply/show/points (height map)."""
import argparse
import os
import re
import sys
import tomllib
import warnings

from . import level

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


# ==========================
# ===  Config            ===
# ==========================

def load_cfg(path, overrides):
    with open(path, 'rb') as f:
        cfg = tomllib.load(f)
    for item in overrides or []:
        key, _, val = item.partition('=')
        node = cfg
        *parents, leaf = key.strip().split('.')
        for p in parents:
            node = node.setdefault(p, {})
        try:
            node[leaf] = tomllib.loads('v = ' + val.strip())['v']
        except tomllib.TOMLDecodeError:
            node[leaf] = val.strip()      # bare words like mode=through
    return cfg


# ==========================
# ===  cam               ===
# ==========================

def cmd_cam(a):
    from . import board as bd, gcode, toolpath as tp, preview

    cfg = load_cfg(a.machine, a.set)
    m, tools = cfg['machine'], cfg['tools']
    raw = bd.load(a.gerbers, override={'top': a.top, 'bottom': a.bottom, 'outline': a.outline})
    sides = list(raw.copper)
    if a.side == 'auto':
        sides = sides[:1] if len(sides) == 1 else ['top']
        if len(raw.copper) > 1:
            print('плата двусторонняя, беру top; низ: --side bottom или both')
    elif a.side == 'both':
        sides = [s for s in ('top', 'bottom') if s in raw.copper]
    else:
        if a.side not in raw.copper:
            sys.exit('слоя меди %s нет, есть: %s' % (a.side, ', '.join(raw.copper)))
        sides = [a.side]

    ops_wanted = set(a.ops.split(','))
    out_dir = a.out or (os.path.join(a.gerbers, 'cnc') if os.path.isdir(a.gerbers)
                        else os.path.splitext(a.gerbers)[0] + '_cnc')
    os.makedirs(out_dir, exist_ok=True)
    name = a.name or raw.name
    hm = level.HeightMap.load(a.map) if a.map else None
    total = 0.0

    def write(op, fname, b, copper):
        nonlocal total
        tool = tools[op.tool]
        op.paths = tp.order(op.paths) if op.name != 'cutout' else op.paths
        text = gcode.emit(op, m, tool, 'pcbmill %s' % name)
        if hm is not None:
            text = level.apply_map(text.splitlines(), hm, a.seg)
        path = os.path.join(out_dir, fname)
        with open(path, 'w') as f:
            f.write(text)
        cut, minutes = gcode.stats(op, m, tool)
        total += minutes
        print('  %-28s %5d проходов, рез %6.0f мм, ~%3.0f мин' % (fname, len(op.paths), cut, minutes))
        for s in op.notes:
            print('      ' + s)

    first = True
    for side in sides:
        b = bd.transform(raw, cfg['board']['origin'], cfg['board']['margin'], mirror=(side == 'bottom'))
        copper = b.copper[side]
        x0, y0, x1, y1 = b.outline.bounds
        print('%s: плата %.1f×%.1f мм, X %.2f..%.2f Y %.2f..%.2f%s'
              % (side, x1 - x0, y1 - y0, x0, x1, y0, y1, ', зеркально' if side == 'bottom' else ''))
        ops = []
        if 'iso' in ops_wanted:
            c = cfg['isolation']
            op = tp.isolation(copper, b.outline, tools[c['tool']], c)
            write(op, '%s_%s_iso.tap' % (name, side), b, copper)
            ops.append(op)
        # holes and outline are milled once, from the first side
        if first and 'drill' in ops_wanted and b.holes:
            c = cfg['drill']
            op = tp.drilling(b.holes, tools[c['tool']], c, cfg['board']['thickness'])
            write(op, '%s_%s_drill.tap' % (name, side), b, copper)
            ops.append(op)
        if first and 'cut' in ops_wanted:
            c = cfg['cutout']
            op = tp.cutout(b.outline, tools[c['tool']], c, cfg['board']['thickness'])
            write(op, '%s_%s_cut.tap' % (name, side), b, copper)
            ops.append(op)
        png = os.path.join(out_dir, '%s_%s.png' % (name, side))
        preview.render(b, copper, ops, png, '%s %s' % (name, side))
        print('  превью %s' % png)
        first = False
    print('всего ~%.0f мин резания' % total)


# ==========================
# ===  height map        ===
# ==========================

def tap_lines(path):
    with open(path) as f:
        return f.readlines()


def cmd_probe(a):
    b = level.cut_bounds(tap_lines(a.tap))
    xs, ys, pts = level.grid_points(b, a.step, a.margin)
    with open(a.out, 'w') as f:
        f.write(level.probe_program(pts, a.dialect, a.clear, a.depth, a.feed, a.log))
    print('рабочая зона X %.2f..%.2f  Y %.2f..%.2f' % b)
    print('сетка %d×%d = %d касаний → %s' % (len(xs), len(ys), len(pts), a.out))


def cmd_points(a):
    b = level.cut_bounds(tap_lines(a.tap))
    _, _, pts = level.grid_points(b, a.step, a.margin)
    with open(a.out, 'w') as f:
        f.write('# впишите Z касания третьим числом; первая точка задаёт ноль\n')
        for x, y in pts:
            f.write('%.3f,%.3f,\n' % (x, y))
    print('%d точек → %s' % (len(pts), a.out))


def cmd_map(a):
    off = tuple(float(t) for t in a.offset.split(',')) if a.offset else None
    hm, off, n = level.build_map(a.log, tap_lines(a.tap) if a.tap else None, a.margin, off, a.tol)
    hm.save(a.out)
    if off != (0.0, 0.0):
        print('сдвиг лога относительно программы X %.3f Y %.3f' % off)
    print('касаний %d → %s' % (n, a.out))
    print(hm.table())


def cmd_show(a):
    print(level.HeightMap.load(a.map).table())


def cmd_apply(a):
    hm = level.HeightMap.load(a.map)
    ref = tuple(float(t) for t in a.ref.split(',')) if a.ref else None
    out = a.out or re.sub(r'(\.\w+)?$', r'_level\1', a.tap, count=1)
    text = level.apply_map(tap_lines(a.tap), hm, a.seg, 0.0, a.strip_m6, ref)
    with open(out, 'w') as f:
        f.write(text)
    print('%s → %s, строк %d' % (a.tap, out, text.count('\n')))


# ==========================
# ===  Arguments         ===
# ==========================

def main():
    warnings.simplefilter('ignore', SyntaxWarning)
    p = argparse.ArgumentParser(prog='pcbmill', description='G-code для фрезеровки печатных плат')
    sp = p.add_subparsers(dest='cmd', required=True)

    g = sp.add_parser('cam', help='гербера и сверловка → .tap')
    g.add_argument('gerbers', help='папка с Gerber/Excellon (или zip)')
    g.add_argument('-o', '--out', help='куда писать (по умолчанию <папка>/cnc)')
    g.add_argument('-n', '--name', help='имя для файлов')
    g.add_argument('-m', '--machine', default=os.path.join(HERE, 'machine.toml'))
    g.add_argument('--top', help='файл меди сверху, если не распознался сам')
    g.add_argument('--bottom', help='файл меди снизу')
    g.add_argument('--outline', help='файл контура платы')
    g.add_argument('--side', default='auto', choices=['auto', 'top', 'bottom', 'both'])
    g.add_argument('--ops', default='iso,drill,cut', help='iso,drill,cut')
    g.add_argument('--map', help='карта высот: сразу выдать выровненные программы')
    g.add_argument('--seg', type=float, default=1.0, help='шаг дробления под карту, мм')
    g.add_argument('-s', '--set', action='append', metavar='KEY=VAL',
                   help='переопределить профиль: -s isolation.depth=0.12')
    g.set_defaults(fn=cmd_cam)

    g = sp.add_parser('probe', help='программа щупа по габариту .tap')
    g.add_argument('tap'); g.add_argument('-o', '--out', default='probe.nc')
    g.add_argument('--dialect', choices=['grbl', 'mach3', 'linuxcnc'], default='grbl')
    g.add_argument('--step', type=float, default=10, help='шаг сетки, мм')
    g.add_argument('--margin', type=float, default=0, help='вылет сетки за рабочую зону, мм')
    g.add_argument('--clear', type=float, default=1.5, help='высота переездов, мм')
    g.add_argument('--depth', type=float, default=2, help='на сколько вниз искать касание, мм')
    g.add_argument('--feed', type=float, default=30, help='подача щупа, мм/мин')
    g.add_argument('--log', default='probe.txt', help='файл лога (LinuxCNC)')
    g.set_defaults(fn=cmd_probe)

    g = sp.add_parser('points', help='шаблон CSV для ручного замера')
    g.add_argument('tap'); g.add_argument('-o', '--out', default='manual.csv')
    g.add_argument('--step', type=float, default=20)
    g.add_argument('--margin', type=float, default=0)
    g.set_defaults(fn=cmd_points)

    g = sp.add_parser('map', help='лог касаний → карта высот')
    g.add_argument('log'); g.add_argument('-o', '--out', default='heightmap.csv')
    g.add_argument('--tap', help='программа, по которой строилась сетка: совместить с ней X/Y лога')
    g.add_argument('--margin', type=float, default=0, help='тот же margin, что в probe')
    g.add_argument('--offset', help='X,Y рабочего нуля в координатах лога; писать --offset=-200,-150')
    g.add_argument('--tol', type=float, default=0.3, help='склейка координат сетки, мм')
    g.set_defaults(fn=cmd_map)

    g = sp.add_parser('show', help='напечатать карту')
    g.add_argument('map'); g.set_defaults(fn=cmd_show)

    g = sp.add_parser('apply', help='наложить карту на готовую программу')
    g.add_argument('tap'); g.add_argument('map'); g.add_argument('-o', '--out')
    g.add_argument('--seg', type=float, default=1.0, help='макс. длина отрезка, мм')
    g.add_argument('--ref', help='X,Y точки, где после смены фрезы заново выставлен Z0')
    g.add_argument('--strip-m6', action='store_true', help='закомментировать T/M6 (GRBL на них ругается)')
    g.set_defaults(fn=cmd_apply)

    a = p.parse_args()
    a.fn(a)
