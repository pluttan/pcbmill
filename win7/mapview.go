package main

// The machine table seen from above in characters: the program cut, the map
// grid before the scan and the copper heights after it, with axes in mm.

import (
	"fmt"
	"math"
	"strings"
)

// rect is a box on the table, mm.
type rect struct{ X0, X1, Y0, Y1 float64 }

func (r rect) union(o rect) rect {
	return rect{math.Min(r.X0, o.X0), math.Max(r.X1, o.X1), math.Min(r.Y0, o.Y0), math.Max(r.Y1, o.Y1)}
}

// mapRows: no taller than this, the grid commands and the table go under it.
const mapRows = 20

// shades from low copper to high, each with its colour.
var (
	shades     = []string{"░", "▒", "▓", "█"}
	shadeColor = []int{cLightBlue, cLightCyan, cLightGreen, cYellow}
)

// tableCanvas maps the table to character cells. A console cell is about twice
// as high as it is wide, so a mm across takes twice the cells of a mm up. One
// spare cell on every side keeps the picture off the axes.
type tableCanvas struct {
	area       rect
	sx, sy     float64 // cells per mm
	cols, rows int
	cell       [][]string
}

func newCanvas(area rect, maxCols, maxRows int) *tableCanvas {
	spanX, spanY := area.X1-area.X0, area.Y1-area.Y0
	sx := math.Inf(1)
	if spanX > 1e-9 {
		sx = float64(maxCols-3) / spanX
	}
	if spanY > 1e-9 {
		sx = math.Min(sx, 2*float64(maxRows-3)/spanY)
	}
	if math.IsInf(sx, 1) || sx <= 0 {
		sx = 1
	}
	c := &tableCanvas{area: area, sx: sx, sy: sx / 2}
	c.cols = int(math.Round(spanX*c.sx)) + 3
	c.rows = int(math.Round(spanY*c.sy)) + 3
	c.cell = make([][]string, c.rows)
	for r := range c.cell {
		c.cell[r] = strings.Split(spaces(c.cols), "")
	}
	return c
}

func (c *tableCanvas) col(x float64) int { return 1 + int(math.Round((x-c.area.X0)*c.sx)) }
func (c *tableCanvas) row(y float64) int { return 1 + int(math.Round((c.area.Y1-y)*c.sy)) }
func (c *tableCanvas) x(col int) float64 { return c.area.X0 + float64(col-1)/c.sx }
func (c *tableCanvas) y(row int) float64 { return c.area.Y1 - float64(row-1)/c.sy }

func (c *tableCanvas) set(col, row int, s string) {
	if row >= 0 && row < c.rows && col >= 0 && col < c.cols {
		c.cell[row][col] = s
	}
}

// frame draws a thin box along the edges of r: points on the edge of the cut
// sit on the line, points of a wider grid outside it.
func (c *tableCanvas) frame(r rect) {
	c0, c1, r0, r1 := c.col(r.X0), c.col(r.X1), c.row(r.Y1), c.row(r.Y0)
	for k := c0 + 1; k < c1; k++ {
		c.set(k, r0, paint(cFrame, "─"))
		c.set(k, r1, paint(cFrame, "─"))
	}
	for k := r0 + 1; k < r1; k++ {
		c.set(c0, k, paint(cFrame, "│"))
		c.set(c1, k, paint(cFrame, "│"))
	}
	c.set(c0, r0, paint(cFrame, "┌"))
	c.set(c1, r0, paint(cFrame, "┐"))
	c.set(c0, r1, paint(cFrame, "└"))
	c.set(c1, r1, paint(cFrame, "┘"))
}

// mm is a coordinate for an axis label: no trailing zeros.
func mm(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	if s == "-0" {
		return "0"
	}
	return s
}

const gutter = 7 // Y labels left of the axis

