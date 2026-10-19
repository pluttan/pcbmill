"""Figures for the Habr article (docs/article/habr-pcbmill.md).

Two kinds of data. What the CAM sees on the test board comes from the Go code
itself: `make article-figs` runs the figs-tagged test win7/figdump_test.go,
which loads testboard/pcbmill-test.zip and dumps the copper raster, the field
of distances, the isolation passes and the shorts into data/*.json. The rest
are measurements copied from the notes that recorded them, named next to each
constant, so the text and the pictures read the same numbers.

Each figure is a self-contained card, filled edge to edge: a neutral dark grey
ground, the Catppuccin Mocha colours on it and the GOST Type BU font
(docs/fonts/GOSTtypeB.ttf).

Run:  make article-figs   ->  docs/article/img/*.png

Signed: pluttan
"""

from __future__ import annotations

import base64
import json
import math
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
from matplotlib import font_manager, rcParams
from matplotlib import patheffects
from matplotlib.colors import LinearSegmentedColormap
from matplotlib.patches import FancyBboxPatch
from matplotlib.text import Text
from matplotlib.transforms import Affine2D

HERE = Path(__file__).resolve().parent
OUT = HERE / "img"
DATA = HERE / "data"

# ==============================
# ===  Measurements          ===
# ==============================

# The V-bit of the isolation: tip and angle (win7/cam.go defaultCam).
TIP, ANGLE = 0.1, 45.0
# Board rules: the least gap, and the one we advise (docs/pcb-rules.json).
GAP_MIN, GAP_ADVISED = 0.2, 0.3

# kb pcbmill/autolevel-pcb.md, "Подлёт по прогнозу": commands per point on the
# simulator, and how far above the forecast the copper was on the real map.
COMMANDS = (("касание шагами от безопасной высоты", 14.4, "naive"),
            ("подлёт к прогнозу, затем шаги 0,01", 8.0, "mine"))
FORECAST = (("ближайшая точка или прямая по двум", 0.15, "naive"),
            ("плоскость через 4 соседние точки", 0.03, "mine"))

# The "Программы листа" page of pcbscan for the test board (docs/article/img/app_programs.png), minutes.
STAGES = (("изоляция", 18), ("вскрытие маски", 22), ("отверстия", 3), ("V-рез", 8))

# kb pcbmill/gerber-cam-go.md: cut length of the isolation, mm, Go vs Python.
AGAINST_PY = ((1291, 1293), (564, 563), (3329, 3328), (5813, 5813))

# ==============================
# ===  Look                  ===
# ==============================

C = {
    "card": "#171717",     # Habr dark --background-primary, hsl(0, 0%, 9%)
    "ink": "#cdd6f4",      # Catppuccin Mocha: Text
    "muted": "#a6adc8",    # Subtext 0
    "track": "#1c1c1c",
    "grid": "#242424",
    "mine": "#74c7ec",     # Sapphire: tool paths, my side of a comparison
    "warn": "#f38ba8",     # Red: shorts, the edge of the rules
    "naive": "#6c7086",    # Overlay 0
    "copper": "#fab387",   # Peach
    "board": "#0e0e0e",
    "fwd": "#74c7ec",      # Sapphire
    "back": "#cba6f7",     # Mauve
}
W = 8.0
DPI = 300
PILL = 13


def style():
    for name in ("GOSTtypeB.ttf", "Exo2-BoldItalic.ttf"):
        font_manager.fontManager.addfont(str(HERE.parent / "fonts" / name))
    rcParams.update({
        "font.family": "GOST Type BU", "font.size": 11,
        "text.color": C["ink"], "axes.labelcolor": C["muted"],
        "xtick.color": C["muted"], "ytick.color": C["muted"],
        "axes.edgecolor": C["grid"], "figure.dpi": DPI, "savefig.dpi": DPI,
    })


