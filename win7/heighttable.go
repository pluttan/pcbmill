package main

// The height map as numbers: a row per Y, a column per X, millimetres from
// the first point; what the operator compares with the machine.

import (
	"fmt"
	"image"

	"pcbscan/gfx"
	"pcbscan/ui"
)

// drawHeightTable draws the table into r; returns the height used.
func drawHeightTable(c *ui.Ctx, r image.Rectangle, hm *HeightMap) int {
	t := c.T
	head := ui.Style{Kind: ui.Mono, Size: ui.SizeSmall, Color: t.Overlay2}
	cw := c.Measure("+0.000", head) + t.Si(10)
	lh := c.LineH(head) + t.Si(2)
	cols := (r.Dx() - cw) / cw
	if cols < 1 {
		return 0
	}
	if cols > len(hm.xs) {
		cols = len(hm.xs)
	}
	rows := (r.Dy() - 2*lh) / lh
	if rows > len(hm.ys) {
		rows = len(hm.ys)
	}
	if rows < 1 {
		return 0
	}
	lo, hi := hm.z[0][0], hm.z[0][0]
	for _, row := range hm.z {
		for _, v := range row {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	y := r.Min.Y
	c.Label(r.Min.X, y, "Y \\ X", head)
	for i := 0; i < cols; i++ {
		c.Label(r.Min.X+cw*(i+1), y, fmt.Sprintf("%6.1f", hm.xs[i]), head)
	}
	y += lh
	for jj := 0; jj < rows; jj++ {
		j := len(hm.ys) - 1 - jj // the far side on top, as the sheet lies
		c.Label(r.Min.X, y, fmt.Sprintf("%6.1f", hm.ys[j]), head)
		for i := 0; i < cols; i++ {
			v := hm.z[j][i]
			col := heightColor(v, lo, hi)
			x := r.Min.X + cw*(i+1)
			c.C.FillRoundRect(float64(x)-t.S(3), float64(y), float64(cw)-t.S(4), float64(lh)-t.S(2), t.S(3), gfx.Alpha(col, 0.16))
			c.Label(x, y, fmt.Sprintf("%+.3f", v), ui.Style{Kind: ui.Mono, Size: ui.SizeSmall, Color: col})
		}
		y += lh
	}
	if rows < len(hm.ys) || cols < len(hm.xs) {
		c.Label(r.Min.X, y, fmt.Sprintf("показано %d x %d из %d x %d: окно мало", cols, rows, len(hm.xs), len(hm.ys)), head)
		y += lh
	}
	return y - r.Min.Y
}
