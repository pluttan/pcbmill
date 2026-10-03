package main

// The picture of a layer on a fine grid of pixels: shapes are filled into it,
// a map of distances is built from it, and the lines of equal distance are
// traced back into paths. Row 0 is the lowest Y, so the grid is turned the
// same way as the board.

import (
	"math"
	"sort"
)

type raster struct {
	x0, y0, px float64 // the corner of pixel (0, 0) and the size of a pixel, mm
	w, h       int
	bits       []byte // 1: copper, an opening, the board
}

// maxPixels keeps the memory of the 32-bit exe in bounds: a byte and two
// bytes of distance per pixel.
const maxPixels = 40e6

// newRaster covers r with pixels of px mm, coarser when the board is too big.
func newRaster(r rect, px float64) *raster {
	if a := (r.X1 - r.X0) * (r.Y1 - r.Y0) / (px * px); a > maxPixels {
		px *= math.Sqrt(a / maxPixels)
	}
	w := int(math.Ceil((r.X1-r.X0)/px)) + 1
	h := int(math.Ceil((r.Y1-r.Y0)/px)) + 1
	return &raster{x0: r.X0, y0: r.Y0, px: px, w: w, h: h, bits: make([]byte, w*h)}
}

// world: the centre of pixel (i, j) in mm; fractional i, j are fine.
func (r *raster) world(i, j float64) pt { return pt{r.x0 + (i+0.5)*r.px, r.y0 + (j+0.5)*r.px} }

func (r *raster) col(x float64) float64 { return (x-r.x0)/r.px - 0.5 }
func (r *raster) row(y float64) float64 { return (y-r.y0)/r.px - 0.5 }

func (r *raster) at(i, j int) bool {
	return i >= 0 && j >= 0 && i < r.w && j < r.h && r.bits[j*r.w+i] != 0
}

// span sets pixels of row j whose centres lie in [xa, xb).
func (r *raster) span(j int, xa, xb float64, v byte) {
	a := int(math.Ceil(r.col(xa)))
	b := int(math.Ceil(r.col(xb))) - 1
	if a < 0 {
		a = 0
	}
	if b >= r.w {
		b = r.w - 1
	}
	row := r.bits[j*r.w : (j+1)*r.w]
	for i := a; i <= b; i++ {
		row[i] = v
	}
}

func (r *raster) draw(s shape) {
	v := byte(0)
	if s.dark {
		v = 1
	}
	if s.R > 0 {
		r.circle(s.C, s.R, v)
	}
	if len(s.polys) > 0 {
		r.fill(s.polys, v)
	}
}

func (r *raster) circle(c pt, rad float64, v byte) {
	j0 := int(math.Ceil(r.row(c.Y - rad)))
	j1 := int(math.Floor(r.row(c.Y + rad)))
	if j0 < 0 {
		j0 = 0
	}
	if j1 >= r.h {
		j1 = r.h - 1
	}
	for j := j0; j <= j1; j++ {
		dy := r.world(0, float64(j)).Y - c.Y
		if q := rad*rad - dy*dy; q >= 0 {
			hw := math.Sqrt(q)
			r.span(j, c.X-hw, c.X+hw, v)
		}
	}
}

// fill: polygons by the even-odd rule, sampled at the pixel centres.
func (r *raster) fill(polys [][]pt, v byte) {
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, p := range polys {
		for _, q := range p {
			ymin, ymax = math.Min(ymin, q.Y), math.Max(ymax, q.Y)
		}
	}
	j0 := int(math.Ceil(r.row(ymin)))
	j1 := int(math.Ceil(r.row(ymax))) - 1
	if j0 < 0 {
		j0 = 0
	}
	if j1 >= r.h {
		j1 = r.h - 1
	}
	if j1 < j0 {
		return
	}
	xs := make([][]float64, j1-j0+1)
	for _, p := range polys {
		n := len(p)
		for k := 0; k < n; k++ {
			a, b := p[k], p[(k+1)%n]
			if a.Y == b.Y {
				continue
			}
			if a.Y > b.Y {
				a, b = b, a
			}
			// rows whose centre is in [a.Y, b.Y)
			ja := int(math.Ceil(r.row(a.Y)))
			jb := int(math.Ceil(r.row(b.Y))) - 1
			if ja < j0 {
				ja = j0
			}
			if jb > j1 {
				jb = j1
			}
			for j := ja; j <= jb; j++ {
				y := r.world(0, float64(j)).Y
				xs[j-j0] = append(xs[j-j0], a.X+(y-a.Y)*(b.X-a.X)/(b.Y-a.Y))
			}
		}
	}
	for k, row := range xs {
		sort.Float64s(row)
		for i := 0; i+1 < len(row); i += 2 {
			r.span(j0+k, row[i], row[i+1], v)
		}
	}
}

