package webui

import (
	"bytes"
	"context"
	"image"
	"sync"
)

// ==========================
// ===  Frame store       ===
// ==========================

// histLen frames of dirty rectangles are kept; a page further behind gets
// the whole frame.
const histLen = 64

type dirtyEntry struct {
	seq  uint64
	rect image.Rectangle
	full bool // size changed: older pictures are useless
}

// frameStore keeps only the newest frame: Present overwrites it in place,
// so frames the page has not fetched yet are dropped for free.
type frameStore struct {
	mu   sync.Mutex
	pix  *image.RGBA
	seq  uint64
	hist [histLen]dirtyEntry
	wake chan struct{} // closed and replaced on every change

	encMu    sync.Mutex // one encoder at a time, owns the buffers below
	enc      pngWriter
	scratch  *image.RGBA
	cacheKey [2]uint64
	cache    encoded
}

// encoded is one PNG ready for a response; data is never modified after.
type encoded struct {
	data         []byte
	seq          uint64
	rect         image.Rectangle
	full         bool
	fullW, fullH int
	encodeTime   float64 // ms, for the latency header
}

func (f *frameStore) init() {
	f.wake = make(chan struct{})
}

func (f *frameStore) wakeAll() {
	f.mu.Lock()
	close(f.wake)
	f.wake = make(chan struct{})
	f.mu.Unlock()
}

// present copies img and records what changed since the previous frame.
func (f *frameStore) present(img *image.RGBA) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	f.mu.Lock()
	var e dirtyEntry
	if f.pix == nil || f.pix.Rect.Dx() != w || f.pix.Rect.Dy() != h {
		f.pix = image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			copy(f.pix.Pix[y*f.pix.Stride:y*f.pix.Stride+w*4], img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):])
		}
		e = dirtyEntry{rect: f.pix.Rect, full: true}
	} else {
		e.rect = diffCopy(f.pix, img)
		if e.rect.Empty() {
			f.mu.Unlock()
			return // same picture: the page already has it
		}
	}
	f.seq++
	e.seq = f.seq
	f.hist[f.seq%histLen] = e
	close(f.wake)
	f.wake = make(chan struct{})
	f.mu.Unlock()
}

// diffCopy copies the rows of src that differ into dst and returns the
// bounding box of the changes in dst coordinates.
func diffCopy(dst, src *image.RGBA) image.Rectangle {
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	sb := src.Bounds()
	rowLen := w * 4
	x0, x1, y0, y1 := w, 0, h, 0
	for y := 0; y < h; y++ {
		d := dst.Pix[y*dst.Stride : y*dst.Stride+rowLen]
		s := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y):]
		s = s[:rowLen]
		if bytes.Equal(d, s) {
			continue
		}
		// skip equal blocks fast, then pin the exact pixel
		lo := 0
		for lo+64 <= rowLen && bytes.Equal(d[lo:lo+64], s[lo:lo+64]) {
			lo += 64
		}
		for d[lo] == s[lo] {
			lo++
		}
		hi := rowLen
		for hi-64 >= lo && bytes.Equal(d[hi-64:hi], s[hi-64:hi]) {
			hi -= 64
		}
		for d[hi-1] == s[hi-1] {
			hi--
		}
		copy(d[lo:hi], s[lo:hi])
		px0, px1 := lo/4, (hi+3)/4
		if px0 < x0 {
			x0 = px0
		}
		if px1 > x1 {
			x1 = px1
		}
		if y < y0 {
			y0 = y
		}
		y1 = y + 1
	}
	if x0 >= x1 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// ==========================
// ===  Long-poll fetch   ===
// ==========================

// waitNewer blocks until a frame newer than seq exists, or extra fires
// (cursor/title change), or ctx ends. True when a frame is ready.
func (f *frameStore) waitNewer(ctx context.Context, seq uint64, extra func() bool) bool {
	for {
		f.mu.Lock()
		ready := f.pix != nil && (f.seq > seq || seq > f.seq)
		wake := f.wake
		f.mu.Unlock()
		if ready {
			return true
		}
		if extra() {
			return false
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return false
		}
	}
}

// fetch returns the newest frame as a PNG of the part that changed since
// the page's frame `since` (the whole frame when unknown).
func (f *frameStore) fetch(since uint64) encoded {
	f.encMu.Lock()
	defer f.encMu.Unlock()

	f.mu.Lock()
	seq := f.seq
	rect, full := f.changedSince(since)
	key := [2]uint64{since, seq}
	if full {
		key[0] = 0
	}
	if f.cache.data != nil && f.cacheKey == key {
		f.mu.Unlock()
		return f.cache
	}
	// crop under the lock so Present cannot tear it, encode outside
	w, h := rect.Dx(), rect.Dy()
	if f.scratch == nil || cap(f.scratch.Pix) < w*h*4 {
		f.scratch = image.NewRGBA(image.Rect(0, 0, w, h))
	} else {
		f.scratch.Pix = f.scratch.Pix[:w*h*4]
		f.scratch.Stride = w * 4
		f.scratch.Rect = image.Rect(0, 0, w, h)
	}
	for y := 0; y < h; y++ {
		o := f.pix.PixOffset(rect.Min.X, rect.Min.Y+y)
		copy(f.scratch.Pix[y*w*4:(y+1)*w*4], f.pix.Pix[o:o+w*4])
	}
	fw, fh := f.pix.Rect.Dx(), f.pix.Rect.Dy()
	f.mu.Unlock()

	t := nowMs()
	var buf bytes.Buffer
	buf.Grow(len(f.cache.data) + 4096)
	f.enc.encode(&buf, f.scratch)
	f.cacheKey = key
	f.cache = encoded{data: buf.Bytes(), seq: seq, rect: rect, full: full, fullW: fw, fullH: fh, encodeTime: nowMs() - t}
	return f.cache
}

// changedSince: union of dirty rectangles after `since`; caller holds mu.
func (f *frameStore) changedSince(since uint64) (image.Rectangle, bool) {
	all := f.pix.Rect
	if since == 0 || since >= f.seq || f.seq-since >= histLen {
		return all, true
	}
	var r image.Rectangle
	for s := since + 1; s <= f.seq; s++ {
		e := f.hist[s%histLen]
		if e.seq != s || e.full {
			return all, true
		}
		r = r.Union(e.rect)
	}
	return r.Intersect(all), r.Eq(all)
}
