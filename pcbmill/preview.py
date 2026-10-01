"""PNG preview of copper and toolpaths, Catppuccin Mocha palette."""
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.patches import Polygon as MplPolygon

from .toolpath import polys

BASE, SURFACE0, SURFACE2 = '#1e1e2e', '#313244', '#585b70'
TEXT, OVERLAY0 = '#cdd6f4', '#6c7086'
COLORS = {'isolation': '#89b4fa', 'drill': '#fab387', 'cutout': '#a6e3a1'}
RED = '#f38ba8'


def fill(ax, geom, color, alpha=1.0, z=1):
    for p in polys(geom):
        ax.add_patch(MplPolygon(list(p.exterior.coords), closed=True, fc=color, ec='none',
                                alpha=alpha, zorder=z))
        for r in p.interiors:
            ax.add_patch(MplPolygon(list(r.coords), closed=True, fc=BASE, ec='none', zorder=z))


def render(board, copper, ops, path, title):
    fig, ax = plt.subplots(figsize=(10, 10), dpi=150)
    fig.patch.set_facecolor(BASE)
    ax.set_facecolor(BASE)
    fill(ax, board.outline, SURFACE0, z=0)
    if copper is not None:
        fill(ax, copper, SURFACE2, z=1)
    last = None
    for op in ops:
        c = COLORS.get(op.name, TEXT)
        for p in op.paths:
            xs = [q[0] for q in p.pts]
            ys = [q[1] for q in p.pts]
            if last is not None:
                ax.plot([last[0], xs[0]], [last[1], ys[0]], color=OVERLAY0, lw=0.3,
                        ls=(0, (2, 2)), zorder=2)
            if len(xs) == 1:
                ax.plot(xs, ys, 'o', ms=2.5, color=c, zorder=4)
            else:
                ax.plot(xs, ys, color=c, lw=0.5, zorder=3)
            last = (xs[-1], ys[-1])
        for m in op.marks:
            fill(ax, m.buffer(0.15), RED, z=5)
            ax.plot([m.centroid.x], [m.centroid.y], 'o', ms=9, mfc='none', mec=RED, mew=1.2, zorder=6)
    ax.set_aspect('equal')
    ax.autoscale()
    ax.tick_params(colors=TEXT, labelsize=7)
    for s in ax.spines.values():
        s.set_color(SURFACE2)
    ax.grid(color=SURFACE0, lw=0.4)
    ax.set_title(title, color=TEXT, fontsize=9)
    fig.tight_layout()
    fig.savefig(path, facecolor=BASE)
    plt.close(fig)