def ru(v: float, digits: int = 2, strip: bool = True) -> str:
    """Comma decimal; a no-break space between thousands."""
    text = f"{v:,.{digits}f}".replace(",", " ").replace(".", ",")
    if strip and "," in text:
        text = text.rstrip("0").rstrip(",")
    return text


# Titles in Exo 2 Bold Italic (OFL, docs/fonts/Exo2-OFL.txt); the rest in slanted GOST.
TITLE = {"family": "Exo 2", "fontweight": "bold", "fontstyle": "italic", "gid": "title"}


class Slanted(patheffects.AbstractPathEffect):
    """Draw a text effect with its glyphs leaned forward: GOST Type BU has no italic face."""

    def __init__(self, inner, deg: float):
        super().__init__()
        self.inner, self.deg = inner, deg

    def draw_path(self, renderer, gc, tpath, affine, rgbFace=None):
        self.inner.draw_path(renderer, gc, tpath, Affine2D().skew_deg(self.deg, 0) + affine, rgbFace)


# ГОСТ 2.304: the slanted type leans at 75 degrees to the line.
SLANT = 15


def slant(fig):
    """Lean every text of the figure; lines and pictures stay upright."""
    for t in fig.findobj(Text):
        if t.get_text() == "●" or t.get_gid() == "title":
            continue
        effects = t.get_path_effects() or [patheffects.Normal()]
        t.set_path_effects([Slanted(e, SLANT) for e in effects])


# The card: a dark round field on the Habr grey, a plain edge. The shade terms stay at
# zero: a soft edge was tried and turned down.
INSET = {"margin": 0.1, "radius": 0.2, "shift": 0.06, "blur": 0.1,
         "dark": 0.0, "light": 0.0, "panel": "#040404"}  # two plain grounds, no shading


def inset(fig, dpi: int = DPI):
    """Lay the pressed-in card under everything on the figure."""
    from PIL import Image, ImageDraw, ImageFilter
    w, h = (int(round(v * dpi)) for v in fig.get_size_inches())
    m, r = (int(INSET[k] * dpi) for k in ("margin", "radius"))
    d, b = (int(INSET[k] * dpi) for k in ("shift", "blur"))

    def box(dx=0, dy=0):
        im = Image.new("L", (w, h), 0)
        ImageDraw.Draw(im).rounded_rectangle((m + dx, m + dy, w - m + dx, h - m + dy), r, fill=255)
        return im

    panel = np.asarray(box(), float) / 255
    # Inside the panel, where the edge shifted away leaves a gap, the shadow falls.
    dark = np.asarray(box(d, d).filter(ImageFilter.GaussianBlur(b)), float) / 255
    light = np.asarray(box(-d, -d).filter(ImageFilter.GaussianBlur(b)), float) / 255
    dark = (1 - dark) * panel * INSET["dark"]
    light = (1 - light) * panel * INSET["light"]
    rgb = lambda c: np.array([int(c[i:i + 2], 16) for i in (1, 3, 5)], float) / 255
    ground = rgb(C["card"]) * (1 - panel[..., None]) + rgb(INSET["panel"]) * panel[..., None]
    # The shade goes to black, the far edge back to the grey round the card.
    ground = ground * (1 - dark[..., None])
    ground = ground * (1 - light[..., None]) + rgb(C["card"]) * light[..., None]
    ax = fig.add_axes([0, 0, 1, 1], zorder=-100)
    ax.imshow(np.clip(ground, 0, 1), extent=(0, 1, 0, 1), aspect="auto", interpolation="none")
    ax.set_xlim(0, 1)
    ax.set_ylim(0, 1)
    ax.axis("off")


