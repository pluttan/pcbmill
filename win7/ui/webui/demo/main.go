// Command demo opens a webui window: a gradient, a square that follows the
// mouse and the typed text. Try it with `go run ./ui/webui/demo`.
// -anim repaints the whole window every tick (worst case for the link);
// add ?stats to the page URL to see the frame rate in the browser.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/signal"
	"time"

	"pcbscan/gfx"
	"pcbscan/ui"
	"pcbscan/ui/webui"
)

// ==========================
// ===  State             ===
// ==========================

type demo struct {
	win    *webui.Window
	canvas *gfx.Canvas
	face   *gfx.Face
	faceSz float64

	mx, my  int
	inside  bool
	focused bool
	text    []rune
	lastKey string
	phase   int
	events  int
	shown   int
}

func main() {
	noBrowser := flag.Bool("no-browser", false, "only print the URL")
	anim := flag.Bool("anim", false, "repaint everything every tick")
	fps := flag.Int("fps", 60, "ticks per second with -anim")
	flag.Parse()

	win, url, err := webui.Open("webui demo", !*noBrowser)
	if err != nil && win == nil {
		log.Fatal(err)
	}
	if err != nil {
		log.Print(err)
	}
	fmt.Println("window:", url)

	d := &demo{win: win}
	d.resize(win.Size())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	tick := time.NewTicker(time.Second / time.Duration(*fps))
	defer tick.Stop()
	report := time.NewTicker(time.Second)
	defer report.Stop()

	for {
		select {
		case ev, ok := <-win.Events():
			if !ok {
				return
			}
			d.handle(ev)
			// drain what is already queued, then draw once
			for more := true; more; {
				select {
				case ev, ok := <-win.Events():
					if !ok {
						return
					}
					d.handle(ev)
				default:
					more = false
				}
			}
			d.draw()
		case <-tick.C:
			if *anim {
				d.phase++
				d.draw()
			}
		case <-report.C:
			if *anim {
				fmt.Printf("presented %d frames/s\n", d.shown)
			}
			d.shown = 0
		case <-stop:
			win.Close()
			return
		}
	}
}

// ==========================
// ===  Input             ===
// ==========================

func (d *demo) handle(ev ui.Event) {
	d.events++
	switch ev.Kind {
	case ui.EvResize:
		d.resize(ev.W, ev.H)
	case ui.EvMouseMove, ui.EvMouseDown, ui.EvMouseUp, ui.EvWheel:
		d.mx, d.my, d.inside = ev.X, ev.Y, true
		if ev.Kind == ui.EvMouseDown {
			d.lastKey = fmt.Sprintf("button %d", ev.Button)
		}
		if ev.Kind == ui.EvWheel {
			d.lastKey = fmt.Sprintf("wheel %+.2f", ev.Wheel)
		}
	case ui.EvLeave:
		d.inside = false
	case ui.EvFocus:
		d.focused = ev.Focused
	case ui.EvKey:
		d.lastKey = fmt.Sprintf("key %d mods %d", ev.Key, ev.Mods)
		switch ev.Key {
		case ui.KeyBackspace:
			if len(d.text) > 0 {
				d.text = d.text[:len(d.text)-1]
			}
		case ui.KeyEnter, ui.KeyEsc:
			d.text = d.text[:0]
		}
	case ui.EvChar:
		d.text = append(d.text, ev.Rune)
	}
	d.win.SetTitle("webui demo: " + string(d.text))
}

func (d *demo) resize(w, h int) {
	if d.canvas != nil && d.canvas.Img.Rect.Dx() == w && d.canvas.Img.Rect.Dy() == h {
		return
	}
	d.canvas = gfx.NewCanvas(w, h)
	sz := 18 * d.win.Scale()
	if d.face == nil || d.faceSz != sz {
		d.face, d.faceSz = loadFace(sz), sz
	}
}

func loadFace(px float64) *gfx.Face {
	p := gfx.FindFont("DejaVuSans.ttf", "LiberationSans-Regular.ttf", "Arial.ttf",
		"Helvetica.ttc", "Verdana.ttf", "Tahoma.ttf")
	if p == "" {
		return nil
	}
	f, err := gfx.LoadFont(p)
	if err != nil {
		return nil
	}
	return f.Face(px)
}

// ==========================
// ===  Drawing           ===
// ==========================

func (d *demo) draw() {
	img := d.canvas.Img
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			o := x * 4
			row[o] = uint8(30 + (x+d.phase*4)*90/(w+1)%90)
			row[o+1] = uint8(30 + y*60/(h+1))
			row[o+2] = 46 + uint8(x*y%3)
			row[o+3] = 255
		}
	}
	s := d.win.Scale()
	cursor := ui.CursorArrow
	if d.inside {
		side := int(60 * s)
		r := image.Rect(d.mx-side/2, d.my-side/2, d.mx+side/2, d.my+side/2)
		d.canvas.FillRect(r, gfx.Hex("#f38ba8"))
		cursor = ui.CursorHand // the square sits under the pointer
	}
	d.win.SetCursor(cursor)
	lines := []string{
		"Typed: " + string(d.text) + "_",
		fmt.Sprintf("size %dx%d scale %.2f focus %v events %d", w, h, s, d.focused, d.events),
		"last: " + d.lastKey,
	}
	for i, l := range lines {
		d.text1(int(16*s), int((28+float64(i)*26)*s), l)
	}
	d.win.Present(img)
	d.shown++
}

// text1 writes one line, or a row of blocks when no font was found.
func (d *demo) text1(x, y int, s string) {
	col := gfx.Hex("#cdd6f4")
	if d.face != nil {
		d.canvas.Text(d.face, float64(x), float64(y), s, col)
		return
	}
	cw := int(9 * d.win.Scale())
	for _, r := range s {
		if r != ' ' {
			d.canvas.FillRect(image.Rect(x, y-cw*3/2, x+cw-2, y), color.NRGBA{205, 214, 244, 255})
		}
		x += cw
	}
}
