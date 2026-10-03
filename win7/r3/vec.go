// Package r3 is a small software 3D renderer with a z-buffer: meshes,
// antialiased lines and markers drawn into an *image.RGBA without a GPU.
// World units are millimetres and Z is up (the copper top is Z=0).
package r3

import "math"

// ==========================
// ===  Vectors           ===
// ==========================

// Vec is a point or a direction in world space.
type Vec struct{ X, Y, Z float64 }

func (a Vec) Add(b Vec) Vec             { return Vec{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec) Sub(b Vec) Vec             { return Vec{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec) Scale(s float64) Vec       { return Vec{a.X * s, a.Y * s, a.Z * s} }
func (a Vec) Dot(b Vec) float64         { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec) Len() float64              { return math.Sqrt(a.Dot(a)) }
func (a Vec) Lerp(b Vec, t float64) Vec { return a.Add(b.Sub(a).Scale(t)) }

func (a Vec) Cross(b Vec) Vec {
	return Vec{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

// Norm returns the unit vector; a zero or NaN vector stays zero so callers
// never push NaN into the rasterizer.
func (a Vec) Norm() Vec {
	l := a.Len()
	if !(l > 1e-300) || math.IsInf(l, 0) {
		return Vec{}
	}
	return a.Scale(1 / l)
}

func (a Vec) bad() bool {
	s := a.X + a.Y + a.Z
	return math.IsNaN(s) || math.IsInf(s, 0)
}

// ==========================
// ===  Scalar helpers    ===
// ==========================

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func to8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
