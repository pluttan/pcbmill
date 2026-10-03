//go:build windows

package winui

import (
	"image"
	"unsafe"

	"pcbscan/ui"
)

// ==========================
// ===  Frames            ===
// ==========================

// Present copies the frame into the back buffer as BGRA (what a 32-bit DIB
// holds) and asks for a repaint; it never waits for the window thread.
func (w *Window) Present(img *image.RGBA) {
	if img == nil {
		return
	}
	b := img.Bounds()
	fw, fh := b.Dx(), b.Dy()
	if fw <= 0 || fh <= 0 {
		return
	}
	w.fmu.Lock()
	need := fw * fh * 4
	if cap(w.frame) < need {
		w.frame = make([]byte, need)
	}
	dst := w.frame[:need]
	for y := 0; y < fh; y++ {
		src := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):]
		row := dst[y*fw*4 : (y+1)*fw*4]
		for i := 0; i < len(row); i += 4 {
			row[i], row[i+1], row[i+2], row[i+3] = src[i+2], src[i+1], src[i], 0xFF
		}
	}
	w.frame, w.fw, w.fh = dst, fw, fh
	w.exposed = false
	w.fmu.Unlock()
	pInvalidateRect.Call(w.hwnd, 0, 0)
}

// paint blits the last frame at 0,0 and fills what it does not cover, so a
// frame of the old size during a resize never leaves garbage.
func (w *Window) paint() {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	var cr rect
	pGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&cr)))

	w.fmu.Lock()
	fw, fh := w.fw, w.fh
	expose := false
	if len(w.frame) > 0 {
		bi := bitmapInfo{Width: int32(fw), Height: -int32(fh), Planes: 1, BitCount: 32}
		bi.Size = uint32(unsafe.Sizeof(bi) - unsafe.Sizeof(bi.Colors))
		pSetDIBitsToDevice.Call(hdc, 0, 0, uintptr(fw), uintptr(fh), 0, 0, 0, uintptr(fh),
			uintptr(unsafe.Pointer(&w.frame[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors)
	} else {
		fw, fh = 0, 0
		expose = !w.exposed
		w.exposed = true
	}
	w.fmu.Unlock()

	if int32(fw) < cr.Right {
		r := rect{int32(fw), 0, cr.Right, cr.Bottom}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), bgBrush)
	}
	if int32(fh) < cr.Bottom {
		r := rect{0, int32(fh), cr.Right, cr.Bottom}
		if int32(fw) < r.Right {
			r.Right = int32(fw)
		}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), bgBrush)
	}
	if expose { // nothing to show yet: ask the program for a frame
		w.post(ui.Event{Kind: ui.EvExpose})
	}
}

// ==========================
// ===  Icon              ===
// ==========================

// SetIcon makes the caption and taskbar icons from img, scaled to the two
// sizes the system asks for.
func (w *Window) SetIcon(img *image.RGBA) {
	if img == nil || img.Bounds().Empty() {
		return
	}
	bx, _, _ := pGetSystemMetrics.Call(smCxIcon)
	by, _, _ := pGetSystemMetrics.Call(smCyIcon)
	sx, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	sy, _, _ := pGetSystemMetrics.Call(smCySmIcon)
	big := makeIcon(img, int(bx), int(by))
	small := makeIcon(img, int(sx), int(sy))
	oldBig, _, _ := pSendMessageW.Call(w.hwnd, wmSetIcon, iconBig, big)
	oldSmall, _, _ := pSendMessageW.Call(w.hwnd, wmSetIcon, iconSmall, small)
	w.fmu.Lock()
	w.bigIcon, w.smIcon = big, small
	w.fmu.Unlock()
	// the previous icons were ours (the class has none): free them
	for _, h := range []uintptr{oldBig, oldSmall} {
		if h != 0 && h != big && h != small {
			pDestroyIcon.Call(h)
		}
	}
}

func (w *Window) dropIcons() {
	w.fmu.Lock()
	big, small := w.bigIcon, w.smIcon
	w.bigIcon, w.smIcon = 0, 0
	w.fmu.Unlock()
	for _, h := range []uintptr{big, small} {
		if h != 0 {
			pDestroyIcon.Call(h)
		}
	}
}

// makeIcon builds an alpha icon: a 32-bit top-down DIB section for colour
// plus an all-zero mask (the alpha channel decides what is transparent).
func makeIcon(img *image.RGBA, iw, ih int) uintptr {
	if iw <= 0 || ih <= 0 {
		iw, ih = 32, 32
	}
	bi := bitmapInfo{Width: int32(iw), Height: -int32(ih), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi) - unsafe.Sizeof(bi.Colors))
	var bits unsafe.Pointer
	color, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), dibRGBColors,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == nil {
		return 0
	}
	defer pDeleteObject.Call(color)
	px := unsafe.Slice((*byte)(bits), iw*ih*4)
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	// box filter: each icon pixel averages the source pixels under it
	for y := 0; y < ih; y++ {
		y0, y1 := b.Min.Y+y*sh/ih, b.Min.Y+(y+1)*sh/ih
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < iw; x++ {
			x0, x1 := b.Min.X+x*sw/iw, b.Min.X+(x+1)*sw/iw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					o := img.PixOffset(xx, yy)
					r += int(img.Pix[o])
					g += int(img.Pix[o+1])
					bl += int(img.Pix[o+2])
					a += int(img.Pix[o+3])
					n++
				}
			}
			o := (y*iw + x) * 4
			if a > 0 { // image.RGBA is premultiplied, icons are not
				px[o], px[o+1], px[o+2] = unmul(bl, a), unmul(g, a), unmul(r, a)
			} else {
				px[o], px[o+1], px[o+2] = 0, 0, 0
			}
			px[o+3] = byte(a / n)
		}
	}
	zeros := make([]byte, (iw+15)/16*2*ih) // rows of a 1-bit bitmap are WORD aligned
	mask, _, _ := pCreateBitmap.Call(uintptr(iw), uintptr(ih), 1, 1, uintptr(unsafe.Pointer(&zeros[0])))
	if mask == 0 {
		return 0
	}
	defer pDeleteObject.Call(mask)
	ii := iconInfo{FIcon: 1, HbmMask: mask, HbmColor: color}
	h, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return h
}

func unmul(c, a int) byte {
	v := c * 255 / a
	if v > 255 {
		v = 255
	}
	return byte(v)
}
