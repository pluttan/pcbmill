package main

// Gerber RS-274X: a layer read into shapes in millimetres, in the order of the
// file, each dark (adds copper) or clear (takes it away). Circles stay circles,
// everything else becomes polygons; arcs are cut into short chords.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type pt struct{ X, Y float64 }

// shape is one piece of the image: a circle (R > 0) or polygons filled by the
// even-odd rule.
type shape struct {
	dark  bool
	C     pt
	R     float64
	polys [][]pt
}

type gerber struct {
	shapes   []shape
	lines    [][]pt // the centre lines of strokes and region contours: the board outline
	function string // %TF.FileFunction, lower case: "copper,l1,top", "soldermask,top"
}

// arcTol is the largest gap between an arc and its chords, mm.
const arcTol = 0.002

// arcPoints: the arc from a to b round c, both ends included.
func arcPoints(a, b, c pt, cw bool) []pt {
	r := math.Hypot(a.X-c.X, a.Y-c.Y)
	a1 := math.Atan2(a.Y-c.Y, a.X-c.X)
	a2 := math.Atan2(b.Y-c.Y, b.X-c.X)
	if cw {
		for a2 >= a1-1e-9 {
			a2 -= 2 * math.Pi
		}
	} else {
		for a2 <= a1+1e-9 {
			a2 += 2 * math.Pi
		}
	}
	step := math.Pi / 4
	if r > arcTol {
		step = 2 * math.Acos(1-arcTol/r)
	}
	n := int(math.Ceil(math.Abs(a2-a1) / math.Max(step, 1e-3)))
	if n < 2 {
		n = 2
	}
	r2 := math.Hypot(b.X-c.X, b.Y-c.Y) // the ends may sit on slightly different radii
	out := make([]pt, 0, n+1)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		a, rr := a1+(a2-a1)*t, r+(r2-r)*t
		out = append(out, pt{c.X + rr*math.Cos(a), c.Y + rr*math.Sin(a)})
	}
	out[0], out[n] = a, b
	return out
}

// circlePoly: a regular polygon round c, n corners, the first at angle rot.
func circlePoly(c pt, r float64, n int, rot float64) []pt {
	out := make([]pt, n)
	for i := range out {
		a := rot + 2*math.Pi*float64(i)/float64(n)
		out[i] = pt{c.X + r*math.Cos(a), c.Y + r*math.Sin(a)}
	}
	return out
}

func rotate(p pt, deg float64) pt {
	if deg == 0 {
		return p
	}
	s, c := math.Sincos(deg * math.Pi / 180)
	return pt{p.X*c - p.Y*s, p.X*s + p.Y*c}
}

func move(ps []pt, d pt) []pt {
	out := make([]pt, len(ps))
	for i, p := range ps {
		out[i] = pt{p.X + d.X, p.Y + d.Y}
	}
	return out
}

// ==========================
// ===  Apertures         ===
// ==========================

// aperture: the shapes it leaves when flashed at (0, 0). round is the
// diameter of a plain circle, the only aperture a stroke may round its ends with.
type aperture struct {
	parts []shape
	round float64
	rect  []pt // corners of a plain rectangle or polygon, for strokes
}

