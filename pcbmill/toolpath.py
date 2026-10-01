"""Toolpaths: isolation around copper, drill marks/holes, board cutout."""
import math
from dataclasses import dataclass, field

from shapely.geometry import LineString, MultiPolygon, Point, Polygon
from shapely.strtree import STRtree


@dataclass
class Path:
    pts: list                 # [(x, y, z), ...] cutting moves; first point is the plunge target
    peck: float = 0.0         # >0: drill cycle down to pts[0].z in steps


@dataclass
class Operation:
    name: str
    tool: str
    paths: list = field(default_factory=list)
    notes: list = field(default_factory=list)
    marks: list = field(default_factory=list)   # shapes to highlight in preview (problems)


def cut_width(tool, depth):
    if tool['type'] == 'vbit':
        return tool['tip'] + 2 * depth * math.tan(math.radians(tool['angle'] / 2))
    return tool['diameter']


def polys(g):
    if g.is_empty:
        return []
    if isinstance(g, Polygon):
        return [g]
    if isinstance(g, MultiPolygon):
        return list(g.geoms)
    return [p for p in getattr(g, 'geoms', []) if isinstance(p, Polygon)]


def lines(g):
    """All linework of a geometry as coordinate lists."""
    if g.is_empty:
        return []
    if g.geom_type in ('LineString', 'LinearRing'):
        return [list(g.coords)]
    if g.geom_type == 'Polygon':
        return [list(g.exterior.coords)] + [list(r.coords) for r in g.interiors]
    out = []
    for p in getattr(g, 'geoms', []):
        out += lines(p)
    return out


# ==========================
# ===  Path ordering     ===
# ==========================

def order(paths, start=(0.0, 0.0)):
    """Greedy nearest-neighbour; closed loops start at their nearest vertex, open ones may flip."""
    left = list(paths)
    out = []
    cx, cy = start
    while left:
        best = None
        for i, p in enumerate(left):
            closed = len(p.pts) > 2 and p.pts[0][:2] == p.pts[-1][:2]
            cands = range(len(p.pts) - 1) if closed and not p.peck else [0, len(p.pts) - 1]
            for k in cands:
                x, y = p.pts[k][:2]
                d = (x - cx) ** 2 + (y - cy) ** 2
                if best is None or d < best[0]:
                    best = (d, i, k, closed)
        _, i, k, closed = best
        p = left.pop(i)
        pts = p.pts
        if closed and k:
            pts = pts[k:-1] + pts[:k] + [pts[k]]
        elif not closed and k:
            pts = pts[::-1]
        out.append(Path(pts, p.peck))
        cx, cy = pts[-1][:2]
    return out


# ==========================
# ===  Isolation         ===
# ==========================

def narrow_gaps(copper, width):
    """Spots where two separate copper islands are closer than the cut width:
    the tool cannot pass between them, so they stay shorted."""
    islands = polys(copper)
    closed = copper.buffer(width / 2, quad_segs=8).buffer(-width / 2, quad_segs=8)
    tree = STRtree(islands)
    bad, gaps = [], []
    for g in polys(closed.difference(copper)):
        if g.area < 1e-4:
            continue
        hits = tree.query(g.buffer(1e-3), predicate='intersects')
        if len(hits) >= 2:
            bad.append(g)
            near = [islands[i] for i in hits]
            gaps.append(min(a.distance(b) for k, a in enumerate(near) for b in near[k + 1:]))
    return bad, gaps


