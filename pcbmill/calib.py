"""Calibration boards for the V-bit.

calib_speed: one small pattern repeated in a grid of feed x spindle speed, to see
which pair cuts copper clean instead of tearing it.
calib_geom: at the chosen feed and speed, how thin a track, how narrow a gap and
how dense a footprint this machine really makes; tracks at 0, 90 and 45 degrees,
interdigitated combs to ring out with a multimeter, a copper pour, a depth row.

Numbers are engraved next to every sample, so the board reads without a key."""
import copy

from shapely.geometry import LineString, Point, box
from shapely.ops import unary_union

from . import gcode, toolpath as tp

# ==========================
# ===  Engraved digits   ===
# ==========================

# stroke digits in a 1 x 2 box, drawn as few polylines as possible: fewer plunges
GLYPHS = {
    '0': [[(0, 0), (1, 0), (1, 2), (0, 2), (0, 0)]],
    '1': [[(0.2, 1.6), (0.6, 2), (0.6, 0)]],
    '2': [[(0, 2), (1, 2), (1, 1), (0, 1), (0, 0), (1, 0)]],
    '3': [[(0, 2), (1, 2), (1, 0), (0, 0)], [(0.2, 1), (1, 1)]],
    '4': [[(0, 2), (0, 1), (1, 1)], [(1, 2), (1, 0)]],
    '5': [[(1, 2), (0, 2), (0, 1), (1, 1), (1, 0), (0, 0)]],
    '6': [[(1, 2), (0, 2), (0, 0), (1, 0), (1, 1), (0, 1)]],
    '7': [[(0, 2), (1, 2), (1, 0)]],
    '8': [[(0, 1), (0, 0), (1, 0), (1, 2), (0, 2), (0, 1), (1, 1)]],
    '9': [[(0, 0), (1, 0), (1, 2), (0, 2), (0, 1), (1, 1)]],
    '.': [[(0.1, 0), (0.3, 0), (0.3, 0.2), (0.1, 0.2), (0.1, 0)]],
}
ADVANCE = {'.': 0.6}


def text_width(s, h):
    return sum(ADVANCE.get(c, 1.5) for c in s) * h / 2


def engrave(s, x, y, h, depth):
    """Text with its lower left corner at (x, y), digits h mm tall, as cutting paths."""
    u = h / 2
    out = []
    for c in s:
        for stroke in GLYPHS[c]:
            out.append(tp.Path([(x + px * u, y + py * u, -depth) for px, py in stroke]))
        x += ADVANCE.get(c, 1.5) * u
    return out


def label(v):
    """0.15 -> '.15', 24000 -> '24': short enough to fit under a 4 mm sample."""
    if v < 1:
        return ('%g' % v)[1:]
    return '%g' % v


# ==========================
# ===  Copper shapes     ===
# ==========================

def track(pts, w):
    return LineString(pts).buffer(w / 2, quad_segs=8)


def pad(cx, cy, w, h):
    return box(cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2)


def ring(cx, cy, r, w):
    return Point(cx, cy).buffer(r + w / 2, quad_segs=32).difference(Point(cx, cy).buffer(r - w / 2, quad_segs=32))


def shifted(geom, dx, dy):
    from shapely import affinity
    return affinity.translate(geom, dx, dy)


def speed_cell():
    """6 x 6 mm: two parallel tracks 0.5 apart, a vertical track, a 45 degree bend, a pad."""
    return unary_union([
        track([(0.6, 5.2), (5.4, 5.2)], 0.3),
        track([(0.6, 4.4), (5.4, 4.4)], 0.3),
        track([(0.8, 0.6), (0.8, 3.4)], 0.3),
        track([(1.8, 0.6), (1.8, 1.8), (3.0, 3.0), (3.6, 3.0)], 0.3),
        Point(4.4, 1.4).buffer(0.8, quad_segs=16),
    ])


def comb(g, finger=0.3, n=6, length=5.0):
    """Two interdigitated combs with clearance g, a 1.4 mm pad on each for the probes.
    The teeth start 0.6 above the pads, or the first tooth would touch the other pad."""
    bus, lift = 0.6, 0.6
    h = lift + n * finger + (n - 1) * g
    left = [box(0, 0, bus, h), box(0, -1.8, 1.4, 0)]
    right = [box(length - bus, 0, length, h), box(length - 1.4, -1.8, length, 0)]
    for k in range(n):
        y = lift + k * (finger + g)
        if k % 2 == 0:
            left.append(box(bus, y, length - bus - g, y + finger))
        else:
            right.append(box(bus + g, y, length - bus, y + finger))
    return unary_union(left), unary_union(right), h