func stdAperture(kind string, p []float64) (*aperture, error) {
	need := func(n int) error {
		if len(p) < n {
			return fmt.Errorf("апертура %s: мало чисел", kind)
		}
		return nil
	}
	switch kind {
	case "C":
		if err := need(1); err != nil {
			return nil, err
		}
		return &aperture{parts: []shape{{dark: true, R: p[0] / 2}}, round: p[0]}, nil
	case "R":
		if err := need(2); err != nil {
			return nil, err
		}
		w, h := p[0]/2, p[1]/2
		r := []pt{{-w, -h}, {w, -h}, {w, h}, {-w, h}}
		return &aperture{parts: []shape{{dark: true, polys: [][]pt{r}}}, rect: r}, nil
	case "O":
		if err := need(2); err != nil {
			return nil, err
		}
		w, h := p[0], p[1]
		if math.Abs(w-h) < 1e-9 {
			return &aperture{parts: []shape{{dark: true, R: w / 2}}, round: w}, nil
		}
		var ps []pt
		if w > h { // half circles at the left and the right
			r, d := h/2, w/2-h/2
			ps = append(arcPoints(pt{d, -r}, pt{d, r}, pt{d, 0}, false), arcPoints(pt{-d, r}, pt{-d, -r}, pt{-d, 0}, false)...)
		} else {
			r, d := w/2, h/2-w/2
			ps = append(arcPoints(pt{r, d}, pt{-r, d}, pt{0, d}, false), arcPoints(pt{-r, -d}, pt{r, -d}, pt{0, -d}, false)...)
		}
		return &aperture{parts: []shape{{dark: true, polys: [][]pt{ps}}}, rect: ps}, nil
	case "P":
		if err := need(2); err != nil {
			return nil, err
		}
		rot := 0.0
		if len(p) > 2 {
			rot = p[2]
		}
		n := int(p[1])
		if n < 3 {
			n = 3
		}
		ps := circlePoly(pt{}, p[0]/2, n, rot*math.Pi/180)
		return &aperture{parts: []shape{{dark: true, polys: [][]pt{ps}}}, rect: ps}, nil
	}
	return nil, fmt.Errorf("неизвестная апертура %s", kind)
}

// ==========================
// ===  Macros            ===
// ==========================

// expr evaluates a macro expression: numbers, $n, + - x X / and brackets.
type exprP struct {
	s    string
	i    int
	vars map[int]float64
}

func evalExpr(s string, vars map[int]float64) (float64, error) {
	e := &exprP{s: strings.ReplaceAll(s, " ", ""), vars: vars}
	v, err := e.sum()
	if err == nil && e.i < len(e.s) {
		err = fmt.Errorf("лишнее в выражении %q", s)
	}
	return v, err
}

func (e *exprP) sum() (float64, error) {
	v, err := e.prod()
	for err == nil && e.i < len(e.s) && (e.s[e.i] == '+' || e.s[e.i] == '-') {
		op := e.s[e.i]
		e.i++
		var w float64
		if w, err = e.prod(); op == '+' {
			v += w
		} else {
			v -= w
		}
	}
	return v, err
}

func (e *exprP) prod() (float64, error) {
	v, err := e.unary()
	for err == nil && e.i < len(e.s) && (e.s[e.i] == 'x' || e.s[e.i] == 'X' || e.s[e.i] == '/') {
		op := e.s[e.i]
		e.i++
		var w float64
		if w, err = e.unary(); op == '/' {
			if w == 0 {
				return 0, fmt.Errorf("деление на 0 в макросе")
			}
			v /= w
		} else {
			v *= w
		}
	}
	return v, err
}

func (e *exprP) unary() (float64, error) {
	if e.i < len(e.s) && (e.s[e.i] == '-' || e.s[e.i] == '+') {
		neg := e.s[e.i] == '-'
		e.i++
		v, err := e.unary()
		if neg {
			v = -v
		}
		return v, err
	}
	if e.i < len(e.s) && e.s[e.i] == '(' {
		e.i++
		v, err := e.sum()
		if err == nil {
			if e.i >= len(e.s) || e.s[e.i] != ')' {
				return 0, fmt.Errorf("нет ) в %q", e.s)
			}
			e.i++
		}
		return v, err
	}
	if e.i < len(e.s) && e.s[e.i] == '$' {
		j := e.i + 1
		for j < len(e.s) && e.s[j] >= '0' && e.s[j] <= '9' {
			j++
		}
		n, err := strconv.Atoi(e.s[e.i+1 : j])
		if err != nil {
			return 0, fmt.Errorf("плохая переменная в %q", e.s)
		}
		e.i = j
		return e.vars[n], nil
	}
	j := e.i
	for j < len(e.s) && (e.s[j] >= '0' && e.s[j] <= '9' || e.s[j] == '.') {
		j++
	}
	v, err := strconv.ParseFloat(e.s[e.i:j], 64)
	if err != nil {
		return 0, fmt.Errorf("не число в макросе %q", e.s)
	}
	e.i = j
	return v, nil
}

