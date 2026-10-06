package main

// Gerber -> NcStudio programs: isolation round the copper with the 0.1 V-bit,
// opening of the solder mask over the pads, drilling, and a V-groove along
// the board edge to break the boards apart. The same rules as pcbmill/toolpath.py,
// built on a fine pixel grid instead of polygon offsets.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// camCfg: everything the operator may change, saved next to the exe.
type camCfg struct {
	Tip, Angle      float64 // the V-bit for isolation and the mask
	Spindle         float64
	IsoDepth        float64
	Passes          int
	IsoFeed, Plunge float64
	IsoTest         bool // the probe of feeds and depths of isolation on the sheet
	MaskDepth       float64
	MaskFeed        float64
	MaskFlat        float64 // a flat end mill of this diameter for the mask, 0: the V-bit
	MaskTest        bool
	DrillMode       int // drillThrough, drillSpot, drillBoth
	Thickness       float64
	DrillFeed       float64
	VDepth, VStep   float64
	VFeed           float64
	Side            int // sideTop, sideBottom: the copper side milled, face up
}

const (
	drillThrough = iota
	drillSpot
	drillBoth
)

const (
	sideTop = iota
	sideBottom
)

// defaultCam: the numbers of machine.toml, which the Vesi programs ran with.
func defaultCam() camCfg {
	return camCfg{Tip: 0.1, Angle: 45, Spindle: 8000, IsoDepth: 0.1, Passes: 2, IsoFeed: 300, Plunge: 200,
		MaskDepth: 0.03, MaskFeed: 200, MaskTest: true, DrillMode: drillThrough, Thickness: 1.5, DrillFeed: 60,
		VDepth: 0.8, VStep: 0.2, VFeed: 200, Side: sideTop}
}

// maskWidth: the trace of the mask tool at a depth.
func (c camCfg) maskWidth(depth float64) float64 {
	if c.MaskFlat > 0 {
		return c.MaskFlat
	}
	return c.cutWidth(depth)
}

// cutWidth of the V-bit at a depth.
func (c camCfg) cutWidth(depth float64) float64 {
	return c.Tip + 2*depth*math.Tan(c.Angle/2*math.Pi/180)
}

const (
	safeZ   = 4.0   // above clamps, at the start and the end
	travelZ = 1.0   // between paths
	overlap = 0.3   // of isolation passes, part of the cut width
	simpTol = 0.003 // how far a simplified path may stray, mm
	pecks   = 0.5   // drilling in steps of
	spotZ   = 0.3   // depth of a drill mark
)

// p3 is a point of a path; a path cuts at Z from its first point on.
type p3 struct{ X, Y, Z float64 }

type cpath struct {
	pts          []p3
	peck         float64 // a drilled hole: down to pts[0].Z in steps of peck
	feed, plunge float64 // of this path only (the probes); 0: those of the file
	spindle      float64 // the probes: the speed for this path, 0: as it is
}

func (p cpath) feedOr(f float64) float64 {
	if p.feed > 0 {
		return p.feed
	}
	return f
}

func (p cpath) plungeOr(f float64) float64 {
	if p.plunge > 0 {
		return p.plunge
	}
	return f
}

// camOp: one program file.
type camOp struct {
	file    string // the part after the board name: "1_iso_top"
	title   string // for the screen: "Изоляция, верх"
	paths   []cpath
	plunge  float64
	feed    float64
	spindle float64
	notes   []string
	warn    []string
	marks   []pt    // places to show on the picture: shorts, tiny openings
	shortW  float64 // isolation: the cut width the shorts were found for
	tool    string  // what goes into the spindle: "V 0.1/45", "сверло 0.8"
}

// ==========================
// ===  Placement         ===
// ==========================

// place puts a board on the sheet: the corner of its outline box at the
// origin, the bottom side turned over left to right inside that box, then
// turned rot quarter turns counter-clockwise and moved by dx, dy.
type place struct {
	box    rect
	mirror bool
	rot    int
	dx, dy float64
}

// size: the box of the board as it lies on the sheet.
func (p place) size() (float64, float64) {
	w, h := p.box.X1-p.box.X0, p.box.Y1-p.box.Y0
	if p.rot%2 == 1 {
		return h, w
	}
	return w, h
}

