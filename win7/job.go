package main

// The job: a sheet of copper with several boards on it, laid out by the
// program along their real outlines, one height map over the boards, and
// the programs of the sheet stage by stage: tracks, mask, drilling, V-groove.
// It lives in the folder job next to the exe; job.txt keeps what was chosen
// and what is done, so the work goes on after a restart.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type jobSrc struct {
	Path string // relative to the exe when it lies next to it
	N    int    // how many of this board
}

type jobPlace struct {
	Src int
	Rot int
	X   float64 // the lower left corner of the board box on the sheet
	Y   float64
}

type Job struct {
	Dir               string
	W, H, Margin, Gap float64 // the sheet, mm; Margin: bare copper round the boards
	Step              float64 // of the height map
	Srcs              []jobSrc
	Places            []jobPlace
	Probe             *pt               // the mask probe on the sheet
	IsoProbe          *pt               // the probe of isolation feeds and depths
	Done              map[string]string // stage or file -> when it was done
	boards            []*board
}

func newJob(dir string) *Job {
	return &Job{Dir: dir, W: 100, H: 100, Margin: 8, Gap: 3, Step: 10, Done: map[string]string{}}
}

const jobFile = "job.txt"

// zero: where Z0 is touched after every change of tool, on bare copper in
// the corner of the sheet, outside every board: the mask never covers it.
func (j *Job) zero() pt { return pt{j.Margin / 2, j.Margin / 2} }

// zeroSpotR: the radius of the circle the mask program bares at the zero.
const zeroSpotR = 1.0

// zeroSpot: rings from the edge of the circle inwards at the mask depth and
// the centre, so the copper under the zero is bare for the touches of Z0.
func zeroSpot(at pt, c camCfg) []cpath {
	rt := c.maskWidth(c.MaskDepth) / 2
	var out []cpath
	for r := zeroSpotR - rt; r > 0.5*rt; r -= 0.9 * rt {
		n := int(math.Max(12, math.Ceil(2*math.Pi*r/0.1)))
		var ring cpath
		for i := 0; i <= n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			ring.pts = append(ring.pts, p3{at.X + r*math.Cos(a), at.Y + r*math.Sin(a), -c.MaskDepth})
		}
		out = append(out, ring)
	}
	return append(out, cpath{pts: []p3{{at.X, at.Y, -c.MaskDepth}}})
}

func (j *Job) save() error {
	if err := os.MkdirAll(j.Dir, 0755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# pcbscan: задание (лист, платы, раскладка, что сделано)\r\n")
	fmt.Fprintf(&b, "sheet %s %s %s %s\r\n", num(j.W), num(j.H), num(j.Margin), num(j.Gap))
	fmt.Fprintf(&b, "step %s\r\n", num(j.Step))
	for _, s := range j.Srcs {
		fmt.Fprintf(&b, "src %d %s\r\n", s.N, s.Path)
	}
	for _, p := range j.Places {
		fmt.Fprintf(&b, "place %d %d %s %s\r\n", p.Src, p.Rot, num(p.X), num(p.Y))
	}
	if j.Probe != nil {
		fmt.Fprintf(&b, "probe %s %s\r\n", num(j.Probe.X), num(j.Probe.Y))
	}
	if j.IsoProbe != nil {
		fmt.Fprintf(&b, "isoprobe %s %s\r\n", num(j.IsoProbe.X), num(j.IsoProbe.Y))
	}
	var keys []string
	for k := range j.Done {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "done %s %s\r\n", k, j.Done[k])
	}
	tmp := filepath.Join(j.Dir, jobFile+".new")
	if err := os.WriteFile(tmp, []byte(b.String()), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(j.Dir, jobFile))
}

