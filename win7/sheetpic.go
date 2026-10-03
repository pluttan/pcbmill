package main

// The sheet seen from above as it lies on the table: the copper of every
// board in shades by how much of a character cell it covers, the outlines in
// dots, the corner where Z0 is touched, and on top what the item chosen on
// the screen is about: the points of the height map, the path of a program,
// the narrow places. The only colour of the program is here.

import (
	"math"
	"strings"
)

type sheetOverlay struct {
	points [][2]float64 // the height map
	paths  []cpath      // a program
	marks  []pt         // narrow places, tiny openings
	circle *[3]float64  // x, y, r: a redo
}

// sheetBase caches the copper of the sheet for one size of the picture.
var sheetBase struct {
	key   string
	cells [][]string
}

func sheetKey(j *Job, mirror bool, cols, rows int) string {
	var b strings.Builder
	b.WriteString(num(j.W) + "x" + num(j.H) + num(float64(cols)) + "/" + num(float64(rows)))
	if mirror {
		b.WriteString("m")
	}
	for _, p := range j.Places {
		b.WriteString("|" + num(float64(p.Src)) + "," + num(float64(p.Rot)) + "," + num(p.X) + "," + num(p.Y))
	}
	return b.String()
}

// sheetPic: the sheet w x h cells at most.
func sheetPic(j *Job, mirror bool, ov sheetOverlay, w, h int) []string {
	if j.W <= 0 || j.H <= 0 || w < 10 || h < 4 {
		return nil
	}
	sx := math.Min(float64(w-1)/j.W, 2*float64(h-1)/j.H) // cells per mm across; a cell is twice as high
	cols, rows := int(math.Ceil(j.W*sx)), int(math.Ceil(j.H*sx/2))
	if cols < 2 || rows < 2 {
		return nil
	}
	key := sheetKey(j, mirror, cols, rows)
	if sheetBase.key != key {
		sheetBase.key, sheetBase.cells = key, sheetCells(j, mirror, sx, cols, rows)
	}
	cells := make([][]string, rows)
	for r := range cells {
		cells[r] = append([]string(nil), sheetBase.cells[r]...)
	}
	cell := func(q pt) (int, int, bool) { // a sheet point to its cell, row 0 on top
		i := int(q.X * sx)
		r := rows - 1 - int(q.Y*sx/2)
		return i, r, i >= 0 && r >= 0 && i < cols && r < rows
	}
	set := func(q pt, s string) {
		if i, r, ok := cell(q); ok {
			cells[r][i] = s
		}
	}
	line := func(a, b pt, s string) {
		n := int(math.Hypot(b.X-a.X, b.Y-a.Y)*sx*2) + 1
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(n)
			set(pt{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t}, s)
		}
	}
	for _, p := range ov.paths {
		if len(p.pts) == 1 {
			set(pt{p.pts[0].X, p.pts[0].Y}, tint(cLightCyan, "o"))
		}
		for k := 1; k < len(p.pts); k++ {
			line(pt{p.pts[k-1].X, p.pts[k-1].Y}, pt{p.pts[k].X, p.pts[k].Y}, tint(cLightCyan, "·"))
		}
	}
	if c := ov.circle; c != nil {
		n := int(2*math.Pi*c[2]*sx) + 8
		for k := 0; k < n; k++ {
			a := 2 * math.Pi * float64(k) / float64(n)
			set(pt{c[0] + c[2]*math.Cos(a), c[1] + c[2]*math.Sin(a)}, tint(cLightMagenta, "∙"))
		}
		set(pt{c[0], c[1]}, tint(cLightMagenta, "+"))
	}
	for _, q := range ov.points {
		set(pt{q[0], q[1]}, tint(cWhite, "■"))
	}
	for _, m := range ov.marks {
		set(m, tint(cLightRed, "■"))
	}
	set(j.zero(), tint(cLightMagenta, "♦"))
	out := make([]string, rows)
	for r := range cells {
		out[r] = strings.Join(cells[r], "")
	}
	return out
}

// sheetCells: the copper in shades (two samples across and four up a
// cell), the outlines over it, the edge of the sheet round it.
func sheetCells(j *Job, mirror bool, sx float64, cols, rows int) [][]string {
	px := 1 / (2 * sx)
	r := newRaster(rect{0, float64(cols) * 2 * px, 0, float64(rows) * 4 * px}, px)
	if j.loadBoards() == nil {
		for _, p := range j.Places {
			b, pl := j.boards[p.Src], j.placeOf(p, mirror)
			side := lyTop
			if mirror || b.copper[lyTop] == nil {
				side = lyBottom
			}
			g := b.copper[side]
			if g == nil {
				continue
			}
			for _, s := range g.shapes {
				t := shape{dark: s.dark, R: s.R}
				if s.R > 0 {
					t.C = pl.at(s.C)
				}
				for _, ps := range s.polys {
					q := make([]pt, len(ps))
					for i, v := range ps {
						q[i] = pl.at(v)
					}
					t.polys = append(t.polys, q)
				}
				r.draw(t)
			}
		}
	}
	cells := make([][]string, rows)
	for cr := range cells {
		cells[cr] = make([]string, cols)
		jr := rows - 1 - cr // raster rows go up
		for i := range cells[cr] {
			n := 0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if r.at(2*i+dx, 4*jr+dy) {
						n++
					}
				}
			}
			s := " " // the bare copper of the sheet
			switch {
			case n >= 7:
				s = tint(cBrown, "█")
			case n >= 5:
				s = tint(cBrown, "▓")
			case n >= 3:
				s = tint(cBrown, "▒")
			case n >= 1:
				s = tint(cBrown, "░")
			}
			cells[cr][i] = s
		}
	}
	if j.loadBoards() == nil {
		for _, p := range j.Places {
			b, pl := j.boards[p.Src], j.placeOf(p, mirror)
			for _, l := range b.outline {
				for k := 1; k < len(l); k++ {
					a, e := pl.at(l[k-1]), pl.at(l[k])
					n := int(math.Hypot(e.X-a.X, e.Y-a.Y)*sx*2) + 1
					for s := 0; s <= n; s++ {
						t := float64(s) / float64(n)
						q := pt{a.X + (e.X-a.X)*t, a.Y + (e.Y-a.Y)*t}
						i, cr := int(q.X*sx), rows-1-int(q.Y*sx/2)
						if i >= 0 && cr >= 0 && i < cols && cr < rows {
							if c := strip(cells[cr][i]); c == " " {
								cells[cr][i] = tint(cWhite, "·")
							}
						}
					}
				}
			}
		}
	}
	// the edge of the sheet, thin and dark
	for i := 0; i < cols; i++ {
		cells[0][i], cells[rows-1][i] = tint(cDark, "─"), tint(cDark, "─")
	}
	for cr := 0; cr < rows; cr++ {
		cells[cr][0], cells[cr][cols-1] = tint(cDark, "│"), tint(cDark, "│")
	}
	cells[0][0], cells[0][cols-1] = tint(cDark, "┌"), tint(cDark, "┐")
	cells[rows-1][0], cells[rows-1][cols-1] = tint(cDark, "└"), tint(cDark, "┘")
	if j.Probe != nil {
		for x := j.Probe.X; x < j.Probe.X+probeW; x += 1 / sx {
			i, cr := int(x*sx), rows-1-int((j.Probe.Y+probeH/2)*sx/2)
			if i >= 0 && cr >= 0 && i < cols && cr < rows {
				cells[cr][i] = tint(cCyan, "▒")
			}
		}
	}
	return cells
}
