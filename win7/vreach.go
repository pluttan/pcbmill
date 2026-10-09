package main

// The V-groove to the edge of the sheet. A groove only round the board does
// not break clean: the crack runs off at the corners. Every side of a board
// goes on as a straight groove to the edge of the sheet, or up to the next
// board or probe when one stands in the way.

import "math"

// vReachGap: the groove stops this far from another board or probe.
const vReachGap = 1.0

// vcutReach: the grooves past the ends of every side of every board, at
// every depth of the V-cut.
func (j *Job) vcutReach(mirror bool, c camCfg) []cpath {
	var rs []rect
	for _, p := range j.Places {
		w, h := j.placeOf(p, mirror).size()
		rs = append(rs, rect{p.X, p.X + w, p.Y, p.Y + h})
	}
	var stops []rect // what a groove must not cross
	for _, r := range rs {
		stops = append(stops, r)
	}
	for _, b := range j.probeBoxes() {
		stops = append(stops, rect{b.at.X, b.at.X + b.w, b.at.Y, b.at.Y + b.h})
	}
	var segs [][2]pt
	for i, r := range rs {
		// along X at y = r.Y0 and r.Y1, along Y at x = r.X0 and r.X1
		for _, y := range []float64{r.Y0, r.Y1} {
			if a := reach(stops, i, y, r.X0, -1, 0, j.W, false); a < r.X0 {
				segs = append(segs, [2]pt{{r.X0, y}, {a, y}})
			}
			if b := reach(stops, i, y, r.X1, 1, 0, j.W, false); b > r.X1 {
				segs = append(segs, [2]pt{{r.X1, y}, {b, y}})
			}
		}
		for _, x := range []float64{r.X0, r.X1} {
			if a := reach(stops, i, x, r.Y0, -1, 0, j.H, true); a < r.Y0 {
				segs = append(segs, [2]pt{{x, r.Y0}, {x, a}})
			}
			if b := reach(stops, i, x, r.Y1, 1, 0, j.H, true); b > r.Y1 {
				segs = append(segs, [2]pt{{x, r.Y1}, {x, b}})
			}
		}
	}
	var out []cpath
	for _, s := range segs {
		if math.Hypot(s[1].X-s[0].X, s[1].Y-s[0].Y) < 0.5 {
			continue
		}
		for _, z := range levels(c.VDepth, c.VStep) {
			out = append(out, cpath{pts: []p3{{s[0].X, s[0].Y, -z}, {s[1].X, s[1].Y, -z}}})
		}
	}
	return out
}

// reach: how far a groove on the line (at = y for a groove along X, x along
// Y) goes from `from` in the direction dir before a stop or the sheet edge.
func reach(stops []rect, self int, at, from, dir, lo, hi float64, alongY bool) float64 {
	end := hi
	if dir < 0 {
		end = lo
	}
	for k, s := range stops {
		if k == self {
			continue
		}
		a0, a1, b0, b1 := s.Y0, s.Y1, s.X0, s.X1 // across the line, along it
		if alongY {
			a0, a1, b0, b1 = s.X0, s.X1, s.Y0, s.Y1
		}
		if at < a0-vReachGap || at > a1+vReachGap {
			continue // the line passes by
		}
		if dir > 0 && b0 >= from-1e-9 {
			end = math.Min(end, b0-vReachGap)
		}
		if dir < 0 && b1 <= from+1e-9 {
			end = math.Max(end, b1+vReachGap)
		}
	}
	if dir > 0 {
		return math.Max(end, from)
	}
	return math.Min(end, from)
}
