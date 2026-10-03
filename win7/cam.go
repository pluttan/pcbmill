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
	MaskDepth       float64
	MaskFeed        float64
	MaskFlat        float64 // a flat end mill of this diameter for the mask, 0: the V-bit
	MaskTest        bool
	DrillMode       int // drillThrough, drillSpot, drillBoth
	Thickness       float64
	DrillFeed       float64
	VDepth, VStep   float64
	VFeed           float64
	Side            int // sideTop, sideBottom, sideBoth
	CopiesX         int
	CopiesY         int
	Gap             float64
}

const (
	drillThrough = iota
	drillSpot
	drillBoth
)

const (
	sideTop = iota
	sideBottom
	sideBoth
)

// defaultCam: the numbers of machine.toml, which the Vesi programs ran with.
func defaultCam() camCfg {
	return camCfg{Tip: 0.1, Angle: 45, Spindle: 8000, IsoDepth: 0.1, Passes: 2, IsoFeed: 300, Plunge: 200,
		MaskDepth: 0.03, MaskFeed: 200, MaskTest: true, DrillMode: drillThrough, Thickness: 1.5, DrillFeed: 60,
		VDepth: 0.8, VStep: 0.2, VFeed: 200, Side: sideTop, CopiesX: 1, CopiesY: 1, Gap: 2}
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
	pts  []p3
	peck float64 // a drilled hole: down to pts[0].Z in steps of peck
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
	marks   []pt // places to show on the picture: shorts, tiny openings
}

// ==========================
// ===  Placement         ===
// ==========================

// place moves a board point onto the table: the corner of the outline goes to
// X0 Y0, the bottom side is turned over left to right.
type place struct {
	box    rect
	mirror bool
	dx, dy float64 // the copy
}

func (p place) at(q pt) pt {
	x := q.X - p.box.X0
	if p.mirror {
		x = p.box.X1 - q.X
	}
	return pt{x + p.dx, q.Y - p.box.Y0 + p.dy}
}

// copies: the offsets of the boards on the blank.
func (c camCfg) copies(b *board) []pt {
	w, h := b.box.X1-b.box.X0, b.box.Y1-b.box.Y0
	var out []pt
	for iy := 0; iy < c.CopiesY; iy++ {
		for ix := 0; ix < c.CopiesX; ix++ {
			out = append(out, pt{float64(ix) * (w + c.Gap), float64(iy) * (h + c.Gap)})
		}
	}
	return out
}