// macroAperture runs the statements of a macro with the given parameters.
func macroAperture(name string, body []string, params []float64, unit float64) (*aperture, error) {
	vars := map[int]float64{}
	for i, v := range params {
		vars[i+1] = v
	}
	ap := &aperture{}
	for _, st := range body {
		st = strings.TrimSpace(st)
		if st == "" || strings.HasPrefix(st, "0 ") || st == "0" {
			continue // a comment
		}
		if st[0] == '$' { // $4=$1x2
			eq := strings.IndexByte(st, '=')
			if eq < 0 {
				return nil, fmt.Errorf("макрос %s: %q", name, st)
			}
			n, err := strconv.Atoi(st[1:eq])
			if err != nil {
				return nil, fmt.Errorf("макрос %s: %q", name, st)
			}
			if vars[n], err = evalExpr(st[eq+1:], vars); err != nil {
				return nil, err
			}
			continue
		}
		var a []float64
		for _, f := range strings.Split(st, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			v, err := evalExpr(f, vars)
			if err != nil {
				return nil, fmt.Errorf("макрос %s: %v", name, err)
			}
			a = append(a, v)
		}
		if len(a) == 0 {
			continue
		}
		parts, err := macroPrimitive(int(a[0]), a[1:], unit)
		if err != nil {
			return nil, fmt.Errorf("макрос %s: %v", name, err)
		}
		ap.parts = append(ap.parts, parts...)
	}
	return ap, nil
}