// ==========================
// ===  Distances         ===
// ==========================

// distScale: distances are kept in eighths of a pixel, in uint16.
const distScale = 8

const distFar = math.MaxUint16

// sqrtTab: the distance for a squared one under sqrtN, the root being slow.
const sqrtN = 1 << 16

var sqrtTab = func() []uint16 {
	t := make([]uint16, sqrtN)
	for i := range t {
		t[i] = uint16(math.Sqrt(float64(i))*distScale + 0.5)
	}
	return t
}()

// distTo: for every pixel the distance (eighths of a pixel, centre to centre)
// to the nearest pixel whose bit is target; the exact Euclidean transform of
// Felzenszwalb and Huttenlocher, a column pass then a row pass.
func (r *raster) distTo(target byte, out []uint16) []uint16 {
	w, h := r.w, r.h
	if len(out) < w*h {
		out = make([]uint16, w*h)
	}
	// columns: the vertical distance to the nearest target pixel, far = none;
	// walked a row at a time, so the memory is read in order
	for j := 0; j < h; j++ {
		row, bits := out[j*w:(j+1)*w], r.bits[j*w:(j+1)*w]
		if j == 0 {
			for i := range row {
				if bits[i] == target {
					row[i] = 0
				} else {
					row[i] = distFar
				}
			}
			continue
		}
		prev := out[(j-1)*w : j*w]
		for i := range row {
			switch {
			case bits[i] == target:
				row[i] = 0
			case prev[i] < distFar-1:
				row[i] = prev[i] + 1
			default:
				row[i] = distFar
			}
		}
	}
	for j := h - 2; j >= 0; j-- {
		row, below := out[j*w:(j+1)*w], out[(j+1)*w:(j+2)*w]
		for i := range row {
			if below[i] < distFar-1 && below[i]+1 < row[i] {
				row[i] = below[i] + 1
			}
		}
	}
	f := make([]float64, w)
	v := make([]int, w)
	z := make([]float64, w+1)
	const inf = 1e20
	for j := 0; j < h; j++ {
		row := out[j*w : (j+1)*w]
		any := false
		for i, g := range row {
			if g == distFar {
				f[i] = inf
			} else {
				f[i] = float64(g) * float64(g)
				any = true
			}
		}
		if !any {
			continue // stays far
		}
		// the lower envelope of the parabolas f[q] + (x-q)^2
		k := 0
		v[0], z[0], z[1] = 0, -inf, inf
		for q := 1; q < w; q++ {
			if f[q] >= inf {
				continue
			}
			if f[v[0]] >= inf { // the first real parabola
				v[0] = q
				continue
			}
			for {
				p := v[k]
				s := ((f[q] + float64(q*q)) - (f[p] + float64(p*p))) / float64(2*q-2*p)
				if s <= z[k] && k > 0 {
					k--
					continue
				}
				if s <= z[k] { // k == 0: q replaces the first
					v[0] = q
					z[1] = inf
					break
				}
				k++
				v[k], z[k], z[k+1] = q, s, inf
				break
			}
		}
		k = 0
		for q := 0; q < w; q++ {
			for z[k+1] < float64(q) {
				k++
			}
			p := v[k]
			dd := float64((q-p)*(q-p)) + f[p] // a whole number: squares of pixels
			if dd < sqrtN {
				row[q] = sqrtTab[int(dd)]
			} else if s := math.Sqrt(dd) * distScale; s >= distFar {
				row[q] = distFar
			} else {
				row[q] = uint16(s + 0.5)
			}
		}
	}
	return out
}

// ==========================
// ===  Lines of a level  ===
// ==========================

// levelLines traces where field crosses t (marching squares on the pixel
// centres). The region of values under t (above: over t) is kept on the left
// of every loop, so loops round it run counter-clockwise. Pixels outside the
// grid count as outside the region.
func (r *raster) levelLines(field []uint16, t float64, above bool) [][]pt {
	return r.levelsLines(field, []float64{t}, above)
}

