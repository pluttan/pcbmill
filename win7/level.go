package main

// G-code bounds, probe grid, height map and Z compensation: the same rules as
// pcbmill/level.py so a map taken here levels programs exactly like there.

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reComment = regexp.MustCompile(`\(.*?\)`)
	reWord    = regexp.MustCompile(`([A-Za-z])\s*(-?\d*\.?\d+)`)
	reNum     = regexp.MustCompile(`-?\d+\.?\d*`)
)

type word struct {
	L byte
	V float64
}

func words(line string) []word {
	line = reComment.ReplaceAllString(line, "")
	if i := strings.IndexByte(line, ';'); i >= 0 {
		line = line[:i]
	}
	var out []word
	for _, m := range reWord.FindAllStringSubmatch(line, -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		out = append(out, word{strings.ToUpper(m[1])[0], v})
	}
	return out
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, strings.TrimRight(sc.Text(), "\r"))
	}
	return out, sc.Err()
}

// CutBounds: box of all moves below Z0.
func CutBounds(lines []string) (x0, x1, y0, y1 float64, err error) {
	var x, y, z float64
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for _, l := range lines {
		for _, w := range words(l) {
			switch w.L {
			case 'X':
				x = w.V
			case 'Y':
				y = w.V
			case 'Z':
				z = w.V
			case 'G':
				if w.V == 2 || w.V == 3 {
					return 0, 0, 0, 0, fmt.Errorf("дуги G2/G3 не поддерживаются, выгрузите программу отрезками")
				}
			}
		}
		if z < 0 {
			x0, x1 = math.Min(x0, x), math.Max(x1, x)
			y0, y1 = math.Min(y0, y), math.Max(y1, y)
		}
	}
	if math.IsInf(x0, 1) {
		return 0, 0, 0, 0, fmt.Errorf("в программе нет ходов ниже Z0")
	}
	return
}

func axis(lo, hi, step float64) []float64 {
	n := int(math.Max(1, math.Ceil((hi-lo)/step)))
	out := make([]float64, n+1)
	for i := range out {
		out[i] = lo + (hi-lo)*float64(i)/float64(n)
	}
	return out
}

// GridPoints walks the box as a snake to keep travel short.
func GridPoints(x0, x1, y0, y1, step float64) [][2]float64 {
	xs, ys := axis(x0, x1, step), axis(y0, y1, step)
	var pts [][2]float64
	for j, y := range ys {
		for i := range xs {
			k := i
			if j%2 == 1 {
				k = len(xs) - 1 - i
			}
			pts = append(pts, [2]float64{xs[k], y})
		}
	}
	return pts
}

type HeightMap struct {
	xs, ys []float64
	z      [][]float64 // z[j][i]
}

func snap(vals []float64, tol float64) []float64 {
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	var groups [][]float64
	for _, v := range s {
		if len(groups) == 0 || v-groups[len(groups)-1][len(groups[len(groups)-1])-1] > tol {
			groups = append(groups, []float64{v})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], v)
		}
	}
	out := make([]float64, len(groups))
	for i, g := range groups {
		for _, v := range g {
			out[i] += v
		}
		out[i] /= float64(len(g))
	}
	return out
}

func nearest(vs []float64, v float64) int {
	best := 0
	for i := range vs {
		if math.Abs(vs[i]-v) < math.Abs(vs[best]-v) {
			best = i
		}
	}
	return best
}

// MapFromPoints: heights relative to the first point (where Z0 is set later).
func MapFromPoints(pts []Point) (*HeightMap, error) {
	if len(pts) == 0 {
		return nil, fmt.Errorf("в карте нет точек")
	}
	var xv, yv []float64
	for _, p := range pts {
		xv, yv = append(xv, p.X), append(yv, p.Y)
	}
	h := &HeightMap{xs: snap(xv, 0.3), ys: snap(yv, 0.3)}
	sum := make([][]float64, len(h.ys))
	cnt := make([][]int, len(h.ys))
	for j := range sum {
		sum[j], cnt[j] = make([]float64, len(h.xs)), make([]int, len(h.xs))
	}
	for _, p := range pts {
		i, j := nearest(h.xs, p.X), nearest(h.ys, p.Y)
		sum[j][i] += p.Z - pts[0].Z
		cnt[j][i]++
	}
	h.z = make([][]float64, len(h.ys))
	for j := range h.z {
		h.z[j] = make([]float64, len(h.xs))
		for i := range h.z[j] {
			if cnt[j][i] > 0 {
				h.z[j][i] = sum[j][i] / float64(cnt[j][i])
				continue
			}
			// gap: inverse distance weighting over measured points
			var ws, wz float64
			for _, p := range pts {
				d := math.Pow(p.X-h.xs[i], 2) + math.Pow(p.Y-h.ys[j], 2)
				ws += 1 / d
				wz += (p.Z - pts[0].Z) / d
			}
			h.z[j][i] = wz / ws
		}
	}
	return h, nil
}