// macroPrimitive: one statement of a macro; lengths in file units are scaled
// by unit, the exposure 0 clears.
func macroPrimitive(code int, a []float64, unit float64) ([]shape, error) {
	arg := func(i int) float64 {
		if i < len(a) {
			return a[i]
		}
		return 0
	}
	if len(a) < 1 {
		return nil, fmt.Errorf("пустой примитив %d", code)
	}
	dark := arg(0) != 0
	poly := func(ps []pt, rot float64) shape {
		for i := range ps {
			ps[i] = rotate(pt{ps[i].X * unit, ps[i].Y * unit}, rot)
		}
		return shape{dark: dark, polys: [][]pt{ps}}
	}
	switch code {
	case 1: // circle: exposure, diameter, x, y, rotation
		c := rotate(pt{arg(2) * unit, arg(3) * unit}, arg(4))
		return []shape{{dark: dark, C: c, R: arg(1) * unit / 2}}, nil
	case 2, 20: // vector line: exposure, width, x1, y1, x2, y2, rotation
		w := arg(1) / 2
		x1, y1, x2, y2 := arg(2), arg(3), arg(4), arg(5)
		l := math.Hypot(x2-x1, y2-y1)
		if l == 0 {
			return nil, nil
		}
		nx, ny := -(y2-y1)/l*w, (x2-x1)/l*w
		return []shape{poly([]pt{{x1 + nx, y1 + ny}, {x2 + nx, y2 + ny}, {x2 - nx, y2 - ny}, {x1 - nx, y1 - ny}}, arg(6))}, nil
	case 21: // center line: exposure, width, height, cx, cy, rotation
		w, h, cx, cy := arg(1)/2, arg(2)/2, arg(3), arg(4)
		return []shape{poly([]pt{{cx - w, cy - h}, {cx + w, cy - h}, {cx + w, cy + h}, {cx - w, cy + h}}, arg(5))}, nil
	case 22: // lower left line: exposure, width, height, x, y, rotation
		w, h, x, y := arg(1), arg(2), arg(3), arg(4)
		return []shape{poly([]pt{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}, arg(5))}, nil
	case 4: // outline: exposure, n, x0, y0 ... xn, yn, rotation
		n := int(arg(1))
		if len(a) < 2+2*(n+1) {
			return nil, fmt.Errorf("контур: мало точек")
		}
		ps := make([]pt, 0, n+1)
		for i := 0; i <= n; i++ {
			ps = append(ps, pt{a[2+2*i], a[3+2*i]})
		}
		return []shape{poly(ps, arg(2+2*(n+1)))}, nil
	case 5: // polygon: exposure, n, cx, cy, diameter, rotation
		n := int(arg(1))
		if n < 3 {
			n = 3
		}
		ps := circlePoly(pt{arg(2), arg(3)}, arg(4)/2, n, 0)
		return []shape{poly(ps, arg(5))}, nil
	case 7: // thermal: cx, cy, outer, inner, gap, rotation (always dark)
		cx, cy, ro, ri, g, rot := arg(0), arg(1), arg(2)/2, arg(3)/2, arg(4)/2, arg(5)
		var out []shape
		for q := 0; q < 4; q++ {
			// one quarter ring between the gaps, built at the origin and turned
			a0 := math.Asin(math.Min(1, g/ro))
			b0 := 0.0
			if ri > g {
				b0 = math.Asin(g / ri)
			}
			var ps []pt
			for _, p := range arcPoints(pt{ro * math.Cos(a0), ro * math.Sin(a0)}, pt{ro * math.Cos(math.Pi/2-a0), ro * math.Sin(math.Pi/2-a0)}, pt{}, false) {
				ps = append(ps, p)
			}
			if ri > g {
				for _, p := range arcPoints(pt{ri * math.Cos(math.Pi/2-b0), ri * math.Sin(math.Pi/2-b0)}, pt{ri * math.Cos(b0), ri * math.Sin(b0)}, pt{}, true) {
					ps = append(ps, p)
				}
			} else {
				ps = append(ps, pt{g, g})
			}
			for i := range ps {
				ps[i] = rotate(ps[i], float64(90*q))
				ps[i] = pt{ps[i].X + cx, ps[i].Y + cy}
			}
			s := poly(ps, rot)
			s.dark = true
			out = append(out, s)
		}
		return out, nil
	case 6: // moiré: rings and a cross, drawn plainly
		cx, cy, d, th, gap, n, cth, clen, rot := arg(0), arg(1), arg(2), arg(3), arg(4), int(arg(5)), arg(6), arg(7), arg(8)
		var out []shape
		for i := 0; i < n && d > 0; i++ {
			o := circlePoly(pt{cx, cy}, d/2, 64, 0)
			in := circlePoly(pt{cx, cy}, math.Max(d/2-th, 0), 64, 0)
			s := poly(o, rot)
			s.dark = true
			s2 := poly(in, rot)
			s.polys = append(s.polys, s2.polys...)
			out = append(out, s)
			d -= 2 * (th + gap)
		}
		for _, r := range [][]pt{{{cx - clen/2, cy - cth/2}, {cx + clen/2, cy - cth/2}, {cx + clen/2, cy + cth/2}, {cx - clen/2, cy + cth/2}},
			{{cx - cth/2, cy - clen/2}, {cx + cth/2, cy - clen/2}, {cx + cth/2, cy + clen/2}, {cx - cth/2, cy + clen/2}}} {
			s := poly(r, rot)
			s.dark = true
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("неизвестный примитив %d", code)
}

// ==========================
// ===  The file          ===
// ==========================

type gerberState struct {
	g          *gerber
	unit       float64 // mm per file unit
	intD, decD int
	trailing   bool // trailing zeros kept (leading omitted is the usual)
	incr       bool
	aps        map[int]*aperture
	macros     map[string][]string
	ap         *aperture
	apN        int
	pos        pt
	interp     int // 1 line, 2 cw, 3 ccw
	multi      bool
	region     bool
	contour    []pt
	contours   [][]pt
	dark       bool
	lastD      int
	sr         *stepRepeat
}

type stepRepeat struct {
	nx, ny int
	dx, dy float64
	from   int // first shape of the block
}

func parseGerber(src string) (*gerber, error) {
	s := &gerberState{g: &gerber{}, unit: 1, intD: 3, decD: 6, aps: map[int]*aperture{},
		macros: map[string][]string{}, interp: 1, dark: true, lastD: 2}
	i := 0
	for i < len(src) {
		c := src[i]
		if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
			i++
			continue
		}
		if c == '%' { // extended commands up to the next %
			j := strings.IndexByte(src[i+1:], '%')
			if j < 0 {
				return nil, fmt.Errorf("нет закрывающего %%")
			}
			if err := s.extended(src[i+1 : i+1+j]); err != nil {
				return nil, err
			}
			i += j + 2
			continue
		}
		j := strings.IndexByte(src[i:], '*')
		if j < 0 {
			break
		}
		blk := strings.TrimSpace(strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, src[i:i+j]))
		i += j + 1
		if strings.HasPrefix(blk, "G04") || strings.HasPrefix(blk, "G4 ") || blk == "G4" {
			continue
		}
		if blk == "M02" || blk == "M2" || blk == "M00" || blk == "M01" {
			break
		}
		if err := s.data(blk); err != nil {
			return nil, err
		}
	}
	s.endSR()
	return s.g, nil
}