def isolation(copper, outline, tool, cfg):
    depth = cfg['depth']
    w = cut_width(tool, depth)
    step = w * (1 - cfg['overlap'])
    area = outline.buffer(w)
    op = Operation('isolation', cfg['tool'])
    for k in range(cfg['passes']):
        g = copper.buffer(w / 2 + k * step, quad_segs=8).intersection(area)
        for ring in lines(g.boundary):
            ring = LineString(ring).simplify(0.002).coords
            if len(ring) >= 2:
                op.paths.append(Path([(x, y, -depth) for x, y in ring]))
    op.marks, gaps = narrow_gaps(copper, w)
    op.notes.append('ширина реза %.3f мм на глубине %.3f, обходов %d, полоса %.3f мм'
                    % (w, depth, cfg['passes'], w + step * (cfg['passes'] - 1)))
    if op.marks:
        pts = ', '.join('(%.1f; %.1f)' % (m.centroid.x, m.centroid.y) for m in op.marks[:8])
        op.notes.append('ВНИМАНИЕ: %d мест, где зазор меди (от %.3f мм) уже реза, там останется замыкание: %s%s'
                        % (len(op.marks), min(gaps), pts, ' …' if len(op.marks) > 8 else ''))
        if tool['type'] == 'vbit':
            need = (min(gaps) - tool['tip']) / (2 * math.tan(math.radians(tool['angle'] / 2)))
            if need > 0.03:
                op.notes.append('чтобы пройти везде, глубина не больше %.3f мм: -s isolation.depth=%.3f'
                                % ((math.floor(need * 1000) / 1000,) * 2))
            else:
                op.notes.append('эта фреза туда не пролезет ни на какой глубине, нужна тоньше')
    return op


# ==========================
# ===  Drilling          ===
# ==========================

def drilling(holes, tool, cfg, thickness):
    op = Operation('drill', cfg['tool'])
    if cfg['mode'] == 'through':
        depth, peck = thickness + 0.3, cfg['peck']
    else:
        depth, peck = cfg['depth'], 0.0
    sizes = {}
    for h in holes:
        sizes[round(h.d, 2)] = sizes.get(round(h.d, 2), 0) + 1
        if h.x2 is None:
            op.paths.append(Path([(h.x, h.y, -depth)], peck))
        else:
            op.paths.append(Path([(h.x, h.y, -depth), (h.x2, h.y2, -depth)]))
    w = cut_width(tool, depth)
    op.notes.append('%d отверстий, диаметры %s' % (len(holes),
                    ', '.join('%.2f×%d' % kv for kv in sorted(sizes.items()))))
    if cfg['mode'] == 'spot':
        op.notes.append('наколка на %.2f мм, след %.2f мм; сверлить отдельно' % (depth, w))
    elif tool['type'] == 'vbit':
        op.notes.append('ВНИМАНИЕ: сквозная сверловка V-фрезой даст конус %.1f мм на поверхности'
                        % cut_width(tool, depth))
    return op


# ==========================
# ===  Cutout            ===
# ==========================

def levels(depth, stepdown):
    n = max(1, math.ceil(depth / stepdown - 1e-9))
    return [depth * (i + 1) / n for i in range(n)]


def cutout(outline, tool, cfg, thickness):
    op = Operation('cutout', cfg['tool'])
    depth = cfg['depth']
    if cfg['mode'] == 'mill':
        r = cut_width(tool, depth) / 2
        shape = outline.buffer(r, quad_segs=16)
    else:
        shape = outline
    rings = [list(p.exterior.segmentize(0.5).coords) for p in polys(shape)]
    rings += [list(r.segmentize(0.5).coords) for p in polys(shape) for r in p.interiors]
    tab_z = -(thickness - cfg['tab_height'])
    for ring in rings:
        ls = LineString(ring)
        L = ls.length
        centers = []
        if cfg['mode'] == 'mill' and cfg['tabs'] > 0:
            centers = [L * (i + 0.5) / cfg['tabs'] for i in range(cfg['tabs'])]
        half = cfg['tab_width'] / 2 + cut_width(tool, depth) / 2
        for z in levels(depth, cfg['stepdown']):
            pts = []
            for x, y in ring:
                s = ls.project(Point(x, y))
                zz = -z
                if zz < tab_z and any(abs(s - c) < half for c in centers):
                    zz = tab_z
                pts.append((x, y, zz))
            op.paths.append(Path(pts))
    if cfg['mode'] == 'score':
        op.notes.append('V-канавка глубиной %.2f мм за %d проходов, плату потом сломать по канавке'
                        % (depth, len(levels(depth, cfg['stepdown']))))
    else:
        op.notes.append('контур на %.2f мм, перемычек %d' % (depth, cfg['tabs']))
        if depth < thickness:
            op.notes.append('ВНИМАНИЕ: глубина контура меньше толщины платы %.2f' % thickness)
    return op