// loadJob reads job.txt; a missing one is a new job.
func loadJob(dir string) (*Job, error) {
	j := newJob(dir)
	data, err := os.ReadFile(filepath.Join(dir, jobFile))
	if os.IsNotExist(err) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	for n, l := range strings.Split(string(data), "\n") {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		bad := fmt.Errorf("%s, строка %d: %q", jobFile, n+1, strings.TrimSpace(l))
		nums := func(k int) ([]float64, error) {
			if len(f) < k+1 {
				return nil, bad
			}
			out := make([]float64, k)
			for i := range out {
				v, err := strconv.ParseFloat(f[i+1], 64)
				if err != nil {
					return nil, bad
				}
				out[i] = v
			}
			return out, nil
		}
		switch f[0] {
		case "sheet":
			v, err := nums(4)
			if err != nil {
				return nil, err
			}
			j.W, j.H, j.Margin, j.Gap = v[0], v[1], v[2], v[3]
		case "step":
			v, err := nums(1)
			if err != nil {
				return nil, err
			}
			j.Step = v[0]
		case "src":
			if len(f) < 3 {
				return nil, bad
			}
			k, err := strconv.Atoi(f[1])
			if err != nil || k < 1 {
				return nil, bad
			}
			path := strings.TrimSpace(strings.SplitN(strings.TrimSpace(l), " ", 3)[2])
			j.Srcs = append(j.Srcs, jobSrc{path, k})
		case "place":
			v, err := nums(4)
			if err != nil {
				return nil, err
			}
			j.Places = append(j.Places, jobPlace{int(v[0]), int(v[1]) & 3, v[2], v[3]})
		case "probe":
			v, err := nums(2)
			if err != nil {
				return nil, err
			}
			j.Probe = &pt{v[0], v[1]}
		case "isoprobe":
			v, err := nums(2)
			if err != nil {
				return nil, err
			}
			j.IsoProbe = &pt{v[0], v[1]}
		case "done":
			if len(f) >= 2 {
				j.Done[f[1]] = strings.Join(f[2:], " ")
			}
		}
	}
	for _, p := range j.Places {
		if p.Src < 0 || p.Src >= len(j.Srcs) {
			return nil, fmt.Errorf("%s: плата %d вне списка", jobFile, p.Src)
		}
	}
	return j, nil
}

// srcPath: where the files of a board are.
func (j *Job) srcPath(s jobSrc) string {
	if filepath.IsAbs(s.Path) {
		return s.Path
	}
	return filepath.Join(filepath.Dir(j.Dir), s.Path)
}

// loadBoards reads the boards of the job once.
func (j *Job) loadBoards() error {
	if len(j.boards) == len(j.Srcs) {
		return nil
	}
	j.boards = nil
	for _, s := range j.Srcs {
		b, err := loadBoard(j.srcPath(s))
		if err != nil {
			return err
		}
		j.boards = append(j.boards, b)
	}
	return nil
}

func (j *Job) mark(key string) {
	j.Done[key] = time.Now().Format("02.01_15:04")
}

// placeOf: the placement of one board on the sheet.
func (j *Job) placeOf(p jobPlace, mirror bool) place {
	return place{box: j.boards[p.Src].box, mirror: mirror, rot: p.Rot, dx: p.X, dy: p.Y}
}

// ==========================
// ===  Layout            ===
// ==========================

// nestCell: the layout grid, mm. Boards land on whole millimetres.
const nestCell = 1.0

// cellsOf: the cells a board covers lying as p (corner at 0, 0): per row of
// cells the spans [a, b] of cells it touches at all.
func cellsOf(b *board, p place) [][][2]int {
	p.dx, p.dy = 0, 0
	w, h := p.size()
	var loops [][]pt
	if b.closed {
		for _, l := range b.outline {
			ll := make([]pt, len(l))
			for i, q := range l {
				ll[i] = p.at(q)
			}
			loops = append(loops, ll)
		}
	} else {
		loops = [][]pt{{{0, 0}, {w, 0}, {w, h}, {0, h}}}
	}
	const sub = 4 // fine pixels per cell, any of them marks the cell
	r := newRaster(rect{0, w, 0, h}, nestCell/sub)
	r.fill(loops, 1)
	for _, l := range loops { // the edge itself too: a thin outline line is the board
		for k := 1; k < len(l); k++ {
			r.draw(shape{dark: true, polys: [][]pt{segQuad(l[k-1], l[k], r.px)}})
		}
	}
	cw, ch := int(math.Ceil(w/nestCell)), int(math.Ceil(h/nestCell))
	rows := make([][][2]int, ch)
	for cj := 0; cj < ch; cj++ {
		a := -1
		for ci := 0; ci <= cw; ci++ {
			on := false
			for dy := 0; dy < sub && !on && ci < cw; dy++ {
				for dx := 0; dx < sub; dx++ {
					if r.at(ci*sub+dx, cj*sub+dy) {
						on = true
						break
					}
				}
			}
			switch {
			case on && a < 0:
				a = ci
			case !on && a >= 0:
				rows[cj] = append(rows[cj], [2]int{a, ci - 1})
				a = -1
			}
		}
	}
	return rows
}

type occGrid struct {
	w, h int
	sum  [][]int // prefix sums of taken cells per row
	on   [][]bool
}

