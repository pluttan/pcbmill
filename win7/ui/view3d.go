package ui

// The 3D view: a scene rendered in software, turned with the left mouse
// button, moved with the right one, zoomed with the wheel. The picture is
// cached and drawn again only when the scene or the camera changed; while the
// mouse drags it is drawn plainly, after that once more smoothed (twice the
// pixels, averaged). The pixels of a frame have a budget: a big view (a
// Retina screen) is drawn at half size while it moves and not smoothed at rest.

import (
	"image"
	"math"
	"time"

	"pcbscan/gfx"
	"pcbscan/r3"
)

// Scene is what a 3D view shows.
type Scene interface {
	// Version changes whenever the content changes.
	Version() int
	// Bounds: the box the camera is fitted to.
	Bounds() (min, max r3.Vec)
	// Render draws into the frame; px is the size of one design pixel in
	// frame pixels (line widths, markers).
	Render(f *r3.Frame, cam *r3.Camera, px float64)
	// Overlay draws flat things over the picture (labels, a colour bar);
	// project with cam and the size of r.
	Overlay(c *Ctx, r image.Rectangle, cam *r3.Camera)
}

// ViewHit: a click on the view without dragging.
type ViewHit struct {
	Click bool
	X, Y  float64 // in the view, pixels
	Cam   r3.Camera
	W, H  int
}

type viewState struct {
	cam      r3.Camera
	fitted   bool
	frame    *r3.Frame
	img      *image.RGBA
	ver      int
	lastCam  r3.Camera
	w, h, ss int
	div      int         // a moving frame is drawn this many times smaller
	up       *image.RGBA // the small frame blown back up
	drag     int         // 0 none, 1 orbit, 2 pan
	last     image.Point
	moved    float64
	busyAt   time.Time
	home     func(*r3.Camera, r3.Vec, r3.Vec, int, int)
}

// The pixel budgets of a frame: a moving one, a smoothed one at rest.
const (
	movePixels   = 1200000
	smoothPixels = 6000000
)

// Camera presets.
func homeCam(cam *r3.Camera, min, max r3.Vec, w, h int) {
	cam.Yaw, cam.Pitch, cam.FOV = -math.Pi/2+0.55, 0.62, 0.6
	cam.Fit(min, max, w, h)
}

func topCam(cam *r3.Camera, min, max r3.Vec, w, h int) {
	cam.Yaw, cam.Pitch, cam.FOV = -math.Pi/2, math.Pi/2-0.02, 0.6
	cam.Fit(min, max, w, h)
}