// extended: the statements between % and %, each ending with *.
func (s *gerberState) extended(body string) error {
	body = strings.NewReplacer("\r", "", "\n", "").Replace(body)
	if strings.HasPrefix(body, "AM") { // a macro: name*statements*
		parts := strings.Split(body[2:], "*")
		var st []string
		for _, p := range parts[1:] {
			if strings.TrimSpace(p) != "" {
				st = append(st, p)
			}
		}
		s.macros[parts[0]] = st
		return nil
	}
	for _, st := range strings.Split(body, "*") {
		if st == "" {
			continue
		}
		switch {
		case strings.HasPrefix(st, "FS"):
			if err := s.format(st); err != nil {
				return err
			}
		case strings.HasPrefix(st, "MO"):
			if strings.HasPrefix(st[2:], "IN") {
				s.unit = 25.4
			} else {
				s.unit = 1
			}
		case strings.HasPrefix(st, "AD"):
			if err := s.define(st[2:]); err != nil {
				return err
			}
		case strings.HasPrefix(st, "LP"):
			s.dark = !strings.HasPrefix(st[2:], "C")
		case strings.HasPrefix(st, "SR"):
			s.endSR()
			sr := &stepRepeat{nx: 1, ny: 1, from: len(s.g.shapes)}
			for _, f := range splitWords(st[2:]) {
				v, _ := strconv.ParseFloat(f.num, 64)
				switch f.l {
				case 'X':
					sr.nx = int(v)
				case 'Y':
					sr.ny = int(v)
				case 'I':
					sr.dx = v * s.unit
				case 'J':
					sr.dy = v * s.unit
				}
			}
			if sr.nx > 1 || sr.ny > 1 {
				s.sr = sr
			}
		case strings.HasPrefix(st, "TF.FileFunction,"):
			s.g.function = strings.ToLower(strings.TrimPrefix(st, "TF.FileFunction,"))
		}
	}
	return nil
}

