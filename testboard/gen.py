"""The test board of the mill: every rule of docs/pcb-rules.md on one board.

python3 testboard/gen.py OUT_DIR
Writes OUT_DIR/pcbmill-test.kicad_pcb and .kicad_pro; make testboard fills the
zones, runs DRC and exports the Gerber zip for pcbscan.

The board is 84 x 58 mm: with the 8 mm margin of pcbscan it takes a 100 x 100
blank, and the strip above it holds the machine probes of pcbscan.
Copper on F.Cu only. Every zone is a letter, see docs/testboard.md.
"""

import json
import math
import os
import sys
import uuid

NAME = "pcbmill-test"
W, H = 84.0, 58.0          # the board
OX, OY = 100.0, 100.0      # its top left corner on the KiCad page
HOLE = 1.0                 # the only drill (docs/pcb-rules.md)

_uid = uuid.UUID("7e570000-0000-4000-8000-000000000000").int


def uid():
    global _uid
    _uid += 1
    return str(uuid.UUID(int=_uid))


def f(v):
    s = f"{v:.4f}".rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


def P(x, y):
    return OX + x, OY + y


nets = [""]


def net(name):
    if name not in nets:
        nets.append(name)
    return nets.index(name), name


items = []      # board level s-expressions
fps = []        # footprints


# ==========================
# ===  Primitives        ===
# ==========================

def pad_smd(num, x, y, w, h, n=None, shape="rect"):
    nt = f' (net {n[0]} "{n[1]}")' if n else ""
    return (f'    (pad "{num}" smd {shape} (at {f(x)} {f(y)}) (size {f(w)} {f(h)}) '
            f'(layers "F.Cu" "F.Mask"){nt} (uuid "{uid()}"))')


def pad_tht(num, x, y, size, n=None, shape="circle", drill=HOLE):
    nt = f' (net {n[0]} "{n[1]}")' if n else ""
    return (f'    (pad "{num}" thru_hole {shape} (at {f(x)} {f(y)}) (size {f(size[0])} {f(size[1])}) '
            f'(drill {f(drill)}) (layers "F.Cu" "F.Mask"){nt} (uuid "{uid()}"))')


def pad_npth(x, y, d=HOLE):
    return (f'    (pad "" np_thru_hole circle (at {f(x)} {f(y)}) (size {f(d)} {f(d)}) '
            f'(drill {f(d)}) (layers "F.Cu" "F.Mask") (uuid "{uid()}"))')


def footprint(name, x, y, pads, jumper=False):
    X, Y = P(x, y)
    out = [f'  (footprint "pcbmill-test:{name}" (layer "F.Cu") (uuid "{uid()}") (at {f(X)} {f(Y)})',
           f'    (property "Reference" "{name}" (at 0 0 0) (layer "F.Fab") (hide yes) (uuid "{uid()}") '
           f'(effects (font (size 1 1) (thickness 0.15))))',
           f'    (property "Value" "{name}" (at 0 0 0) (layer "F.Fab") (hide yes) (uuid "{uid()}") '
           f'(effects (font (size 1 1) (thickness 0.15))))',
           "    (attr smd board_only exclude_from_pos_files exclude_from_bom)"]
    if jumper:
        out.append('    (jumper_pad_groups ("1" "2"))')
    out += pads
    out.append("  )")
    fps.append("\n".join(out))


def track(a, b, w, n):
    A, B = P(*a), P(*b)
    items.append(f'  (segment (start {f(A[0])} {f(A[1])}) (end {f(B[0])} {f(B[1])}) '
                 f'(width {f(w)}) (layer "F.Cu") (net {n[0]}) (uuid "{uid()}"))')


def text(t, x, y, h=1.8, th=0.3, layer="F.Cu", just="left"):
    X, Y = P(x, y)
    items.append(f'  (gr_text "{t}" (at {f(X)} {f(Y)} 0) (layer "{layer}") (uuid "{uid()}") '
                 f'(effects (font (size {f(h)} {f(h)}) (thickness {f(th)})) (justify {just})))')


def poly(pts, layer="F.Cu"):
    s = " ".join(f"(xy {f(P(x, y)[0])} {f(P(x, y)[1])})" for x, y in pts)
    items.append(f'  (gr_poly (pts {s}) (stroke (width 0) (type solid)) (fill yes) (layer "{layer}") (uuid "{uid()}"))')