func LoadPoints(path string) ([]Point, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	var pts []Point
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		m := reNum.FindAllString(l, 3)
		if len(m) == 3 {
			x, _ := strconv.ParseFloat(m[0], 64)
			y, _ := strconv.ParseFloat(m[1], 64)
			z, _ := strconv.ParseFloat(m[2], 64)
			pts = append(pts, Point{x, y, z})
		}
	}
	return pts, nil
}

func (h *HeightMap) At(x, y float64) float64 {
	cell := func(vs []float64, v float64) (int, int, float64) {
		if len(vs) == 1 {
			return 0, 0, 0
		}
		v = math.Min(math.Max(v, vs[0]), vs[len(vs)-1])
		k := 0
		for k < len(vs)-2 && v > vs[k+1] {
			k++
		}
		return k, k + 1, (v - vs[k]) / (vs[k+1] - vs[k])
	}
	i0, i1, tx := cell(h.xs, x)
	j0, j1, ty := cell(h.ys, y)
	a := h.z[j0][i0]*(1-tx) + h.z[j0][i1]*tx
	b := h.z[j1][i0]*(1-tx) + h.z[j1][i1]*tx
	return a*(1-ty) + b*ty
}

func (h *HeightMap) Table() string {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range h.z {
		for _, v := range r {
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "точек %dx%d, перепад %.3f мм (от %.3f до %.3f)\n", len(h.xs), len(h.ys), hi-lo, lo, hi)
	fmt.Fprintf(&b, "      Y\\X ")
	for _, x := range h.xs {
		fmt.Fprintf(&b, "%8.1f", x)
	}
	b.WriteString("\n")
	for j := len(h.ys) - 1; j >= 0; j-- {
		fmt.Fprintf(&b, "%10.1f ", h.ys[j])
		for _, v := range h.z[j] {
			fmt.Fprintf(&b, "%8.3f", v)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func f4(v float64) string {
	s := strconv.FormatFloat(math.Round(v*10000)/10000, 'f', 4, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" || s == "" {
		return "0"
	}
	return s
}

// ApplyMap cuts moves near the surface into seg-long pieces and adds the map to Z.
func ApplyMap(lines []string, h *HeightMap, seg float64) ([]string, error) {
	var x, y, z float64
	mode := 0
	var out []string
	for _, line := range lines {
		ws := words(line)
		var gs []float64
		tgt := map[byte]float64{}
		feed := ""
		for _, w := range ws {
			switch w.L {
			case 'G':
				gs = append(gs, w.V)
				if w.V == 0 || w.V == 1 {
					mode = int(w.V)
				} else if w.V == 2 || w.V == 3 {
					return nil, fmt.Errorf("дуги G2/G3 не поддерживаются")
				}
			case 'X', 'Y', 'Z':
				tgt[w.L] = w.V
			case 'F':
				feed = "F" + f4(w.V)
			}
		}
		skip := len(tgt) == 0
		for _, g := range gs {
			if g == 4 || g == 10 || g == 28 || g == 30 || g == 53 || g == 92 {
				skip = true
			}
		}
		if skip {
			out = append(out, line)
			continue
		}
		nx, ny, nz := x, y, z
		if v, ok := tgt['X']; ok {
			nx = v
		}
		if v, ok := tgt['Y']; ok {
			ny = v
		}
		_, hasZ := tgt['Z']
		if hasZ {
			nz = tgt['Z']
		}
		if mode == 1 && math.Min(z, nz) < 0.5 {
			n := int(math.Max(1, math.Ceil(math.Hypot(nx-x, ny-y)/seg)))
			for k := 1; k <= n; k++ {
				t := float64(k) / float64(n)
				px, py, pz := x+(nx-x)*t, y+(ny-y)*t, z+(nz-z)*t
				f := ""
				if k == 1 {
					f = feed
				}
				out = append(out, "G1X"+f4(px)+"Y"+f4(py)+"Z"+f4(pz+h.At(px, py))+f)
			}
		} else if hasZ && nz < 2 {
			// low rapids follow the relief too, or a bump is hit at rapid speed
			out = append(out, fmt.Sprintf("G%dX%sY%sZ%s%s", mode, f4(nx), f4(ny), f4(nz+h.At(nx, ny)), feed))
		} else {
			out = append(out, line)
		}
		x, y, z = nx, ny, nz
	}
	return out, nil
}
