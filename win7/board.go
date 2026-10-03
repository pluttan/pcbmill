package main

// A board from the files of the factory: a zip or a folder of Gerber and
// Excellon files from KiCad, EasyEDA, Eagle, Altium. Which file is which layer
// is read from the X2 attribute inside, else guessed from the name.

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The layers we use.
const (
	lyTop     = "top"
	lyBottom  = "bottom"
	lyTopMask = "topmask"
	lyBotMask = "botmask"
	lyOutline = "outline"
	lyDrill   = "drill"
)

type srcFile struct {
	name string
	data string
}

type board struct {
	name    string
	files   map[string][]string // layer -> file names used
	copper  map[string]*gerber  // lyTop, lyBottom
	mask    map[string]*gerber  // lyTopMask, lyBotMask
	outline [][]pt              // loops (closed: last == first) and open lines of the edge
	closed  bool                // every outline line is a loop
	holes   []hole
	box     rect // the outline, or the copper when there is none
	notes   []string
}

var (
	reDrillHead = regexp.MustCompile(`(?m)^\s*(M48|T\d+C[\d.])`)
	reFunc      = regexp.MustCompile(`%TF\.FileFunction,([^*]*)\*%`)
	reNotCopper = regexp.MustCompile(`(?i)mask|silk|paste|legend|overlay|assy|assembly|courtyard|stencil|fab|adhes|drl_map|drill_map|map\.|\.g[tb][osp]$|\.gb?a$`)
	nameRules   = []struct {
		kind string
		rx   *regexp.Regexp
	}{
		{lyTopMask, regexp.MustCompile(`(?i)F[._]Mask|\.gts$|topsoldermask|top[ _-]?(solder)?[ _-]?mask|mask[ _-]?top|\.stc$|\.tsm$`)},
		{lyBotMask, regexp.MustCompile(`(?i)B[._]Mask|\.gbs$|bottomsoldermask|bottom[ _-]?(solder)?[ _-]?mask|mask[ _-]?bottom|\.sts$|\.bsm$`)},
		{lyTop, regexp.MustCompile(`(?i)F[._]Cu|\.gtl$|toplayer|\.cmp$|top[ _-]?copper|copper[ _-]?top|\.top$|[_-]top\.(gbr|ger|art)$`)},
		{lyBottom, regexp.MustCompile(`(?i)B[._]Cu|\.gbl$|bottomlayer|\.sol$|bottom[ _-]?copper|copper[ _-]?bottom|\.bot$|[_-]bottom\.(gbr|ger|art)$`)},
		{lyOutline, regexp.MustCompile(`(?i)Edge[._]Cuts|\.gko$|\.gm1$|\.gml$|boardoutline|outline|profile|\.dim$|mechanical ?1`)},
	}
	skipExt = map[string]bool{".png": true, ".jpg": true, ".svg": true, ".pdf": true, ".tap": true, ".nc": true,
		".ngc": true, ".gcode": true, ".csv": true, ".md": true, ".zip": true, ".log": true, ".json": true,
		".html": true, ".txt": true, ".kicad_pcb": true, ".kicad_pro": true, ".kicad_sch": true, ".exe": true,
		".ini": true, ".command": true, ".rar": true, ".7z": true}
)

// classify: the layer of a file, "" when it is none of ours.
func classify(name, data string) string {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	if reDrillHead.MatchString(head) && !strings.Contains(head, "%FS") {
		return lyDrill
	}
	if !strings.Contains(head, "%FS") && !strings.Contains(head, "G04") {
		return ""
	}
	if m := reFunc.FindStringSubmatch(head); m != nil {
		f := strings.ToLower(m[1])
		switch {
		case strings.HasPrefix(f, "copper"):
			if strings.Contains(f, "bot") {
				return lyBottom
			}
			if strings.Contains(f, "top") {
				return lyTop
			}
			return "" // an inner layer
		case strings.HasPrefix(f, "soldermask"):
			if strings.Contains(f, "bot") {
				return lyBotMask
			}
			return lyTopMask
		case strings.HasPrefix(f, "profile"):
			return lyOutline
		}
		return ""
	}
	base := filepath.Base(name)
	for _, r := range nameRules[:2] { // masks before the rule that throws masks away
		if r.rx.MatchString(base) {
			return r.kind
		}
	}
	if reNotCopper.MatchString(base) {
		return ""
	}
	for _, r := range nameRules[2:] {
		if r.rx.MatchString(base) {
			return r.kind
		}
	}
	return ""
}