# ==========================
# ===  Programs          ===
# ==========================

def spin(s):
    # NcStudio .tap files here carry no dwell: wait for the spindle with a slow move in the air
    return ['G0Z4.000', 'S%dM3' % s, 'G1Z2.000F30.0']


def cut(op, mcfg, tool, feed):
    t = dict(tool, feed=feed, plunge=min(feed, tool['plunge']))
    op.paths = tp.order(op.paths)
    return gcode.body(op, mcfg, t), gcode.stats(op, mcfg, t)[1]


def wrap(mcfg, rpm, body):
    head = ['T1M6', 'G0Z%s' % gcode.n(mcfg['safe_z']), 'G0X0.000Y0.000S%dM3' % rpm, 'G1Z2.000F30.0']
    return '\n'.join(head + body + gcode.footer(mcfg)) + '\n'


AREA = box(-10, -10, 100, 100)  # isolation clips to the board; these boards are all copper blank


def speed_program(cfg, feeds=(100, 200, 300, 450), rpms=(24000, 20000, 16000, 12000, 8000)):
    """Rows of spindle speed (top: fastest), columns of feed; returns (text, copper, ops, minutes)."""
    m, iso = cfg['machine'], cfg['isolation']
    tool = cfg['tools'][iso['tool']]
    pitch, x0, y0 = 7.5, 7.0, 6.0
    body, minutes, ops, copper = [], 0.0, [], []
    for r, s in enumerate(rpms):
        if r:
            body += spin(s)
        y = y0 + (len(rpms) - 1 - r) * pitch
        for c, f in enumerate(feeds):
            cell = shifted(speed_cell(), x0 + c * pitch, y)
            copper.append(cell)
            op = tp.isolation(cell, AREA, tool, iso)
            text, t = cut(op, m, tool, f)
            body += text
            minutes += t
            ops.append(op)
    # labels last, at the last (slowest) speed: their quality does not matter
    marks = tp.Operation('isolation', iso['tool'])
    for r, s in enumerate(rpms):
        y = y0 + (len(rpms) - 1 - r) * pitch + 2.2
        marks.paths += engrave(label(s / 1000), 0.8, y, 1.6, 0.05)
    for c, f in enumerate(feeds):
        x = x0 + c * pitch + 3 - text_width(label(f), 1.6) / 2
        marks.paths += engrave(label(f), x, 2.6, 1.6, 0.05)
    text, t = cut(marks, m, tool, 200)
    body += text
    minutes += t
    ops.append(marks)
    return wrap(m, rpms[0], body), unary_union(copper), ops, minutes