// View3D draws the scene in r and handles the mouse over it.
func (c *Ctx) View3D(id string, r image.Rectangle, sc Scene) ViewHit {
	t := c.T
	vs := c.st.views[id]
	if vs == nil {
		vs = &viewState{frame: r3.NewFrame(1, 1), home: homeCam}
		c.st.views[id] = vs
	}
	w, h := r.Dx(), r.Dy()
	if w < 8 || h < 8 {
		return ViewHit{}
	}
	min, max := sc.Bounds()
	if !vs.fitted {
		vs.home(&vs.cam, min, max, w, h)
		vs.fitted = true
	}
	hit := ViewHit{W: w, H: h}
	e := c.in.ev
	over := c.Over(r)
	// the corner buttons first: a click on them is not a click on the scene
	bs := t.Si(28)
	bar := image.Rect(r.Max.X-t.Si(8)-3*bs-2*t.Si(4), r.Min.Y+t.Si(8), r.Max.X-t.Si(8), r.Min.Y+t.Si(8)+bs)
	overBar := c.Over(bar)
	if over && !overBar {
		if e.Kind == EvMouseDown {
			vs.drag = 1
			if e.Button != BtnLeft || e.Mods&ModShift != 0 { // Shift: a trackpad has no right drag
				vs.drag = 2
			}
			vs.last, vs.moved = image.Pt(e.X, e.Y), 0
			c.st.active = id
		}
		if d := c.Wheel(r); d != 0 {
			vs.cam.Zoom(d, float64(e.X-r.Min.X), float64(e.Y-r.Min.Y), w, h)
			vs.busyAt = time.Now()
		}
	}
	if vs.drag != 0 && c.st.active == id {
		mx, my := c.in.mx, c.in.my
		dx, dy := float64(mx-vs.last.X), float64(my-vs.last.Y)
		if dx != 0 || dy != 0 {
			vs.moved += math.Abs(dx) + math.Abs(dy)
			if vs.drag == 1 {
				vs.cam.Orbit(dx, dy)
			} else {
				vs.cam.Pan(dx, dy, w, h)
			}
			vs.last = image.Pt(mx, my)
			vs.busyAt = time.Now()
		}
		c.cursor = CursorMove
		if e.Kind == EvMouseUp {
			if vs.moved < 4 && vs.drag == 1 && over {
				hit.Click = true
				hit.X, hit.Y = float64(e.X-r.Min.X), float64(e.Y-r.Min.Y)
			}
			vs.drag = 0
		}
	} else if e.Kind == EvMouseUp {
		vs.drag = 0
	}
	// draw: smoothed when nothing moves, plain while it does; a big view
	// moves at half size
	ss, div := 2, 1
	if 4*w*h > smoothPixels {
		ss = 1
	}
	if time.Since(vs.busyAt) < 250*time.Millisecond {
		ss, div = 1, moveDiv(w, h)
		c.animate = true
	}
	ver := sc.Version()
	if vs.img == nil || ver != vs.ver || vs.cam != vs.lastCam || w != vs.w || h != vs.h || ss != vs.ss || div != vs.div {
		if ver != vs.ver && ss == 2 && vs.img != nil {
			ss, div = 1, moveDiv(w, h) // live content (a scan) changes often: keep it quick, smooth when it rests
			vs.busyAt = time.Now()
			c.animate = true
		}
		fw, fh := (w*ss+div-1)/div, (h*ss+div-1)/div
		vs.frame.Resize(fw, fh)
		vs.frame.Clear(t.Base, t.Crust)
		sc.Render(vs.frame, &vs.cam, t.S(1)*float64(ss)/float64(div))
		switch {
		case ss == 2:
			vs.img = vs.frame.Downsample2()
		case div > 1:
			vs.up = upscale(vs.up, vs.frame.Img, w, h, div)
			vs.img = vs.up
		default:
			vs.img = vs.frame.Img
		}
		vs.ver, vs.lastCam, vs.w, vs.h, vs.ss, vs.div = ver, vs.cam, w, h, ss, div
	}
	c.WithClip(r, func() {
		c.C.DrawImage(r.Min, vs.img)
		sc.Overlay(c, r, &vs.cam)
	})
	// the corner buttons
	cells := SplitX(bar, 3, t.Si(4))
	c.C.FillRoundRect(float64(bar.Min.X-t.Si(4)), float64(bar.Min.Y-t.Si(4)), float64(bar.Dx()+t.Si(8)), float64(bar.Dy()+t.Si(8)),
		t.S(9), gfx.Alpha(t.Crust, 0.7))
	if c.IconButton(id+"/fit", cells[0], IconFit, false) {
		vs.cam.Fit(min, max, w, h)
	}
	if c.IconButton(id+"/top", cells[1], IconTop, false) {
		vs.home = topCam
		vs.home(&vs.cam, min, max, w, h)
	}
	if c.IconButton(id+"/3d", cells[2], IconCube, false) {
		vs.home = homeCam
		vs.home(&vs.cam, min, max, w, h)
	}
	if over && vs.drag == 0 && !overBar {
		c.cursor = CursorMove
	}
	hit.Cam = vs.cam
	return hit
}

// RefitView: the next frame fits the camera to the scene again (a new sheet).
func (c *Ctx) RefitView(id string) {
	if vs := c.st.views[id]; vs != nil {
		vs.fitted = false
	}
}

// moveDiv: how many times smaller a moving frame of w x h is drawn to keep
// within movePixels.
func moveDiv(w, h int) int {
	d := 1
	for w*h/(d*d) > movePixels {
		d++
	}
	return d
}

// upscale blows src up div times into a w x h image (reused when it fits):
// each pixel becomes a div x div square.
func upscale(dst, src *image.RGBA, w, h, div int) *image.RGBA {
	if dst == nil || dst.Rect.Dx() != w || dst.Rect.Dy() != h {
		dst = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := 0; y < h; y++ {
		sy := y / div
		if sy >= sh {
			sy = sh - 1
		}
		d := dst.Pix[y*dst.Stride : y*dst.Stride+4*w]
		if y%div != 0 {
			copy(d, dst.Pix[(y-1)*dst.Stride:(y-1)*dst.Stride+4*w])
			continue
		}
		s := src.Pix[sy*src.Stride:]
		for x := 0; x < w; x++ {
			sx := x / div
			if sx >= sw {
				sx = sw - 1
			}
			copy(d[4*x:4*x+4], s[4*sx:4*sx+4])
		}
	}
	return dst
}
