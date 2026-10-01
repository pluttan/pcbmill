"""Height map: probe program, probe log parsing, and Z compensation of a G-code program."""
import math
import re
import sys

WORD = re.compile(r'([A-Za-z])\s*(-?\d*\.?\d+)')
NUM = re.compile(r'-?\d+\.?\d*(?:[eE]-?\d+)?')


# ==========================
# ===  G-code parsing    ===
# ==========================

def words(line):
    line = re.sub(r'\(.*?\)', '', line).split(';')[0]
    return [(l.upper(), float(v)) for l, v in WORD.findall(line)]


def cut_bounds(lines, cut_z=0.0):
    """Bounding box of moves below cut_z."""
    x = y = z = 0.0
    xs, ys = [], []
    for line in lines:
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


# ==========================
# ===  Probe program     ===
# ==========================

def axis(lo, hi, step):
    n = max(1, math.ceil((hi - lo) / step))
    return [lo + (hi - lo) * i / n for i in range(n + 1)]


def grid_points(b, step, margin):
    x0, x1, y0, y1 = b[0] - margin, b[1] + margin, b[2] - margin, b[3] + margin
    xs, ys = axis(x0, x1, step), axis(y0, y1, step)
    pts = []
    for j, y in enumerate(ys):              # serpentine keeps travel short
        row = xs if j % 2 == 0 else xs[::-1]
        pts += [(x, y) for x in row]
    return xs, ys, pts


def probe_program(pts, dialect, clear, depth, feed, logname):
    probe = 'G31' if dialect == 'mach3' else 'G38.2'
    out = ['(pcbmill: карта высот, %d точек)' % len(pts),
           'G21 G90 G17', 'G0 Z%.3f' % clear]
    if dialect == 'mach3':
        out.append('M40 (открыть файл записи касаний)')
    if dialect == 'linuxcnc':
        out.append('(PROBEOPEN %s)' % logname)
    x, y = pts[0]
    # the first touch defines Z0; the whole map is relative to it
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


# ==========================
# ===  Height map        ===
# ==========================

def read_probe_log(path):
    """x,y,z triples. GRBL: [PRB:x,y,z:1]; Mach3/LinuxCNC/CSV: first three numbers of a line."""
    pts = []
    for line in open(path, errors='replace'):
        m = re.search(r'PRB:([^\]:]+)(?::(\d))?', line)
        if m:
            if m.group(2) == '0':
                continue                     # probe did not trigger
            v = [float(t) for t in m.group(1).split(',')[:3]]
        else:
            if line.lstrip().startswith('#'):
                continue
            v = [float(t) for t in NUM.findall(line)[:3]]
        if len(v) == 3:
            pts.append(tuple(v))
    return pts


def snap(vals, tol):
    """Cluster nearly equal coordinates (machine positions jitter by a few microns)."""
    out = []
    for v in sorted(vals):
        if not out or v - out[-1][-1] > tol:
            out.append([v])
        else:
            out[-1].append(v)
    return [sum(g) / len(g) for g in out]


class HeightMap:
    def __init__(self, xs, ys, z):
        self.xs, self.ys, self.z = xs, ys, z      # z[j][i] at (xs[i], ys[j])

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

    @classmethod
    def load(cls, path):
        return cls.from_points(read_probe_log(path), 0.05)

    def fill_holes(self):
        missing = [(j, i) for j, r in enumerate(self.z) for i, v in enumerate(r) if v is None]
        known = [(self.xs[i], self.ys[j], v) for j, r in enumerate(self.z)
                 for i, v in enumerate(r) if v is not None]
        for j, i in missing:                 # inverse distance weighting
            x, y = self.xs[i], self.ys[j]
            w = [(1 / ((kx - x) ** 2 + (ky - y) ** 2), kz) for kx, ky, kz in known]
            self.z[j][i] = sum(a * b for a, b in w) / sum(a for a, _ in w)
        return len(missing)

    def at(self, x, y):
        """Bilinear; outside the grid the edge value is held."""
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
            f.write('# x,y,z  (pcbmill heightmap)\n')
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


def build_map(log, tap_lines=None, margin=0.0, offset=None, tol=0.3):
    """Probe log may be in machine coordinates: Z is taken relative to the first touch
    (where Z0 was set), X/Y are aligned to the grid corner of the program it was built for."""
    pts = read_probe_log(log)
    if not pts:
        sys.exit('в логе не нашлось ни одного касания')
    z0 = pts[0][2]
    off = (0.0, 0.0)
    if offset:
        off = offset
    elif tap_lines is not None:
        b = cut_bounds(tap_lines)
        off = (min(p[0] for p in pts) - (b[0] - margin), min(p[1] for p in pts) - (b[2] - margin))
    return HeightMap.from_points([(x, y, z - z0) for x, y, z in pts], tol, off), off, len(pts)


# ==========================
# ===  Z compensation    ===
# ==========================

def fmt(v):
    return ('%.4f' % v).rstrip('0').rstrip('.') if v else '0'


def apply_map(lines, hm, seg=1.0, cut_z=0.0, strip_m6=False, ref=None):
    z0 = hm.at(*ref) if ref else 0.0
    x = y = z = 0.0
    mode = 0
    out = []
    for raw in lines:
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
            # cutting near the surface: split into short segments and follow the relief
            n = max(1, math.ceil(math.hypot(nx - x, ny - y) / seg))
            for k in range(1, n + 1):
                t = k / n
                px, py, pz = x + (nx - x) * t, y + (ny - y) * t, z + (nz - z) * t
                out.append('G1X%sY%sZ%s%s' % (fmt(round(px, 4)), fmt(round(py, 4)),
                                             fmt(round(pz + hm.at(px, py) - z0, 4)),
                                             feed if k == 1 else ''))
        elif 'Z' in tgt and nz < cut_z + 2:
            # low rapids are shifted too, or a bump would be hit at rapid speed
            out.append('G%dX%sY%sZ%s%s' % (mode, fmt(nx), fmt(ny),
                                           fmt(round(nz + hm.at(nx, ny) - z0, 4)), feed))
        else:
            out.append(line)
        x, y, z = nx, ny, nz
    return '\n'.join(out) + '\n'