// levelsLines: the loops of several levels in one walk over the grid; ts go up.
func (r *raster) levelsLines(field []uint16, ts []float64, above bool) [][]pt {
	w, h := r.w, r.h
	outside := float64(distFar + 1)
	if above {
		outside = -1
	}
	val := func(i, j int) float64 {
		if i < 0 || j < 0 || i >= w || j >= h {
			return outside
		}
		return float64(field[j*w+i])
	}
	// a crossing on the edge between pixel (i, j) and its right (horizontal)
	// or upper (vertical) neighbour; the grid is padded by one on each side
	pw := w + 2
	id := func(i, j int, up bool) int64 {
		k := int64((j+1)*pw+(i+1)) * 2
		if up {
			k++
		}
		return k
	}
	nexts := make([]map[int64]int64, len(ts))
	for k := range nexts {
		nexts[k] = map[int64]int64{}
	}
	var v [4]float64
	// cell (i, j) has corners (i,j) (i+1,j) (i+1,j+1) (i,j+1), counter-clockwise
	for j := -1; j < h; j++ {
		inner := j >= 0 && j+1 < h
		var lo, hi []uint16
		if inner {
			lo, hi = field[j*w:(j+1)*w], field[(j+1)*w:(j+2)*w]
		}
		for i := -1; i < w; i++ {
			if inner && i >= 0 && i+1 < w {
				a, b, c, d := lo[i], lo[i+1], hi[i+1], hi[i]
				if a == b && b == c && c == d {
					continue // the common case: nothing crosses a flat cell
				}
				v = [4]float64{float64(a), float64(b), float64(c), float64(d)}
			} else {
				v = [4]float64{val(i, j), val(i+1, j), val(i+1, j+1), val(i, j+1)}
			}
			mn, mx := math.Min(math.Min(v[0], v[1]), math.Min(v[2], v[3])), math.Max(math.Max(v[0], v[1]), math.Max(v[2], v[3]))
			// the levels between the lowest and the highest corner
			k0 := sort.SearchFloat64s(ts, mn)
			for k := k0; k < len(ts) && ts[k] <= mx; k++ {
				t := ts[k]
				var in [4]bool
				for c := range in {
					if above {
						in[c] = v[c] > t
					} else {
						in[c] = v[c] < t
					}
				}
				if in[0] == in[1] && in[1] == in[2] && in[2] == in[3] {
					continue
				}
				edges := [4]int64{id(i, j, false), id(i+1, j, true), id(i, j+1, false), id(i, j, true)}
				// crossings in counter-clockwise order: edge e runs from corner e to e+1
				var from, to [2]int
				nf, nt := 0, 0
				for e := 0; e < 4; e++ {
					a, b := in[e], in[(e+1)%4]
					if a && !b {
						from[nf] = e
						nf++
					} else if !a && b {
						to[nt] = e
						nt++
					}
				}
				next := nexts[k]
				if nf == 1 {
					next[edges[from[0]]] = edges[to[0]]
					continue
				}
				// a saddle: the centre decides whether the inside corners join
				m := (v[0] + v[1] + v[2] + v[3]) / 4
				cin := m < t
				if above {
					cin = m > t
				}
				for _, e := range from[:nf] {
					o := (e + 3) % 4 // the out->in crossing before e
					if cin {
						o = (e + 1) % 4 // after e
					}
					next[edges[e]] = edges[o]
				}
			}
		}
	}
	var loops [][]pt
	for k, next := range nexts {
		t := ts[k]
		point := func(e int64) pt {
			up := e%2 == 1
			cell := e / 2
			i, j := int(cell%int64(pw))-1, int(cell/int64(pw))-1
			a := val(i, j)
			var b float64
			if up {
				b = val(i, j+1)
			} else {
				b = val(i+1, j)
			}
			s := 0.5
			if a != b {
				s = (t - a) / (b - a)
			}
			s = math.Max(0, math.Min(1, s))
			if up {
				return r.world(float64(i), float64(j)+s)
			}
			return r.world(float64(i)+s, float64(j))
		}
		keys := make([]int64, 0, len(next))
		for e := range next {
			keys = append(keys, e)
		}
		sort.Slice(keys, func(a, b int) bool { return keys[a] < keys[b] }) // the same paths every run
		for _, e0 := range keys {
			if _, ok := next[e0]; !ok {
				continue
			}
			var loop []pt
			e := e0
			for {
				n, ok := next[e]
				if !ok {
					break
				}
				loop = append(loop, point(e))
				delete(next, e)
				e = n
				if e == e0 {
					break
				}
			}
			if len(loop) >= 3 {
				loop = append(loop, loop[0])
				loops = append(loops, loop)
			}
		}
	}
	return loops
}