// render adds the axes: Y up the left with a tick at every grid row, X along
// the bottom with a tick at every grid column, labels where they fit.
func (c *tableCanvas) render(xs, ys []float64) []string {
	yTick := map[int]float64{}
	for i := len(ys) - 1; i >= 0; i-- { // the first value of a row wins
		yTick[c.row(ys[i])] = ys[i]
	}
	xTick := map[int]bool{}
	for _, x := range xs {
		xTick[c.col(x)] = true
	}
	var out []string
	out = append(out, fmt.Sprintf("%*s ", gutter-1, "Y, мм")+paint(cFrame, "▲"))
	for r := 0; r < c.rows; r++ {
		l, axis := spaces(gutter), "│"
		if v, ok := yTick[r]; ok {
			l, axis = fmt.Sprintf("%*s ", gutter-1, mm(v)), "┤"
		}
		out = append(out, l+paint(cFrame, axis)+strings.Join(c.cell[r], ""))
	}
	var ax strings.Builder
	for k := 0; k < c.cols; k++ {
		if xTick[k] {
			ax.WriteString("┬")
		} else {
			ax.WriteString("─")
		}
	}
	out = append(out, spaces(gutter)+paint(cFrame, "└"+ax.String()+"─►")+" X, мм")
	// X labels: the ends first, then whatever fits between them with a space around
	width := gutter + 1 + c.cols + 2
	lab := []rune(spaces(width + 6))
	used := make([]bool, len(lab))
	put := func(x float64) {
		t := []rune(mm(x))
		at := gutter + 1 + c.col(x) - (len(t)-1)/2
		if at < 0 {
			at = 0
		}
		if at+len(t) > len(lab) {
			at = len(lab) - len(t)
		}
		for k := at - 1; k <= at+len(t); k++ {
			if k >= 0 && k < len(used) && used[k] {
				return
			}
		}
		for k, r := range t {
			lab[at+k], used[at+k] = r, true
		}
	}
	if len(xs) > 0 {
		put(xs[0])
		put(xs[len(xs)-1])
		for _, x := range xs[1:] {
			put(x)
		}
	}
	out = append(out, strings.TrimRight(string(lab), " "))
	w := 0
	for _, l := range out {
		if vlen(l) > w {
			w = vlen(l)
		}
	}
	for i := range out {
		out[i] = padRight(out[i], w)
	}
	return out
}