func (c camCfg) placed(b *board, mirror bool, loops [][]pt, z float64) []cpath {
	var out []cpath
	for _, d := range c.copies(b) {
		p := place{b.box, mirror, d.X, d.Y}
		for _, l := range loops {
			ps := make([]p3, len(l))
			for i, q := range l {
				w := p.at(q)
				ps[i] = p3{w.X, w.Y, z}
			}
			out = append(out, cpath{pts: ps})
		}
	}
	return out
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
	mirror := side == lyBottom
	name, title := "1_iso_top", "Изоляция, верх"
	if mirror {
		name, title = "1_iso_bot", "Изоляция, низ (зеркально)"
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
	op.paths = c.placed(b, mirror, loops, -c.IsoDepth)
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
	p := place{b.box, mirror, 0, 0}
	for _, s := range shorts {
		op.marks = append(op.marks, p.at(s))
	}
	if len(shorts) > 0 {
		var where []string
		for i, s := range op.marks {
			if i == 6 {
				where = append(where, "...")
				break
			}
			where = append(where, fmt.Sprintf("(%.1f; %.1f)", s.X, s.Y))
		}
		op.warn = append(op.warn, fmt.Sprintf("%d мест, где медь ближе ширины реза %.2f мм: там останется перемычка: %s",
			len(shorts), w, strings.Join(where, " ")))
	}
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
	mirror := side == lyBotMask
	name, title := "3_mask_top", "Вскрытие маски, верх"
	if mirror {
		name, title = "3_mask_bot", "Вскрытие маски, низ (зеркально)"
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
	op.paths = c.placed(b, mirror, append(loops, dots...), -c.MaskDepth)
	op.notes = append(op.notes, fmt.Sprintf("окон %d, след фрезы %.3f мм на глубине %.3f", len(tops), wm, c.MaskDepth))
	if len(dots) > 0 {
		op.warn = append(op.warn, fmt.Sprintf("%d окон уже следа фрезы %.2f мм: там одно погружение в середину, окно выйдет чуть шире", len(dots), wm))
		p := place{b.box, mirror, 0, 0}
		for _, d := range dots {
			op.marks = append(op.marks, p.at(d[0]))
		}
	}
	return op, field
}

// maskTest: squares 3 x 3 mm above the board at depths 0.01..0.06, to find the
// depth that bares the copper and leaves it whole.
func maskTest(b *board, c camCfg) *camOp {
	op := &camOp{file: "3_mask_probe", title: "Проба глубины маски (над платой)", plunge: c.Plunge, feed: c.MaskFeed, spindle: c.Spindle}
	h := (b.box.Y1-b.box.Y0)*float64(c.CopiesY) + c.Gap*float64(c.CopiesY-1)
	for i := 0; i < 6; i++ {
		d := 0.01 * float64(i+1)
		rt := c.maskWidth(d) / 2
		x0, y0 := float64(i)*4.5, h+3
		for o := rt; o < 1.5; o += 0.9 * rt {
			a, z := 3-o, -d
			op.paths = append(op.paths, cpath{pts: []p3{{x0 + o, y0 + o, z}, {x0 + a, y0 + o, z}, {x0 + a, y0 + a, z}, {x0 + o, y0 + a, z}, {x0 + o, y0 + o, z}}})
		}
	}
	op.notes = append(op.notes, fmt.Sprintf("6 квадратов 3х3 мм на Y %.0f мм, слева направо глубина 0.01 ... 0.06", h+3))
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
	put := func(h hole, d place) (pt, pt) { return d.at(h.P), d.at(h.P2) }
	if c.DrillMode == drillSpot || c.DrillMode == drillBoth {
		op := &camOp{file: "2_drill_spot", title: "Наколка отверстий (V-фреза)", plunge: c.Plunge, feed: c.Plunge, spindle: c.Spindle}
		for _, d := range c.copies(b) {
			p := place{b.box, false, d.X, d.Y}
			for _, h := range b.holes {
				a, _ := put(h, p)
				op.paths = append(op.paths, cpath{pts: []p3{{a.X, a.Y, -spotZ}}})
			}
		}
		op.notes = append(op.notes, fmt.Sprintf("%d наколок на %.1f мм, сверлить потом", len(op.paths), spotZ))
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
		op := &camOp{file: fmt.Sprintf("2_drill_%.1f", d), title: fmt.Sprintf("Сверло %.1f мм", d),
			plunge: c.DrillFeed, feed: c.DrillFeed, spindle: c.Spindle}
		n, slots := 0, 0
		for _, cp := range c.copies(b) {
			p := place{b.box, false, cp.X, cp.Y}
			for _, h := range sizes[d] {
				a, e := put(h, p)
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
		note := fmt.Sprintf("%d отверстий насквозь на %.1f мм (по файлу: %s)", n, depth, strings.Join(exact, ", "))
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
	for _, d := range c.copies(b) {
		p := place{b.box, false, d.X, d.Y}
		for _, l := range b.outline {
			for _, z := range lv {
				ps := make([]p3, len(l))
				for i, q := range l {
					w := p.at(q)
					ps[i] = p3{w.X, w.Y, -z}
				}
				op.paths = append(op.paths, cpath{pts: ps})
			}
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
		out = append(out, cpath{pts: pts, peck: p.peck})
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
	for _, p := range op.paths {
		s := p.pts[0]
		if at == nil || at.X != s.X || at.Y != s.Y {
			out = append(out, "G0Z"+n3(travelZ), "G0X"+n3(s.X)+"Y"+n3(s.Y))
		}
		if p.peck > 0 {
			d := 0.0
			for d < -s.Z-1e-9 {
				d = math.Min(-s.Z, d+p.peck)
				out = append(out, fmt.Sprintf("G1Z%sF%.1f", n3(-d), op.plunge))
				if d < -s.Z-1e-9 {
					out = append(out, "G0Z0.500")
				} else {
					out = append(out, "G0Z"+n3(travelZ))
				}
			}
			at = &pt{s.X, s.Y}
			continue
		}
		out = append(out, fmt.Sprintf("G1Z%sF%.1f", n3(s.Z), op.plunge))
		lz := s.Z
		for i, q := range p.pts[1:] {
			l := "X" + n3(q.X) + "Y" + n3(q.Y)
			if math.Abs(q.Z-lz) > 1e-6 {
				l += "Z" + n3(q.Z)
				lz = q.Z
			}
			if i == 0 {
				l = "G1" + l + fmt.Sprintf("F%.1f", op.feed)
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
		for i := 1; i < len(p.pts); i++ {
			a, b := p.pts[i-1], p.pts[i]
			cut += math.Hypot(b.X-a.X, b.Y-a.Y)
		}
		if p.peck > 0 {
			n := math.Ceil(-p.pts[0].Z / p.peck)
			t += (n*(-p.pts[0].Z)/2 + travelZ) / op.plunge
		} else {
			t += (travelZ - p.pts[0].Z) / op.plunge
		}
	}
	return cut, t + cut/op.feed + 0.1
}

// ==========================
// ===  All of it         ===
// ==========================

// camRun makes every program of a board; say gets a line as each is done.
func camRun(b *board, c camCfg, say func(string)) []*camOp {
	var ops []*camOp
	var field []uint16
	sides := []string{lyTop, lyBottom}
	switch c.Side {
	case sideTop:
		sides = sides[:1]
	case sideBottom:
		sides = sides[1:]
	}
	if c.Side == sideTop && b.copper[lyTop] == nil && b.copper[lyBottom] != nil {
		sides = []string{lyBottom} // a one sided board drawn on the bottom
	}
	for _, s := range sides {
		if b.copper[s] == nil {
			continue
		}
		say("изоляция: считаю " + map[string]string{lyTop: "верх", lyBottom: "низ"}[s])
		var op *camOp
		op, field = isolation(b, s, c, field)
		ops = append(ops, op)
	}
	ops = append(ops, drilling(b, c)...)
	for _, s := range sides {
		m := map[string]string{lyTop: lyTopMask, lyBottom: lyBotMask}[s]
		if b.mask[m] == nil {
			continue
		}
		say("маска: считаю окна")
		var op *camOp
		op, field = maskOpen(b, m, c, field)
		ops = append(ops, op)
		if c.MaskTest && (s == sides[len(sides)-1] || b.mask[lyBotMask] == nil) {
			ops = append(ops, maskTest(b, c)) // once, after the last mask
			c.MaskTest = false
		}
	}
	ops = append(ops, vcut(b, c))
	for _, op := range ops {
		op.paths = order(op.paths, pt{})
	}
	return ops
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
