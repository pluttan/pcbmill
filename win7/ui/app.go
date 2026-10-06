package ui

// The window runs on its own goroutine: it reads input, lets the screens of
// the program react to it and draws them. The work (the machine, files)
// runs on another goroutine, blocking as long as it likes; it changes what
// the screens show with Do and waits for answers on channels the screens
// write to. Everything the screens draw is guarded by one mutex.

import (
	"image"
	"sync"
	"time"

	"pcbscan/gfx"
)

// Layer is one screen or a dialog over it: it draws itself into the frame
// and reacts to the input of the frame through the Ctx.
type Layer interface {
	Draw(c *Ctx)
}

// LayerFunc lets a plain function be a layer.
type LayerFunc func(c *Ctx)

func (f LayerFunc) Draw(c *Ctx) { f(c) }

type App struct {
	Disp  Display
	Theme *Theme

	mu     sync.Mutex
	base   Layer
	modals []Layer
	wake   chan struct{}
	frame  *gfx.Canvas
	in     input
	st     state
	anim   time.Time // redraw by itself until then (progress, a moving view)
	wakeAt time.Time // one redraw then (a blinking caret); zero: none
	closed bool
	// OnClose is called on the window goroutine when the operator closes
	// the window; nil closes at once.
	OnClose func()
	// Texts of the last frame, for tests that wait for something on screen.
	texts []TextAt
}

// NewApp starts the window goroutine.
func NewApp(d Display, th *Theme) *App {
	a := &App{Disp: d, Theme: th, wake: make(chan struct{}, 1)}
	a.st.init()
	go a.loop()
	return a
}

// ==========================
// ===  The work side     ===
// ==========================

// Show replaces the screen under the dialogs.
func (a *App) Show(l Layer) {
	a.mu.Lock()
	a.base = l
	a.mu.Unlock()
	a.Wake()
}

// Push puts a dialog on top; Pop takes it away.
func (a *App) Push(l Layer) {
	a.mu.Lock()
	a.modals = append(a.modals, l)
	a.mu.Unlock()
	a.Wake()
}

func (a *App) Pop(l Layer) {
	a.mu.Lock()
	for i := len(a.modals) - 1; i >= 0; i-- {
		if a.modals[i] == l {
			a.modals = append(a.modals[:i], a.modals[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	a.Wake()
}

// Do changes what the screens show, under the lock, and redraws.
func (a *App) Do(f func()) {
	a.mu.Lock()
	f()
	a.mu.Unlock()
	a.Wake()
}

// Wake asks for a new frame.
func (a *App) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Texts: the strings drawn in the last frame.
func (a *App) Texts() []TextAt {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]TextAt(nil), a.texts...)
}

// Closed: the window is gone.
func (a *App) Closed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// ==========================
// ===  The window side   ===
// ==========================

func (a *App) loop() {
	evs := a.Disp.Events()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		var batch []Event
		select {
		case e, ok := <-evs:
			if !ok {
				a.mu.Lock()
				a.closed = true
				a.mu.Unlock()
				return
			}
			batch = append(batch, e)
		case <-a.wake:
		case <-tick.C:
			a.mu.Lock()
			now := time.Now()
			live := now.Before(a.anim) || !a.wakeAt.IsZero() && !now.Before(a.wakeAt)
			a.mu.Unlock()
			if !live {
				continue
			}
		}
		// take what else is waiting: a burst of mouse moves becomes one frame
	more:
		for len(batch) < 256 {
			select {
			case e, ok := <-evs:
				if !ok {
					break more
				}
				batch = append(batch, e)
			default:
				break more
			}
		}
		a.handle(batch)
	}
}

// handle runs one pass per discrete event (a click, a key) so none is lost,
// mouse moves only update the position; the last pass is shown.
func (a *App) handle(batch []Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	passes := 0
	for _, e := range batch {
		switch e.Kind {
		case EvClose:
			if a.OnClose != nil {
				a.mu.Unlock()
				a.OnClose()
				a.mu.Lock()
			} else {
				a.Disp.Close()
			}
			continue
		case EvMouseMove:
			a.in.mx, a.in.my, a.in.inside = e.X, e.Y, true
			continue
		case EvLeave:
			a.in.inside = false
			continue
		case EvResize, EvExpose, EvFocus:
			if e.Kind == EvFocus {
				a.in.focused = e.Focused
			}
			continue
		}
		if e.Kind == EvMouseDown || e.Kind == EvMouseUp || e.Kind == EvWheel {
			a.in.mx, a.in.my, a.in.inside = e.X, e.Y, true
		}
		if e.Kind == EvMouseDown && int(e.Button) < len(a.in.down) {
			a.in.down[e.Button] = true
		}
		a.in.ev = e
		a.pass(false)
		a.in.ev = Event{}
		if e.Kind == EvMouseUp && int(e.Button) < len(a.in.down) {
			a.in.down[e.Button] = false
		}
		passes++
	}
	a.pass(true)
}

// pass draws the layers once; show puts the frame on the screen.
func (a *App) pass(show bool) {
	w, h := a.Disp.Size()
	if w < 1 || h < 1 {
		return
	}
	if a.frame == nil || a.frame.Img.Bounds().Dx() != w || a.frame.Img.Bounds().Dy() != h {
		a.frame = gfx.NewCanvas(w, h)
	}
	a.Theme.setScale(a.Disp.Scale())
	c := &Ctx{App: a, C: a.frame, T: a.Theme, in: &a.in, st: &a.st, W: w, H: h}
	c.C.Clip = image.Rect(0, 0, w, h)
	c.C.Clear(a.Theme.Base)
	c.texts = a.texts[:0:0]
	a.st.beginFrame()
	layers := append([]Layer(nil), a.base)
	layers = append(layers, a.modals...)
	for i, l := range layers {
		if l == nil {
			continue
		}
		c.live = i == len(layers)-1 // only the top layer gets the input
		if i > 0 && i == len(layers)-1 {
			c.C.FillRect(image.Rect(0, 0, w, h), a.Theme.Dim)
		}
		l.Draw(c)
	}
	c.finish()
	a.texts = c.texts
	if c.animate {
		a.anim = time.Now().Add(200 * time.Millisecond)
	}
	a.wakeAt = c.wakeAt
	if show {
		a.Disp.SetCursor(c.cursor)
		a.Disp.Present(a.frame.Img)
	}
}
