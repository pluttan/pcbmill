"""Gerber/Excellon -> shapely geometry: copper per side, board outline, holes."""
import math
import os
import re
import tempfile
import warnings
import zipfile
from pathlib import Path
from dataclasses import dataclass, field

import shapely
from shapely import affinity
from shapely.geometry import LineString, Point, Polygon, box
from shapely.ops import polygonize, unary_union

from gerbonara import ExcellonFile, GerberFile, MM
import gerbonara.graphic_primitives as gp

ARC_TOL = 0.005  # mm, chord error when flattening arcs


@dataclass
class Hole:
    x: float
    y: float
    d: float
    # slot end point; None for a round hole
    x2: float | None = None
    y2: float | None = None
    plated: bool | None = None


@dataclass
class Board:
    name: str
    outline: Polygon
    copper: dict = field(default_factory=dict)   # 'top'/'bottom' -> MultiPolygon
    holes: list = field(default_factory=list)


# ==========================
# ===  Primitive shapes  ===
# ==========================

def arc_points(x1, y1, x2, y2, cx, cy, cw):
    r = math.hypot(x1 - cx, y1 - cy)
    a1 = math.atan2(y1 - cy, x1 - cx)
    a2 = math.atan2(y2 - cy, x2 - cx)
    if cw:
        while a2 >= a1 - 1e-9:
            a2 -= 2 * math.pi
    else:
        while a2 <= a1 + 1e-9:
            a2 += 2 * math.pi
    # chord error r(1-cos(t/2)) <= ARC_TOL
    step = 2 * math.acos(max(-1.0, 1 - ARC_TOL / r)) if r > ARC_TOL else math.pi / 4
    n = max(2, math.ceil(abs(a2 - a1) / max(step, 1e-3)))
    return [(cx + r * math.cos(a1 + (a2 - a1) * i / n),
             cy + r * math.sin(a1 + (a2 - a1) * i / n)) for i in range(n + 1)]


def prim_shape(p):
    if isinstance(p, gp.Circle):
        return Point(p.x, p.y).buffer(p.r, quad_segs=16)
    if isinstance(p, gp.Line):
        if math.isclose(p.x1, p.x2) and math.isclose(p.y1, p.y2):
            return Point(p.x1, p.y1).buffer(p.width / 2, quad_segs=16)
        return LineString([(p.x1, p.y1), (p.x2, p.y2)]).buffer(p.width / 2, quad_segs=16)
    if isinstance(p, gp.Arc):
        pts = arc_points(p.x1, p.y1, p.x2, p.y2, p.cx, p.cy, p.clockwise)
        return LineString(pts).buffer(p.width / 2, quad_segs=16)
    if isinstance(p, gp.Rectangle):
        r = box(p.x - p.w / 2, p.y - p.h / 2, p.x + p.w / 2, p.y + p.h / 2)
        return affinity.rotate(r, p.rotation, origin=(p.x, p.y), use_radians=True)
    if isinstance(p, gp.ArcPoly):
        pts = []
        for (x1, y1), (x2, y2), (cw, (cx, cy)) in p.segments:
            if cw is None:
                pts.append((x1, y1))
            else:
                pts += arc_points(x1, y1, x2, y2, cx, cy, cw)[:-1]
        if len(pts) < 3:
            return None
        poly = Polygon(pts)
        return poly if poly.is_valid else poly.buffer(0)
    return None


def layer_shape(layer):
    """Union of a gerber layer honoring polarity order (clear objects cut earlier dark ones)."""
    result = Polygon()
    batch, dark = [], True
    def flush():
        nonlocal result
        if not batch:
            return
        u = unary_union(batch)
        result = result.union(u) if dark else result.difference(u)
        batch.clear()
    for obj in layer.objects:
        for p in obj.to_primitives(MM):
            s = prim_shape(p)
            if s is None or s.is_empty:
                continue
            if p.polarity_dark != dark:
                flush()
                dark = p.polarity_dark
            batch.append(s)
    flush()
    return result


