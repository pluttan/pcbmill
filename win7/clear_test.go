package main

import (
	"math"
	"testing"
)

// TestClearing: the end mill of the clearing keeps off the copper: its edge
// stays at the middle of the first isolation pass or farther, and it does
// not leave the board; with the clearing on a later pass drops no crumb.
func TestClearing(t *testing.T) {
	b, err := loadBoard(boardSamples(t)["2032Charger"])
	if err != nil {
		t.Fatal(err)
	}
	c := defaultCam()
	c.ClearReach = 2
	op := clearing(b, lyTop, c)
	if len(op.paths) == 0 {
		t.Fatal("выборка без путей")
	}
	g := b.copper[lyTop]
	cu, _ := g.bounds()
	r := newRaster(rect{cu.X0 - 4, cu.X1 + 4, cu.Y0 - 4, cu.Y1 + 4}, 0.005)
	for _, s := range g.shapes {
		r.draw(s)
	}
	field := r.distTo(1, nil)
	near := func(q pt) float64 {
		i, j := int(math.Round(r.col(q.X))), int(math.Round(r.row(q.Y)))
		return (float64(field[j*r.w+i])/distScale - 0.5) * r.px
	}
	edge := c.cutWidth(c.IsoDepth) / 2 // the edge of the mill: the middle of the first pass
	worst := 9.0
	for _, p := range op.paths {
		for _, q := range p.pts {
			worst = math.Min(worst, near(pt{q.X, q.Y})-c.HoleTool/2)
			if q.X < b.box.X0 || q.X > b.box.X1 || q.Y < b.box.Y0 || q.Y > b.box.Y1 {
				t.Fatalf("центр фрезы (%.2f; %.2f) за платой", q.X, q.Y)
			}
		}
	}
	if worst < edge-0.02 {
		t.Errorf("край фрезы в %.3f мм от меди, нужно не ближе %.3f", worst, edge)
	}
	iso, _ := isolation(b, lyTop, c, nil)
	for _, n := range iso.notes {
		if len(n) > 0 && n[0] == 0xd0 && containsCrumb(n) {
			t.Errorf("с выборкой обрывки пропущены: %s", n)
		}
	}
	t.Logf("путей %d, край фрезы не ближе %.3f мм; %v", len(op.paths), worst, op.notes)
}

func containsCrumb(s string) bool {
	for i := 0; i+len("пропущено") <= len(s); i++ {
		if s[i:i+len("пропущено")] == "пропущено" {
			return true
		}
	}
	return false
}
