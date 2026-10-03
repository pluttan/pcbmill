package main

// Excellon drill files: holes and slots in millimetres. KiCad writes decimal
// numbers, EasyEDA and Eagle integers with the format in the header.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// hole: a round hole, or a slot from P to P2 when Slot.
type hole struct {
	P, P2 pt
	D     float64
	Slot  bool
}

func parseExcellon(src string) ([]hole, error) {
	unit := 1.0        // mm per file unit
	intD, decD := 3, 3 // for numbers without a point
	lz := true         // leading zeros kept: the point goes after intD digits
	fmtSet := false
	tools := map[int]float64{}
	var out []hole
	d := 0.0
	var pos pt
	down := false // routing: M15 lowered the tool, G01 cuts a slot
	route := false
	inHeader := false
	num := func(n string) (float64, error) {
		if n == "" {
			return 0, fmt.Errorf("пустое число")
		}
		if strings.Contains(n, ".") {
			v, err := strconv.ParseFloat(n, 64)
			return v * unit, err
		}
		neg := strings.HasPrefix(n, "-")
		n = strings.TrimLeft(n, "+-")
		var v float64
		if lz {
			for len(n) < intD+decD {
				n += "0"
			}
			iv, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return 0, err
			}
			v = float64(iv) / math.Pow(10, float64(len(n)-intD))
		} else {
			iv, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return 0, err
			}
			v = float64(iv) / math.Pow(10, float64(decD))
		}
		if neg {
			v = -v
		}
		return v * unit, nil
	}
	xy := func(s string) (pt, bool, error) {
		p := pos
		got := false
		for _, w := range splitWords(s) {
			if w.l != 'X' && w.l != 'Y' {
				continue
			}
			v, err := num(w.num)
			if err != nil {
				return p, false, fmt.Errorf("сверловка: %q: %v", s, err)
			}
			if w.l == 'X' {
				p.X = v
			} else {
				p.Y = v
			}
			got = true
		}
		return p, got, nil
	}
	for _, raw := range strings.Split(src, "\n") {
		l := strings.ToUpper(strings.TrimSpace(raw))
		if l == "" || l[0] == ';' {
			continue
		}
		switch {
		case l == "M48":
			inHeader = true
			continue
		case l == "%" || l == "M95":
			inHeader = false
			continue
		case l == "M30" || l == "M00":
			return out, nil
		case strings.HasPrefix(l, "METRIC") || strings.HasPrefix(l, "INCH") || strings.HasPrefix(l, "M71") || strings.HasPrefix(l, "M72"):
			inch := strings.HasPrefix(l, "INCH") || l == "M72"
			if inch {
				unit = 25.4
			} else {
				unit = 1
			}
			if !fmtSet {
				if inch {
					intD, decD = 2, 4
				} else {
					intD, decD = 3, 3
				}
			}
			if strings.Contains(l, ",TZ") {
				lz = false
			} else if strings.Contains(l, ",LZ") {
				lz = true
			}
			if k := strings.LastIndexByte(l, ','); k >= 0 && strings.Contains(l[k:], ".") && strings.Contains(l[k:], "0") {
				f := l[k+1:]
				if dot := strings.IndexByte(f, '.'); dot >= 0 {
					intD, decD, fmtSet = dot, len(f)-dot-1, true
				}
			}
			continue
		case strings.HasPrefix(l, "FMAT") || strings.HasPrefix(l, "ICI") || strings.HasPrefix(l, "VER"):
			continue
		}
		if l[0] == 'T' {
			j := 1
			for j < len(l) && l[j] >= '0' && l[j] <= '9' {
				j++
			}
			n, err := strconv.Atoi(l[1:j])
			if err != nil {
				continue
			}
			if c := strings.IndexByte(l, 'C'); c >= 0 { // T1C0.8 or T01F00S00C0.0300
				k := c + 1
				for k < len(l) && (l[k] >= '0' && l[k] <= '9' || l[k] == '.') {
					k++
				}
				v, err := strconv.ParseFloat(l[c+1:k], 64)
				if err == nil {
					tools[n] = v * unit
				}
				if inHeader {
					continue
				}
			}
			d = tools[n]
			continue
		}
		if inHeader {
			continue
		}
		switch {
		case strings.HasPrefix(l, "G00"):
			p, _, err := xy(l[3:])
			if err != nil {
				return nil, err
			}
			pos, route, down = p, true, false
			continue
		case l == "M15":
			down = true
			continue
		case l == "M16" || l == "M17":
			down = false
			continue
		case l == "G05":
			route, down = false, false
			continue
		case strings.HasPrefix(l, "G01") && route:
			p, _, err := xy(l[3:])
			if err != nil {
				return nil, err
			}
			if down {
				out = append(out, hole{P: pos, P2: p, D: d, Slot: true})
			}
			pos = p
			continue
		}
		if k := strings.Index(l, "G85"); k > 0 { // X1Y1G85X2Y2
			a, _, err := xy(l[:k])
			if err != nil {
				return nil, err
			}
			pos = a
			b, _, err := xy(l[k+3:])
			if err != nil {
				return nil, err
			}
			out = append(out, hole{P: a, P2: b, D: d, Slot: true})
			pos = b
			continue
		}
		if l[0] == 'X' || l[0] == 'Y' {
			p, ok, err := xy(l)
			if err != nil {
				return nil, err
			}
			if ok {
				pos = p
				out = append(out, hole{P: p, D: d})
			}
		}
	}
	return out, nil
}
