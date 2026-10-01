"""Dry-run a G-code program against the machine profile before it reaches the machine.

Everything here is about not letting the machine do something nobody intended:
rapids through material, cuts deeper than the blank, inch mode, commands the
controller rejects mid-job, cutting with the spindle stopped, missing feed.
"""
import math
import re
from dataclasses import dataclass, field

TOKEN = re.compile(r'([A-Za-z])\s*([-+]?\d*\.?\d+)')

# what each controller accepts; anything else stops the job half way
SUPPORTED = {
    'grbl': {'G': {0, 1, 2, 3, 4, 10, 17, 18, 19, 20, 21, 28, 30, 38.2, 38.3, 38.4, 38.5,
                   40, 43.1, 49, 53, 54, 55, 56, 57, 58, 59, 61, 80, 90, 91, 91.1, 92, 93, 94},
             'M': {0, 1, 2, 3, 4, 5, 7, 8, 9, 30}},
    'mach3': {'G': {0, 1, 2, 3, 4, 10, 17, 18, 19, 20, 21, 28, 30, 31, 40, 49, 53, 54, 55,
                    56, 57, 58, 59, 61, 64, 80, 81, 82, 83, 90, 91, 92, 93, 94},
              'M': {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 30, 40, 41, 47, 48, 49, 98, 99}},
    'linuxcnc': {'G': {0, 1, 2, 3, 4, 10, 17, 18, 19, 20, 21, 28, 30, 38.2, 38.3, 38.4, 38.5,
                       40, 43, 49, 53, 54, 55, 56, 57, 58, 59, 61, 64, 80, 81, 82, 83, 90,
                       91, 92, 93, 94},
                 'M': {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 30, 48, 49}},
}
PROBE = {38.2, 38.3, 38.4, 38.5, 31}


@dataclass
class Report:
    path: str
    errors: list = field(default_factory=list)
    warnings: list = field(default_factory=list)
    bounds: tuple = None          # cutting moves (x0, x1, y0, y1, zmin)
    cut_mm: float = 0.0
    rapid_mm: float = 0.0
    minutes: float = 0.0

    def err(self, n, msg):
        self.errors.append('строка %d: %s' % (n, msg))

    def warn(self, n, msg):
        self.warnings.append('строка %d: %s' % (n, msg))

    @property
    def ok(self):
        return not self.errors

    def text(self, limit=10):
        s = ['%s: %s' % (self.path, 'годится' if self.ok else 'ОПАСНО, не запускать')]
        if self.bounds:
            x0, x1, y0, y1, z = self.bounds
            s.append('  рез в X %.2f..%.2f  Y %.2f..%.2f, глубже всего Z %.3f; рез %.0f мм, ~%.0f мин'
                     % (x0, x1, y0, y1, z, self.cut_mm, self.minutes))
        for title, items in (('ошибка', self.errors), ('внимание', self.warnings)):
            for m in items[:limit]:
                s.append('  %s: %s' % (title, m))
            if len(items) > limit:
                s.append('  … ещё %d (%s)' % (len(items) - limit, title))
        return '\n'.join(s)


