//go:build windows

package winui

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"pcbscan/ui"
)

// ==========================
// ===  Window            ===
// ==========================

// Window is one top-level window; it implements ui.Display.
type Window struct {
	hwnd  uintptr
	scale float64

	// input queue: filled by the window thread without ever blocking it,
	// drained by pump into out
	qmu   sync.Mutex
	qcond *sync.Cond
	queue []ui.Event
	done  bool
	out   chan ui.Event

	// back buffer, written by Present, read by WM_PAINT
	fmu      sync.Mutex
	frame    []byte // BGRA, top-down, stride fw*4
	fw, fh   int
	exposed  bool // EvExpose sent while there is no frame yet
	cw, ch   int  // client size
	minim    bool
	noAct    bool
	cursor   ui.Cursor
	tracking bool // TrackMouseEvent armed for WM_MOUSELEAVE
	surr     rune // high surrogate waiting for its pair
	bigIcon  uintptr
	smIcon   uintptr
	actDone  chan struct{}
}

var _ ui.Display = (*Window)(nil)

// queueCap bounds the events waiting for a slow reader.
const queueCap = 1024

var (
	classOnce sync.Once
	classErr  error
	className = utf16Ptr("pcbscanWindow")
	bgBrush   uintptr
	procCB    = syscall.NewCallback(wndProc)

	// hwnd -> Window; creating is the one being built, its first messages
	// arrive before CreateWindowEx returns the handle
	winMu    sync.Mutex
	wins     = map[uintptr]*Window{}
	creating *Window
	openMu   sync.Mutex
)

func register() error {
	classOnce.Do(func() {
		pSetProcessDPIAware.Call() // before any window: no bitmap stretching by the system
		inst, _, _ := pGetModuleHandleW.Call(0)
		cur, _, _ := pLoadCursorW.Call(0, idcArrow)
		bgBrush, _, _ = pCreateSolidBrush.Call(bgColor)
		wc := wndClassEx{
			Style:      csHRedraw | csVRedraw,
			WndProc:    procCB,
			Instance:   inst,
			Cursor:     cur,
			Background: 0, // WM_ERASEBKGND is ours: no flash of a background colour
			ClassName:  className,
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			classErr = e
		}
	})
	return classErr
}

// screenScale reads LOGPIXELSX of the screen; meaningful after SetProcessDPIAware.
func screenScale() float64 {
	dc, _, _ := pGetDC.Call(0)
	if dc == 0 {
		return 1
	}
	defer pReleaseDC.Call(0, dc)
	dpi, _, _ := pGetDeviceCaps.Call(dc, logPixelsX)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// Open creates a window with a client area of w x h pixels on its own locked
// OS thread and returns once it is visible.
func Open(title string, w, h int) (*Window, error) {
	win := &Window{out: make(chan ui.Event), cursor: ui.CursorArrow}
	win.qcond = sync.NewCond(&win.qmu)
	ready := make(chan error, 1)
	go win.run(title, w, h, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	go win.pump()
	return win, nil
}

// run owns the window: every message of it is handled on this thread.
func (w *Window) run(title string, cw, ch int, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := register(); err != nil {
		ready <- err
		return
	}
	w.scale = screenScale()
	style := uintptr(wsOverlappedWindow | wsClipChildren)
	exStyle := uintptr(wsExAppWindow)
	r := rect{0, 0, int32(cw), int32(ch)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, exStyle)
	inst, _, _ := pGetModuleHandleW.Call(0)

	openMu.Lock()
	winMu.Lock()
	creating = w
	winMu.Unlock()
	hwnd, _, e := pCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr(title))), style,
		cwUseDefault, cwUseDefault, uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		0, 0, inst, 0)
	winMu.Lock()
	creating = nil
	winMu.Unlock()
	openMu.Unlock()
	if hwnd == 0 {
		if e == nil || e == syscall.Errno(0) {
			e = errors.New("CreateWindowEx failed")
		}
		ready <- e
		return
	}
	w.hwnd = hwnd
	winMu.Lock()
	wins[hwnd] = w
	winMu.Unlock()

	var cr rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	w.fmu.Lock()
	w.cw, w.ch = int(cr.Right), int(cr.Bottom)
	w.fmu.Unlock()
	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)
	ready <- nil

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT or an error
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	w.qmu.Lock()
	w.done = true
	w.qcond.Broadcast()
	w.qmu.Unlock()
}

// ==========================
// ===  Event queue       ===
// ==========================

// post never blocks: a burst of mouse moves becomes the last one, and when
// the reader is far behind mouse moves are the first thing dropped.
func (w *Window) post(e ui.Event) {
	w.qmu.Lock()
	defer w.qmu.Unlock()
	if w.done {
		return
	}
	n := len(w.queue)
	if e.Kind == ui.EvMouseMove && n > 0 && w.queue[n-1].Kind == ui.EvMouseMove {
		w.queue[n-1] = e
		return
	}
	if n >= queueCap {
		if e.Kind == ui.EvMouseMove {
			return
		}
		i := 0
		for i < n && w.queue[i].Kind != ui.EvMouseMove {
			i++
		}
		if i == n {
			return // nothing cheaper to drop: lose the newest
		}
		w.queue = append(w.queue[:i], w.queue[i+1:]...)
	}
	w.queue = append(w.queue, e)
	w.qcond.Signal()
}

