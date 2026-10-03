package gfx

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// ==========================
// ===  Byte reading      ===
// ==========================

// u16/u32 read big-endian values; callers check bounds beforehand.
func u16(b []byte, o int) uint16 { return uint16(b[o])<<8 | uint16(b[o+1]) }
func u32(b []byte, o int) uint32 {
	return uint32(b[o])<<24 | uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
}

// reader is a bounds-checked cursor; overruns set bad and yield zeros,
// so parsers check once at the end instead of after every field.
type reader struct {
	b   []byte
	p   int
	bad bool
}

func (r *reader) need(n int) bool {
	if r.bad || n < 0 || r.p < 0 || r.p+n > len(r.b) {
		r.bad = true
		return false
	}
	return true
}

func (r *reader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.p]
	r.p++
	return v
}

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := u16(r.b, r.p)
	r.p += 2
	return v
}

func (r *reader) i16() int16 { return int16(r.u16()) }

func (r *reader) skip(n int) {
	if r.need(n) {
		r.p += n
	}
}

// ==========================
// ===  Font              ===
// ==========================

// Font is a parsed TrueType font (glyf outlines).
type Font struct {
	data       []byte
	unitsPerEm float64
	ascent     float64 // font units
	descent    float64 // font units, negative below baseline
	lineGap    float64
	numGlyphs  int
	locLong    bool
	loca       []byte
	glyf       []byte
	hmtx       []byte
	numHMet    int
	cmap       []byte // the chosen subtable
	cmapFmt    int
	kern       map[uint32]int16
	xHeight    float64 // font units, 0 if unknown
	capHeight  float64

	mu     sync.Mutex
	runes  map[rune]uint16
	shapes map[uint16]*outline
	faces  map[int64]*Face
}

var errFont = errors.New("gfx: malformed font")

