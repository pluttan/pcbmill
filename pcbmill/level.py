#!/usr/bin/env python3
"""pcbprobe: карта высот платы для фрезеровки печатных плат без автоуровня.

Три шага:
  grid  — по программе .tap строит сетку точек и пишет программу щупа;
  map   — из лога касаний (GRBL, Mach3, LinuxCNC или ручной CSV) собирает карту;
  apply — режет рабочие ходы на короткие отрезки и добавляет к Z высоту рельефа.

Плюс show (посмотреть карту) и points (шаблон для ручного замера).
"""
import argparse
import math
import re
import sys

WORD = re.compile(r'([A-Za-z])\s*(-?\d*\.?\d+)')
NUM = re.compile(r'-?\d+\.?\d*(?:[eE]-?\d+)?')


# ---------- разбор G-кода ----------

def words(line):
    """Слова строки без комментариев: [('G', 1.0), ('X', 3.2), ...]."""
    line = re.sub(r'\(.*?\)', '', line).split(';')[0]
    return [(l.upper(), float(v)) for l, v in WORD.findall(line)]


def cut_bounds(path, cut_z=0.0):
    """Габарит рабочих ходов (где Z ниже cut_z)."""
    x = y = z = 0.0
    xs, ys = [], []
    for line in open(path):
        for l, v in words(line):
            if l == 'X': x = v
            elif l == 'Y': y = v
            elif l == 'Z': z = v
            elif l == 'G' and int(v) in (2, 3):
                sys.exit('дуги G2/G3 не поддерживаются, выгрузите программу отрезками')
        if z < cut_z:
            xs.append(x); ys.append(y)
    if not xs:
        sys.exit('в программе нет ходов ниже Z0')
    return min(xs), max(xs), min(ys), max(ys)


# ---------- сетка и программа щупа ----------

def axis(lo, hi, step):
    n = max(1, math.ceil((hi - lo) / step))
    return [lo + (hi - lo) * i / n for i in range(n + 1)]


def grid_points(b, step, margin):
    x0, x1, y0, y1 = b[0] - margin, b[1] + margin, b[2] - margin, b[3] + margin
    xs, ys = axis(x0, x1, step), axis(y0, y1, step)
    pts = []
    for j, y in enumerate(ys):              # змейкой, чтобы меньше ездить
        row = xs if j % 2 == 0 else xs[::-1]
        pts += [(x, y) for x in row]
    return xs, ys, pts


def probe_program(pts, dialect, clear, depth, feed, logname):
    probe = 'G31' if dialect == 'mach3' else 'G38.2'
    out = ['(pcbprobe: карта высот, %d точек)' % len(pts),
           'G21 G90 G17', 'G0 Z%.3f' % clear]
    if dialect == 'mach3':
        out.append('M40 (открыть файл записи касаний)')
    if dialect == 'linuxcnc':
        out.append('(PROBEOPEN %s)' % logname)
    x, y = pts[0]
    # первая точка задаёт ноль Z: от неё отсчитывается вся карта
    out += ['G0 X%.3f Y%.3f' % (x, y), '%s Z%.3f F%.0f' % (probe, -depth, feed),
            'G92 Z0' if dialect == 'mach3' else 'G10 L20 P1 Z0',
            'G0 Z%.3f' % clear]
    for x, y in pts:
        out += ['G0 X%.3f Y%.3f' % (x, y),
                '%s Z%.3f F%.0f' % (probe, -depth, feed),
                'G0 Z%.3f' % clear]
    if dialect == 'mach3':
        out.append('M41 (закрыть файл)')
    if dialect == 'linuxcnc':
        out.append('(PROBECLOSE)')
    out += ['G0 X%.3f Y%.3f' % pts[0], 'M2']
    return '\n'.join(out) + '\n'


# ---------- карта высот ----------