def card(height: float, title: str, subtitle: str, legend=()):
    fig = plt.figure(figsize=(W, height), facecolor=C["card"])
    inset(fig)
    top = 1 - 0.42 / height
    fig.text(0.055, top + 0.04 / height, title, fontsize=21, va="top", **TITLE)
    fig.text(0.055, top - 0.42 / height, subtitle, fontsize=10.5,
             color=C["muted"], va="top")
    x = 0.055
    for label, key in legend:
        fig.text(x, top - 0.68 / height, "●", fontsize=12, color=C[key], va="top", family="DejaVu Sans")
        fig.text(x + 0.022, top - 0.685 / height, label, fontsize=10.5,
                 color=C["muted"], va="top")
        x += 0.045 + 0.0105 * len(label)
    return fig


def axes(fig, height: float, header: float):
    ax = fig.add_axes([0.055, 0.35 / height, 0.89, 1 - (header + 0.5) / height])
    ax.set_facecolor("none")
    return ax


def line_axes(fig, height: float, header: float):
    ax = fig.add_axes([0.11, 0.85 / height, 0.82, 1 - (header + 1.0) / height])
    ax.set_facecolor("none")
    return ax


def pill(ax, y, x0, x1, color, **kw):
    ax.plot([x0, x1], [y, y], color=color, linewidth=PILL, solid_capstyle="round", **kw)


def bare(ax):
    for side in ax.spines.values():
        side.set_visible(False)
    ax.tick_params(which="both", length=0)


def save(fig, name: str):
    OUT.mkdir(exist_ok=True)
    slant(fig)
    fig.savefig(OUT / name, facecolor=C["card"])
    plt.close(fig)
    print(f"  {name}")


def cut_width(depth: float) -> float:
    """win7/cam.go cutWidth: tip + 2 depth tan(angle / 2)."""
    return TIP + 2 * depth * math.tan(math.radians(ANGLE / 2))


# ==============================
# ===  Dumped CAM data       ===
# ==============================

def load(name: str) -> dict:
    return json.loads((DATA / name).read_text())


def bits(r: dict) -> np.ndarray:
    return np.frombuffer(base64.b64decode(r["Bits"]), np.uint8).reshape(r["H"], r["W"])


def extent(r: dict):
    return [r["X0"], r["X0"] + r["W"] * r["Px"], r["Y0"], r["Y0"] + r["H"] * r["Px"]]


def copper_img(ax, r: dict, alpha=1.0):
    cmap = LinearSegmentedColormap.from_list("cu", [C["board"], C["copper"]])
    ax.imshow(bits(r), origin="lower", extent=extent(r), cmap=cmap, vmin=0, vmax=1,
              interpolation="nearest", alpha=alpha)


def paths_in(paths, box, ax, **kw):
    x0, x1, y0, y1 = box
    for p in paths:
        p = np.array(p)
        if p[:, 0].max() < x0 or p[:, 0].min() > x1 or p[:, 1].max() < y0 or p[:, 1].min() > y1:
            continue
        ax.plot(p[:, 0], p[:, 1], **kw)


def panel(ax, box, label):
    ax.set_xlim(box[0], box[1])
    ax.set_ylim(box[2], box[3])
    ax.set_aspect("equal")
    ax.set_xticks([])
    ax.set_yticks([])
    for s in ax.spines.values():
        s.set_color(C["grid"])
    ax.set_title(label, fontsize=11, color=C["ink"], loc="left", pad=6)


# ==============================
# ===  The figures           ===
# ==============================

