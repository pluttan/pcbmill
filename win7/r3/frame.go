package r3

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
)

// ==========================
// ===  Frame             ===
// ==========================

// Frame is a colour image plus a depth buffer. Z holds a value that grows
// towards the camera (1/depth in perspective, -depth in ortho); -Inf is empty.
// All scratch memory lives here so redrawing every frame makes no garbage.
type Frame struct {
	W, H int
	Img  *image.RGBA
	Z    []float32

	verts  []cvert
	back   [][4]float64
	chunks [][]tri
	bins   [][]uint32
	segs   []seg
	sbins  [][]int32
	acc    []uint32 // line coverage: alpha<<24 | rgb, 0 = untouched
	touch  [][]int32
	ds     *image.RGBA
	spec   [specN + 2]float64
	specSh float64
}

// NewFrame allocates a w x h frame.
func NewFrame(w, h int) *Frame {
	f := &Frame{}
	f.Resize(w, h)
	return f
}

// Resize changes the size, reusing memory when it fits; contents are undefined.
func (f *Frame) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if f.Img != nil && f.W == w && f.H == h {
		return
	}
	n := w * h
	f.W, f.H = w, h
	if f.Img != nil && cap(f.Img.Pix) >= 4*n {
		f.Img = &image.RGBA{Pix: f.Img.Pix[:4*n], Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
	} else {
		f.Img = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	if cap(f.Z) >= n {
		f.Z = f.Z[:n]
	} else {
		f.Z = make([]float32, n)
	}
	if cap(f.acc) >= n {
		f.acc = f.acc[:n]
		for i := range f.acc {
			f.acc[i] = 0
		}
	} else {
		f.acc = make([]uint32, n)
	}
}

// Clear paints a vertical gradient and empties the depth buffer.
func (f *Frame) Clear(top, bottom color.NRGBA) {
	inf := float32(math.Inf(-1))
	parallel(f.H, func(y int) {
		t := 0.0
		if f.H > 1 {
			t = float64(y) / float64(f.H-1)
		}
		a := float64(top.A) + (float64(bottom.A)-float64(top.A))*t
		k := a / 255
		r := (float64(top.R) + (float64(bottom.R)-float64(top.R))*t) * k
		g := (float64(top.G) + (float64(bottom.G)-float64(top.G))*t) * k
		b := (float64(top.B) + (float64(bottom.B)-float64(top.B))*t) * k
		row := f.Img.Pix[y*f.Img.Stride : y*f.Img.Stride+4*f.W]
		row[0], row[1], row[2], row[3] = to8(r), to8(g), to8(b), to8(a)
		for n := 4; n < len(row); n *= 2 {
			copy(row[n:], row[:n])
		}
		z := f.Z[y*f.W : (y+1)*f.W]
		for i := range z {
			z[i] = inf
		}
	})
}

// Downsample2 box-filters the frame to half size for a supersampled,
// antialiased result. The returned image is reused by the next call.
func (f *Frame) Downsample2() *image.RGBA {
	w, h := f.W/2, f.H/2
	if w < 1 || h < 1 {
		return f.Img
	}
	if f.ds == nil || f.ds.Rect.Dx() != w || f.ds.Rect.Dy() != h {
		f.ds = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	src, dst := f.Img, f.ds
	parallel(h, func(y int) {
		s0 := src.Pix[2*y*src.Stride:]
		s1 := src.Pix[(2*y+1)*src.Stride:]
		d := dst.Pix[y*dst.Stride:]
		for x := 0; x < w; x++ {
			o := 8 * x
			for c := 0; c < 4; c++ {
				v := uint32(s0[o+c]) + uint32(s0[o+4+c]) + uint32(s1[o+c]) + uint32(s1[o+4+c])
				d[4*x+c] = uint8((v + 2) >> 2)
			}
		}
	})
	return dst
}

// Shade fades covered pixels towards fog with distance: the farthest
// drawn surface gets `amount` of fog, the nearest none.
func (f *Frame) Shade(fog color.NRGBA, amount float64) {
	if !(amount > 0) {
		return
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, z := range f.Z {
		if z > -math.MaxFloat32 {
			if z < lo {
				lo = z
			}
			if z > hi {
				hi = z
			}
		}
	}
	if !(hi > lo) {
		return
	}
	amount = clampf(amount, 0, 1)
	span := float64(hi - lo)
	fr, fg, fb := float64(fog.R), float64(fog.G), float64(fog.B)
	parallel(f.H, func(y int) {
		pix := f.Img.Pix[y*f.Img.Stride:]
		for x := 0; x < f.W; x++ {
			z := f.Z[y*f.W+x]
			if !(z > -math.MaxFloat32) {
				continue
			}
			t := float64(hi-z) / span * amount
			p := pix[4*x : 4*x+3]
			p[0] = to8(float64(p[0]) + (fr-float64(p[0]))*t)
			p[1] = to8(float64(p[1]) + (fg-float64(p[1]))*t)
			p[2] = to8(float64(p[2]) + (fb-float64(p[2]))*t)
		}
	})
}

// ==========================
// ===  Parallel helpers  ===
// ==========================

func workers() int {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	return n
}

// parallel runs fn(0..n-1) on all cores, handing out indices dynamically
// so uneven work (a band full of geometry) does not stall the others.
func parallel(n int, fn func(i int)) {
	nw := mini(workers(), n)
	if nw <= 1 {
		for i := 0; i < n; i++ {
			fn(i)
		}
		return
	}
	var next int32 = -1
	var wg sync.WaitGroup
	wg.Add(nw)
	for k := 0; k < nw; k++ {
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt32(&next, 1))
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}

// bands splits the frame rows into chunks a few times more than the cores.
func (f *Frame) bands() (n, h int) {
	n = mini(workers()*4, f.H)
	if n < 1 {
		n = 1
	}
	h = (f.H + n - 1) / n
	n = (f.H + h - 1) / h
	return n, h
}
