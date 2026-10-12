package main

import (
	"math"
	"path/filepath"
	"testing"
)

// isoBridges: the trace of the tool (w wide) over the copper of a board;
// every pair of copper pieces of the drawing that is still joined by copper
// the tool left is a bridge. Their places, one per joined group.
func isoBridges(g *gerber, op *camOp, w float64) []pt {
	cu, _ := g.bounds()
	r := newRaster(rect{cu.X0 - 1, cu.X1 + 1, cu.Y0 - 1, cu.Y1 + 1}, 0.01)
	for _, s := range g.shapes {
		r.draw(s)
	}
	cut := &raster{x0: r.x0, y0: r.y0, px: r.px, w: r.w, h: r.h, bits: make([]byte, len(r.bits))}
	for _, p := range op.paths {
		for i, q := range p.pts {
			cut.draw(shape{dark: true, C: pt{q.X, q.Y}, R: w / 2})
			if i == 0 {
				continue
			}
			a := p.pts[i-1]
			l := math.Hypot(q.X-a.X, q.Y-a.Y)
			if l < 1e-9 {
				continue
			}
			nx, ny := -(q.Y-a.Y)/l*w/2, (q.X-a.X)/l*w/2
			cut.draw(shape{dark: true, polys: [][]pt{{{a.X + nx, a.Y + ny}, {q.X + nx, q.Y + ny}, {q.X - nx, q.Y - ny}, {a.X - nx, a.Y - ny}}}})
		}
	}
	rem := &raster{x0: r.x0, y0: r.y0, px: r.px, w: r.w, h: r.h, bits: make([]byte, len(r.bits))}
	for k := range rem.bits {
		if cut.bits[k] == 0 {
			rem.bits[k] = 1 // the blank is copper all over: what the tool did not touch stays
		}
	}
	// a piece of the gap the tool left that touches uncut copper of two
	// islands joins them: its middle is the place of the bridge
	gap := &raster{x0: r.x0, y0: r.y0, px: r.px, w: r.w, h: r.h, bits: make([]byte, len(r.bits))}
	for k := range gap.bits {
		if rem.bits[k] != 0 && r.bits[k] == 0 {
			gap.bits[k] = 1
		}
	}
	islands, gaps := r.pieces(), gap.pieces()
	type touch struct {
		isl    map[int]bool
		sx, sy float64
		n      int
	}
	by := map[int]*touch{}
	var order []int
	for j := 1; j < r.h-1; j++ {
		for i := 1; i < r.w-1; i++ {
			k := j*r.w + i
			if gap.bits[k] == 0 {
				continue
			}
			g := gaps.label(i, j)
			t := by[g]
			if t == nil {
				t = &touch{isl: map[int]bool{}}
				by[g] = t
				order = append(order, g)
			}
			t.sx, t.sy, t.n = t.sx+float64(i), t.sy+float64(j), t.n+1
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if n := (j+d[1])*r.w + i + d[0]; r.bits[n] != 0 && rem.bits[n] != 0 {
					t.isl[islands.label(i+d[0], j+d[1])] = true
				}
			}
		}
	}
	var out []pt
	for _, g := range order {
		if t := by[g]; len(t.isl) > 1 {
			out = append(out, r.world(t.sx/float64(t.n), t.sy/float64(t.n)))
		}
	}
	return out
}

// TestIsoNoBridges: on the test board the isolation leaves no copper joining
// two pieces of the drawing, except the narrow places it reports.
func TestIsoNoBridges(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	b, err := loadBoard(zip)
	if err != nil {
		t.Skip(err)
	}
	c := defaultCam()
	op, _ := isolation(b, lyTop, c, nil)
	br := isoBridges(b.copper[lyTop], op, c.cutWidth(c.IsoDepth))
	if len(br) > len(op.marks) {
		t.Errorf("перемычек %d, узких мест в отчёте %d: %v", len(br), len(op.marks), br)
	}
	t.Logf("перемычек %d, узких мест %d", len(br), len(op.marks))
}