# ==========================
# ===  Board outline     ===
# ==========================

def outline_shape(layer):
    lines = []
    for obj in layer.objects:
        for p in obj.to_primitives(MM):
            if isinstance(p, gp.Line):
                lines.append(LineString([(p.x1, p.y1), (p.x2, p.y2)]))
            elif isinstance(p, gp.Arc):
                lines.append(LineString(arc_points(p.x1, p.y1, p.x2, p.y2, p.cx, p.cy, p.clockwise)))
            elif isinstance(p, gp.ArcPoly):
                s = prim_shape(p)
                if s is not None:
                    lines.append(s.exterior)
    # snap tiny gaps between segments so polygonize can close the ring
    merged = shapely.set_precision(unary_union(lines), 0.001)
    if merged.is_empty:
        return None
    polys = list(polygonize(merged))
    if not polys:
        print('контур платы не замкнут, беру выпуклую оболочку')
        return merged.convex_hull
    # outer edge = largest; inner ones (cutouts) are holes of the board
    polys.sort(key=lambda p: p.area, reverse=True)
    outer = Polygon(polys[0].exterior)
    inner = [p for p in polys[1:] if outer.contains(p)]
    return outer.difference(unary_union(inner)) if inner else outer


# ==========================
# ===  Holes             ===
# ==========================

def drill_holes(layer):
    holes = []
    for obj in layer.objects:
        obj = obj.converted(MM)
        d = obj.tool.equivalent_width(MM)
        plated = getattr(obj, 'plated', None)
        if hasattr(obj, 'x1'):
            holes.append(Hole(obj.x1, obj.y1, d, obj.x2, obj.y2, plated))
        else:
            holes.append(Hole(obj.x, obj.y, d, plated=plated))
    return holes


# ==========================
# ===  Loading           ===
# ==========================

SKIP_EXT = {'.png', '.jpg', '.svg', '.pdf', '.tap', '.nc', '.ngc', '.gcode', '.csv',
            '.md', '.zip', '.log', '.json', '.html', '.kicad_pcb', '.kicad_pro', '.kicad_sch'}

# file-name patterns of the common EDA exports, checked after X2 attributes
NAME_RULES = [
    ('top',     r'F[._]Cu|\.gtl$|toplayer|\.cmp$|top[ _-]?copper|copper[ _-]?top|\.top$|[_-]top\.(gbr|ger|art)$'),
    ('bottom',  r'B[._]Cu|\.gbl$|bottomlayer|\.sol$|bottom[ _-]?copper|copper[ _-]?bottom|\.bot$|[_-]bottom\.(gbr|ger|art)$'),
    ('outline', r'Edge[._]Cuts|\.gko$|\.gm1$|\.gml$|boardoutline|outline|profile|\.dim$|mechanical ?1'),
]

NOT_COPPER = r'mask|silk|paste|legend|overlay|assy|assembly|courtyard|stencil|fab|\.g[tb][osp]$'


def cam_files(path):
    """Only top-level fabrication files: our own cnc/ output must not be picked up on rerun."""
    out = []
    for f in sorted(os.listdir(path)):
        full = os.path.join(path, f)
        if os.path.isfile(full) and os.path.splitext(f)[1].lower() not in SKIP_EXT \
                and not f.startswith('.'):
            out.append(Path(full))
    return out


def head(path, n=8192):
    with open(path, 'rb') as f:
        return f.read(n).decode('latin-1')


def classify(path):
    """'top' | 'bottom' | 'outline' | 'drill' | None."""
    h = head(path)
    if re.search(r'^\s*M48', h, re.M) or re.search(r'^\s*T\d+C[\d.]', h, re.M):
        return 'drill'
    if '%FS' not in h and 'G04' not in h:
        return None                       # not a gerber at all
    m = re.search(r'%TF\.FileFunction,([^*]*)\*%', h)
    if m:
        f = m.group(1).lower()
        if f.startswith('copper'):
            return 'bottom' if 'bot' in f else 'top' if 'top' in f else None
        if f.startswith('profile'):
            return 'outline'
        return None                       # mask, silk, paste...
    name = path.name
    if re.search(NOT_COPPER, name, re.I):
        return None
    for kind, rx in NAME_RULES:
        if re.search(rx, name, re.I):
            return kind
    return None


