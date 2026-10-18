"""Shared drawing helpers for the pcbmill deck: photos, dot lists, card grids, plain tables."""
import itertools

from deckkit import M, W

_clip = itertools.count()


def photo(s, x, y, w, h, name, cap=None):
    """A photo cropped into a rounded frame, the caption under it."""
    cid = f"ph{next(_clip)}"
    s.raw(lambda t: (
        f'<defs><clipPath id="{cid}"><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="12"/></clipPath></defs>'
        f'<image href="img/{name}" x="{x}" y="{y}" width="{w}" height="{h}" '
        f'preserveAspectRatio="xMidYMid slice" clip-path="url(#{cid})"/>'
        f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="12" fill="none" stroke="{t["muted"]}" stroke-width="1"/>'),
        "back")
    if cap:
        s.text(x, y + h + 20, cap, 14, "muted")


def bullets(s, x, y, items, width, size=18):
    """A vertical list with teal dots; returns the y under the last item."""
    for t in items:
        s.dot(x + 6, y + 9, 4, "teal")
        y = s.para(x + 24, y + 9, t, width - 24, size, "ink") + 10
    return y


def cards(s, items, y=224, cols=2, h=96, gap=20, size=22, sub_size=16, kinds=None, x=M, width=None):
    """Grid of named cards; items are (id, title, sub); kinds maps id to a node kind."""
    width = width or W - 2 * M
    w = (width - (cols - 1) * 24) / cols
    kinds = kinds or {}
    for i, (nid, t, sub) in enumerate(items):
        s.node(nid, x + (i % cols) * (w + 24), y + (i // cols) * (h + gap), w, h,
               kinds.get(nid, "plain"), t, sub, size=size, sub_size=sub_size)
    rows = (len(items) + cols - 1) // cols
    return y + rows * (h + gap) - gap


def table(s, x, y, widths, rows, size=16, row_h=40, accent_col=None):
    """Plain table: the first row is the header in muted colour, thin lines between rows."""
    total = sum(widths)
    for r, cells in enumerate(rows):
        cx = x
        for c, cell in enumerate(cells):
            col = "muted" if r == 0 else ("accent" if c == accent_col else "ink")
            s.text(cx, y + r * row_h, cell, size, col)
            cx += widths[c]
        s.line([(x, y + r * row_h + 14), (x + total, y + r * row_h + 14)], "soft", 1)
    return y + len(rows) * row_h