// readSources: the files of a zip or of a folder (with its sub-folders).
func readSources(path string) ([]srcFile, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var out []srcFile
	if !st.IsDir() {
		z, err := zip.OpenReader(path)
		if err != nil {
			return nil, fmt.Errorf("%s: не zip (%v)", filepath.Base(path), err)
		}
		defer z.Close()
		for _, f := range z.File {
			if f.FileInfo().IsDir() || skipExt[strings.ToLower(filepath.Ext(f.Name))] || f.UncompressedSize64 > 64<<20 {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			out = append(out, srcFile{f.Name, string(b)})
		}
		return out, nil
	}
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || skipExt[strings.ToLower(filepath.Ext(p))] || fi.Size() > 64<<20 ||
			strings.HasPrefix(fi.Name(), ".") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err == nil {
			rel, _ := filepath.Rel(path, p)
			out = append(out, srcFile{rel, string(b)})
		}
		return nil
	})
	return out, err
}

func prefixOf(name string) string {
	base := filepath.Base(name)
	return strings.ToLower(regexp.MustCompile(`[-_.]`).Split(base, 2)[0])
}

// loadBoard reads every layer we use from a zip or a folder.
func loadBoard(path string) (*board, error) {
	srcs, err := readSources(path)
	if err != nil {
		return nil, err
	}
	byKind := map[string][]srcFile{}
	for _, f := range srcs {
		if k := classify(f.name, f.data); k != "" {
			byKind[k] = append(byKind[k], f)
		}
	}
	// several boards in one folder: keep the name prefix with the most layers
	pre := map[string]int{}
	for k, fs := range byKind {
		if k == lyDrill {
			continue
		}
		seen := map[string]bool{}
		for _, f := range fs {
			if p := prefixOf(f.name); !seen[p] {
				seen[p] = true
				pre[p]++
			}
		}
	}
	if len(pre) > 1 {
		dup := false
		for k, fs := range byKind {
			if k != lyDrill && len(fs) > 1 {
				dup = true
			}
		}
		if dup {
			var ps []string
			for p := range pre {
				ps = append(ps, p)
			}
			sort.Strings(ps)
			best := ps[0]
			for _, p := range ps {
				if pre[p] > pre[best] {
					best = p
				}
			}
			for k, fs := range byKind {
				var keep []srcFile
				for _, f := range fs {
					if prefixOf(f.name) == best {
						keep = append(keep, f)
					}
				}
				if len(keep) > 0 || k != lyDrill { // drills named otherwise (EasyEDA) stay
					byKind[k] = keep
				}
			}
		}
	}
	for k := range byKind {
		sort.Slice(byKind[k], func(i, j int) bool { return byKind[k][i].name < byKind[k][j].name })
	}
	b := &board{files: map[string][]string{}, copper: map[string]*gerber{}, mask: map[string]*gerber{}}
	base := filepath.Base(path)
	b.name = strings.TrimSuffix(base, filepath.Ext(base))
	for _, k := range []string{lyTop, lyBottom, lyTopMask, lyBotMask, lyOutline} {
		if len(byKind[k]) == 0 {
			continue
		}
		f := byKind[k][0]
		g, err := parseGerber(f.data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", f.name, err)
		}
		b.files[k] = []string{f.name}
		if _, ok := g.bounds(); !ok && k != lyOutline {
			b.notes = append(b.notes, fmt.Sprintf("%s пустой", filepath.Base(f.name)))
			continue
		}
		switch k {
		case lyTop, lyBottom:
			b.copper[k] = g
		case lyTopMask, lyBotMask:
			b.mask[k] = g
		case lyOutline:
			b.outline, b.closed = chainLines(g.lines, 0.02)
		}
	}
	for _, f := range byKind[lyDrill] {
		hs, err := parseExcellon(f.data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", f.name, err)
		}
		b.files[lyDrill] = append(b.files[lyDrill], f.name)
		b.holes = append(b.holes, hs...)
	}
	if len(b.copper) == 0 {
		return nil, fmt.Errorf("в %s нет слоёв меди (F_Cu, .gtl, TopLayer...)", base)
	}
	cu := rect{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, g := range b.copper {
		if r, ok := g.bounds(); ok {
			cu = cu.union(r)
		}
	}
	if len(b.outline) > 0 {
		b.box = linesBox(b.outline)
		if !b.closed {
			b.notes = append(b.notes, "контур платы не замкнут: V-рез пойдёт по линиям как есть")
		}
	} else {
		b.box = rect{cu.X0 - 1, cu.X1 + 1, cu.Y0 - 1, cu.Y1 + 1}
		b.outline = [][]pt{{{b.box.X0, b.box.Y0}, {b.box.X1, b.box.Y0}, {b.box.X1, b.box.Y1}, {b.box.X0, b.box.Y1}, {b.box.X0, b.box.Y0}}}
		b.closed = true
		b.notes = append(b.notes, "нет слоя контура: беру габарит меди + 1 мм")
	}
	return b, nil
}

func linesBox(ls [][]pt) rect {
	r := rect{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	for _, l := range ls {
		for _, p := range l {
			r.X0, r.X1 = math.Min(r.X0, p.X), math.Max(r.X1, p.X)
			r.Y0, r.Y1 = math.Min(r.Y0, p.Y), math.Max(r.Y1, p.Y)
		}
	}
	return r
}

// chainLines joins strokes whose ends meet (within tol) into lines; true when
// all of them came out closed.
func chainLines(ls [][]pt, tol float64) ([][]pt, bool) {
	type end struct{ X, Y int64 }
	key := func(p pt) end { return end{int64(math.Round(p.X / tol)), int64(math.Round(p.Y / tol))} }
	near := func(p pt) []end { // the cell and its neighbours: ends that round apart still meet
		k := key(p)
		var out []end
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				out = append(out, end{k.X + dx, k.Y + dy})
			}
		}
		return out
	}
	var segs [][]pt
	for _, l := range ls {
		if len(l) >= 2 && math.Hypot(l[len(l)-1].X-l[0].X, l[len(l)-1].Y-l[0].Y)+pathLen(l) > tol {
			segs = append(segs, l)
		}
	}
	at := map[end][]int{}
	for i, s := range segs {
		at[key(s[0])] = append(at[key(s[0])], i)
		at[key(s[len(s)-1])] = append(at[key(s[len(s)-1])], i)
	}
	used := make([]bool, len(segs))
	take := func(p pt) ([]pt, bool) { // an unused segment starting or ending at p, turned to start there
		for _, k := range near(p) {
			for _, i := range at[k] {
				if used[i] {
					continue
				}
				s := segs[i]
				if math.Hypot(s[0].X-p.X, s[0].Y-p.Y) <= tol*1.5 {
					used[i] = true
					return s, true
				}
				if e := s[len(s)-1]; math.Hypot(e.X-p.X, e.Y-p.Y) <= tol*1.5 {
					used[i] = true
					r := make([]pt, len(s))
					for j := range s {
						r[j] = s[len(s)-1-j]
					}
					return r, true
				}
			}
		}
		return nil, false
	}
	isClosed := func(l []pt) bool {
		return len(l) > 2 && math.Hypot(l[0].X-l[len(l)-1].X, l[0].Y-l[len(l)-1].Y) <= tol*1.5
	}
	var out [][]pt
	all := true
	for i, s := range segs {
		if used[i] {
			continue
		}
		used[i] = true
		line := append([]pt(nil), s...)
		for !isClosed(line) { // forward
			n, ok := take(line[len(line)-1])
			if !ok {
				break
			}
			line = append(line, n[1:]...)
		}
		for !isClosed(line) { // and backward
			n, ok := take(line[0])
			if !ok {
				break
			}
			rev := make([]pt, 0, len(n)+len(line))
			for j := len(n) - 1; j >= 1; j-- {
				rev = append(rev, n[j])
			}
			line = append(rev, line...)
		}
		if isClosed(line) {
			line[len(line)-1] = line[0]
		} else {
			all = false
		}
		out = append(out, line)
	}
	return out, all
}