def read_probe_log(path):
    """Тройки x,y,z из лога. GRBL: [PRB:x,y,z:1]; Mach3/LinuxCNC/CSV: первые три числа строки."""
    pts = []
    for line in open(path, errors='replace'):
        m = re.search(r'PRB:([^\]:]+)(?::(\d))?', line)
        if m:
            if m.group(2) == '0':
                continue                     # касания не было
            v = [float(t) for t in m.group(1).split(',')[:3]]
        else:
            if line.lstrip().startswith('#'):
                continue
            v = [float(t) for t in NUM.findall(line)[:3]]
        if len(v) == 3:
            pts.append(tuple(v))
    return pts


def snap(vals, tol):
    """Склеить близкие координаты (лог в машинных координатах, мелкий дрожь)."""
    out = []
    for v in sorted(vals):
        if not out or v - out[-1][-1] > tol:
            out.append([v])
        else:
            out[-1].append(v)
    return [sum(g) / len(g) for g in out]


class HeightMap:
    def __init__(self, xs, ys, z):
        self.xs, self.ys, self.z = xs, ys, z      # z[j][i] для ys[j], xs[i]

    @classmethod
    def from_points(cls, pts, tol=0.3, offset=(0.0, 0.0)):
        xs = snap([p[0] for p in pts], tol)
        ys = snap([p[1] for p in pts], tol)
        acc = [[[] for _ in xs] for _ in ys]
        for x, y, z in pts:
            i = min(range(len(xs)), key=lambda k: abs(xs[k] - x))
            j = min(range(len(ys)), key=lambda k: abs(ys[k] - y))
            acc[j][i].append(z)
        z = [[sum(c) / len(c) if c else None for c in row] for row in acc]
        hm = cls([x - offset[0] for x in xs], [y - offset[1] for y in ys], z)
        hm.fill_holes()
        return hm

    def fill_holes(self):
        missing = [(j, i) for j, r in enumerate(self.z) for i, v in enumerate(r) if v is None]
        known = [(self.xs[i], self.ys[j], v) for j, r in enumerate(self.z)
                 for i, v in enumerate(r) if v is not None]
        for j, i in missing:                 # пропуск = среднее по обратным расстояниям
            x, y = self.xs[i], self.ys[j]
            w = [(1 / ((kx - x) ** 2 + (ky - y) ** 2), kz) for kx, ky, kz in known]
            self.z[j][i] = sum(a * b for a, b in w) / sum(a for a, _ in w)
        return len(missing)

    def at(self, x, y):
        """Билинейная интерполяция; за краем сетки — значение края."""
        def cell(vs, v):
            if len(vs) == 1:
                return 0, 0, 0.0
            v = min(max(v, vs[0]), vs[-1])
            k = 0
            while k < len(vs) - 2 and v > vs[k + 1]:
                k += 1
            return k, k + 1, (v - vs[k]) / (vs[k + 1] - vs[k])
        i0, i1, tx = cell(self.xs, x)
        j0, j1, ty = cell(self.ys, y)
        z = self.z
        a = z[j0][i0] * (1 - tx) + z[j0][i1] * tx
        b = z[j1][i0] * (1 - tx) + z[j1][i1] * tx
        return a * (1 - ty) + b * ty

    def save(self, path):
        with open(path, 'w') as f:
            f.write('# x,y,z  (pcbprobe heightmap)\n')
            for j, y in enumerate(self.ys):
                for i, x in enumerate(self.xs):
                    f.write('%.3f,%.3f,%.4f\n' % (x, y, self.z[j][i]))

    def table(self):
        flat = [v for r in self.z for v in r]
        lo, hi = min(flat), max(flat)
        s = ['точек %d×%d, мин %.3f, макс %.3f, перепад %.3f мм'
             % (len(self.xs), len(self.ys), lo, hi, hi - lo)]
        s.append('      Y\\X ' + ''.join('%8.1f' % x for x in self.xs))
        for j in range(len(self.ys) - 1, -1, -1):
            s.append('%10.1f ' % self.ys[j] + ''.join('%8.3f' % v for v in self.z[j]))
        return '\n'.join(s)