// simplify drops points closer than tol to the line of their neighbours
// (Douglas-Peucker); a closed loop keeps being closed.
func simplify(ps []pt, tol float64) []pt {
	if len(ps) < 3 {
		return ps
	}
	keep := make([]bool, len(ps))
	keep[0], keep[len(ps)-1] = true, true
	type span struct{ a, b int }
	closed := ps[0] == ps[len(ps)-1]
	stack := []span{{0, len(ps) - 1}}
	if closed { // split the loop at its farthest point from the start
		far, fd := 0, -1.0
		for i, p := range ps {
			if d := math.Hypot(p.X-ps[0].X, p.Y-ps[0].Y); d > fd {
				far, fd = i, d
			}
		}
		keep[far] = true
		stack = []span{{0, far}, {far, len(ps) - 1}}
	}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		a, b := ps[s.a], ps[s.b]
		dx, dy := b.X-a.X, b.Y-a.Y
		l := math.Hypot(dx, dy)
		far, fd := -1, tol
		for i := s.a + 1; i < s.b; i++ {
			p := ps[i]
			var d float64
			if l < 1e-12 {
				d = math.Hypot(p.X-a.X, p.Y-a.Y)
			} else {
				d = math.Abs(dx*(p.Y-a.Y)-dy*(p.X-a.X)) / l
			}
			if d > fd {
				far, fd = i, d
			}
		}
		if far >= 0 {
			keep[far] = true
			stack = append(stack, span{s.a, far}, span{far, s.b})
		}
	}
	out := make([]pt, 0, len(ps)/4+2)
	for i, p := range ps {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// ==========================
// ===  Pieces            ===
// ==========================

// runs label the connected pieces of the set pixels (4-neighbours) without a
// label per pixel: a label per run of a row, joined with union-find.
type runs struct {
	r      *raster
	start  []int   // first run of each row, len h+1
	a, b   []int32 // pixels [a, b] of each run
	parent []int32
}

func (r *raster) pieces() *runs {
	u := &runs{r: r, start: make([]int, r.h+1)}
	for j := 0; j < r.h; j++ {
		u.start[j] = len(u.a)
		row := r.bits[j*r.w : (j+1)*r.w]
		for i := 0; i < r.w; {
			if row[i] == 0 {
				i++
				continue
			}
			k := i
			for k < r.w && row[k] != 0 {
				k++
			}
			u.a, u.b = append(u.a, int32(i)), append(u.b, int32(k-1))
			i = k
		}
	}
	u.start[r.h] = len(u.a)
	u.parent = make([]int32, len(u.a))
	for i := range u.parent {
		u.parent[i] = int32(i)
	}
	for j := 1; j < r.h; j++ { // join runs that overlap the row below
		p, q := u.start[j-1], u.start[j]
		for q < u.start[j+1] && p < u.start[j] {
			if u.b[p] < u.a[q] {
				p++
				continue
			}
			if u.b[q] < u.a[p] {
				q++
				continue
			}
			u.union(p, q)
			if u.b[p] < u.b[q] {
				p++
			} else {
				q++
			}
		}
	}
	return u
}

func (u *runs) find(x int) int {
	for int(u.parent[x]) != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = int(u.parent[x])
	}
	return x
}

func (u *runs) union(a, b int) {
	a, b = u.find(a), u.find(b)
	if a != b {
		u.parent[b] = int32(a)
	}
}

// label: the piece of pixel (i, j), -1 when the pixel is not set.
func (u *runs) label(i, j int) int {
	if j < 0 || j >= u.r.h {
		return -1
	}
	lo, hi := u.start[j], u.start[j+1]
	k := sort.Search(hi-lo, func(n int) bool { return int(u.b[lo+n]) >= i }) + lo
	if k < hi && int(u.a[k]) <= i {
		return u.find(k)
	}
	return -1
}