def rect(x0, y0, x1, y1, layer="F.Cu"):
    poly([(x0, y0), (x1, y0), (x1, y1), (x0, y1)], layer)


def zone(n, x0, y0, x1, y1, clearance, spoke, gap=0.5):
    pts = " ".join(f"(xy {f(P(x, y)[0])} {f(P(x, y)[1])})" for x, y in ((x0, y0), (x1, y0), (x1, y1), (x0, y1)))
    items.append(f'  (zone (net {n[0]}) (net_name "{n[1]}") (layer "F.Cu") (uuid "{uid()}") (hatch edge 0.5)\n'
                 f'    (connect_pads (clearance {f(clearance)}))\n'
                 f'    (min_thickness 0.25) (filled_areas_thickness no)\n'
                 f'    (fill yes (thermal_gap {f(gap)}) (thermal_bridge_width {f(spoke)}) (island_removal_mode 0))\n'
                 f'    (polygon (pts {pts}))\n'
                 f'  )')


def label(t, x, y):
    """The letter of a zone at its top left corner."""
    text(t, x, y + 1.5, h=2.0, th=0.35)


def small(t, x, y, h=1.2):
    """The value under a sample: 1.2 mm digits, they mill at 0.25 mm."""
    text(t, x, y, h=h, th=0.25)


# ==========================
# ===  Zones             ===
# ==========================

def zone_a(x0, y0):
    """A: combs of two nets, fingers 0.4 mm, the gap grows left to right.
    Ring the two pads of each comb: open is right, a short is copper left."""
    label("A", x0, y0)
    finger, length, n = 0.4, 6.0, 3
    x = x0 + 4.5
    for g in (0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.5):
        k = int(round(g * 100))
        a, b = net(f"A{k}a"), net(f"A{k}b")
        pitch = 2 * (finger + g)
        span = n * pitch
        top = y0 + 2.5
        bot = top + 0.25 + length + g + 0.25
        track((x - 1.2, top), (x + span, top), 0.5, a)
        track((x, bot), (x + span + 1.2, bot), 0.5, b)
        for i in range(n):
            fa = x + i * pitch + finger / 2
            fb = fa + finger + g
            # the round end of a finger stops g short of the other spine
            track((fa, top), (fa, bot - 0.25 - g - finger / 2), finger, a)
            track((fb, bot), (fb, top + 0.25 + g + finger / 2), finger, b)
        footprint(f"A{k}", x + span / 2, (top + bot) / 2,
                  [pad_smd("1", -span / 2 - 1.2, -(bot - top) / 2, 1.6, 1.6, a, "circle"),
                   pad_smd("2", span / 2 + 1.2, (bot - top) / 2, 1.6, 1.6, b, "circle")])
        small(f"{g:.2f}".lstrip("0"), x + span / 2 - 1.0, bot + 2.0, 1.0)
        x += span + 4.8


def zone_b(x0, y0):
    """B: tracks of every width, 16 mm, a pad at each end."""
    label("B", x0, y0)
    y = y0 + 4.2
    for w in (0.15, 0.2, 0.25, 0.3, 0.4, 0.5, 0.8, 1.0):
        n = net(f"B{int(round(w * 100))}")
        track((x0 + 2.0, y), (x0 + 18.0, y), w, n)
        footprint(f"B{int(round(w * 100))}", x0 + 10.0, y,
                  [pad_smd("1", -8, 0, 1.6, 1.6, n, "circle"), pad_smd("2", 8, 0, 1.6, 1.6, n, "circle")])
        small(f"{w:g}", x0 + 19.5, y + 0.5, 1.0)
        y += 2.2


def zone_c(x0, y0):
    """C: round pads round a 1.0 mm hole, the ring grows; two ovals."""
    label("C", x0, y0)
    x = x0 + 4.5
    for d in (1.4, 1.6, 1.8, 2.0, 2.2, 2.5):
        footprint(f"C{int(d * 10)}", x, y0 + 4.0, [pad_tht("1", 0, 0, (d, d), net(f"C{int(d * 10)}"))])
        small(f"{d:g}", x - 1.1, y0 + 7.4, 1.0)
        x += 3.7
    for w, h in ((1.3, 2.0), (1.5, 2.2)):
        footprint(f"CO{int(w * 10)}", x, y0 + 4.0, [pad_tht("1", 0, 0, (w, h), net(f"CO{int(w * 10)}"), "oval")])
        small(f"{w:g}x{h:g}", x - 2.0, y0 + 7.4, 1.0)
        x += 4.8