// endSR repeats the shapes of an open step-and-repeat block.
func (s *gerberState) endSR() {
	sr := s.sr
	if sr == nil {
		return
	}
	s.sr = nil
	block := append([]shape(nil), s.g.shapes[sr.from:]...)
	for ix := 0; ix < sr.nx; ix++ {
		for iy := 0; iy < sr.ny; iy++ {
			if ix == 0 && iy == 0 {
				continue
			}
			d := pt{float64(ix) * sr.dx, float64(iy) * sr.dy}
			for _, sh := range block {
				n := shape{dark: sh.dark, C: pt{sh.C.X + d.X, sh.C.Y + d.Y}, R: sh.R}
				for _, p := range sh.polys {
					n.polys = append(n.polys, move(p, d))
				}
				s.g.shapes = append(s.g.shapes, n)
			}
		}
	}
}

func (s *gerberState) format(st string) error {
	if len(st) > 2 && st[2] == 'T' {
		s.trailing = true
	}
	s.incr = strings.Contains(st[2:5], "I")
	if k := strings.IndexByte(st, 'X'); k >= 0 && k+2 < len(st) {
		s.intD = int(st[k+1] - '0')
		s.decD = int(st[k+2] - '0')
		return nil
	}
	return fmt.Errorf("плохой формат %s", st)
}

// define: an aperture, D10C,0.5 or D11RoundRect,0.1X...
func (s *gerberState) define(st string) error {
	j := 1
	for j < len(st) && st[j] >= '0' && st[j] <= '9' {
		j++
	}
	n, err := strconv.Atoi(st[1:j])
	if err != nil {
		return fmt.Errorf("плохая апертура %s", st)
	}
	name, args := st[j:], ""
	if k := strings.IndexByte(name, ','); k >= 0 {
		name, args = name[:k], name[k+1:]
	}
	var p []float64
	for _, f := range strings.Split(args, "X") {
		if f == "" {
			continue
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return fmt.Errorf("плохая апертура %s", st)
		}
		p = append(p, v)
	}
	var ap *aperture
	if body, ok := s.macros[name]; ok {
		ap, err = macroAperture(name, body, p, s.unit)
	} else {
		for i := range p {
			if !(name == "P" && i >= 1 && i <= 2) { // the count of corners and the angle are not lengths
				p[i] *= s.unit
			}
		}
		ap, err = stdAperture(name, p)
	}
	if err != nil {
		return err
	}
	s.aps[n] = ap
	return nil
}

type gword struct {
	l   byte
	num string
}

func splitWords(b string) []gword {
	var out []gword
	for i := 0; i < len(b); {
		l := b[i]
		j := i + 1
		for j < len(b) && (b[j] >= '0' && b[j] <= '9' || b[j] == '.' || b[j] == '-' || b[j] == '+') {
			j++
		}
		out = append(out, gword{l, b[i+1 : j]})
		i = j
	}
	return out
}

// coord turns a number of the file into mm.
func (s *gerberState) coord(n string) (float64, error) {
	if strings.Contains(n, ".") {
		v, err := strconv.ParseFloat(n, 64)
		return v * s.unit, err
	}
	neg := strings.HasPrefix(n, "-")
	n = strings.TrimLeft(n, "+-")
	if s.trailing {
		for len(n) < s.intD+s.decD {
			n += "0"
		}
	}
	v, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("плохое число %q", n)
	}
	f := float64(v) / math.Pow(10, float64(s.decD))
	if neg {
		f = -f
	}
	return f * s.unit, nil
}