def geom_program(cfg, feed, rpm):
    m, iso = cfg['machine'], cfg['isolation']
    tool = cfg['tools'][iso['tool']]
    parts, labels = [], []

    # track widths: rows 0, 90 and 45 degrees, one column per width
    widths = [0.1, 0.15, 0.2, 0.25, 0.3, 0.4, 0.5]
    for i, w in enumerate(widths):
        x = 1.0 + i * 4.4
        parts += [track([(x, 45.0), (x + 3.4, 45.0)], w),
                  track([(x + 1.7, 38.6), (x + 1.7, 43.4)], w),
                  track([(x + 0.2, 33.4), (x + 3.2, 36.4)], w)]
        labels.append((label(w), x + 1.7 - text_width(label(w), 1.6) / 2, 30.6))

    # clearances: combs to ring out; a short between the two pads = the gap did not cut
    for i, g in enumerate([0.15, 0.2, 0.25, 0.3, 0.4, 0.5]):
        x, y = 34.0 + (i % 3) * 9.5, 40.0 - (i // 3) * 10.0
        a, b, h = comb(g)
        parts += [shifted(a, x, y), shifted(b, x, y)]
        labels.append((label(g), x + 2.5 - text_width(label(g), 1.6) / 2, y + h + 0.8))

    # dense footprints
    for k in range(4):   # SOIC: 1.27 pitch, pads 0.6 x 1.55
        for sy in (-2.7, 2.7):
            parts.append(pad(2.0 + k * 1.27, 21.0 + sy, 0.6, 1.55))
    for k in range(8):   # TSSOP: 0.65 pitch, pads 0.35 x 1.2
        for sy in (-2.9, 2.9):
            parts.append(pad(9.5 + k * 0.65, 21.0 + sy, 0.35, 1.2))
    for k in range(10):  # QFP: 0.5 pitch, pads 0.25 x 1.2, fanned out with 0.2 tracks
        cx = 17.0 + k * 0.5
        parts.append(pad(cx, 23.0, 0.25, 1.2))
        tx = 19.25 + (k - 4.5) * 0.9  # fan out around the middle: gentle angles keep the gaps
        parts.append(track([(cx, 22.4), (cx, 21.8), (tx, 18.8), (tx, 17.3)], 0.2))
    for k in range(3):   # DIP 2.54 with one 0.25 track between neighbouring pads
        parts.append(Point(28.5 + k * 2.54, 21.0).buffer(0.85, quad_segs=16))
    for k in range(2):
        parts.append(track([(28.5 + 1.27 + k * 2.54, 17.5), (28.5 + 1.27 + k * 2.54, 24.5)], 0.25))

    # copper pour around a track and two pads, clearance 0.3
    feats = unary_union([track([(36.0, 18.0), (42.0, 24.0), (48.0, 24.0)], 0.3),
                         Point(37.0, 24.0).buffer(0.8, quad_segs=16),
                         Point(47.0, 18.0).buffer(0.8, quad_segs=16),
                         track([(47.0, 18.0), (47.0, 21.0)], 0.25)])
    pour = box(35.0, 16.0, 49.0, 26.0)
    parts.append(pour.difference(feats.buffer(0.3, quad_segs=8)).union(feats))

    # corners: zigzag with 90 and 45 degree turns, a ring, a sharp 30 degree V
    parts.append(track([(1, 8), (4, 8), (4, 11), (7, 11), (9, 9), (11, 11), (13, 11), (13, 8), (15, 8)], 0.3))
    parts.append(ring(20.0, 9.5, 2.0, 0.3))
    parts.append(track([(25.0, 12.0), (26.5, 6.5), (28.0, 12.0)], 0.3))

    copper = unary_union(parts)
    body, minutes, ops = [], 0.0, []
    op = tp.isolation(copper, AREA, tool, iso)
    text, t = cut(op, m, tool, feed)
    body += text
    minutes += t
    ops.append(op)

    # depth row: the same sample at five depths, single pass to show the groove itself
    drow = []
    for i, d in enumerate([0.05, 0.075, 0.1, 0.125, 0.15]):
        x = 33.0 + i * 6.0
        sample = unary_union([track([(x, 9.0), (x + 4.0, 9.0)], 0.3), track([(x, 8.4 - 0.3), (x + 4.0, 8.4 - 0.3)], 0.3),
                              track([(x + 0.5, 4.5), (x + 0.5, 7.0)], 0.2)])
        drow.append(sample)
        c = dict(iso, depth=d, passes=1)
        op = tp.isolation(sample, AREA, tool, c)
        text, t = cut(op, m, tool, feed)
        body += text
        minutes += t
        ops.append(op)
        labels.append((label(d), x + 2.0 - text_width(label(d), 1.6) / 2, 1.8))

    marks = tp.Operation('isolation', iso['tool'])
    for s, x, y in labels:
        marks.paths += engrave(s, x, y, 1.6, 0.05)
    text, t = cut(marks, m, tool, 200)
    body += text
    minutes += t
    ops.append(marks)
    return wrap(m, rpm, body), unary_union([copper] + drow), ops, minutes


class Blank:
    """Just enough of a board for preview.render: copper and every cut, plus a margin."""
    def __init__(self, geom, ops, margin=2.0):
        xs = [q[0] for op in ops for p in op.paths for q in p.pts]
        ys = [q[1] for op in ops for p in op.paths for q in p.pts]
        g0, g1, g2, g3 = geom.bounds
        x0, y0, x1, y1 = min(xs + [g0]), min(ys + [g1]), max(xs + [g2]), max(ys + [g3])
        self.outline = box(x0 - margin, y0 - margin, x1 + margin, y1 + margin)
        self.holes = []


def write(cfg, out_dir, feed, rpm):
    import os
    from . import preview
    os.makedirs(out_dir, exist_ok=True)
    made = []
    for name, (text, copper, ops, minutes) in [
            ('calib_speed', speed_program(cfg)),
            ('calib_geom', geom_program(cfg, feed, rpm))]:
        path = os.path.join(out_dir, name + '.tap')
        with open(path, 'w', newline='\r\n') as f:
            f.write(text)
        b = Blank(copper, ops)
        x0, y0, x1, y1 = b.outline.bounds
        preview.render(b, copper, ops, os.path.join(out_dir, name + '.png'),
                       '%s  %.0f x %.0f mm  ~%.0f мин' % (name, x1 - x0, y1 - y0, minutes))
        made.append((path, x1 - x0, y1 - y0, minutes, ops))
    return made