func (p place) at(q pt) pt {
	w, h := p.box.X1-p.box.X0, p.box.Y1-p.box.Y0
	x, y := q.X-p.box.X0, q.Y-p.box.Y0
	if p.mirror {
		x = p.box.X1 - q.X
	}
	switch p.rot & 3 {
	case 1:
		x, y = h-y, x
	case 2:
		x, y = w-x, h-y
	case 3:
		x, y = y, w-x
	}
	return pt{x + p.dx, y + p.dy}
}

func rawPaths(loops [][]pt, z float64) []cpath {
	out := make([]cpath, 0, len(loops))
	for _, l := range loops {
		ps := make([]p3, len(l))
		for i, q := range l {
			ps[i] = p3{q.X, q.Y, z}
		}
		out = append(out, cpath{pts: ps})
	}
	return out
}

// placeOp: the operation of one board for every place it has on the sheet.
// Turning the board over turns the loops round as well, so they are run
// backwards to keep the copper on the left of the tool.
func placeOp(op *camOp, pls []place) *camOp {
	out := *op
	out.paths, out.marks = nil, nil
	for _, p := range pls {
		for _, c := range op.paths {
			ps := make([]p3, len(c.pts))
			for i, q := range c.pts {
				w := p.at(pt{q.X, q.Y})
				k := i
				if p.mirror {
					k = len(c.pts) - 1 - i
				}
				ps[k] = p3{w.X, w.Y, q.Z}
			}
			out.paths = append(out.paths, cpath{pts: ps, peck: c.peck})
		}
		for _, m := range op.marks {
			out.marks = append(out.marks, p.at(m))
		}
	}
	return &out
}

// ==========================
// ===  Isolation         ===
// ==========================

// boardMask: the pixels inside the outline, nil when the outline is open.
func boardMask(b *board, r *raster) *raster {
	if !b.closed {
		return nil
	}
	m := &raster{x0: r.x0, y0: r.y0, px: r.px, w: r.w, h: r.h, bits: make([]byte, len(r.bits))}
	m.fill(b.outline, 1)
	return m
}

