package main

import "math"

// rect is a box on the table, mm.
type rect struct{ X0, X1, Y0, Y1 float64 }

func (r rect) union(o rect) rect {
	return rect{math.Min(r.X0, o.X0), math.Max(r.X1, o.X1), math.Min(r.Y0, o.Y0), math.Max(r.Y1, o.Y1)}
}