func pathLen(ps []pt) float64 {
	l := 0.0
	for i := 1; i < len(ps); i++ {
		l += math.Hypot(ps[i].X-ps[i-1].X, ps[i].Y-ps[i-1].Y)
	}
	return l
}

// boards: zips and folders next to the exe that have copper in them.
func boardSources(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		p := filepath.Join(dir, n)
		if e.IsDir() || strings.EqualFold(filepath.Ext(n), ".zip") {
			if hasCopper(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func hasCopper(p string) bool {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		ents, _ := os.ReadDir(p)
		n := 0
		for _, e := range ents { // only a look at the top of the folder: a quick list
			if e.IsDir() || skipExt[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			if n++; n > 200 {
				break
			}
			f, err := os.Open(filepath.Join(p, e.Name()))
			if err != nil {
				continue
			}
			buf := make([]byte, 8192)
			k, _ := io.ReadFull(f, buf)
			f.Close()
			if c := classify(e.Name(), string(buf[:k])); c == lyTop || c == lyBottom {
				return true
			}
		}
		return false
	}
	z, err := zip.OpenReader(p)
	if err != nil {
		return false
	}
	defer z.Close()
	for _, f := range z.File {
		if f.FileInfo().IsDir() || skipExt[strings.ToLower(filepath.Ext(f.Name))] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		buf := make([]byte, 8192)
		k, _ := io.ReadFull(rc, buf)
		rc.Close()
		if c := classify(f.Name, string(buf[:k])); c == lyTop || c == lyBottom {
			return true
		}
	}
	return false
}
