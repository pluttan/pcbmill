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
	islands := r.pieces()
	left := rem.pieces()
	owner := map[int]int{} // a piece of what is left -> the first island in it
	var out []pt
	seen := map[int]bool{}
	for j := 0; j < r.h; j++ {
		for i := 0; i < r.w; i++ {
			k := j*r.w + i
			if r.bits[k] == 0 || rem.bits[k] == 0 {
				continue
			}
			isl, piece := islands.label(i, j), left.label(i, j)
			if o, ok := owner[piece]; !ok {
				owner[piece] = isl
			} else if o != isl && !seen[piece] {
				seen[piece] = true
				out = append(out, r.world(float64(i), float64(j)))
			}
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