# ---------- наложение карты на программу ----------

def fmt(v):
    return ('%.4f' % v).rstrip('0').rstrip('.') if v else '0'


def apply_map(src, hm, seg, cut_z, strip_m6, ref):
    z0 = hm.at(*ref) if ref else 0.0
    x = y = z = 0.0
    mode = 0
    out = []
    for raw in open(src):
        line = raw.rstrip('\n')
        ws = words(line)
        if strip_m6 and any(l == 'M' and v == 6 for l, v in ws):
            out.append('(%s)' % line.strip())
            continue
        gs = [int(v) for l, v in ws if l == 'G']
        for g in gs:
            if g in (0, 1):
                mode = g
            elif g in (2, 3):
                sys.exit('дуги G2/G3 не поддерживаются')
        tgt = {l: v for l, v in ws if l in 'XYZ'}
        if not tgt or any(g in (4, 10, 28, 30, 38, 53, 92) for g in gs):
            out.append(line)
            continue
        nx, ny, nz = tgt.get('X', x), tgt.get('Y', y), tgt.get('Z', z)
        feed = ''.join('F' + fmt(v) for l, v in ws if l == 'F')
        if mode == 1 and min(z, nz) < cut_z + 0.5:
            # рабочий ход у поверхности: режем на отрезки и поднимаем/опускаем Z
            n = max(1, math.ceil(math.hypot(nx - x, ny - y) / seg))
            for k in range(1, n + 1):
                t = k / n
                px, py, pz = x + (nx - x) * t, y + (ny - y) * t, z + (nz - z) * t
                out.append('G1X%sY%sZ%s%s' % (fmt(round(px, 4)), fmt(round(py, 4)),
                                             fmt(round(pz + hm.at(px, py) - z0, 4)),
                                             feed if k == 1 else ''))
        elif 'Z' in tgt and nz < cut_z + 2:
            # быстрый подвод к поверхности тоже со смещением, иначе на горбе заглубится
            out.append('G%dX%sY%sZ%s%s' % (mode, fmt(nx), fmt(ny),
                                           fmt(round(nz + hm.at(nx, ny) - z0, 4)), feed))
        else:
            out.append(line)
        x, y, z = nx, ny, nz
    return '\n'.join(out) + '\n'


# ---------- команды ----------

def cmd_grid(a):
    b = cut_bounds(a.tap)
    xs, ys, pts = grid_points(b, a.step, a.margin)
    with open(a.out, 'w') as f:
        f.write(probe_program(pts, a.dialect, a.clear, a.depth, a.feed, a.log))
    print('рабочая зона X %.2f..%.2f  Y %.2f..%.2f' % b)
    print('сетка %d×%d = %d касаний, шаг X %.2f Y %.2f мм → %s'
          % (len(xs), len(ys), len(pts), xs[1] - xs[0] if len(xs) > 1 else 0,
             ys[1] - ys[0] if len(ys) > 1 else 0, a.out))


def cmd_points(a):
    b = cut_bounds(a.tap)
    xs, ys, pts = grid_points(b, a.step, a.margin)
    with open(a.out, 'w') as f:
        f.write('# впишите Z касания третьим числом; первая точка — ноль\n')
        for x, y in pts:
            f.write('%.3f,%.3f,\n' % (x, y))
    print('%d точек → %s' % (len(pts), a.out))