func (s *gerberState) data(blk string) error {
	ws := splitWords(blk)
	var x, y, ii, jj *float64
	d := 0
	for _, w := range ws {
		switch w.l {
		case 'G':
			g, _ := strconv.Atoi(w.num)
			switch g {
			case 1, 2, 3:
				s.interp = g
			case 36:
				s.region, s.contour, s.contours = true, nil, nil
			case 37:
				s.closeContour()
				s.flushRegion()
				s.region = false
			case 74:
				s.multi = false
			case 75:
				s.multi = true
			}
		case 'D':
			n, _ := strconv.Atoi(w.num)
			if n >= 10 {
				s.ap, s.apN = s.aps[n], n
			} else {
				d = n
			}
		case 'X', 'Y', 'I', 'J':
			v, err := s.coord(w.num)
			if err != nil {
				return err
			}
			switch w.l {
			case 'X':
				x = &v
			case 'Y':
				y = &v
			case 'I':
				ii = &v
			case 'J':
				jj = &v
			}
		case 'M', 'N':
		}
	}
	if x == nil && y == nil && d == 0 {
		return nil
	}
	if d == 0 {
		d = s.lastD // the deprecated modal D01
	}
	s.lastD = d
	to := s.pos
	if s.incr {
		if x != nil {
			to.X += *x
		}
		if y != nil {
			to.Y += *y
		}
	} else {
		if x != nil {
			to.X = *x
		}
		if y != nil {
			to.Y = *y
		}
	}
	var off pt
	if ii != nil {
		off.X = *ii
	}
	if jj != nil {
		off.Y = *jj
	}
	switch d {
	case 1:
		path := []pt{s.pos, to}
		if s.interp != 1 {
			path = arcPoints(s.pos, to, s.center(s.pos, to, off), s.interp == 2)
		}
		if s.region {
			if len(s.contour) == 0 {
				s.contour = []pt{s.pos}
			}
			s.contour = append(s.contour, path[1:]...)
		} else {
			s.g.lines = append(s.g.lines, path)
			if err := s.stroke(path); err != nil {
				return err
			}
		}
	case 2:
		if s.region {
			s.closeContour()
		}
	case 3:
		if s.ap == nil {
			return fmt.Errorf("засветка без апертуры D%d", s.apN)
		}
		s.flash(to)
	}
	s.pos = to
	return nil
}

// center of an arc: in the multi quadrant mode I, J are signed; in the single
// quadrant one they are not and the centre is the one giving an arc to 90°
// with both ends at the same distance.
func (s *gerberState) center(a, b, off pt) pt {
	if s.multi {
		return pt{a.X + off.X, a.Y + off.Y}
	}
	best, bestErr := pt{a.X + off.X, a.Y + off.Y}, math.Inf(1)
	for _, sx := range []float64{1, -1} {
		for _, sy := range []float64{1, -1} {
			c := pt{a.X + sx*math.Abs(off.X), a.Y + sy*math.Abs(off.Y)}
			ra, rb := math.Hypot(a.X-c.X, a.Y-c.Y), math.Hypot(b.X-c.X, b.Y-c.Y)
			a1, a2 := math.Atan2(a.Y-c.Y, a.X-c.X), math.Atan2(b.Y-c.Y, b.X-c.X)
			sw := a2 - a1
			if s.interp == 2 {
				sw = -sw
			}
			for sw < 0 {
				sw += 2 * math.Pi
			}
			if sw > math.Pi/2+1e-6 {
				continue
			}
			if e := math.Abs(ra - rb); e < bestErr {
				best, bestErr = c, e
			}
		}
	}
	return best
}

func (s *gerberState) closeContour() {
	if len(s.contour) >= 3 {
		s.contours = append(s.contours, s.contour)
		s.g.lines = append(s.g.lines, append(append([]pt(nil), s.contour...), s.contour[0]))
	}
	s.contour = nil
}

// flushRegion: each contour of a region is filled on its own, so contours
// that overlap add up rather than cancel.
func (s *gerberState) flushRegion() {
	for _, c := range s.contours {
		s.g.shapes = append(s.g.shapes, shape{dark: s.dark, polys: [][]pt{c}})
	}
	s.contours = nil
}

func (s *gerberState) flash(at pt) {
	for _, p := range s.ap.parts {
		dark := p.dark == s.dark // a clear part of a dark flash clears, of a clear one darkens
		if p.R > 0 {
			s.g.shapes = append(s.g.shapes, shape{dark: dark, C: pt{p.C.X + at.X, p.C.Y + at.Y}, R: p.R})
			continue
		}
		sh := shape{dark: dark}
		for _, ps := range p.polys {
			sh.polys = append(sh.polys, move(ps, at))
		}
		s.g.shapes = append(s.g.shapes, sh)
	}
}

