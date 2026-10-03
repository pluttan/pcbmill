//go:build windows

// Command demo opens a winui window, draws a gradient with a square under the
// mouse and logs every event to winui_demo.log next to the exe.
// F2 toggles no-activate mode, F3 cycles the cursor, F5 activates the window
// two seconds later (time to click another one), F6 moves and resizes it,
// F7 shows a frame smaller than the window (the fill colour around it).
// A middle click leaves no-activate mode and takes the focus.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"time"

	"pcbscan/ui"
	"pcbscan/ui/winui"
)

func main() {
	exe, _ := os.Executable()
	f, err := os.Create(filepath.Join(filepath.Dir(exe), "winui_demo.log"))
	if err != nil {
		winui.Fatal("winui demo", err.Error())
		return
	}
	defer f.Close()
	lg := log.New(f, "", log.Lmicroseconds)

	w, err := winui.Open("winui demo, проверка", 800, 500)
	if err != nil {
		winui.Fatal("winui demo", err.Error())
		return
	}
	w.SetIcon(icon())
	lg.Printf("open scale=%.2f hwnd=%#x", w.Scale(), w.Hwnd())
	x, y, bw, bh := w.Bounds()
	lg.Printf("bounds %d,%d %dx%d", x, y, bw, bh)

	mx, my := 100, 100
	noAct := false
	cur := ui.CursorArrow
	draw := func() {
		cw, ch := w.Size()
		w.Present(frame(cw, ch, mx, my))
	}
	draw()
	for e := range w.Events() {
		lg.Print(describe(e))
		switch e.Kind {
		case ui.EvMouseMove, ui.EvMouseDown:
			mx, my = e.X, e.Y
			draw()
			if e.Kind == ui.EvMouseDown && e.Button == ui.BtnMiddle {
				noAct = false
				w.SetNoActivate(false)
				go func() {
					w.Activate()
					lg.Print("activated by click")
				}()
			}
		case ui.EvResize, ui.EvExpose:
			draw()
		case ui.EvKey:
			switch e.Key {
			case ui.KeyF1 + 1:
				noAct = !noAct
				w.SetNoActivate(noAct)
				w.SetTitle(fmt.Sprintf("winui demo, no-activate=%v", noAct))
			case ui.KeyF1 + 2:
				cur = (cur + 1) % 5
				w.SetCursor(cur)
			case ui.KeyF1 + 4:
				go func() {
					time.Sleep(2 * time.Second)
					w.Activate()
					lg.Print("activated")
				}()
			case ui.KeyF1 + 5:
				w.SetBounds(40, 40, 1000, 700)
				x, y, bw, bh := w.Bounds()
				lg.Printf("bounds %d,%d %dx%d", x, y, bw, bh)
			case ui.KeyF1 + 6:
				cw, ch := w.Size()
				w.Present(frame(cw/2, ch/2, mx/2, my/2))
			}
		case ui.EvClose:
			w.Close()
		}
	}
	lg.Print("events closed, exit")
}

func describe(e ui.Event) string {
	switch e.Kind {
	case ui.EvKey:
		return fmt.Sprintf("key %d mods %d", e.Key, e.Mods)
	case ui.EvChar:
		return fmt.Sprintf("char %q", e.Rune)
	case ui.EvMouseMove:
		return fmt.Sprintf("move %d,%d", e.X, e.Y)
	case ui.EvMouseDown, ui.EvMouseUp:
		return fmt.Sprintf("button kind=%d btn=%d at %d,%d", e.Kind, e.Button, e.X, e.Y)
	case ui.EvWheel:
		return fmt.Sprintf("wheel %.2f at %d,%d", e.Wheel, e.X, e.Y)
	case ui.EvResize:
		return fmt.Sprintf("resize %dx%d", e.W, e.H)
	case ui.EvFocus:
		return fmt.Sprintf("focus %v", e.Focused)
	case ui.EvExpose:
		return "expose"
	case ui.EvClose:
		return "close"
	case ui.EvLeave:
		return "leave"
	}
	return fmt.Sprintf("event %+v", e)
}

func frame(w, h, mx, my int) *image.RGBA {
	if w < 1 || h < 1 {
		w, h = 1, 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			img.Pix[o] = byte(x * 255 / w)
			img.Pix[o+1] = byte(y * 255 / h)
			img.Pix[o+2] = 160
			img.Pix[o+3] = 255
		}
	}
	for y := my - 20; y < my+20; y++ {
		for x := mx - 20; x < mx+20; x++ {
			if image.Pt(x, y).In(img.Rect) {
				img.SetRGBA(x, y, color.RGBA{0xf9, 0xe2, 0xaf, 0xff})
			}
		}
	}
	return img
}

func icon() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := x-32, y-32
			if dx*dx+dy*dy < 30*30 {
				img.SetRGBA(x, y, color.RGBA{0xa6, 0xe3, 0xa1, 0xff})
			}
		}
	}
	return img
}