def cmd_map(a):
    pts = read_probe_log(a.log)
    if not pts:
        sys.exit('в логе не нашлось ни одного касания')
    # лог бывает в машинных координатах: Z считаем от первого касания (там ставился ноль),
    # а X/Y совмещаем с углом сетки, построенной по той же программе
    z0 = pts[0][2]
    off = (0.0, 0.0)
    if a.offset:
        off = tuple(float(t) for t in a.offset.split(','))
    elif a.tap:
        b = cut_bounds(a.tap)
        off = (min(p[0] for p in pts) - (b[0] - a.margin), min(p[1] for p in pts) - (b[2] - a.margin))
    hm = HeightMap.from_points([(x, y, z - z0) for x, y, z in pts], a.tol, off)
    if off != (0.0, 0.0):
        print('сдвиг лога относительно программы X %.3f Y %.3f' % off)
    hm.save(a.out)
    print('касаний %d' % len(pts))
    print(hm.table())


def load_map(path):
    return HeightMap.from_points(read_probe_log(path), 0.05)


def cmd_show(a):
    print(load_map(a.map).table())


def cmd_apply(a):
    hm = load_map(a.map)
    ref = tuple(float(t) for t in a.ref.split(',')) if a.ref else None
    text = apply_map(a.tap, hm, a.seg, 0.0, a.strip_m6, ref)
    with open(a.out, 'w') as f:
        f.write(text)
    print('%s → %s, строк %d' % (a.tap, a.out, text.count('\n')))


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sp = p.add_subparsers(dest='cmd', required=True)

    g = sp.add_parser('grid', help='программа щупа по габариту платы')
    g.add_argument('tap'); g.add_argument('-o', '--out', default='probe.nc')
    g.add_argument('--dialect', choices=['grbl', 'mach3', 'linuxcnc'], default='grbl')
    g.add_argument('--step', type=float, default=10, help='шаг сетки, мм')
    g.add_argument('--margin', type=float, default=0, help='вылет сетки за рабочую зону, мм')
    g.add_argument('--clear', type=float, default=1.5, help='высота переездов, мм')
    g.add_argument('--depth', type=float, default=2, help='на сколько вниз искать касание, мм')
    g.add_argument('--feed', type=float, default=30, help='подача щупа, мм/мин')
    g.add_argument('--log', default='probe.txt', help='файл лога (LinuxCNC)')
    g.set_defaults(fn=cmd_grid)

    g = sp.add_parser('points', help='шаблон CSV для ручного замера')
    g.add_argument('tap'); g.add_argument('-o', '--out', default='manual.csv')
    g.add_argument('--step', type=float, default=20)
    g.add_argument('--margin', type=float, default=0)
    g.set_defaults(fn=cmd_points)

    g = sp.add_parser('map', help='лог касаний → карта высот')
    g.add_argument('log'); g.add_argument('-o', '--out', default='heightmap.csv')
    g.add_argument('--tol', type=float, default=0.3, help='склейка координат сетки, мм')
    g.add_argument('--tap', help='программа, по которой строилась сетка: совместить с ней X/Y лога')
    g.add_argument('--margin', type=float, default=0, help='тот же margin, что в grid')
    g.add_argument('--offset', help='X,Y рабочего нуля в координатах лога (вместо --tap); пишите --offset=-200,-150')
    g.set_defaults(fn=cmd_map)

    g = sp.add_parser('show', help='напечатать карту')
    g.add_argument('map'); g.set_defaults(fn=cmd_show)

    g = sp.add_parser('apply', help='наложить карту на программу')
    g.add_argument('tap'); g.add_argument('map'); g.add_argument('-o', '--out')
    g.add_argument('--seg', type=float, default=1.0, help='макс. длина отрезка, мм')
    g.add_argument('--ref', help='X,Y точки, где после смены фрезы заново выставлен Z0')
    g.add_argument('--strip-m6', action='store_true', help='закомментировать T/M6 (GRBL на них ругается)')
    g.set_defaults(fn=cmd_apply)

    a = p.parse_args()
    if a.cmd == 'apply' and not a.out:
        a.out = re.sub(r'(\.\w+)?$', r'_level\1', a.tap, count=1)
    a.fn(a)


if __name__ == '__main__':
    main()