// isolation: passes round the copper of one side at growing distance; where
// two islands are closer than the cut the tool cannot pass, they are reported.
func isolation(b *board, side string, c camCfg, field []uint16) (*camOp, []uint16) {
	g := b.copper[side]
	w := c.cutWidth(c.IsoDepth)
	step := w * (1 - overlap)
	reach := w/2 + float64(c.Passes-1)*step
	cu, _ := g.bounds()
	area := cu.union(b.box)
	m := reach + w + 0.5
	r := newRaster(rect{area.X0 - m, area.X1 + m, area.Y0 - m, area.Y1 + m}, 0.01)
	for _, s := range g.shapes {
		r.draw(s)
	}
	name, title := "1_iso", "Фрезеровка дорожек"
	if side == lyBottom {
		title = "Фрезеровка дорожек, низ (зеркально)"
	}
	op := &camOp{file: name, title: title, plunge: c.Plunge, feed: c.IsoFeed, spindle: c.Spindle}
	// the cut stays within w of the board, as in pcbmill: outline.buffer(w);
	// needed only when the copper comes that close to the edge
	field = r.distTo(1, field)
	limited := false
	if bm := boardMask(b, r); bm != nil && !(cu.X0-reach > b.box.X0+w && cu.X1+reach < b.box.X1-w &&
		cu.Y0-reach > b.box.Y0+w && cu.Y1+reach < b.box.Y1-w) {
		far := bm.distTo(1, nil)
		lim := uint16(math.Min((w/r.px+0.5)*distScale, distFar-1))
		for k, v := range far {
			if v > lim {
				field[k] = distFar // too far from the board
			}
		}
		limited = true
	}
	var ts []float64
	for k := 0; k < c.Passes; k++ {
		ts = append(ts, ((w/2+float64(k)*step)/r.px+0.5)*distScale)
	}
	var loops [][]pt
	for _, l := range r.levelsLines(field, ts, false) {
		if l = simplify(l, simpTol); len(l) >= 3 {
			loops = append(loops, l)
		}
	}
	op.paths = rawPaths(loops, -c.IsoDepth)
	op.notes = append(op.notes, fmt.Sprintf("ширина реза %.3f мм на глубине %.2f, обходов %d, полоса %.2f мм",
		w, c.IsoDepth, c.Passes, w+step*float64(c.Passes-1)))
	// shorts: closing \ copper, the pieces of it touching two islands
	if limited {
		field = r.distTo(1, field) // the board limit off again: a short at the edge is a short too
	}
	t0 := (w/2/r.px + 0.5) * distScale
	gap := &raster{x0: r.x0, y0: r.y0, px: r.px, w: r.w, h: r.h, bits: make([]byte, len(r.bits))}
	for k, v := range field {
		if float64(v) < t0 {
			gap.bits[k] = 1 // where the tool centre cannot be
		}
	}
	field = gap.distTo(0, field)
	for k, v := range field {
		gap.bits[k] = 0
		if float64(v) > t0 && r.bits[k] == 0 {
			gap.bits[k] = 1 // not copper, and the tool never reaches it
		}
	}
	cuPieces := r.pieces()
	gp := gap.pieces()
	type acc struct {
		sx, sy float64
		n      int
		isl    map[int]bool
	}
	accs := map[int]*acc{}
	for j := 0; j < gap.h; j++ {
		for q := gp.start[j]; q < gp.start[j+1]; q++ {
			id := gp.find(q)
			a := accs[id]
			if a == nil {
				a = &acc{isl: map[int]bool{}}
				accs[id] = a
			}
			for i := int(gp.a[q]); i <= int(gp.b[q]); i++ {
				a.sx += float64(i)
				a.sy += float64(j)
				a.n++
				for _, nb := range [4][2]int{{i - 1, j}, {i + 1, j}, {i, j - 1}, {i, j + 1}} {
					if r.at(nb[0], nb[1]) && len(a.isl) < 3 {
						a.isl[cuPieces.label(nb[0], nb[1])] = true
					}
				}
			}
		}
	}
	var shorts []pt
	for _, a := range accs {
		if len(a.isl) >= 2 && float64(a.n)*r.px*r.px > 1e-4 {
			shorts = append(shorts, r.world(a.sx/float64(a.n), a.sy/float64(a.n)))
		}
	}
	sort.Slice(shorts, func(i, j int) bool {
		if shorts[i].Y != shorts[j].Y {
			return shorts[i].Y > shorts[j].Y
		}
		return shorts[i].X < shorts[j].X
	})
	op.marks = shorts
	op.shortW = w
	return op, field
}

// ==========================
// ===  Solder mask       ===
// ==========================