// LoadFont reads and parses a font file.
func LoadFont(path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := ParseFont(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// ParseFont parses TTF/OTF (glyf outlines) data; for a .ttc collection
// the first font is used.
func ParseFont(data []byte) (*Font, error) {
	return ParseFontIndex(data, 0)
}

// ParseFontIndex parses font number idx of a collection (0 for plain files).
func ParseFontIndex(data []byte, idx int) (f *Font, err error) {
	defer func() {
		// Last line of defence: a parser bug must not take the app down.
		if recover() != nil {
			f, err = nil, errFont
		}
	}()
	if len(data) < 12 {
		return nil, errFont
	}
	base := 0
	switch u32(data, 0) {
	case 0x74746366: // "ttcf"
		if len(data) < 12 {
			return nil, errFont
		}
		n := int(u32(data, 8))
		if idx < 0 || idx >= n || 12+4*idx+4 > len(data) {
			return nil, fmt.Errorf("gfx: no font %d in collection", idx)
		}
		base = int(u32(data, 12+4*idx))
	case 0x00010000, 0x74727565: // TrueType, "true"
	case 0x4F54544F: // "OTTO"
		return nil, errors.New("gfx: CFF outlines are not supported")
	default:
		return nil, errFont
	}
	tables, err := tableDir(data, base)
	if err != nil {
		return nil, err
	}
	f = &Font{data: data, runes: map[rune]uint16{}, shapes: map[uint16]*outline{}, faces: map[int64]*Face{}}
	for _, name := range []string{"head", "hhea", "maxp", "cmap", "hmtx", "loca", "glyf"} {
		if tables[name] == nil {
			if name == "glyf" && tables["CFF "] != nil {
				return nil, errors.New("gfx: CFF outlines are not supported")
			}
			return nil, fmt.Errorf("gfx: font has no %s table", name)
		}
	}
	head := tables["head"]
	if len(head) < 54 {
		return nil, errFont
	}
	f.unitsPerEm = float64(u16(head, 18))
	if f.unitsPerEm < 16 || f.unitsPerEm > 16384 {
		return nil, errFont
	}
	f.locLong = u16(head, 50) != 0
	hhea := tables["hhea"]
	if len(hhea) < 36 {
		return nil, errFont
	}
	f.ascent = float64(int16(u16(hhea, 4)))
	f.descent = float64(int16(u16(hhea, 6)))
	f.lineGap = float64(int16(u16(hhea, 8)))
	f.numHMet = int(u16(hhea, 34))
	if os2 := tables["OS/2"]; len(os2) >= 78 && f.ascent == 0 && f.descent == 0 {
		// Some fonts leave hhea empty and rely on the Windows metrics.
		f.ascent = float64(u16(os2, 74))
		f.descent = -float64(u16(os2, 76))
	}
	maxp := tables["maxp"]
	if len(maxp) < 6 {
		return nil, errFont
	}
	f.numGlyphs = int(u16(maxp, 4))
	f.hmtx = tables["hmtx"]
	if f.numHMet < 1 || f.numHMet > f.numGlyphs || len(f.hmtx) < 4*f.numHMet {
		return nil, errFont
	}
	f.loca, f.glyf = tables["loca"], tables["glyf"]
	per := 2
	if f.locLong {
		per = 4
	}
	if len(f.loca) < per*(f.numGlyphs+1) {
		return nil, errFont
	}
	if err := f.pickCmap(tables["cmap"]); err != nil {
		return nil, err
	}
	f.kern = parseKern(tables["kern"])
	f.xHeight = f.glyphTop('x')
	f.capHeight = f.glyphTop('H')
	return f, nil
}

// tableDir maps table tags to their (bounds-checked) bytes.
func tableDir(data []byte, base int) (map[string][]byte, error) {
	if base < 0 || base+12 > len(data) {
		return nil, errFont
	}
	n := int(u16(data, base+4))
	if base+12+16*n > len(data) {
		return nil, errFont
	}
	t := map[string][]byte{}
	for i := 0; i < n; i++ {
		o := base + 12 + 16*i
		off, ln := int64(u32(data, o+8)), int64(u32(data, o+12))
		if off+ln > int64(len(data)) {
			return nil, errFont
		}
		t[string(data[o:o+4])] = data[off : off+ln]
	}
	return t, nil
}

// ==========================
// ===  Character map     ===
// ==========================

// pickCmap prefers full Unicode (3,10), then BMP (3,1), then platform 0.
func (f *Font) pickCmap(cm []byte) error {
	if len(cm) < 4 {
		return errFont
	}
	n := int(u16(cm, 2))
	if 4+8*n > len(cm) {
		return errFont
	}
	best, bestScore := -1, 0
	for i := 0; i < n; i++ {
		o := 4 + 8*i
		pid, eid, off := u16(cm, o), u16(cm, o+2), int(u32(cm, o+4))
		if off < 0 || off+2 > len(cm) {
			continue
		}
		format := u16(cm, off)
		if format != 4 && format != 12 {
			continue
		}
		score := 0
		switch {
		case pid == 3 && eid == 10:
			score = 4
		case pid == 3 && eid == 1:
			score = 3
		case pid == 0:
			score = 2
		}
		if score > bestScore {
			best, bestScore = off, score
		}
	}
	if best < 0 {
		return errors.New("gfx: font has no Unicode cmap")
	}
	sub := cm[best:]
	f.cmapFmt = int(u16(sub, 0))
	switch f.cmapFmt {
	case 4:
		if len(sub) < 14 {
			return errFont
		}
		ln := int(u16(sub, 2))
		segX2 := int(u16(sub, 6))
		if ln < 16+4*segX2 || ln > len(sub) {
			// Many fonts lie about the length; trust the data if it fits.
			ln = len(sub)
		}
		if 16+4*segX2 > ln || segX2%2 != 0 {
			return errFont
		}
		f.cmap = sub[:ln]
	case 12:
		if len(sub) < 16 {
			return errFont
		}
		n := int64(u32(sub, 12))
		if 16+12*n > int64(len(sub)) {
			return errFont
		}
		f.cmap = sub[:16+12*n]
	}
	return nil
}

// GlyphIndex maps a rune to a glyph id; 0 means missing.
func (f *Font) GlyphIndex(r rune) uint16 {
	f.mu.Lock()
	g, ok := f.runes[r]
	f.mu.Unlock()
	if ok {
		return g
	}
	g = f.lookup(r)
	if g >= uint16(f.numGlyphs) {
		g = 0
	}
	f.mu.Lock()
	f.runes[r] = g
	f.mu.Unlock()
	return g
}

func (f *Font) lookup(r rune) uint16 {
	if r < 0 {
		return 0
	}
	c := f.cmap
	if f.cmapFmt == 12 {
		lo, hi := 0, int(u32(c, 12))
		for lo < hi {
			m := (lo + hi) / 2
			o := 16 + 12*m
			start, end := rune(u32(c, o)), rune(u32(c, o+4))
			switch {
			case r < start:
				hi = m
			case r > end:
				lo = m + 1
			default:
				return uint16(u32(c, o+8) + uint32(r-start))
			}
		}
		return 0
	}
	if r > 0xFFFF {
		return 0
	}
	segs := int(u16(c, 6)) / 2
	ends, starts := 14, 16+2*segs
	deltas, ranges := starts+2*segs, starts+4*segs
	lo, hi := 0, segs
	for lo < hi {
		m := (lo + hi) / 2
		if rune(u16(c, ends+2*m)) < r {
			lo = m + 1
		} else {
			hi = m
		}
	}
	if lo >= segs {
		return 0
	}
	start := rune(u16(c, starts+2*lo))
	if r < start {
		return 0
	}
	delta := u16(c, deltas+2*lo)
	ro := int(u16(c, ranges+2*lo))
	if ro == 0 {
		return uint16(r) + delta
	}
	o := ranges + 2*lo + ro + 2*int(r-start)
	if o+2 > len(c) {
		return 0
	}
	g := u16(c, o)
	if g == 0 {
		return 0
	}
	return g + delta
}

// ==========================
// ===  Metrics           ===
// ==========================

// advance returns the horizontal advance of a glyph in font units.
func (f *Font) advance(g uint16) float64 {
	i := int(g)
	if i >= f.numHMet {
		i = f.numHMet - 1
	}
	return float64(u16(f.hmtx, 4*i))
}

// kerning returns the pair adjustment in font units.
func (f *Font) kerning(a, b uint16) float64 {
	if f.kern == nil {
		return 0
	}
	return float64(f.kern[uint32(a)<<16|uint32(b)])
}

// parseKern reads horizontal format-0 subtables of the classic kern table.
func parseKern(k []byte) map[uint32]int16 {
	r := &reader{b: k}
	if r.u16() != 0 {
		return nil // Apple's version-1 layout is not used by Windows fonts
	}
	n := int(r.u16())
	out := map[uint32]int16{}
	for i := 0; i < n && !r.bad; i++ {
		start := r.p
		r.skip(2)
		ln := int(r.u16())
		cov := r.u16()
		if cov&0x1 != 0 && cov&0x4 == 0 && cov>>8 == 0 {
			np := int(r.u16())
			r.skip(6)
			for j := 0; j < np && !r.bad; j++ {
				a, b, v := r.u16(), r.u16(), r.i16()
				if !r.bad {
					out[uint32(a)<<16|uint32(b)] = v
				}
			}
		}
		r.p = start + ln
		if ln < 6 {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// glyphTop measures the flat top of a reference letter; the outline is
// more trustworthy than OS/2 heights, which many fonts leave at zero.
func (f *Font) glyphTop(r rune) float64 {
	o := f.outline(f.GlyphIndex(r))
	if o == nil {
		return 0
	}
	top := 0.0
	for _, p := range o.pts {
		if p.on && float64(p.y) > top {
			top = float64(p.y)
		}
	}
	return top
}