def fig_width():
    """Why the height matters: the V-bit at three depths, the groove it leaves at the copper surface."""
    h = 4.6
    fig = card(h, "Чем глубже V-фреза, тем шире рез",
               f"Разрез фрезы: кончик {ru(TIP)} мм, угол {ru(ANGLE, 0)}°. "
               "Ширина канавки отмерена по поверхности меди")
    t = math.tan(math.radians(ANGLE / 2))
    cu, half, top, low = 0.035, 0.19, 0.16, -0.2
    # Two colours for the material, white for the marks.
    FR4, CU = "#222222", C["copper"]
    for i, d in enumerate((0.05, 0.1, 0.15)):
        ax = fig.add_axes([0.035 + i * 0.315, 0.04, 0.3, 0.7])
        ax.set_facecolor("none")
        w = cut_width(d)
        color = C["ink"]
        # Textolite with the groove cut out, then the copper layer on both sides of it.
        ax.fill([-half, -w / 2, -TIP / 2, TIP / 2, w / 2, half, half, -half],
                [0, 0, -d, -d, 0, 0, low, low], color=FR4, lw=0, zorder=1)
        ax.fill([-half, -w / 2, -w / 2 + cu * t, -half], [0, 0, -cu, -cu],
                color=CU, lw=0, zorder=2)
        ax.fill([w / 2, half, half, w / 2 - cu * t], [0, 0, -cu, -cu],
                color=CU, lw=0, zorder=2)
        # The bit: its flanks lie on the groove walls.
        bit_x = [-(TIP / 2 + (top + d) * t), -TIP / 2, TIP / 2, TIP / 2 + (top + d) * t]
        bit_y = [top, -d, -d, top]
        ax.fill(bit_x, bit_y, color=C["ink"], alpha=0.24, lw=0, zorder=3)
        ax.plot(bit_x, bit_y, color=C["ink"], lw=1.3, zorder=4)
        # Width under the panel, depth at the side.
        ax.text(0, low - 0.022, f"ширина {ru(w, 3)} мм", ha="center", va="top",
                fontsize=13, fontweight="bold", color=color)
        ax.plot([-half + 0.01, -TIP / 2 - 0.01], [-d, -d], color=C["muted"], lw=0.8,
                ls=(0, (3, 3)), zorder=5)
        ax.text(-half + 0.01, -d - 0.008, f"глубина {ru(d, 2)} мм", va="top",
                fontsize=10, color=C["muted"], zorder=6)
        ax.set_xlim(-half, half)
        ax.set_ylim(low - 0.07, top)
        ax.set_aspect("equal")
        ax.axis("off")
    save(fig, "width.png")


def fig_pipeline():
    """The four steps of the raster CAM on one corner of the test board."""
    d = load("star.json")
    cr = d["crop"]
    box = extent(cr)
    field = np.frombuffer(base64.b64decode(cr["Dist"]), "<u2").reshape(cr["H"], cr["W"])
    mm = np.where(field == 65535, np.nan, field / 8.0 * cr["Px"])
    half = d["cut"] / 2
    h = 4.3
    fig = card(h, "Как растровый CAM находит путь фрезы",
               "Угол тестовой платы, 9 × 9 мм. Расчёт на сетке 0,01 мм, как в программе")
    gs = fig.add_gridspec(1, 4, left=0.04, right=0.96, bottom=0.2, top=0.7, wspace=0.08)
    a = [fig.add_subplot(gs[0, k]) for k in range(4)]
    for ax in a:
        ax.set_facecolor(C["board"])
    copper_img(a[0], cr)
    panel(a[0], box, "1. медь в пикселях")
    cmap = LinearSegmentedColormap.from_list("fd", [C["board"], C["mine"], "#89dceb"])  # to Sapphire, then Sky
    a[1].imshow(np.minimum(mm, 0.5), origin="lower", extent=box, cmap=cmap, interpolation="bilinear")
    panel(a[1], box, "2. расстояние до меди")
    copper_img(a[2], cr, 0.55)
    xs = np.linspace(box[0], box[1], cr["W"])
    ys = np.linspace(box[2], box[3], cr["H"])
    a[2].contour(xs, ys, np.nan_to_num(mm, nan=9), levels=[half, half + d["cut"] * 0.7],
                 colors=[C["mine"]], linewidths=0.8)
    panel(a[2], box, "3. линии равного\nрасстояния")
    copper_img(a[3], cr)
    paths_in(d["iso"], box, a[3], color=C["mine"], lw=0.8)
    sh = [(p["X"], p["Y"]) for p in d["shorts"] or ()
          if box[0] < p["X"] < box[1] and box[2] < p["Y"] < box[3]]
    if sh:
        s = np.array(sh)
        a[3].scatter(s[:, 0], s[:, 1], s=60, facecolor="none", edgecolor=C["warn"], lw=1.6, zorder=5)
    panel(a[3], box, "4. проходы и места\nуже реза")
    fig.text(0.04, 0.095, f"Ширина реза {ru(d['cut'], 3)} мм. Первый проход идёт на половине ширины от меди, "
             "второй дальше с перекрытием 30 %.", fontsize=9.5, color=C["muted"])
    fig.text(0.04, 0.045, "Красные кольца: зазор уже реза, фреза между дорожками не пройдёт.",
             fontsize=9.5, color=C["muted"])
    save(fig, "pipeline.png")