// maskOpen: rings inside each opening of the mask from its edge inwards, a
// step under the radius of the cut so nothing is left between them. An
// opening narrower than the cut gets a single plunge in its middle.
func maskOpen(b *board, side string, c camCfg, field []uint16) (*camOp, []uint16) {
	g := b.mask[side]
	wm := c.maskWidth(c.MaskDepth)
	rt := wm / 2
	step := 0.9 * rt
	mb, _ := g.bounds()
	r := newRaster(rect{mb.X0 - 0.5, mb.X1 + 0.5, mb.Y0 - 0.5, mb.Y1 + 0.5}, 0.01)
	for _, s := range g.shapes {
		r.draw(s)
	}
	name, title := "2_mask", "Вскрытие маски"
	if side == lyBotMask {
		title = "Вскрытие маски, низ (зеркально)"
	}
	op := &camOp{file: name, title: title, plunge: c.Plunge, feed: c.MaskFeed, spindle: c.Spindle}
	field = r.distTo(0, field)
	top := uint16(0)
	for _, v := range field {
		if v > top && v < distFar {
			top = v
		}
	}
	var ts []float64
	for k := 0; ; k++ {
		t := ((rt+float64(k)*step)/r.px + 0.5) * distScale
		if t >= float64(top) || k > 2000 {
			break
		}
		ts = append(ts, t)
	}
	var loops [][]pt
	for _, l := range r.levelsLines(field, ts, true) {
		if l = simplify(l, simpTol); len(l) >= 3 {
			loops = append(loops, l)
		}
	}
	// openings where even the first ring does not fit
	t0 := (rt/r.px + 0.5) * distScale
	ps := r.pieces()
	type best struct {
		v    uint16
		i, j int
	}
	tops := map[int]*best{}
	for j := 0; j < r.h; j++ {
		for q := ps.start[j]; q < ps.start[j+1]; q++ {
			id := ps.find(q)
			bb := tops[id]
			if bb == nil {
				bb = &best{}
				tops[id] = bb
			}
			for i := int(ps.a[q]); i <= int(ps.b[q]); i++ {
				if v := field[j*r.w+i]; v > bb.v {
					*bb = best{v, i, j}
				}
			}
		}
	}
	var dots [][]pt
	for _, bb := range tops {
		if float64(bb.v) <= t0 && bb.v > 0 {
			w := r.world(float64(bb.i), float64(bb.j))
			dots = append(dots, []pt{w})
		}
	}
	sort.Slice(dots, func(i, j int) bool {
		if dots[i][0].X != dots[j][0].X {
			return dots[i][0].X < dots[j][0].X
		}
		return dots[i][0].Y < dots[j][0].Y
	})
	op.paths = rawPaths(append(loops, dots...), -c.MaskDepth)
	op.notes = append(op.notes, fmt.Sprintf("окон %d, след фрезы %.3f мм на глубине %.3f", len(tops), wm, c.MaskDepth))
	if len(dots) > 0 {
		op.warn = append(op.warn, fmt.Sprintf("%d окон уже следа фрезы %.2f мм: там одно погружение в середину, окно выйдет чуть шире", len(dots), wm))
		for _, d := range dots {
			op.marks = append(op.marks, d[0])
		}
	}
	return op, field
}

// probeW, probeH: the room of the mask probe on the sheet.
const probeW, probeH = 25.5, 3.0

// The isolation probe: grooves isoProbeLen long, a row per depth, a column
// per feed, and over them a row of plunges, one per plunge feed.
var (
	isoProbeDepths  = []float64{0.04, 0.06, 0.08, 0.10, 0.12, 0.15}
	isoProbeFeeds   = []float64{100, 200, 300, 400, 600, 800}
	isoProbePlunges = []float64{50, 100, 150, 200, 300, 400}
	isoProbeSpeeds  = []float64{6000, 8000, 10000, 12000, 15000, 18000}
	drillProbeFeeds = []float64{20, 30, 45, 60, 90, 120}
	vProbeDepths    = []float64{0.4, 0.6, 0.8, 1.0, 1.2}
)

const (
	isoProbeLen, isoProbeDX, isoProbeDY = 5.0, 7.0, 2.0
	isoProbeW                           = 6*isoProbeDX - (isoProbeDX - isoProbeLen)
	isoProbeH                           = 8 * isoProbeDY
	drillProbeDX                        = 4.0
	drillProbeW, drillProbeH            = 5*drillProbeDX + 1, 1.0
	vProbeLen, vProbeDY                 = 8.0, 2.5
	vProbeW, vProbeH                    = vProbeLen, 4*vProbeDY + 1
)