def check(lines, cfg, path='<g-code>', kind=None):
    """kind: 'iso' | 'drill' | 'cut' | 'probe' | 'test' | None (generic)."""
    m, board = cfg['machine'], cfg['board']
    dialect = m['dialect']
    sup = SUPPORTED[dialect]
    limits = m.get('limits', [1e9, 1e9, 1e9])
    blank = board.get('blank')
    thick = board['thickness']
    max_feed = m.get('max_feed', 2000)
    deepest = {'iso': -0.3, 'drill': -(thick + 0.5), 'cut': -(thick + 0.3),
               'probe': -5.0, 'test': -0.5}.get(kind, -(thick + 0.5))

    r = Report(path)
    x = y = 0.0
    z = None                        # unknown until the program sets it
    mode = None
    units = absolute = None
    feed = None
    spindle = False
    probed = False                  # G38/G31 seen: Z after it is where the touch was
    xs, ys, zs = [], [], []
    last_line = 0
    ended = False

    for n, raw in enumerate(lines, 1):
        body = raw.split(';')[0]
        if re.search(r'\([^)]*\(', body) or body.count('(') != body.count(')'):
            r.err(n, 'скобки комментария вложены или не закрыты: контроллер не разберёт строку')
            continue
        line = re.sub(r'\(.*?\)', '', body).strip()
        if not line or line == '%':
            continue
        last_line = n
        if ended:
            r.warn(n, 'команды после конца программы M2/M30 не выполнятся')
            ended = False
        words = [(l.upper(), float(v)) for l, v in TOKEN.findall(line)]
        rest = TOKEN.sub('', line).strip()
        if rest:
            r.err(n, 'непонятный текст «%s»' % rest)
        gs = [v for l, v in words if l == 'G']
        ms = [v for l, v in words if l == 'M']
        for g in gs:
            if g not in sup['G']:
                r.err(n, 'G%g не поддерживается в %s' % (g, dialect))
        for mm in ms:
            if mm not in sup['M']:
                r.err(n, 'M%g не поддерживается в %s (станок остановится с ошибкой)' % (mm, dialect))
        for g in gs:
            if g in (20, 21):
                units = g
                if g == 20:
                    r.err(n, 'G20: дюймы, все размеры станут в 25.4 раза больше')
            if g in (90, 91):
                absolute = g == 90
                if g == 91:
                    r.warn(n, 'G91: относительные координаты, проверка дальше неточна')
        for l, v in words:
            if l == 'F':
                if v <= 0:
                    r.err(n, 'подача F%g' % v)
                elif v > max_feed:
                    r.err(n, 'подача F%g больше предела станка %g' % (v, max_feed))
                feed = v
            if l == 'S' and v > m.get('max_spindle', 30000):
                r.err(n, 'обороты S%g больше предела' % v)
        if 3 in ms or 4 in ms:
            spindle = True
        if 5 in ms:
            spindle = False
        if 2 in ms or 30 in ms:
            ended = True

        motion = [g for g in gs if g in (0, 1, 2, 3) or g in PROBE]
        if motion:
            mode = motion[-1]
        coords = {l: v for l, v in words if l in 'XYZ'}
        if not coords or any(g in (4, 10, 28, 30, 53, 92) for g in gs):
            if 92 in gs or 10 in gs:
                z = 0.0 if 'Z' in coords else z     # zero is re-set here
            continue
        if mode is None:
            r.err(n, 'движение без G0/G1: режим не задан')
            continue
        if units is None:
            r.warn(n, 'нет G21 до первого движения: программа надеется, что станок уже в мм')
            units = 21
        if absolute is None:
            r.warn(n, 'нет G90 до первого движения: программа надеется на абсолютные координаты')
            absolute = True

        nx, ny = coords.get('X', x), coords.get('Y', y)
        nz = coords.get('Z', z if z is not None else 0.0)
        xy_moves = (nx, ny) != (x, y)
        dist = math.dist((x, y, z or 0.0), (nx, ny, nz))

        if mode in PROBE:
            if 'Z' not in coords or nz >= (z if z is not None else 0.0):
                r.err(n, 'щуп должен идти вниз по Z')
            if xy_moves:
                r.err(n, 'щуп с движением по X/Y: зацепит заготовку боком')
            if feed is None or feed > 100:
                r.err(n, 'щуп быстрее 100 мм/мин сломает кончик фрезы')
            if spindle:
                r.err(n, 'щуп при вращающемся шпинделе')
            probed = True
            z = nz
            continue

        if z is None and xy_moves:
            r.err(n, 'первое движение по X/Y до подъёма Z: высота неизвестна, можно пропахать заготовку')
        if mode == 0:
            r.rapid_mm += dist
            if nz < 0 and not probed:
                r.err(n, 'быстрый ход G0 ниже поверхности Z%.3f' % nz)
            elif xy_moves and z is not None and min(z, nz) < 0.2:
                r.err(n, 'быстрый переезд по X/Y на высоте Z%.3f: заденет медь' % min(z, nz))
            elif z is not None and nz < z and nz < 0.2 and kind != 'test':
                r.warn(n, 'быстрый спуск почти до поверхности Z%.3f' % nz)
        else:
            if mode in (2, 3):
                r.warn(n, 'дуги G2/G3: карта высот на них не накладывается')
            if feed is None:
                r.err(n, 'рабочий ход без подачи F: GRBL откажется (error:22)')
            low = min(nz, z if z is not None else nz)
            if low < 0:
                if not spindle:
                    r.err(n, 'рез ниже поверхности при выключенном шпинделе')
                if low < deepest:
                    r.err(n, 'глубина Z%.3f глубже допустимой %.2f для этой операции' % (low, deepest))
                xs += [x, nx]; ys += [y, ny]; zs.append(low)
                r.cut_mm += dist
                if feed:
                    r.minutes += dist / feed
            probed = False
        for axis, v, lim in (('X', nx, limits[0]), ('Y', ny, limits[1])):
            if abs(v) > lim:
                r.err(n, '%s%.1f за пределом хода станка %g' % (axis, v, lim))
        if nz > limits[2]:
            r.err(n, 'Z%.1f выше хода станка %g' % (nz, limits[2]))
        x, y, z = nx, ny, nz

    if xs:
        r.bounds = (min(xs), max(xs), min(ys), max(ys), min(zs))
        if min(xs) < -1 or min(ys) < -1:
            r.warn(last_line, 'рез заходит в минус на %.1f мм: ноль должен стоять не на краю заготовки'
                   % -min(min(xs), min(ys)))
        if blank:
            mg = board.get('margin', 0)
            if max(xs) > blank[0] or max(ys) > blank[1]:
                r.err(last_line, 'рез до X%.1f Y%.1f не помещается на заготовку %g×%g'
                      % (max(xs), max(ys), blank[0], blank[1]))
    if spindle and not ended:
        r.warn(last_line, 'шпиндель не выключен в конце (нет M5)')
    if kind not in ('probe', 'test') and not ended:
        r.warn(last_line, 'нет M2/M30 в конце')
    if z is not None and z < 1 and kind not in ('probe', 'test'):
        r.err(last_line, 'программа кончается с фрезой у заготовки Z%.3f' % z)
    return r


def check_file(path, cfg, kind=None):
    with open(path, errors='replace') as f:
        return check(f.readlines(), cfg, path, kind)


def guess_kind(path):
    for k in ('iso', 'drill', 'cut', 'probe'):
        if re.search(r'[_-]%s' % k, path) or path.endswith(k + '.nc'):
            return k
    return None