def fig_board():
    """The whole test board with every isolation pass."""
    d = load("star.json")
    cu = d["copper"]
    b = d["box"]
    box = (b["X0"] - 1, b["X1"] + 1, b["Y0"] - 1, b["Y1"] + 1)
    h = 6.6
    fig = card(h, "Тестовая плата и проходы изоляции",
               f"84 × 58 мм: {len(d['iso'])} проходов, {len(d['shorts'])} мест уже реза "
               "(образцы нарушений правил задуманы)",
               legend=(("медь", "copper"), ("путь фрезы", "mine"), ("зазор уже реза", "warn")))
    ax = fig.add_axes([0.04, 0.03, 0.92, 0.78])
    ax.set_facecolor(C["board"])
    copper_img(ax, cu)
    paths_in(d["iso"], box, ax, color=C["mine"], lw=0.35)
    s = np.array([(p["X"], p["Y"]) for p in d["shorts"]])
    ax.scatter(s[:, 0], s[:, 1], s=14, facecolor="none", edgecolor=C["warn"], lw=0.9, zorder=5)
    ax.set_xlim(box[0], box[1])
    ax.set_ylim(box[2], box[3])
    ax.set_aspect("equal")
    ax.axis("off")
    save(fig, "board.png")


def fig_stage_grid():
    """The four cutting stages as pcbscan shows them: its own 3D view, cropped.
    The frames come from win7/figstages_test.go (make article-figs)."""
    from PIL import Image
    shots = DATA / "shots"
    names = (("06_stage_Фрезеровка.png", "фрезеровка дорожек"), ("07_stage_Маска.png", "вскрытие маски"),
             ("08_stage_Сверловка.png", "отверстия фрезой 1 мм"), ("09_stage_V-рез.png", "V-рез по контуру"))
    h = 6.4
    fig = card(h, "Четыре этапа одного листа",
               "Окно pcbscan в эмуляции, тестовая плата на листе 100 × 100 мм")
    gs = fig.add_gridspec(2, 2, left=0.03, right=0.97, bottom=0.03, top=0.8, wspace=0.03, hspace=0.14)
    for k, (f, label) in enumerate(names):
        im = Image.open(shots / f).crop((700, 300, 1400, 740))
        ax = fig.add_subplot(gs[k // 2, k % 2])
        ax.imshow(im)
        ax.set_title(label, fontsize=11, color=C["ink"], loc="left", pad=4)
        ax.axis("off")
    save(fig, "stages_grid.png")


def fig_predict():
    """Probing faster: approach the height the neighbours forecast."""
    h = 5.2
    fig = card(h, "Подлёт по прогнозу соседей",
               "Сверху команд на одно касание (симулятор), снизу ошибка прогноза вверх (карта станка)")
    ax = axes(fig, h, 1.25)
    y = 4.2
    ax.text(0, y - 0.15, "команд NcStudio на точку", fontsize=10, color=C["muted"])
    for name, v, key in COMMANDS:
        y -= 0.95
        pill(ax, y, 0.01, 0.99, C["track"], zorder=2)
        pill(ax, y, 0.01, 0.01 + 0.98 * v / 15, C[key], zorder=3)
        ax.text(0, y + 0.27, name, fontsize=11.5, va="bottom")
        ax.text(0.99, y + 0.27, ru(v, 1, strip=False), fontsize=13, fontweight="bold",
                color=C[key] if key != "naive" else C["muted"], ha="right", va="bottom")
    y -= 0.85
    ax.text(0, y, "медь выше прогноза не больше чем на, мм", fontsize=10, color=C["muted"])
    for name, v, key in FORECAST:
        y -= 0.95
        pill(ax, y, 0.01, 0.99, C["track"], zorder=2)
        pill(ax, y, 0.01, 0.01 + 0.98 * v / 0.16, C[key], zorder=3)
        ax.text(0, y + 0.27, name, fontsize=11.5, va="bottom")
        ax.text(0.99, y + 0.27, ru(v, 2, strip=False), fontsize=13, fontweight="bold",
                color=C[key] if key != "naive" else C["muted"], ha="right", va="bottom")
    ax.set_xlim(-0.02, 1.02)
    ax.set_ylim(y - 0.5, 4.3)
    ax.axis("off")
    save(fig, "predict.png")


def fig_stages():
    """Where the time of one board goes."""
    h = 3.9
    total = sum(v for _, v in STAGES)
    word = ("минуту" if total % 10 == 1 and total % 100 != 11 else
            "минуты" if total % 10 in (2, 3, 4) and total % 100 not in (12, 13, 14) else "минут")
    fig = card(h, f"Плата за {total} {word} работы станка",
               "Тестовая плата 84 × 58 мм, время программ по расчёту. Без смены фрез и карты высот")
    ax = axes(fig, h, 1.25)
    for i, (name, v) in enumerate(reversed(STAGES)):
        pill(ax, i, 0.01, 0.99, C["track"], zorder=2)
        pill(ax, i, 0.01, 0.01 + 0.98 * v / 24, C["mine"], zorder=3)
        ax.text(0, i + 0.27, name, fontsize=11.5, va="bottom")
        ax.text(0.99, i + 0.27, f"{v} мин", fontsize=13, fontweight="bold",
                color=C["mine"], ha="right", va="bottom")
    ax.set_xlim(-0.02, 1.02)
    ax.set_ylim(-0.5, 3.75)
    ax.axis("off")
    save(fig, "stages.png")


def board_photo():
    """The test board from photos/testboard-cover.jpg: straight, on black, cut to its edge.

    The edge was found by brightness: the board is 2228 x 1537 px, 84 x 58 mm.
    """
    from PIL import Image
    im = Image.open(HERE / "photos" / "testboard-cover.jpg").convert("RGB")
    return im.crop((72, 173, 2301, 1711))


def fig_cover():
    """Feed cover at twice 780x440: the test board whole on the right, the title on the left."""
    board = np.asarray(board_photo(), float) / 255
    bh, bw = board.shape[:2]
    fig = plt.figure(figsize=(7.8, 4.4), dpi=100, facecolor=C["card"])
    inset(fig, 300)
    h = 270 / 440
    w = h * 440 * bw / bh / 780
    ax = fig.add_axes([1 - w - 30 / 780, (1 - h) / 2, w, h])
    im = ax.imshow(board, interpolation="lanczos")
    # The board has round corners on black: clip them round on the card instead.
    im.set_clip_path(FancyBboxPatch((-0.5, -0.5), bw, bh, transform=ax.transData,
                                    boxstyle=f"round,pad=0,rounding_size={bh * 0.03:.0f}"))
    ax.axis("off")
    for y, line, color in ((0.84, "Свой CAM", C["ink"]), (0.72, "для фрезеровки", C["ink"]),
                           (0.60, "плат", C["mine"])):
        fig.text(0.05, y, line, fontsize=27, va="top", color=color, **TITLE)
    fig.text(0.05, 0.40, "Windows 7, станок без API,\nкарта высот самой фрезой", fontsize=12.5,
             color=C["muted"], va="top", linespacing=1.45)
    fig.text(0.05, 0.1, "дорожка и зазор 0,2 мм", fontsize=12, color=C["ink"], va="bottom")
    OUT.mkdir(exist_ok=True)
    slant(fig)
    fig.savefig(OUT / "cover.png", dpi=300, facecolor=C["card"])
    plt.close(fig)
    print("  cover.png")


def hand_board():
    """The test board from photos/testboard-hand.jpg, squared to 84 x 58 mm.

    The corners were read off the photo by eye; the fingers reach over the left and
    the right edge, so a strip on both sides is cut away.
    """
    from PIL import Image
    im = Image.open(HERE / "photos" / "testboard-hand.jpg").convert("RGB")
    src = [(103, 362), (978, 357), (996, 962), (101, 978)]
    w, h = 1680, 1160
    dst = [(0, 0), (w, 0), (w, h), (0, h)]
    a, b = [], []
    for (x, y), (u, v) in zip(dst, src):
        a.append([x, y, 1, 0, 0, 0, -u * x, -u * y])
        b.append(u)
        a.append([0, 0, 0, x, y, 1, -v * x, -v * y])
        b.append(v)
    coef = np.linalg.solve(np.array(a, float), np.array(b, float))
    board = im.transform((w, h), Image.PERSPECTIVE, tuple(coef), Image.BICUBIC)
    return board.crop((48, 4, 1650, h - 4))


# Photos of the article: what is cut away from each, in the pixels of the photo.
PHOTOS = (
    ("milled-board", (0, 270, 960, 1215)),     # the room above and the hand below
    ("machine-table", (60, 0, 960, 960)),      # the red frame and the cable below
    ("first-board", (0, 0, 372, 360)),         # the finger on the right
    ("testboard-hand", None),                  # squared, the fingers cut away
)


# Window shots of pcbscan in the article, framed like the photos and kept as PNG.
SCREENS = ("app_programs", "app_scan_running", "firstmap")


def framed(im, out: str, jpeg: bool):
    """Lay a picture on the grey ground alone, its corners round."""
    from PIL import Image
    pad = 0.12
    ph = np.asarray(im, float) / 255
    h, w = ph.shape[:2]
    side = INSET["margin"] + pad
    width = 7.0
    height = (width - 2 * side) * h / w + 2 * side
    fig = plt.figure(figsize=(width, height), facecolor=C["card"])
    ax = fig.add_axes([side / width, side / height, 1 - 2 * side / width, 1 - 2 * side / height])
    shown = ax.imshow(ph, interpolation="lanczos")
    shown.set_clip_path(FancyBboxPatch((-0.5, -0.5), w, h, transform=ax.transData,
                                       boxstyle=f"round,pad=0,rounding_size={min(w, h) * 0.025:.0f}"))
    ax.axis("off")
    OUT.mkdir(exist_ok=True)
    tmp = OUT / f"_{out}.png"
    fig.savefig(tmp, facecolor=C["card"])
    plt.close(fig)
    if jpeg:
        Image.open(tmp).convert("RGB").save(OUT / f"{out}.jpg", quality=90)
        tmp.unlink()
    else:
        tmp.rename(OUT / f"{out}.png")
    print(f"  {out}")


def fig_photos():
    """The photos and the window shots of the article, framed."""
    from PIL import Image
    for name, box in PHOTOS:
        im = hand_board() if box is None else \
            Image.open(HERE / "photos" / f"{name}.jpg").convert("RGB").crop(box)
        framed(im, f"photo-{name}", jpeg=True)
    for name in SCREENS:
        framed(Image.open(OUT / f"{name}.png").convert("RGB"), f"frame-{name}", jpeg=False)


def main():
    style()
    print(f"рисую в {OUT}")
    for fig in (fig_width, fig_pipeline, fig_board, fig_stage_grid, fig_predict, fig_photos,
                fig_stages, fig_cover):
        fig()


if __name__ == "__main__":
    main()
