package gfx

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// ==========================
// ===  Area rasterizer   ===
// ==========================

// raster accumulates signed area per pixel from line segments; a running
// sum along each row then gives exact coverage (the font-rs scheme), so
// antialiasing needs no supersampling.
type raster struct {
	bx, by int // buffer origin in canvas pixels
	w, h   int
	stride int // w+2: segments clamped to the right edge spill two cells
	acc    []float32
}

var rasterPool = sync.Pool{New: func() interface{} { return new(raster) }}

func getRaster(b image.Rectangle) *raster {
	r := rasterPool.Get().(*raster)
	r.reset(b)
	return r
}

func putRaster(r *raster) {
	if cap(r.acc) > 1<<22 {
		r.acc = nil // do not pin huge buffers after a full-screen fill
	}
	rasterPool.Put(r)
}

func (r *raster) reset(b image.Rectangle) {
	r.bx, r.by = b.Min.X, b.Min.Y
	r.w, r.h = b.Dx(), b.Dy()
	if r.w < 0 {
		r.w = 0
	}
	if r.h < 0 {
		r.h = 0
	}
	r.stride = r.w + 2
	n := r.stride * r.h
	if cap(r.acc) < n {
		r.acc = make([]float32, n)
		return
	}
	r.acc = r.acc[:n]
	for i := range r.acc {
		r.acc[i] = 0
	}
}

// polygon adds a closed polygon in canvas coordinates.
func (r *raster) polygon(pts []Pt) {
	n := len(pts)
	if n < 2 {
		return
	}
	for i := 0; i < n; i++ {
		a, b := pts[i], pts[(i+1)%n]
		r.line(a.X, a.Y, b.X, b.Y)
	}
}

// line adds one edge in canvas coordinates. Parts above or below the
// buffer are dropped (rows are independent); parts left or right are
// squashed onto the border, which keeps the winding of visible pixels.
func (r *raster) line(x0, y0, x1, y1 float64) {
	x0 -= float64(r.bx)
	x1 -= float64(r.bx)
	y0 -= float64(r.by)
	y1 -= float64(r.by)
	if y0 == y1 || math.IsNaN(x0+x1+y0+y1) || math.IsInf(x0+x1+y0+y1, 0) {
		return
	}
	dir := 1.0
	if y0 > y1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
		dir = -1
	}
	H, W := float64(r.h), float64(r.w)
	if y1 <= 0 || y0 >= H {
		return
	}
	dxdy := (x1 - x0) / (y1 - y0)
	if y0 < 0 {
		x0 -= y0 * dxdy
		y0 = 0
	}
	if y1 > H {
		x1 = x0 + (H-y0)*dxdy
		y1 = H
	}
	// Split at the vertical borders so each piece is fully inside or outside.
	var ys [4]float64
	var xs [4]float64
	n := 0
	xs[n], ys[n] = x0, y0
	n++
	for _, bx := range [2]float64{0, W} {
		if (x0 < bx && x1 > bx) || (x0 > bx && x1 < bx) {
			t := (bx - x0) / (x1 - x0)
			xs[n], ys[n] = bx, y0+(y1-y0)*t
			n++
		}
	}
	xs[n], ys[n] = x1, y1
	n++
	if n == 4 && ys[1] > ys[2] {
		xs[1], xs[2] = xs[2], xs[1]
		ys[1], ys[2] = ys[2], ys[1]
	}
	for i := 0; i+1 < n; i++ {
		ya, yb := ys[i], ys[i+1]
		if yb <= ya {
			continue
		}
		xa, xb := xs[i], xs[i+1]
		mid := (xa + xb) / 2
		if mid <= 0 {
			xa, xb = 0, 0
		} else if mid >= W {
			xa, xb = W, W
		}
		r.edge(clampF(xa, 0, W), ya, clampF(xb, 0, W), yb, dir)
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// edge accumulates a segment with y0 < y1 and x inside [0, w].
func (r *raster) edge(x0, y0, x1, y1, dir float64) {
	dxdy := (x1 - x0) / (y1 - y0)
	W := float64(r.w)
	x := x0
	yi := int(y0)
	yend := int(math.Ceil(y1))
	if yend > r.h {
		yend = r.h
	}
	for ; yi < yend; yi++ {
		row := r.acc[yi*r.stride : (yi+1)*r.stride]
		top := float64(yi)
		if y0 > top {
			top = y0
		}
		bot := float64(yi + 1)
		if y1 < bot {
			bot = y1
		}
		dy := bot - top
		xnext := clampF(x+dxdy*dy, 0, W)
		d := dy * dir
		xa, xb := x, xnext
		if xa > xb {
			xa, xb = xb, xa
		}
		xaf := math.Floor(xa)
		xai := int(xaf)
		xbc := math.Ceil(xb)
		xbi := int(xbc)
		if xbi <= xai+1 {
			// Whole crossing inside one cell: split by the mean x.
			xmf := 0.5*(x+xnext) - xaf
			row[xai] += float32(d - d*xmf)
			row[xai+1] += float32(d * xmf)
		} else {
			s := 1 / (xb - xa)
			x0f := xa - xaf
			a0 := 0.5 * s * (1 - x0f) * (1 - x0f)
			x1f := xb - xbc + 1
			am := 0.5 * s * x1f * x1f
			row[xai] += float32(d * a0)
			if xbi == xai+2 {
				row[xai+1] += float32(d * (1 - a0 - am))
			} else {
				a1 := s * (1.5 - x0f)
				row[xai+1] += float32(d * (a1 - a0))
				for xi := xai + 2; xi < xbi-1; xi++ {
					row[xi] += float32(d * s)
				}
				a2 := a1 + float64(xbi-xai-3)*s
				row[xbi-1] += float32(d * (1 - a2 - am))
			}
			row[xbi] += float32(d * am)
		}
		x = xnext
	}
}

// ==========================
// ===  Coverage output   ===
// ==========================

// paint blends col onto the canvas image using the accumulated coverage.
func (r *raster) paint(img *image.RGBA, col color.NRGBA) {
	if col.A == 0 {
		return
	}
	mask := make([]uint8, r.w)
	for y := 0; y < r.h; y++ {
		row := r.acc[y*r.stride : y*r.stride+r.w]
		var s float32
		for x, v := range row {
			s += v
			c := s
			if c < 0 {
				c = -c
			}
			if c > 1 {
				c = 1
			}
			mask[x] = uint8(c*255 + 0.5)
		}
		off := img.PixOffset(r.bx, r.by+y)
		blendMask(img.Pix[off:off+r.w*4], mask, col, uint32(col.A))
	}
}

// alpha converts coverage to bytes through lut (index = coverage*255).
func (r *raster) alpha(lut *[256]uint8) []uint8 {
	out := make([]uint8, r.w*r.h)
	for y := 0; y < r.h; y++ {
		row := r.acc[y*r.stride : y*r.stride+r.w]
		var s float32
		for x, v := range row {
			s += v
			c := s
			if c < 0 {
				c = -c
			}
			if c > 1 {
				c = 1
			}
			out[y*r.w+x] = lut[int(c*255+0.5)]
		}
	}
	return out
}

// coverage returns the raw coverage of one buffer pixel (for tests).
func (r *raster) coverage(x, y int) float64 {
	var s float32
	row := r.acc[y*r.stride:]
	for i := 0; i <= x; i++ {
		s += row[i]
	}
	if s < 0 {
		s = -s
	}
	if s > 1 {
		s = 1
	}
	return float64(s)
}

var linearLUT = func() *[256]uint8 {
	var l [256]uint8
	for i := range l {
		l[i] = uint8(i)
	}
	return &l
}()