def zone_d(x0, y0):
    """D: plain holes in pairs, the bridge between them narrows."""
    label("D", x0, y0)
    x = x0 + 4.0
    for gap in (0.4, 0.6, 0.8, 1.0, 1.2):
        footprint(f"D{int(gap * 10)}", x, y0 + 3.0, [pad_npth(0, 0), pad_npth(0, HOLE + gap)])
        small(f"{gap:g}", x - 0.9, y0 + 7.4, 1.0)
        x += 3.0


def zone_d2(x0, y0):
    """D2: five ovals at 2.54 as a header; a 0.3 mm track goes down between
    pins 1-2 and back up between pins 4-5."""
    xr, yr = x0 + 1.5, y0 + 4.5
    t = net("Dtrack")
    pads = [pad_tht(str(i + 1), i * 2.54, 0, (1.5, 2.2), net(f"Dp{i}"), "oval") for i in range(5)]
    pads += [pad_smd("6", 1.27, -3.4, 1.2, 1.2, t, "circle"), pad_smd("7", 3 * 2.54 + 1.27, -3.4, 1.2, 1.2, t, "circle")]
    footprint("Drow", xr, yr, pads)
    a, b = xr + 1.27, xr + 3 * 2.54 + 1.27
    track((a, yr - 3.4), (a, yr + 2.6), 0.3, t)
    track((a, yr + 2.6), (b, yr + 2.6), 0.3, t)
    track((b, yr + 2.6), (b, yr - 3.4), 0.3, t)


def zone_e(x0, y0):
    """E: SMD footprints: 0805, 1206, SOT-23; SOIC at 1.27, TSSOP at 0.65
    with 0.35 pads, and 0.5 with 0.25 pads."""
    label("E", x0, y0)
    footprint("E0805", x0 + 4.5, y0 + 4.5, [pad_smd("1", -0.95, 0, 0.9, 1.25, net("E0805a")),
                                            pad_smd("2", 0.95, 0, 0.9, 1.25, net("E0805b"))])
    footprint("E1206", x0 + 10.5, y0 + 4.5, [pad_smd("1", -1.5, 0, 1.15, 1.8, net("E1206a")),
                                             pad_smd("2", 1.5, 0, 1.15, 1.8, net("E1206b"))])
    footprint("ESOT23", x0 + 16, y0 + 4.5, [pad_smd("1", -0.95, 1.1, 0.6, 0.7, net("ES1")),
                                            pad_smd("2", 0.95, 1.1, 0.6, 0.7, net("ES2")),
                                            pad_smd("3", 0, -1.1, 0.6, 0.7, net("ES3"))])
    small("0805", x0 + 3.0, y0 + 7.8, 1.0)
    small("1206", x0 + 8.9, y0 + 7.8, 1.0)
    x = x0 + 3.5
    for pitch, pw, tag in ((1.27, 0.6, "1.27"), (0.65, 0.35, ".65"), (0.5, 0.25, ".5")):
        pads = []
        for i in range(4):
            xx = (i - 1.5) * pitch
            pads.append(pad_smd(str(i + 1), xx, 1.6, pw, 1.4, net(f"E{tag}_{i}")))
            pads.append(pad_smd(str(8 - i), xx, -1.6, pw, 1.4, net(f"E{tag}_{7 - i}")))
        footprint(f"E{tag}", x, y0 + 13.5, pads)
        small(tag, x - 1.0, y0 + 17.8, 1.0)
        x += 6.5


def zone_f(x0, y0):
    """F: copper text from 1 to 3 mm high; a star of wedges for sharp
    corners; a ring for round paths; a square for straight ones."""
    label("F", x0, y0)
    x = x0 + 3.0
    for h, th in ((1.0, 0.2), (1.5, 0.25), (2.0, 0.3), (2.5, 0.35), (3.0, 0.4)):
        text("Aш8", x, y0 + 4.0, h, th)
        small(f"{h:g}", x, y0 + 7.4, 1.0)
        x += h * 2.6 + 1.0
    cx, cy, r = x0 + 5.0, y0 + 14.0, 4.5
    k = 12
    for i in range(k):
        a0 = 2 * math.pi * i / k
        a1 = a0 + math.pi / k
        poly([(cx, cy), (cx + r * math.cos(a0), cy + r * math.sin(a0)), (cx + r * math.cos(a1), cy + r * math.sin(a1))])
    ring(x0 + 16.0, y0 + 14.0, 4.0, 0.4)
    sq(x0 + 23.5, y0 + 10.0, 8.0, 0.4)