func newOcc(w, h int) *occGrid {
	g := &occGrid{w: w, h: h}
	g.on = make([][]bool, h)
	g.sum = make([][]int, h)
	for j := range g.on {
		g.on[j] = make([]bool, w)
		g.sum[j] = make([]int, w+1)
	}
	return g
}

func (g *occGrid) free(j, a, b int) bool {
	if j < 0 || j >= g.h || a < 0 || b >= g.w {
		return false
	}
	return g.sum[j][b+1]-g.sum[j][a] == 0
}

func (g *occGrid) take(j, a, b int) {
	if j < 0 || j >= g.h {
		return
	}
	if a < 0 {
		a = 0
	}
	if b >= g.w {
		b = g.w - 1
	}
	for i := a; i <= b; i++ {
		g.on[j][i] = true
	}
	for i := 0; i < g.w; i++ {
		v := 0
		if g.on[j][i] {
			v = 1
		}
		g.sum[j][i+1] = g.sum[j][i] + v
	}
}

// fits: the shape at cell (x, y).
func (g *occGrid) fits(rows [][][2]int, x, y int) bool {
	for cj, spans := range rows {
		for _, s := range spans {
			if !g.free(y+cj, x+s[0], x+s[1]) {
				return false
			}
		}
	}
	return true
}

// layout puts every board on the sheet bottom left first, the biggest first,
// trying the four quarter turns; the boards keep Gap between their outlines
// and Margin from the edges. The probes go in last when they are wanted:
// the isolation probe first, it is the bigger.
func (j *Job) layout(mirror, probe, isoProbe bool) error {
	if err := j.loadBoards(); err != nil {
		return err
	}
	cw, ch := int(math.Floor((j.W-2*j.Margin)/nestCell)), int(math.Floor((j.H-2*j.Margin)/nestCell))
	if cw < 1 || ch < 1 {
		return fmt.Errorf("лист %sx%s мм меньше двух отступов по %s мм", num(j.W), num(j.H), num(j.Margin))
	}
	occ := newOcc(cw, ch)
	type item struct {
		src  int
		area float64
	}
	var items []item
	for i, s := range j.Srcs {
		b := j.boards[i].box
		for k := 0; k < s.N; k++ {
			items = append(items, item{i, (b.X1 - b.X0) * (b.Y1 - b.Y0)})
		}
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].area > items[b].area })
	gap := int(math.Ceil(j.Gap/nestCell - 1e-9))
	// put: the best corner for a shape, bottom first, then left
	put := func(shapes [][][][2]int) (int, int, int, bool) {
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				for r, rows := range shapes {
					if rows != nil && occ.fits(rows, x, y) {
						return r, x, y, true
					}
				}
			}
		}
		return 0, 0, 0, false
	}
	takeShape := func(rows [][][2]int, x, y int) {
		for cj, spans := range rows {
			for _, s := range spans {
				for d := -gap; d <= gap; d++ {
					occ.take(y+cj+d, x+s[0]-gap, x+s[1]+gap)
				}
			}
		}
	}
	shapes := map[int][][][][2]int{}
	j.Places, j.Probe, j.IsoProbe = nil, nil, nil
	for n, it := range items {
		sh, ok := shapes[it.src]
		if !ok {
			b := j.boards[it.src]
			for r := 0; r < 4; r++ {
				sh = append(sh, cellsOf(b, place{box: b.box, mirror: mirror, rot: r}))
			}
			shapes[it.src] = sh
		}
		r, x, y, ok := put(sh)
		if !ok {
			return fmt.Errorf("на лист %sx%s мм помещается %d плат из %d: увеличьте лист или уменьшите зазор",
				num(j.W), num(j.H), n, len(items))
		}
		takeShape(sh[r], x, y)
		j.Places = append(j.Places, jobPlace{it.src, r, j.Margin + float64(x)*nestCell, j.Margin + float64(y)*nestCell})
	}
	// a probe is a box; it keeps the gap to the boards and to the other probe
	box := func(w, h float64) *pt {
		pw, ph := int(math.Ceil(w/nestCell)), int(math.Ceil(h/nestCell))
		rows := make([][][2]int, ph)
		for k := range rows {
			rows[k] = [][2]int{{0, pw - 1}}
		}
		_, x, y, ok := put([][][][2]int{rows})
		if !ok {
			return nil
		}
		takeShape(rows, x, y)
		return &pt{j.Margin + float64(x)*nestCell, j.Margin + float64(y)*nestCell}
	}
	if isoProbe {
		if j.IsoProbe = box(isoProbeW, isoProbeH); j.IsoProbe == nil {
			return fmt.Errorf("на листе %sx%s мм нет места %sx%s мм под пробу подач: увеличьте лист или выключите пробу",
				num(j.W), num(j.H), num(isoProbeW), num(isoProbeH))
		}
	}
	if probe {
		j.Probe = box(probeW, probeH)
	}
	return nil
}

