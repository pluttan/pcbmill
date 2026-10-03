package ui

// A window without a screen, for tests and pictures: frames are kept, input
// is fed by the test.

import (
	"image"
	"image/png"
	"os"
	"sync"
)

type Headless struct {
	mu     sync.Mutex
	w, h   int
	scale  float64
	last   *image.RGBA
	frames int
	ev     chan Event
	closed bool
}

func NewHeadless(w, h int) *Headless {
	return &Headless{w: w, h: h, scale: 1, ev: make(chan Event, 4096)}
}

func (d *Headless) Events() <-chan Event { return d.ev }

func (d *Headless) Present(img *image.RGBA) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.last == nil || d.last.Bounds() != img.Bounds() {
		d.last = image.NewRGBA(img.Bounds())
	}
	copy(d.last.Pix, img.Pix)
	d.frames++
}

func (d *Headless) Size() (int, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.w, d.h
}

func (d *Headless) Scale() float64     { return d.scale }
func (d *Headless) SetCursor(Cursor)   {}
func (d *Headless) SetNoActivate(bool) {}
func (d *Headless) SetTitle(string)    {}

func (d *Headless) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.ev)
	}
}

// Send puts input in as if the operator did it.
func (d *Headless) Send(e Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.ev <- e
	}
}

// Resize changes the window size.
func (d *Headless) Resize(w, h int) {
	d.mu.Lock()
	d.w, d.h = w, h
	d.mu.Unlock()
	d.Send(Event{Kind: EvResize, W: w, H: h})
}

// Frames: how many frames were shown so far.
func (d *Headless) Frames() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.frames
}

// Save writes the last frame as PNG.
func (d *Headless) Save(path string) error {
	d.mu.Lock()
	img := d.last
	d.mu.Unlock()
	if img == nil {
		return os.ErrNotExist
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// Last: a copy of the last frame.
func (d *Headless) Last() *image.RGBA {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.last == nil {
		return nil
	}
	out := image.NewRGBA(d.last.Bounds())
	copy(out.Pix, d.last.Pix)
	return out
}