// isoTest: the grooves of the probe from at; the plunges go in at the depth
// of isolation and run on 2 mm at its feed.
func isoTest(at pt, c camCfg) *camOp {
	op := &camOp{file: "1_iso_probe", title: "Проба подач и глубин изоляции", plunge: c.Plunge, feed: c.IsoFeed, spindle: c.Spindle}
	for i, d := range isoProbeDepths {
		y := at.Y + float64(i)*isoProbeDY
		for k, f := range isoProbeFeeds {
			x := at.X + float64(k)*isoProbeDX
			op.paths = append(op.paths, cpath{pts: []p3{{x, y, -d}, {x + isoProbeLen, y, -d}}, feed: f})
		}
	}
	y := at.Y + float64(len(isoProbeDepths))*isoProbeDY
	for k, f := range isoProbePlunges {
		x := at.X + float64(k)*isoProbeDX
		op.paths = append(op.paths, cpath{pts: []p3{{x, y, -c.IsoDepth}, {x + 2, y, -c.IsoDepth}}, plunge: f})
	}
	y += isoProbeDY
	for k, s := range isoProbeSpeeds {
		x := at.X + float64(k)*isoProbeDX
		op.paths = append(op.paths, cpath{pts: []p3{{x, y, -c.IsoDepth}, {x + isoProbeLen, y, -c.IsoDepth}}, spindle: s})
	}
	for i := range op.paths { // the order of paths changes: each says its speed
		if op.paths[i].spindle == 0 {
			op.paths[i].spindle = c.Spindle
		}
	}
	op.notes = append(op.notes,
		fmt.Sprintf("канавки %s мм от X%.1f Y%.1f: снизу вверх глубина %s, слева направо подача %s мм/мин",
			num(isoProbeLen), at.X, at.Y, joinNums(isoProbeDepths), joinNums(isoProbeFeeds)),
		fmt.Sprintf("ряд над ними: врезание на %s мм со скоростью %s мм/мин", num(c.IsoDepth), joinNums(isoProbePlunges)),
		fmt.Sprintf("верхний ряд: обороты %s при подаче %s", joinNums(isoProbeSpeeds), num(c.IsoFeed)))
	return op
}

// drillTest: through holes of the 1.0 drill from at, left to right a faster
// feed of the drill each.
func drillTest(at pt, c camCfg) *camOp {
	op := &camOp{file: "3_drill_probe", title: "Проба подач сверла", plunge: c.DrillFeed, feed: c.DrillFeed, spindle: c.Spindle}
	for k, f := range drillProbeFeeds {
		x := at.X + 0.5 + float64(k)*drillProbeDX
		op.paths = append(op.paths, cpath{pts: []p3{{x, at.Y + 0.5, -(c.Thickness + 0.3)}}, peck: pecks, plunge: f})
	}
	op.notes = append(op.notes, fmt.Sprintf("отверстия от X%.1f Y%.1f, слева направо подача сверла %s мм/мин",
		at.X, at.Y, joinNums(drillProbeFeeds)))
	return op
}

// vcutTest: grooves of the V-cutter from at, bottom up deeper each, in steps
// of the V-cut.
func vcutTest(at pt, c camCfg) *camOp {
	op := &camOp{file: "4_vcut_probe", title: "Проба глубины V-реза", plunge: c.Plunge / 2, feed: c.VFeed, spindle: c.Spindle}
	for i, d := range vProbeDepths {
		y := at.Y + 0.5 + float64(i)*vProbeDY
		for _, z := range levels(d, c.VStep) {
			op.paths = append(op.paths, cpath{pts: []p3{{at.X, y, -z}, {at.X + vProbeLen, y, -z}}})
		}
	}
	op.notes = append(op.notes, fmt.Sprintf("канавки %s мм от X%.1f Y%.1f, снизу вверх глубина %s мм",
		num(vProbeLen), at.X, at.Y, joinNums(vProbeDepths)))
	return op
}

func joinNums(v []float64) string {
	var s []string
	for _, x := range v {
		s = append(s, num(x))
	}
	return strings.Join(s, ", ")
}

// maskTest: squares 3 x 3 mm at depths 0.01..0.06 left to right from at, to
// find the depth that bares the copper and leaves it whole.
func maskTest(at pt, c camCfg) *camOp {
	op := &camOp{file: "2_mask_probe", title: "Проба глубины маски", plunge: c.Plunge, feed: c.MaskFeed, spindle: c.Spindle}
	for i := 0; i < 6; i++ {
		d := 0.01 * float64(i+1)
		rt := c.maskWidth(d) / 2
		x0, y0 := at.X+float64(i)*4.5, at.Y
		for o := rt; o < 1.5; o += 0.9 * rt {
			a, z := 3-o, -d
			op.paths = append(op.paths, cpath{pts: []p3{{x0 + o, y0 + o, z}, {x0 + a, y0 + o, z}, {x0 + a, y0 + a, z}, {x0 + o, y0 + a, z}, {x0 + o, y0 + o, z}}})
		}
	}
	op.notes = append(op.notes, fmt.Sprintf("6 квадратов 3х3 мм от X%.1f Y%.1f, слева направо глубина 0.01 ... 0.06", at.X, at.Y))
	return op
}