def ring(cx, cy, r, w):
    X, Y = P(cx, cy)
    items.append(f'  (gr_circle (center {f(X)} {f(Y)}) (end {f(X + r)} {f(Y)}) (stroke (width {f(w)}) (type solid)) '
                 f'(fill no) (layer "F.Cu") (uuid "{uid()}"))')


def sq(x, y, a, w):
    X, Y = P(x, y)
    items.append(f'  (gr_rect (start {f(X)} {f(Y)}) (end {f(X + a)} {f(Y + a)}) (stroke (width {f(w)}) (type solid)) '
                 f'(fill no) (layer "F.Cu") (uuid "{uid()}"))')


def zone_g(x0, y0):
    """G: three pours, each with a pad on thermals of its own width and
    clearance: 0.3, 0.4, 0.5 left to right."""
    label("G", x0, y0)
    x = x0 + 4.0
    for k, (spoke, clear) in enumerate(((0.3, 0.3), (0.4, 0.4), (0.5, 0.5))):
        n = net(f"G{k}")
        zone(n, x, y0 + 1.0, x + 9, y0 + 9.5, clear, spoke)
        footprint(f"G{k}", x + 4.5, y0 + 5.25, [pad_tht("1", 0, 0, (2.2, 2.2), n)])
        small(f"{spoke:g}", x + 0.8, y0 + 8.4, 1.0)  # the pour goes round it
        x += 10.5


def zone_h(y0, y1):
    """H: copper strips along the right edge, 0.5 to 1.5 mm from it."""
    label("H", W - 7.5, y0)
    y = y0 + 3
    seg = (y1 - y0 - 3) / 4
    for d in (0.5, 0.75, 1.0, 1.5):
        n = net(f"H{int(d * 100)}")
        x = W - d - 0.25
        track((x, y), (x, y + seg - 1.2), 0.5, n)
        small(f"{d:g}", x - 4.0, y + seg / 2, 1.0)
        y += seg


def zone_i(x0, y0):
    """I: openings of the mask over solid copper, 0.2 to 2 mm."""
    label("I", x0, y0)
    rect(x0 + 3.0, y0 + 1.0, x0 + 18.5, y0 + 7.0)
    x = x0 + 4.0
    for s in (0.2, 0.3, 0.5, 0.8, 1.2, 2.0):
        rect(x, y0 + 4 - s / 2, x + s, y0 + 4 + s / 2, "F.Mask")
        x += s + 1.6


def zone_j(x0, y0):
    """J: a wire jumper over a track: two 1.0 mm holes 8 mm apart."""
    label("J", x0, y0)
    n = net("Jwire")
    cx, cy = x0 + 7, y0 + 5
    footprint("J1", cx, cy, [pad_tht("1", -4, 0, (2.2, 2.2), n), pad_tht("2", 4, 0, (2.2, 2.2), n)], jumper=True)
    t = net("Jtrack")
    track((cx, cy - 4.2), (cx, cy + 4.2), 0.4, t)
    footprint("J2", cx, cy, [pad_smd("1", 0, -4.2, 1.6, 1.6, t, "circle"), pad_smd("2", 0, 4.2, 1.6, 1.6, t, "circle")])


def zone_k():
    """K: crosses with a 1.0 mm hole in their centre, at the corners: the
    distance between the holes is 76 x 50 mm."""
    for k, (x, y) in enumerate(((4, 4), (80, 4), (4, 54), (80, 54))):
        n = net(f"K{k}")
        footprint(f"K{k}", x, y, [pad_tht("1", 0, 0, (2.2, 2.2), n)])
        track((x - 2.5, y), (x + 2.5, y), 0.3, n)
        track((x, y - 2.5), (x, y + 2.5), 0.3, n)


# ==========================
# ===  The board         ===
# ==========================

