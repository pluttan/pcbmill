package gfx

import (
	"image"
	"image/color"
	"math"
)

// ==========================
// ===  Canvas            ===
// ==========================

// Canvas draws into Img, limited to Clip (absolute pixel coordinates).
// Img is premultiplied (image.RGBA convention); all colors in the API
// are straight-alpha NRGBA.
type Canvas struct {
	Img  *image.RGBA
	Clip image.Rectangle
}

// NewCanvas allocates a transparent w x h canvas.
func NewCanvas(w, h int) *Canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	return &Canvas{Img: img, Clip: img.Bounds()}
}

// Sub shares the image and narrows the clip to Clip ∩ r.
func (c *Canvas) Sub(r image.Rectangle) *Canvas {
	return &Canvas{Img: c.Img, Clip: c.clip().Intersect(r)}
}

// clip guards against a Clip that strays outside the image.
func (c *Canvas) clip() image.Rectangle {
	return c.Clip.Intersect(c.Img.Bounds())
}

// Clear overwrites the clip area with col, no blending.
func (c *Canvas) Clear(col color.NRGBA) {
	r := c.clip()
	if r.Empty() {
		return
	}
	a := uint32(col.A)
	px := [4]uint8{uint8(div255(uint32(col.R) * a)), uint8(div255(uint32(col.G) * a)),
		uint8(div255(uint32(col.B) * a)), col.A}
	// Fill one row by doubling copies, then copy that row down.
	first := c.Img.Pix[c.Img.PixOffset(r.Min.X, r.Min.Y):][:r.Dx()*4]
	copy(first, px[:])
	for n := 4; n < len(first); n *= 2 {
		copy(first[n:], first[:n])
	}
	for y := r.Min.Y + 1; y < r.Max.Y; y++ {
		copy(c.Img.Pix[c.Img.PixOffset(r.Min.X, y):][:len(first)], first)
	}
}

// FillRect blends col over the integer rectangle r.
func (c *Canvas) FillRect(r image.Rectangle, col color.NRGBA) {
	r = r.Intersect(c.clip())
	if r.Empty() || col.A == 0 {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := c.Img.PixOffset(r.Min.X, y)
		blendSpan(c.Img.Pix[off:], r.Dx(), col, uint32(col.A))
	}
}

// FillRectV fills r with a vertical gradient from top to bottom.
func (c *Canvas) FillRectV(r image.Rectangle, top, bottom color.NRGBA) {
	full := r
	r = r.Intersect(c.clip())
	if r.Empty() {
		return
	}
	den := float64(full.Dy() - 1)
	if den < 1 {
		den = 1
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		col := Mix(top, bottom, float64(y-full.Min.Y)/den)
		off := c.Img.PixOffset(r.Min.X, y)
		blendSpan(c.Img.Pix[off:], r.Dx(), col, uint32(col.A))
	}
}

// fillRectF fills a fractional rectangle with exact edge coverage;
// coverage is separable because all edges are axis aligned.
func (c *Canvas) fillRectF(x0, y0, x1, y1 float64, col color.NRGBA) {
	if x1 <= x0 || y1 <= y0 || col.A == 0 {
		return
	}
	b := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
	r := b.Intersect(c.clip())
	if r.Empty() {
		return
	}
	cov := func(p int, lo, hi float64) float64 {
		return math.Max(0, math.Min(float64(p+1), hi)-math.Max(float64(p), lo))
	}
	// Columns: one partial pixel at each end, a full run in between.
	lx, rx := r.Min.X, r.Max.X-1
	lc, rc := cov(lx, x0, x1), cov(rx, x0, x1)
	inA, inB := lx, rx+1
	if lc < 1 {
		inA++
	}
	if rc < 1 && rx >= inA {
		inB--
	}
	ca := float64(col.A)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		cy := cov(y, y0, y1) * ca
		pix := c.Img.Pix[c.Img.PixOffset(r.Min.X, y):]
		if lc < 1 {
			blendPix(pix[0:4], col, uint32(lc*cy+0.5))
		}
		if inB > inA {
			blendSpan(pix[(inA-lx)*4:], inB-inA, col, uint32(cy+0.5))
		}
		if rc < 1 && rx > lx {
			blendPix(pix[(rx-lx)*4:], col, uint32(rc*cy+0.5))
		}
	}
}