// legend joins items into lines no wider than w.
func legend(items []string, w int) []string {
	var out []string
	cur := ""
	for _, it := range items {
		switch {
		case cur == "":
			cur = it
		case vlen(cur)+3+vlen(it) <= w:
			cur += "   " + it
		default:
			out = append(out, cur)
			cur = it
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// fitRows tries ever lower pictures until one is no taller than h; nil when
// even the smallest is too tall.
func fitRows(h int, f func(rows int) []string) []string {
	top := mapRows
	if h-3 < top {
		top = h - 3
	}
	for rows := top; rows >= 4; rows-- {
		if out := f(rows); len(out) <= h {
			return out
		}
	}
	return nil
}

// gridView: where the touch points fall on the program cut, before the scan.
func gridView(g Grid, cut *rect, w int) []string { return gridViewRows(g, cut, w, mapRows) }

func gridViewRows(g Grid, cut *rect, w, rows int) []string {
	area := rect{g.X0, g.X1, g.Y0, g.Y1}
	if cut != nil {
		area = area.union(*cut)
	}
	c := newCanvas(area, w-gutter-1-2-6, rows)
	if cut != nil {
		c.frame(*cut)
	}
	xs, ys := spread(g.X0, g.X1, g.NX), spread(g.Y0, g.Y1, g.NY)
	for _, y := range ys {
		for _, x := range xs {
			c.set(c.col(x), c.row(y), paint(cTitle, "■"))
		}
	}
	first := g.Points()[0]
	c.set(c.col(first[0]), c.row(first[1]), paint(cZero, "♦"))
	out := c.render(xs, ys)
	items := []string{paint(cTitle, "■") + " точка касания", paint(cZero, "♦") + " первая точка, ноль Z"}
	if cut != nil {
		items = append(items, paint(cFrame, "┌┐")+" рез программы")
	}
	return append(out, legend(items, w)...)
}

// span is the lowest and highest copper of the map.
func (h *HeightMap) span() (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range h.z {
		for _, v := range r {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	return lo, hi
}

func shadeOf(z, lo, hi float64) int {
	if hi-lo < 1e-9 {
		return 0
	}
	k := int((z - lo) / (hi - lo) * float64(len(shades)))
	if k < 0 {
		k = 0
	}
	if k >= len(shades) {
		k = len(shades) - 1
	}
	return k
}

// heightView: the copper height over the map area, interpolated like ApplyMap
// does, darker = higher; ticks on the axes are the touch points.
func heightView(h *HeightMap, cut *rect, w int) []string {
	return heightViewRows(h, cut, w, mapRows)
}

func heightViewRows(h *HeightMap, cut *rect, w, rows int) []string {
	gb := rect{h.xs[0], h.xs[len(h.xs)-1], h.ys[0], h.ys[len(h.ys)-1]}
	area := gb
	if cut != nil {
		area = area.union(*cut)
	}
	c := newCanvas(area, w-gutter-1-2-6, rows)
	lo, hi := h.span()
	for r := c.row(gb.Y1); r <= c.row(gb.Y0); r++ {
		for k := c.col(gb.X0); k <= c.col(gb.X1); k++ {
			s := shadeOf(h.At(c.x(k), c.y(r)), lo, hi)
			c.set(k, r, paint(shadeColor[s], shades[s]))
		}
	}
	// a cut the size of the map is the edge of the shading already: no frame over it
	if cut != nil && (c.col(cut.X0) == c.col(gb.X0) && c.col(cut.X1) == c.col(gb.X1) &&
		c.row(cut.Y0) == c.row(gb.Y0) && c.row(cut.Y1) == c.row(gb.Y1)) {
		cut = nil
	}
	if cut != nil {
		c.frame(*cut)
	}
	c.set(c.col(h.zx), c.row(h.zy), paint(cZero, "♦"))
	out := c.render(h.xs, h.ys)
	var items []string
	if hi-lo < 1e-9 {
		items = append(items, paint(shadeColor[0], shades[0])+" вся медь на одной высоте")
	} else {
		d := (hi - lo) / float64(len(shades))
		for k := range shades {
			items = append(items, fmt.Sprintf("%s %+.3f..%+.3f", paint(shadeColor[k], shades[k]), lo+d*float64(k), lo+d*float64(k+1)))
		}
		items[len(items)-1] += " мм"
	}
	out = append(out, legend(items, w)...)
	stats := []string{paint(cZero, "♦") + " ноль Z, первая точка",
		fmt.Sprintf("мин %+.3f  макс %+.3f  перепад %s мм", lo, hi, paint(cTitle, fmt.Sprintf("%.3f", hi-lo)))}
	if cut != nil {
		stats = append(stats, paint(cFrame, "┌┐")+" рез")
	}
	return append(out, legend(stats, w)...)
}

// heightTable is the map in numbers, to check the picture against: Y down the
// left, top row first as on the picture; too many columns go in several blocks.
func heightTable(h *HeightMap, w int) []string {
	lo, hi := h.span()
	out := []string{fmt.Sprintf("точек %dx%d, перепад %.3f мм (от %+.3f до %+.3f)", len(h.xs), len(h.ys), hi-lo, lo, hi)}
	per := (w - 8) / 8
	if per < 1 {
		per = 1
	}
	for a := 0; a < len(h.xs); a += per {
		b := a + per
		if b > len(h.xs) {
			b = len(h.xs)
		}
		head := paint(cHint, "   Y\\X ")
		for _, x := range h.xs[a:b] {
			head += paint(cHint, fmt.Sprintf("%8.1f", x))
		}
		out = append(out, head)
		for j := len(h.ys) - 1; j >= 0; j-- {
			l := paint(cHint, fmt.Sprintf("%7.1f", h.ys[j]))
			for i := a; i < b; i++ {
				v, s := h.z[j][i], fmt.Sprintf("%8.3f", h.z[j][i])
				switch {
				case hi-lo > 1e-9 && v == hi:
					s = paint(shadeColor[len(shades)-1], s)
				case hi-lo > 1e-9 && v == lo:
					s = paint(shadeColor[0], s)
				}
				l += s
			}
			out = append(out, l)
		}
	}
	return out
}

// scanView is the grid during the scan: measured points in the shade of their
// height, the point being touched now bright (blink alternates its colour on
// every redraw), the ones still to come as dots.
func scanView(g Grid, cut *rect, done []Point, blink bool, w, rows int) []string {
	area := rect{g.X0, g.X1, g.Y0, g.Y1}
	if cut != nil {
		area = area.union(*cut)
	}
	c := newCanvas(area, w-gutter-1-2-6, rows)
	if cut != nil {
		c.frame(*cut)
	}
	pts := g.Points()
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range done {
		lo, hi = math.Min(lo, p.Z), math.Max(hi, p.Z)
	}
	for i, p := range pts {
		mark := paint(cHint, "·")
		switch {
		case i < len(done):
			s := shadeOf(done[i].Z, lo, hi)
			mark = paint(shadeColor[s], shades[s])
		case i == len(done):
			col := cYellow
			if blink {
				col = cWhite
			}
			mark = paint(col, "■")
		}
		c.set(c.col(p[0]), c.row(p[1]), mark)
	}
	out := c.render(spread(g.X0, g.X1, g.NX), spread(g.Y0, g.Y1, g.NY))
	items := []string{paint(cYellow, "■") + " касание сейчас", paint(cHint, "·") + " впереди"}
	if len(done) > 1 && hi-lo > 1e-9 {
		items = append(items, fmt.Sprintf("%s ниже .. %s выше", paint(shadeColor[0], shades[0]),
			paint(shadeColor[len(shades)-1], shades[len(shades)-1])))
	}
	return append(out, legend(items, w)...)
}