// stroke draws a line or an arc (already in chords) with the current aperture.
func (s *gerberState) stroke(path []pt) error {
	if s.ap == nil {
		return fmt.Errorf("линия без апертуры D%d", s.apN)
	}
	if s.ap.round > 0 {
		r := s.ap.round / 2
		if r <= 0 {
			return nil // zero width lines draw nothing
		}
		for i, p := range path {
			s.g.shapes = append(s.g.shapes, shape{dark: s.dark, C: p, R: r}) // round ends and joints
			if i > 0 {
				if q := segQuad(path[i-1], p, r); q != nil {
					s.g.shapes = append(s.g.shapes, shape{dark: s.dark, polys: [][]pt{q}})
				}
			}
		}
		return nil
	}
	if s.ap.rect != nil { // a rectangle or polygon swept along each segment
		for i := 1; i < len(path); i++ {
			var cloud []pt
			cloud = append(cloud, move(s.ap.rect, path[i-1])...)
			cloud = append(cloud, move(s.ap.rect, path[i])...)
			s.g.shapes = append(s.g.shapes, shape{dark: s.dark, polys: [][]pt{hull(cloud)}})
		}
		return nil
	}
	// a macro aperture cannot draw lines by the standard; flash it at the ends
	s.flash(path[0])
	s.flash(path[len(path)-1])
	return nil
}

// segQuad: the rectangle of a line of width 2r from a to b, without the round ends.
func segQuad(a, b pt, r float64) []pt {
	l := math.Hypot(b.X-a.X, b.Y-a.Y)
	if l < 1e-12 {
		return nil
	}
	nx, ny := -(b.Y-a.Y)/l*r, (b.X-a.X)/l*r
	return []pt{{a.X + nx, a.Y + ny}, {b.X + nx, b.Y + ny}, {b.X - nx, b.Y - ny}, {a.X - nx, a.Y - ny}}
}

// hull: the convex hull of points, counter-clockwise (monotone chain).
func hull(ps []pt) []pt {
	ps = append([]pt(nil), ps...)
	sortPts(ps)
	cross := func(o, a, b pt) float64 { return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X) }
	var lo, up []pt
	for _, p := range ps {
		for len(lo) >= 2 && cross(lo[len(lo)-2], lo[len(lo)-1], p) <= 0 {
			lo = lo[:len(lo)-1]
		}
		lo = append(lo, p)
	}
	for i := len(ps) - 1; i >= 0; i-- {
		p := ps[i]
		for len(up) >= 2 && cross(up[len(up)-2], up[len(up)-1], p) <= 0 {
			up = up[:len(up)-1]
		}
		up = append(up, p)
	}
	return append(lo[:len(lo)-1], up[:len(up)-1]...)
}

func sortPts(ps []pt) {
	// insertion sort is enough for the dozen corners of two apertures
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && (ps[j].X < ps[j-1].X || ps[j].X == ps[j-1].X && ps[j].Y < ps[j-1].Y); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

// bounds of a layer, the dark shapes only.
func (g *gerber) bounds() (rect, bool) {
	r := rect{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	ok := false
	add := func(x0, x1, y0, y1 float64) {
		r.X0, r.X1 = math.Min(r.X0, x0), math.Max(r.X1, x1)
		r.Y0, r.Y1 = math.Min(r.Y0, y0), math.Max(r.Y1, y1)
		ok = true
	}
	for _, s := range g.shapes {
		if !s.dark {
			continue
		}
		if s.R > 0 {
			add(s.C.X-s.R, s.C.X+s.R, s.C.Y-s.R, s.C.Y+s.R)
		}
		for _, p := range s.polys {
			for _, q := range p {
				add(q.X, q.X, q.Y, q.Y)
			}
		}
	}
	return r, ok
}
