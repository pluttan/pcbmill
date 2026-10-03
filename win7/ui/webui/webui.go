// Package webui shows the pcbscan window as a page in the local browser:
// on macOS and Linux there is no native window without cgo, so frames go
// to a canvas over HTTP on 127.0.0.1 and input comes back the same way.
package webui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"pcbscan/ui"
)

// ==========================
// ===  Window            ===
// ==========================

// Window implements ui.Display with a browser page.
type Window struct {
	frames frameStore

	mu       sync.Mutex
	w, h     int
	scale    float64
	cursor   ui.Cursor
	title    string
	stateVer uint64 // bumped on cursor/title change, wakes the long-poll
	active   string // id of the page that drives input
	closed   bool

	events  chan ui.Event
	done    chan struct{}
	pending sync.WaitGroup // event handlers that may still send

	ln  net.Listener
	srv *http.Server
	url string
}

// Size before any page has reported its canvas.
const defaultW, defaultH = 1280, 800

// Open starts the page server on 127.0.0.1 (port 8765 when free, otherwise
// any) and returns its URL; openBrowser also opens it in the default browser.
func Open(title string, openBrowser bool) (*Window, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:8765")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, "", err
		}
	}
	w := newWindow(title)
	w.ln = ln
	w.url = fmt.Sprintf("http://%s/", ln.Addr().String())
	w.srv = &http.Server{Handler: w.handler()}
	go w.srv.Serve(ln)
	if openBrowser {
		if err := launchBrowser(w.url); err != nil {
			return w, w.url, fmt.Errorf("browser: %w (open %s by hand)", err, w.url)
		}
	}
	return w, w.url, nil
}

func newWindow(title string) *Window {
	w := &Window{
		w: defaultW, h: defaultH, scale: 1,
		title:  title,
		events: make(chan ui.Event, 1024),
		done:   make(chan struct{}),
	}
	w.frames.init()
	return w
}

func launchBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap, the opener exits at once
	return nil
}

// URL of the page.
func (w *Window) URL() string { return w.url }

// ==========================
// ===  ui.Display        ===
// ==========================

func (w *Window) Events() <-chan ui.Event { return w.events }

func (w *Window) Present(img *image.RGBA) { w.frames.present(img) }

func (w *Window) Size() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w, w.h
}

func (w *Window) Scale() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.scale
}

func (w *Window) SetCursor(c ui.Cursor) {
	w.mu.Lock()
	if w.cursor != c {
		w.cursor = c
		w.stateVer++
		w.mu.Unlock()
		w.frames.wakeAll()
		return
	}
	w.mu.Unlock()
}

// SetNoActivate: a browser page cannot refuse focus; nothing to do.
func (w *Window) SetNoActivate(on bool) {}

func (w *Window) SetTitle(s string) {
	w.mu.Lock()
	if w.title != s {
		w.title = s
		w.stateVer++
		w.mu.Unlock()
		w.frames.wakeAll()
		return
	}
	w.mu.Unlock()
}

// Close stops the server and closes Events once no handler can send.
func (w *Window) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	close(w.done)
	if w.srv != nil {
		w.srv.Close()
	}
	w.pending.Wait()
	close(w.events)
}

// ==========================
// ===  Event delivery    ===
// ==========================

var errClosed = errors.New("window closed")

// beginSend registers a handler that will push events; false after Close.
func (w *Window) beginSend() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	w.pending.Add(1)
	return true
}

// send blocks while the program is behind (back-pressure to the page)
// but never past Close.
func (w *Window) send(ev ui.Event) error {
	select {
	case w.events <- ev:
		return nil
	case <-w.done:
		return errClosed
	}
}

func cursorCSS(c ui.Cursor) string {
	switch c {
	case ui.CursorHand:
		return "pointer"
	case ui.CursorText:
		return "text"
	case ui.CursorMove:
		return "move"
	case ui.CursorWait:
		return "wait"
	}
	return "default"
}

// ==========================
// ===  Helpers           ===
// ==========================

// contextWithTimeoutDone ends ctx after d or when done closes (Close
// must not wait 25 s for parked long-polls).
func contextWithTimeoutDone(parent context.Context, d time.Duration, done <-chan struct{}) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, d)
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

var startTime = time.Now()

func nowMs() float64 { return float64(time.Since(startTime).Microseconds()) / 1000 }