// pump hands the queue to the reader in order and closes Events at the end.
func (w *Window) pump() {
	for {
		w.qmu.Lock()
		for len(w.queue) == 0 && !w.done {
			w.qcond.Wait()
		}
		if len(w.queue) == 0 {
			w.qmu.Unlock()
			close(w.out)
			return
		}
		e := w.queue[0]
		w.queue = w.queue[1:]
		if len(w.queue) == 0 {
			w.queue = w.queue[:0:0] // let a long backlog be collected
		}
		w.qmu.Unlock()
		w.out <- e
	}
}

// ==========================
// ===  ui.Display        ===
// ==========================

func (w *Window) Events() <-chan ui.Event { return w.out }

func (w *Window) Size() (int, int) {
	w.fmu.Lock()
	defer w.fmu.Unlock()
	return w.cw, w.ch
}

func (w *Window) Scale() float64 { return w.scale }

func (w *Window) SetCursor(c ui.Cursor) {
	w.fmu.Lock()
	same := w.cursor == c
	w.cursor = c
	w.fmu.Unlock()
	if !same {
		pPostMessageW.Call(w.hwnd, wmDoCursor, 0, 0)
	}
}

func (w *Window) SetNoActivate(on bool) {
	w.fmu.Lock()
	w.noAct = on
	w.fmu.Unlock()
	idx := int32(gwlExStyle)
	ex, _, _ := pGetWindowLongW.Call(w.hwnd, uintptr(idx))
	if on {
		ex |= wsExNoActivate
	} else {
		ex &^= wsExNoActivate
	}
	ex |= wsExAppWindow // NOACTIVATE alone would drop the taskbar button
	pSetWindowLongW.Call(w.hwnd, uintptr(idx), ex)
	pSetWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0, 0x0001|0x0002|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

func (w *Window) SetTitle(s string) {
	pSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16Ptr(s))))
}

// Close destroys the window on its own thread; Events closes after that.
func (w *Window) Close() {
	pPostMessageW.Call(w.hwnd, wmDoClose, 0, 0)
}

// ==========================
// ===  Placement         ===
// ==========================

func (w *Window) Hwnd() uintptr { return w.hwnd }

// SetBounds places the outer rectangle in screen pixels, out of a maximized
// or minimized state first (those ignore a move), without activating.
func (w *Window) SetBounds(x, y, width, height int) {
	if z, _, _ := pIsZoomed.Call(w.hwnd); z != 0 {
		pShowWindow.Call(w.hwnd, swShowNoActivate)
	} else if w.Minimized() {
		pShowWindow.Call(w.hwnd, swShowNoActivate)
	}
	pSetWindowPos.Call(w.hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		swpNoZOrder|swpNoActivate)
}

func (w *Window) Bounds() (x, y, width, height int) {
	var r rect
	pGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)
}

func (w *Window) Minimized() bool {
	r, _, _ := pIsIconic.Call(w.hwnd)
	return r != 0
}

// Activate brings the window forward with the keyboard focus. It runs on the
// window thread (SetFocus works only there) and waits up to a second.
func (w *Window) Activate() {
	done := make(chan struct{})
	w.fmu.Lock()
	w.actDone = done
	w.fmu.Unlock()
	if r, _, _ := pPostMessageW.Call(w.hwnd, wmDoActivate, 0, 0); r == 0 {
		return
	}
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

func sendKey(vk uint16, flags uint32) {
	in := input{Type: inputKeyboard, Ki: keybdInput{Vk: vk, Flags: flags}}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

// activate: Windows 7 refuses SetForegroundWindow to a process that did not
// get the last input. Joining the input queue of the foreground thread lifts
// that; the ALT press is the fallback. ALT goes down before the switch and up
// after it, so the other window never sees a lone ALT release (which would
// open its menu bar) and ours swallows WM_SYSKEYUP.
func (w *Window) activate() {
	h := w.hwnd
	if w.Minimized() {
		pShowWindow.Call(h, swRestore)
	}
	fg, _, _ := pGetForegroundWindow.Call()
	if fg != h {
		me, _, _ := pGetCurrentThreadId.Call()
		other, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
		attached := false
		if fg != 0 && other != 0 && other != me {
			r, _, _ := pAttachThreadInput.Call(me, other, 1)
			attached = r != 0
		}
		pBringWindowToTop.Call(h)
		pSetForegroundWindow.Call(h)
		if now, _, _ := pGetForegroundWindow.Call(); now != h {
			sendKey(vkMenu, 0)
			pSetForegroundWindow.Call(h)
			sendKey(vkMenu, keyEventUp)
		}
		if attached {
			pAttachThreadInput.Call(me, other, 0)
		}
	}
	pSetFocus.Call(h)
}

// ==========================
// ===  Fatal             ===
// ==========================

// Fatal shows an error box; the exe has no console to print to.
func Fatal(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))),
		mbOK|mbIconError|mbTopMost|mbSetFg)
}