// ==========================
// ===  Drilling          ===
// ==========================

// drillSize: the bit for a hole, to 0.1 mm.
func drillSize(d float64) float64 { return math.Round(d*10) / 10 }

func drilling(b *board, c camCfg) []*camOp {
	var ops []*camOp
	if len(b.holes) == 0 {
		return nil
	}
	if c.DrillMode == drillSpot || c.DrillMode == drillBoth {
		op := &camOp{file: "3_drill_spot", title: "Наколка отверстий (V-фреза)", plunge: c.Plunge, feed: c.Plunge, spindle: c.Spindle}
		for _, h := range b.holes {
			op.paths = append(op.paths, cpath{pts: []p3{{h.P.X, h.P.Y, -spotZ}}})
		}
		op.notes = append(op.notes, fmt.Sprintf("наколки на %.1f мм, сверлить потом", spotZ))
		ops = append(ops, op)
	}
	if c.DrillMode == drillSpot {
		return ops
	}
	sizes := map[float64][]hole{}
	for _, h := range b.holes {
		sizes[drillSize(h.D)] = append(sizes[drillSize(h.D)], h)
	}
	var ds []float64
	for d := range sizes {
		ds = append(ds, d)
	}
	sort.Float64s(ds)
	depth := c.Thickness + 0.3
	for _, d := range ds {
		op := &camOp{file: fmt.Sprintf("3_drill_%.1f", d), title: fmt.Sprintf("Сверло %.1f мм", d),
			plunge: c.DrillFeed, feed: c.DrillFeed, spindle: c.Spindle}
		n, slots := 0, 0
		{
			for _, h := range sizes[d] {
				a, e := h.P, h.P2
				if !h.Slot {
					op.paths = append(op.paths, cpath{pts: []p3{{a.X, a.Y, -depth}}, peck: pecks})
					n++
					continue
				}
				// a slot: along it and back, a peck deeper each way
				var ps []p3
				z := 0.0
				for k := 0; z < depth-1e-9; k++ {
					z = math.Min(depth, z+pecks/2)
					if k%2 == 0 {
						ps = append(ps, p3{a.X, a.Y, -z}, p3{e.X, e.Y, -z})
					} else {
						ps = append(ps, p3{e.X, e.Y, -z}, p3{a.X, a.Y, -z})
					}
				}
				op.paths = append(op.paths, cpath{pts: ps})
				slots++
			}
		}
		var exact []string
		seen := map[string]bool{}
		for _, h := range sizes[d] {
			if s := fmt.Sprintf("%.2f", h.D); !seen[s] {
				seen[s] = true
				exact = append(exact, s)
			}
		}
		sort.Strings(exact)
		note := fmt.Sprintf("%d отв. на плату насквозь на %.1f мм (по файлу: %s)", n, depth, strings.Join(exact, ", "))
		if slots > 0 {
			note += fmt.Sprintf(", пазов %d", slots)
		}
		op.notes = append(op.notes, note)
		ops = append(ops, op)
	}
	return ops
}

// ==========================
// ===  V-groove          ===
// ==========================

func levels(depth, step float64) []float64 {
	n := int(math.Max(1, math.Ceil(depth/step-1e-9)))
	out := make([]float64, n)
	for i := range out {
		out[i] = depth * float64(i+1) / float64(n)
	}
	return out
}

func vcut(b *board, c camCfg) *camOp {
	op := &camOp{file: "4_vcut", title: "V-рез по контуру", plunge: c.Plunge / 2, feed: c.VFeed, spindle: c.Spindle}
	lv := levels(c.VDepth, c.VStep)
	for _, l := range b.outline {
		for _, z := range lv {
			op.paths = append(op.paths, rawPaths([][]pt{l}, -z)...)
		}
	}
	op.notes = append(op.notes, fmt.Sprintf("канавка %.2f мм за %d проходов, плату потом сломать по ней", c.VDepth, len(lv)))
	return op
}

// ==========================
// ===  Order and G-code  ===
// ==========================