def board():
    zone_k()
    zone_a(7.5, 1.0)        # 7.5..74, 0.5..14
    zone_b(1.5, 15.5)       # 1.5..23, 15.5..35
    zone_e(26.0, 15.5)      # 26..45, 15.5..34
    zone_f(47.0, 15.5)      # 47..80, 15.5..34
    zone_c(1.5, 36.0)       # 1.5..37, 36..45
    zone_d(39.0, 36.0)      # 39..55, 36..45
    zone_i(57.0, 37.5)      # 57..77, 37.5..45
    zone_h(36.0, 50.0)      # the right edge, y 36..50
    zone_g(7.5, 47.0)       # 7.5..39, 47..57
    zone_d2(42.5, 47.0)     # 41..54, 47..56
    zone_j(57.0, 47.0)      # 57..71, 47..57

    out = [f'(kicad_pcb (version 20241229) (generator "pcbmill-test") (generator_version "1")',
           '  (general (thickness 1.5) (legacy_teardrops no))',
           '  (paper "A4")',
           '  (title_block (title "pcbmill test board"))',
           '  (layers',
           '    (0 "F.Cu" signal) (2 "B.Cu" signal)',
           '    (9 "F.Adhes" user "F.Adhesive") (11 "B.Adhes" user "B.Adhesive")',
           '    (13 "F.Paste" user) (15 "B.Paste" user)',
           '    (5 "F.SilkS" user "F.Silkscreen") (7 "B.SilkS" user "B.Silkscreen")',
           '    (1 "F.Mask" user) (3 "B.Mask" user)',
           '    (17 "Dwgs.User" user "User.Drawings") (19 "Cmts.User" user "User.Comments")',
           '    (25 "Edge.Cuts" user) (27 "Margin" user)',
           '    (31 "F.CrtYd" user "F.Courtyard") (29 "B.CrtYd" user "B.Courtyard")',
           '    (35 "F.Fab" user) (33 "B.Fab" user)',
           '  )',
           '  (setup (pad_to_mask_clearance 0.05) (allow_soldermask_bridges_in_footprints yes)',
           '    (pcbplotparams (layerselection 0x00000000_00000000_55555555_5755f5ff) (outputdirectory "gerber/")))']
    for i, n in enumerate(nets):
        out.append(f'  (net {i} "{n}")')
    out += fps
    out += items
    x0, y0 = P(0, 0)
    x1, y1 = P(W, H)
    out.append(f'  (gr_rect (start {f(x0)} {f(y0)}) (end {f(x1)} {f(y1)}) '
               f'(stroke (width 0.1) (type solid)) (fill no) (layer "Edge.Cuts") (uuid "{uid()}"))')
    out.append(")")
    return "\n".join(out) + "\n"


def project():
    # the real minima of docs/pcb-rules.md; the board breaks them on purpose
    # in its samples, make testboard lists what DRC finds
    rules = {"min_clearance": 0.3, "min_track_width": 0.3, "min_copper_edge_clearance": 1.0,
             "min_through_hole_diameter": 1.0, "min_hole_to_hole": 0.8, "min_hole_clearance": 0.25,
             "min_via_annular_width": 0.1, "min_via_diameter": 0.5, "min_text_height": 1.0,
             "min_text_thickness": 0.15, "min_silk_clearance": 0.0, "solder_mask_to_copper_clearance": 0.0,
             "min_resolved_spokes": 1, "max_error": 0.005, "allow_blind_buried_vias": False,
             "allow_microvias": False}
    sev = {"lib_footprint_issues": "ignore", "lib_footprint_mismatch": "ignore", "missing_courtyard": "ignore",
           "isolated_copper": "ignore", "solder_mask_bridge": "ignore", "silk_overlap": "ignore"}
    return json.dumps({
        "board": {"design_settings": {"rules": rules, "rule_severities": sev,
                                      "track_widths": [0.0, 0.4], "via_dimensions": []}},
        "meta": {"filename": f"{NAME}.kicad_pro", "version": 3},
        "net_settings": {"classes": [{"name": "Default", "clearance": 0.15, "track_width": 0.4,
                                      "via_diameter": 2.0, "via_drill": 1.0, "priority": 2147483647}],
                         "meta": {"version": 4}},
    }, indent=2) + "\n"


def main():
    out = sys.argv[1]
    os.makedirs(out, exist_ok=True)
    with open(os.path.join(out, f"{NAME}.kicad_pcb"), "w", encoding="utf-8") as fh:
        fh.write(board())
    with open(os.path.join(out, f"{NAME}.kicad_pro"), "w", encoding="utf-8") as fh:
        fh.write(project())


if __name__ == "__main__":
    main()
