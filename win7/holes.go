package main

// Every hole by one end mill: a hole no wider than the mill is drilled by it
// straight down in pecks; a wider one is cut round a helix, the mill going
// down a little each turn, then one more turn at the bottom for a clean wall.
// A slot wider than the mill is cut round its outline, level by level.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const holesFile = "3_holes"

func holeTool(c camCfg) string { return "фреза " + num(c.HoleTool) + " мм" }

// holeFits: a hole this wide is the mill itself, no circle round it.
func holeFits(d float64, c camCfg) bool { return d <= c.HoleTool+0.05 }

func holesByMill(holes []hole, c camCfg) *camOp {
	op := &camOp{file: holesFile, title: "Отверстия фрезой " + num(c.HoleTool) + " мм",
		plunge: c.DrillFeed, feed: c.HoleFeed, spindle: c.Spindle}
	depth := c.Thickness + 0.3
	drilled, round, slots := 0, 0, 0
	sizes := map[string]bool{}
	for _, h := range holes {
		sizes[fmt.Sprintf("%.2f", h.D)] = true
		r := (h.D - c.HoleTool) / 2
		switch {
		case h.Slot && holeFits(h.D, c):
			op.paths = append(op.paths, slotLine(h.P, h.P2, depth))
			slots++
		case h.Slot:
			for _, z := range levels(depth, c.HoleStep) {
				op.paths = append(op.paths, cpath{pts: racetrack(h.P, h.P2, r, -z)})
			}
			slots++
		case holeFits(h.D, c):
			op.paths = append(op.paths, cpath{pts: []p3{{h.P.X, h.P.Y, -depth}}, peck: pecks})
			drilled++
		default:
			op.paths = append(op.paths, cpath{pts: helix(h.P, r, depth, c.HoleStep)})
			round++
		}
	}
	var ds []string
	for s := range sizes {
		ds = append(ds, s)
	}
	sort.Strings(ds)
	op.notes = append(op.notes, fmt.Sprintf("насквозь на %s мм: сверлением %d, по кругу %d, пазов %d (по файлу: %s)",
		num(depth), drilled, round, slots, strings.Join(ds, ", ")))
	return op
}

// circleN: the segments of a circle of radius r, about 0.1 mm each.
func circleN(r float64) int { return int(math.Max(12, math.Ceil(2*math.Pi*r/0.1))) }

// helix: from the top down to depth, step down per turn, then a turn at depth.
func helix(c pt, r, depth, step float64) []p3 {
	n := circleN(r)
	turns := math.Ceil(depth/step - 1e-9)
	var out []p3
	for k := 0; k <= int(turns)*n+n; k++ {
		a := 2 * math.Pi * float64(k) / float64(n)
		z := math.Min(depth, depth*float64(k)/(turns*float64(n)))
		out = append(out, p3{c.X + r*math.Cos(a), c.Y + r*math.Sin(a), -z})
	}
	return out
}

// racetrack: the outline of a slot from a to b, r out from its middle line.
func racetrack(a, b pt, r, z float64) []p3 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		pts := helix(a, r, 0, 1)
		for i := range pts {
			pts[i].Z = z
		}
		return pts
	}
	ux, uy := dx/l, dy/l
	base := math.Atan2(uy, ux)
	n := circleN(r) / 2
	var out []p3
	for _, end := range []struct {
		c pt
		a float64
	}{{b, base - math.Pi/2}, {a, base + math.Pi/2}} { // half round each end
		for k := 0; k <= n; k++ {
			t := end.a + math.Pi*float64(k)/float64(n)
			out = append(out, p3{end.c.X + r*math.Cos(t), end.c.Y + r*math.Sin(t), z})
		}
	}
	return append(out, out[0])
}

// slotLine: a slot as wide as the mill: along it and back, deeper each way.
func slotLine(a, e pt, depth float64) cpath {
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
	return cpath{pts: ps}
}
