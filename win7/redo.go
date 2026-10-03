package main

// A stage again where it did not cut through: the paths of a program inside
// a circle, deeper by some tenths, into a new file. The program is read back
// from its .tap, so a redo of a redo works the same way.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// readProg reads a program of this program's own shape back into paths:
// a path starts with a feed move under Z0 and ends where the tool goes back
// over it; a hole drilled in pecks comes back as one point with its peck.
func readProg(lines []string) (*camOp, error) {
	op := &camOp{plunge: 100, feed: 100, spindle: 10000}
	var x, y, z float64
	z = safeZ
	mode := 0
	var cur *cpath
	var feedSeen, plungeSeen bool
	flush := func() {
		if cur == nil {
			return
		}
		n := len(op.paths)
		if len(cur.pts) == 1 && n > 0 { // pecks of one hole: keep the deepest as one hole
			last := &op.paths[n-1]
			if len(last.pts) == 1 && last.pts[0].X == cur.pts[0].X && last.pts[0].Y == cur.pts[0].Y {
				if cur.pts[0].Z < last.pts[0].Z {
					last.pts[0].Z = cur.pts[0].Z
				}
				last.peck = pecks
				cur = nil
				return
			}
		}
		op.paths = append(op.paths, *cur)
		cur = nil
	}
	for _, l := range lines {
		ws := words(l)
		nx, ny, nz := x, y, z
		var f float64
		for _, w := range ws {
			switch w.L {
			case 'G':
				switch w.V {
				case 0, 1:
					mode = int(w.V)
				case 2, 3:
					return nil, fmt.Errorf("дуги G2/G3: это не программа pcbscan")
				}
			case 'X':
				nx = w.V
			case 'Y':
				ny = w.V
			case 'Z':
				nz = w.V
			case 'F':
				f = w.V
			case 'S':
				op.spindle = w.V
			}
		}
		if f > 0 {
			if nx == x && ny == y && nz != z && !plungeSeen && nz < 0 {
				op.plunge, plungeSeen = f, true
			} else if (nx != x || ny != y) && !feedSeen && mode == 1 {
				op.feed, feedSeen = f, true
			}
		}
		switch {
		case nz >= 0:
			flush()
		case mode == 1 && z >= 0: // down into the copper
			flush()
			cur = &cpath{pts: []p3{{nx, ny, nz}}}
		case mode == 1 && cur != nil:
			if nx != x || ny != y || nz != z {
				cur.pts = append(cur.pts, p3{nx, ny, nz})
			}
		case mode == 0 && nz < 0:
			return nil, fmt.Errorf("G0 под медью в строке %q: не трогаю такую программу", l)
		}
		x, y, z = nx, ny, nz
	}
	flush()
	if !feedSeen {
		op.feed = op.plunge
	}
	return op, nil
}

// clipCircle: the pieces of a path inside the circle c, r.
func clipCircle(p cpath, c pt, r float64) []cpath {
	in := func(q p3) bool { return math.Hypot(q.X-c.X, q.Y-c.Y) <= r }
	if len(p.pts) == 1 {
		if in(p.pts[0]) {
			return []cpath{p}
		}
		return nil
	}
	var out []cpath
	var cur []p3
	end := func() {
		if len(cur) >= 2 {
			out = append(out, cpath{pts: cur})
		}
		cur = nil
	}
	for i := 1; i < len(p.pts); i++ {
		a, b := p.pts[i-1], p.pts[i]
		// where the segment a + t(b-a) crosses the circle
		dx, dy := b.X-a.X, b.Y-a.Y
		fx, fy := a.X-c.X, a.Y-c.Y
		qa, qb, qc := dx*dx+dy*dy, 2*(fx*dx+fy*dy), fx*fx+fy*fy-r*r
		ts := []float64{0}
		if qa > 1e-12 {
			if disc := qb*qb - 4*qa*qc; disc > 0 {
				s := math.Sqrt(disc)
				for _, t := range []float64{(-qb - s) / (2 * qa), (-qb + s) / (2 * qa)} {
					if t > 0 && t < 1 {
						ts = append(ts, t)
					}
				}
			}
		}
		ts = append(ts, 1)
		at := func(t float64) p3 { return p3{a.X + dx*t, a.Y + dy*t, a.Z + (b.Z-a.Z)*t} }
		for k := 1; k < len(ts); k++ {
			m := at((ts[k-1] + ts[k]) / 2)
			if in(m) {
				s, e := at(ts[k-1]), at(ts[k])
				if len(cur) == 0 {
					cur = append(cur, s)
				}
				cur = append(cur, e)
			} else {
				end()
			}
		}
	}
	end()
	return out
}

// redoProg: the program src again inside the circle, deeper by extra; the
// new file is next to it with _redoN in its name. The count of paths kept.
func redoProg(src string, c pt, r, extra float64) (string, int, error) {
	if r <= 0 || extra < 0 || extra > 1 {
		return "", 0, fmt.Errorf("радиус больше 0, углубление от 0 до 1 мм")
	}
	lines, err := readLines(src)
	if err != nil {
		return "", 0, err
	}
	op, err := readProg(lines)
	if err != nil {
		return "", 0, err
	}
	var kept []cpath
	for _, p := range op.paths {
		for _, q := range clipCircle(p, c, r) {
			for i := range q.pts {
				q.pts[i].Z -= extra
			}
			kept = append(kept, q)
		}
	}
	if len(kept) == 0 {
		return "", 0, fmt.Errorf("в круге X%s Y%s R%s нет ни одного реза этой программы", num(c.X), num(c.Y), num(r))
	}
	op.paths = order(kept, c)
	base := strings.TrimSuffix(src, filepath.Ext(src))
	if i := strings.Index(base, "_redo"); i >= 0 {
		base = base[:i]
	}
	var dst string
	for n := 1; ; n++ {
		dst = fmt.Sprintf("%s_redo%d.tap", base, n)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
	}
	if err := os.WriteFile(dst, []byte(strings.Join(op.gcode(), "\r\n")+"\r\n"), 0644); err != nil {
		return "", 0, err
	}
	return dst, len(kept), nil
}