// used: the box the boards take on the sheet.
func (j *Job) used(mirror bool) rect {
	r := rect{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, p := range j.Places {
		pl := j.placeOf(p, mirror)
		w, h := pl.size()
		r = r.union(rect{p.X, p.X + w, p.Y, p.Y + h})
	}
	return r
}

// boardsRaster: the boards as they lie, px mm a pixel, over the sheet.
func (j *Job) boardsRaster(mirror bool, px float64) *raster {
	r := newRaster(rect{0, j.W, 0, j.H}, px)
	for _, p := range j.Places {
		pl := j.placeOf(p, mirror)
		b := j.boards[p.Src]
		var loops [][]pt
		if b.closed {
			for _, l := range b.outline {
				ll := make([]pt, len(l))
				for i, q := range l {
					ll[i] = pl.at(q)
				}
				loops = append(loops, ll)
			}
		} else {
			w, h := pl.size()
			loops = [][]pt{{{p.X, p.Y}, {p.X + w, p.Y}, {p.X + w, p.Y + h}, {p.X, p.Y + h}}}
		}
		r.fill(loops, 1)
	}
	return r
}

// scanPoints: the height map only where the boards are: a grid of Step over
// them, a point kept when a board is within its reach; Z0 first, in the
// corner, so the map and every later Z0 have the same zero.
func (j *Job) scanPoints(mirror bool) [][2]float64 {
	if len(j.Places) == 0 {
		return nil
	}
	u := j.used(mirror)
	g := GridFor(u.X0, u.X1, u.Y0, u.Y1, j.Step)
	r := j.boardsRaster(mirror, 0.5)
	d := r.distTo(1, nil)
	reach := math.Max(g.StepX(), g.StepY())*0.75 + 0.5
	z := j.zero()
	out := [][2]float64{{z.X, z.Y}}
	for _, q := range g.Points() {
		i, k := int(r.col(q[0])+0.5), int(r.row(q[1])+0.5)
		if i < 0 || k < 0 || i >= r.w || k >= r.h {
			continue
		}
		if float64(d[k*r.w+i])/distScale*r.px <= reach {
			out = append(out, q)
		}
	}
	return out
}

// ==========================
// ===  Programs          ===
// ==========================

// stages: the order of the work and the files of each.
var stages = []struct{ key, title, prefix string }{
	{"iso", "Фрезеровка", "1_"},
	{"mask", "Маска", "2_"},
	{"drill", "Сверловка", "3_"},
	{"vcut", "V-рез", "4_"},
}

// cam makes the programs of the sheet: every board once, put on all its
// places, the same operations of all boards in one file.
func (j *Job) cam(c camCfg, say func(string)) ([]*camOp, error) {
	if err := j.loadBoards(); err != nil {
		return nil, err
	}
	mirror := c.Side == sideBottom
	merged := map[string]*camOp{}
	var keys []string
	boardsOn := map[string]int{}
	for i := range j.Srcs {
		var pls []place
		for _, p := range j.Places {
			if p.Src == i {
				pls = append(pls, j.placeOf(p, mirror))
			}
		}
		if len(pls) == 0 {
			continue
		}
		for _, raw := range camBoard(j.boards[i], c, say) {
			op := placeOp(raw, pls)
			boardsOn[op.file] += len(pls)
			m, ok := merged[op.file]
			if !ok {
				merged[op.file] = op
				keys = append(keys, op.file)
				continue
			}
			m.paths = append(m.paths, op.paths...)
			m.marks = append(m.marks, op.marks...)
			m.shortW = math.Max(m.shortW, op.shortW)
		}
	}
	if m := merged["2_mask"]; m != nil { // bare copper at the zero for the touches after the mask
		z := j.zero()
		m.paths = append(m.paths, zeroSpot(z, c)...)
		m.notes = append(m.notes, fmt.Sprintf("круг %s мм без маски в ноле X%s Y%s: касание Z0 после маски",
			num(2*zeroSpotR), num(z.X), num(z.Y)))
	}
	if j.IsoProbe != nil && merged["1_iso"] != nil {
		op := isoTest(*j.IsoProbe, c)
		op.tool = merged["1_iso"].tool
		merged[op.file] = op
		keys = append(keys, op.file)
	}
	if j.Probe != nil && merged["2_mask"] != nil {
		op := maskTest(*j.Probe, c)
		op.tool = merged["2_mask"].tool
		merged[op.file] = op
		keys = append(keys, op.file)
	}
	sort.Strings(keys)
	var ops []*camOp
	for _, k := range keys {
		op := merged[k]
		if n := boardsOn[k]; n > 1 {
			op.notes = append(op.notes, fmt.Sprintf("плат в файле: %d", n))
		}
		if len(op.marks) > 0 && strings.HasPrefix(k, "1_iso") {
			var where []string
			for i, m := range op.marks {
				if i == 6 {
					where = append(where, "...")
					break
				}
				where = append(where, fmt.Sprintf("(%.1f; %.1f)", m.X, m.Y))
			}
			op.warn = append(op.warn, fmt.Sprintf("%d мест, где медь ближе ширины реза %.2f мм, там останется перемычка: %s",
				len(op.marks), op.shortW, strings.Join(where, " ")))
		}
		op.paths = order(op.paths, j.zero())
		ops = append(ops, op)
	}
	return ops, nil
}

// progPath: the program of the sheet for one operation.
func (j *Job) progPath(file string) string { return filepath.Join(j.Dir, "sheet_"+file+".tap") }

// writeProgs puts the programs into the job folder, the old ones of the
// sheet out first, so a drill gone from the boards does not stay as a file.
func (j *Job) writeProgs(ops []*camOp) error {
	if err := os.MkdirAll(j.Dir, 0755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(j.Dir, "sheet_*.tap"))
	notes, _ := filepath.Glob(filepath.Join(j.Dir, "sheet_*.txt"))
	for _, f := range append(old, notes...) {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	for _, op := range ops {
		if err := os.WriteFile(j.progPath(op.file), []byte(strings.Join(op.gcode(), "\r\n")+"\r\n"), 0644); err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString(op.title + "\r\n")
		for _, n := range op.notes {
			b.WriteString("note " + n + "\r\n")
		}
		for _, n := range op.warn {
			b.WriteString("warn " + n + "\r\n")
		}
		for _, m := range op.marks {
			fmt.Fprintf(&b, "mark %.2f %.2f\r\n", m.X, m.Y)
		}
		if err := os.WriteFile(strings.TrimSuffix(j.progPath(op.file), ".tap")+".txt", []byte(b.String()), 0644); err != nil {
			return err
		}
	}
	return nil
}

// progNotes: what was said about a program when it was made: its title,
// the notes, the warnings, the narrow places.
func progNotes(file string) (title string, notes, warn []string, marks []pt) {
	base := strings.TrimSuffix(file, ".tap")
	if i := strings.Index(base, "_redo"); i >= 0 {
		base = base[:i]
	}
	data, err := os.ReadFile(base + ".txt")
	if err != nil {
		return "", nil, nil, nil
	}
	for i, l := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		switch {
		case i == 0:
			title = l
		case strings.HasPrefix(l, "note "):
			notes = append(notes, l[5:])
		case strings.HasPrefix(l, "warn "):
			warn = append(warn, l[5:])
		case strings.HasPrefix(l, "mark "):
			var x, y float64
			if _, err := fmt.Sscanf(l[5:], "%f %f", &x, &y); err == nil {
				marks = append(marks, pt{x, y})
			}
		}
	}
	return
}

// stageFiles: the programs of a stage in the job folder, redone ones after
// the plain ones.
func (j *Job) stageFiles(prefix string) []string {
	all, _ := filepath.Glob(filepath.Join(j.Dir, "sheet_"+prefix+"*.tap"))
	var out []string
	for _, f := range all {
		if !strings.HasSuffix(strings.ToLower(f), "_level.tap") {
			out = append(out, f)
		}
	}
	// the mask probe first (its squares tell the depth), the redos last
	rank := func(f string) int {
		switch {
		case strings.Contains(f, "_redo"):
			return 2
		case strings.Contains(f, "_probe"):
			return 0
		}
		return 1
	}
	sort.SliceStable(out, func(a, b int) bool {
		if ra, rb := rank(out[a]), rank(out[b]); ra != rb {
			return ra < rb
		}
		return out[a] < out[b]
	})
	return out
}

func (j *Job) mapPath() string { return filepath.Join(j.Dir, "scan.csv") }