// ==========================
// ===  Images            ===
// ==========================

// DrawImage blends a premultiplied src with its top-left corner at at.
func (c *Canvas) DrawImage(at image.Point, src *image.RGBA) {
	sb := src.Bounds()
	dst := image.Rectangle{at, at.Add(sb.Size())}
	r := dst.Intersect(c.clip())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := src.PixOffset(sb.Min.X+r.Min.X-at.X, sb.Min.Y+y-at.Y)
		do := c.Img.PixOffset(r.Min.X, y)
		sp := src.Pix[so : so+r.Dx()*4]
		dp := c.Img.Pix[do : do+r.Dx()*4]
		for i := 0; i < len(sp); i += 4 {
			sa := sp[i+3]
			if sa == 255 {
				copy(dp[i:i+4], sp[i:i+4])
			} else if sa != 0 {
				blendPremul(dp[i:i+4], uint32(sp[i]), uint32(sp[i+1]), uint32(sp[i+2]), uint32(sa), 255)
			}
		}
	}
}

// DrawImageScaled stretches src over dst with bilinear filtering.
// Strong minification aliases; prescale such images beforehand.
func (c *Canvas) DrawImageScaled(dst image.Rectangle, src *image.RGBA) {
	sb := src.Bounds()
	if sb.Empty() || dst.Empty() {
		return
	}
	if dst.Size() == sb.Size() {
		c.DrawImage(dst.Min, src)
		return
	}
	r := dst.Intersect(c.clip())
	if r.Empty() {
		return
	}
	sx := float64(sb.Dx()) / float64(dst.Dx())
	sy := float64(sb.Dy()) / float64(dst.Dy())
	maxX, maxY := sb.Dx()-1, sb.Dy()-1
	// Precompute horizontal taps once per call instead of per pixel.
	n := r.Dx()
	x0s := make([]int, n)
	x1s := make([]int, n)
	fxs := make([]uint32, n)
	for i := 0; i < n; i++ {
		fx := (float64(r.Min.X+i-dst.Min.X)+0.5)*sx - 0.5
		x0s[i], x1s[i], fxs[i] = taps(fx, maxX)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		fy := (float64(y-dst.Min.Y)+0.5)*sy - 0.5
		y0, y1, wy := taps(fy, maxY)
		r0 := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y0):]
		r1 := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y1):]
		dp := c.Img.Pix[c.Img.PixOffset(r.Min.X, y):]
		for i := 0; i < n; i++ {
			a, b, wx := x0s[i]*4, x1s[i]*4, fxs[i]
			var px [4]uint32
			for k := 0; k < 4; k++ {
				top := uint32(r0[a+k])*(256-wx) + uint32(r0[b+k])*wx
				bot := uint32(r1[a+k])*(256-wx) + uint32(r1[b+k])*wx
				px[k] = (top*(256-wy) + bot*wy + 1<<15) >> 16
			}
			if px[3] == 0 {
				continue
			}
			p := dp[i*4 : i*4+4]
			if px[3] >= 255 {
				p[0], p[1], p[2], p[3] = uint8(px[0]), uint8(px[1]), uint8(px[2]), 255
				continue
			}
			blendPremul(p, px[0], px[1], px[2], px[3], 255)
		}
	}
}

// taps returns the two neighbour indices and the 8-bit weight of the second.
func taps(f float64, max int) (int, int, uint32) {
	if f <= 0 {
		return 0, 0, 0
	}
	i := int(f)
	if i >= max {
		return max, max, 0
	}
	return i, i + 1, uint32((f-float64(i))*256 + 0.5)
}