func closedPath(p cpath) bool {
	n := len(p.pts)
	return n > 2 && p.pts[0].X == p.pts[n-1].X && p.pts[0].Y == p.pts[n-1].Y && p.peck == 0
}

// order: the nearest path next; loops start at their nearest point, open
// paths may run backwards. Paths at several depths of one line keep their order.
func order(paths []cpath, start pt) []cpath {
	type box struct{ x0, x1, y0, y1 float64 }
	boxes := make([]box, len(paths))
	for i, p := range paths {
		bx := box{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
		for _, q := range p.pts {
			bx.x0, bx.x1 = math.Min(bx.x0, q.X), math.Max(bx.x1, q.X)
			bx.y0, bx.y1 = math.Min(bx.y0, q.Y), math.Max(bx.y1, q.Y)
		}
		boxes[i] = bx
	}
	left := make([]bool, len(paths))
	for i := range left {
		left[i] = true
	}
	out := make([]cpath, 0, len(paths))
	cur := start
	for range paths {
		bi, bk, bd := -1, 0, math.Inf(1)
		for i, p := range paths {
			if !left[i] {
				continue
			}
			bx := boxes[i]
			dx := math.Max(0, math.Max(bx.x0-cur.X, cur.X-bx.x1))
			dy := math.Max(0, math.Max(bx.y0-cur.Y, cur.Y-bx.y1))
			if dx*dx+dy*dy >= bd {
				continue
			}
			cands := []int{0, len(p.pts) - 1}
			if closedPath(p) {
				cands = nil
				for k := 0; k < len(p.pts)-1; k++ {
					cands = append(cands, k)
				}
			} else if p.peck > 0 || len(p.pts) == 1 {
				cands = []int{0}
			}
			for _, k := range cands {
				q := p.pts[k]
				if d := (q.X-cur.X)*(q.X-cur.X) + (q.Y-cur.Y)*(q.Y-cur.Y); d < bd {
					bi, bk, bd = i, k, d
				}
			}
		}
		p := paths[bi]
		left[bi] = false
		pts := p.pts
		switch {
		case closedPath(p) && bk > 0:
			n := len(pts)
			np := make([]p3, 0, n)
			np = append(np, pts[bk:n-1]...)
			np = append(np, pts[:bk+1]...)
			pts = np
		case !closedPath(p) && bk > 0:
			np := make([]p3, len(pts))
			for i := range pts {
				np[i] = pts[len(pts)-1-i]
			}
			pts = np
		}
		out = append(out, cpath{pts: pts, peck: p.peck, feed: p.feed, plunge: p.plunge, spindle: p.spindle})
		last := pts[len(pts)-1]
		cur = pt{last.X, last.Y}
	}
	return out
}

func n3(v float64) string {
	if math.Abs(v) < 5e-4 {
		v = 0
	}
	return fmt.Sprintf("%.3f", v)
}

// gcode in the shape of the .tap files this machine ran: no comments, no
// dwell; the spindle gets its time on a slow move in the air.
func (op *camOp) gcode() []string {
	out := []string{"T1M6", "G0Z" + n3(safeZ), fmt.Sprintf("G0X0.000Y0.000S%.0fM3", op.spindle), "G1Z2.000F30.0"}
	var at *pt
	speed := op.spindle
	for _, p := range op.paths {
		s := p.pts[0]
		if at == nil || at.X != s.X || at.Y != s.Y {
			out = append(out, "G0Z"+n3(travelZ), "G0X"+n3(s.X)+"Y"+n3(s.Y))
		}
		if p.spindle > 0 && p.spindle != speed { // a new speed: its time on a slow move in the air
			speed = p.spindle
			out = append(out, fmt.Sprintf("S%.0fM3", speed), "G1Z2.000F30.0", "G0Z"+n3(travelZ))
		}
		if p.peck > 0 {
			d := 0.0
			for d < -s.Z-1e-9 {
				d = math.Min(-s.Z, d+p.peck)
				out = append(out, fmt.Sprintf("G1Z%sF%.1f", n3(-d), p.plungeOr(op.plunge)))
				if d < -s.Z-1e-9 {
					out = append(out, "G0Z0.500")
				} else {
					out = append(out, "G0Z"+n3(travelZ))
				}
			}
			at = &pt{s.X, s.Y}
			continue
		}
		out = append(out, fmt.Sprintf("G1Z%sF%.1f", n3(s.Z), p.plungeOr(op.plunge)))
		lz := s.Z
		for i, q := range p.pts[1:] {
			l := "X" + n3(q.X) + "Y" + n3(q.Y)
			if math.Abs(q.Z-lz) > 1e-6 {
				l += "Z" + n3(q.Z)
				lz = q.Z
			}
			if i == 0 {
				l = "G1" + l + fmt.Sprintf("F%.1f", p.feedOr(op.feed))
			}
			out = append(out, l)
		}
		e := p.pts[len(p.pts)-1]
		at = &pt{e.X, e.Y}
	}
	return append(out, "G0Z"+n3(safeZ), "G0X0.000Y0.000", "M5", "M30")
}

// minutes: cutting length by the feed, plunges and lifts; rapids not counted.
func (op *camOp) minutes() (float64, float64) {
	cut, t := 0.0, 0.0
	for _, p := range op.paths {
		l := 0.0
		for i := 1; i < len(p.pts); i++ {
			a, b := p.pts[i-1], p.pts[i]
			l += math.Hypot(b.X-a.X, b.Y-a.Y)
		}
		cut += l
		t += l / p.feedOr(op.feed)
		if p.peck > 0 {
			n := math.Ceil(-p.pts[0].Z / p.peck)
			t += (n*(-p.pts[0].Z)/2 + travelZ) / p.plungeOr(op.plunge)
		} else {
			t += (travelZ - p.pts[0].Z) / p.plungeOr(op.plunge)
		}
	}
	return cut, t + 0.1
}

// ==========================
// ===  All of it         ===
// ==========================

// camBoard: the operations of one board in its own coordinates, the side
// of the settings milled; say gets a line before each long part.
func camBoard(b *board, c camCfg, say func(string)) []*camOp {
	var ops []*camOp
	var field []uint16
	side, mask := lyTop, lyTopMask
	if c.Side == sideBottom || b.copper[lyTop] == nil {
		side, mask = lyBottom, lyBotMask // a one sided board drawn on the bottom is milled the same way
	}
	vbit := fmt.Sprintf("V-фреза %s/%s°", num(c.Tip), num(c.Angle))
	if b.copper[side] != nil {
		say(b.name + ": дорожки")
		var op *camOp
		op, field = isolation(b, side, c, field)
		op.tool = vbit
		ops = append(ops, op)
	}
	if b.mask[mask] != nil {
		say(b.name + ": окна маски")
		var op *camOp
		op, field = maskOpen(b, mask, c, field)
		op.tool = vbit
		if c.MaskFlat > 0 {
			op.tool = "плоская фреза " + num(c.MaskFlat)
		}
		ops = append(ops, op)
	}
	for _, op := range drilling(b, c) {
		op.tool = "сверло " + strings.TrimPrefix(op.file, "3_drill_")
		if op.file == "3_drill_spot" {
			op.tool = vbit
		}
		ops = append(ops, op)
	}
	op := vcut(b, c)
	op.tool = "V-резец для V-реза"
	return append(ops, op)
}

// fileBase: a board name a file may carry: latin letters, digits, - and _.
func fileBase(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		s = "board"
	}
	return s
}

// camWrite puts the programs next to the exe; the old programs of this board
// go first, or a drill size gone from the board would stay as a file.
func camWrite(dir, name string, ops []*camOp) ([]string, error) {
	base := fileBase(name)
	old, _ := filepath.Glob(filepath.Join(dir, base+"_[1-4]_*.tap"))
	for _, f := range old {
		os.Remove(f)
	}
	var files []string
	for _, op := range ops {
		f := filepath.Join(dir, base+"_"+op.file+".tap")
		if err := os.WriteFile(f, []byte(strings.Join(op.gcode(), "\r\n")+"\r\n"), 0644); err != nil {
			return files, err
		}
		files = append(files, f)
	}
	return files, nil
}