def prefix(f):
    return re.split(r'[-_.]', f.stem)[0].lower()


def one_board(files):
    """Several boards exported into one folder: keep the one with most layers."""
    prefixes = {prefix(f) for v in files.values() for f in v}
    if len(prefixes) <= 1 or all(len(v) <= 1 for k, v in files.items() if k != 'drill'):
        return files
    score = {p: sum(any(prefix(f) == p for f in v) for v in files.values()) for p in prefixes}
    best = max(sorted(score), key=lambda p: score[p])
    print('в папке несколько плат (%s), беру %s' % (', '.join(sorted(prefixes)), best))
    return {k: [f for f in v if prefix(f) == best] for k, v in files.items()}


def load(path, sides=('top', 'bottom'), override=None):
    """Board from a directory or zip of fabrication files; override = {'top': file, ...}."""
    override = {k: Path(v) for k, v in (override or {}).items() if v}
    if os.path.isfile(path) and zipfile.is_zipfile(path):
        tmp = tempfile.mkdtemp(prefix='pcbmill-zip-')
        with zipfile.ZipFile(path) as z:
            z.extractall(tmp)
        dirs = [r for r, _, fs in os.walk(tmp) if any(classify(Path(r, f)) for f in fs)]
        path = dirs[0] if dirs else tmp
    files = {'top': [], 'bottom': [], 'outline': [], 'drill': []}
    for f in cam_files(path):
        kind = classify(f)
        if kind:
            files[kind].append(f)
    files = one_board(files)
    for k, v in override.items():
        files[k] = [v]
    with warnings.catch_warnings():
        warnings.simplefilter('ignore')
        copper = {}
        for side in sides:
            for f in files[side][:1]:
                copper[side] = layer_shape(GerberFile.open(f))
        if not copper:
            raise SystemExit('в %s не нашлось слоёв меди; укажите файл: --top ФАЙЛ или --bottom ФАЙЛ' % path)
        outline = None
        if files['outline']:
            outline = outline_shape(GerberFile.open(files['outline'][0]))
        if outline is None:
            print('контур платы не найден, беру габарит меди +1 мм (указать: --outline ФАЙЛ)')
            b = unary_union(list(copper.values())).bounds
            outline = box(b[0] - 1, b[1] - 1, b[2] + 1, b[3] + 1)
        holes = []
        for f in files['drill']:
            holes += drill_holes(ExcellonFile.open(f))
    name = os.path.commonprefix([f.stem for v in files.values() for f in v]).rstrip('-_. ') or 'board'
    used = ', '.join('%s=%s' % (k, v[0].name) for k, v in files.items() if v and k != 'drill')
    if files['drill']:
        used += ', drill=' + '+'.join(f.name for f in files['drill'])
    print('файлы: ' + used)
    return Board(name, outline, copper, holes)


def transform(board, origin='corner', margin=0.0, mirror=False):
    """Move outline corner to (margin, margin); mirror X for bottom side milled face-up."""
    minx, miny, maxx, maxy = board.outline.bounds
    def tf(g):
        if mirror:
            g = affinity.scale(g, -1, 1, origin=((minx + maxx) / 2, 0))
        if origin == 'corner':
            g = affinity.translate(g, margin - minx, margin - miny)
        return g
    def tp(x, y):
        if mirror:
            x = minx + maxx - x
        if origin == 'corner':
            x, y = x + margin - minx, y + margin - miny
        return x, y
    holes = []
    for h in board.holes:
        x, y = tp(h.x, h.y)
        x2, y2 = tp(h.x2, h.y2) if h.x2 is not None else (None, None)
        holes.append(Hole(x, y, h.d, x2, y2, h.plated))
    return Board(board.name, tf(board.outline),
                 {k: tf(v) for k, v in board.copper.items()}, holes)
