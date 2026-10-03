//go:build windows

package winui

import (
	"unsafe"

	"pcbscan/ui"
)

// ==========================
// ===  Window procedure  ===
// ==========================

func lookup(hwnd uintptr) *Window {
	winMu.Lock()
	defer winMu.Unlock()
	if w, ok := wins[hwnd]; ok {
		return w
	}
	if creating != nil {
		w := creating
		w.hwnd = hwnd
		wins[hwnd] = w
		return w
	}
	return nil
}

func defProc(hwnd, m, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// wndProc must never unwind into Windows: a panic is swallowed and the
// message gets the default treatment.
func wndProc(hwnd, m, wp, lp uintptr) (ret uintptr) {
	defer func() {
		if recover() != nil {
			ret = defProc(hwnd, m, wp, lp)
		}
	}()
	w := lookup(hwnd)
	if w == nil {
		return defProc(hwnd, m, wp, lp)
	}
	if r, ok := w.handle(m, wp, lp); ok {
		return r
	}
	return defProc(hwnd, m, wp, lp)
}

// handle returns ok=false for what DefWindowProc should do.
func (w *Window) handle(m, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case wmEraseBkgnd:
		return 1, true
	case wmPaint:
		w.paint()
		return 0, true
	case wmSize:
		w.fmu.Lock()
		w.minim = wp == sizeMinimized
		if !w.minim {
			w.cw, w.ch = int(uint16(lp)), int(uint16(lp>>16))
		}
		cw, ch, minim := w.cw, w.ch, w.minim
		w.fmu.Unlock()
		if !minim {
			w.post(ui.Event{Kind: ui.EvResize, W: cw, H: ch})
		}
		return 0, true
	case wmSetFocus, wmKillFocus:
		w.post(ui.Event{Kind: ui.EvFocus, Focused: m == wmSetFocus})
		return 0, true
	case wmClose:
		w.post(ui.Event{Kind: ui.EvClose}) // the program decides and calls Close
		return 0, true
	case wmDoClose:
		pDestroyWindow.Call(w.hwnd)
		return 0, true
	case wmDestroy:
		w.dropIcons()
		pPostQuitMessage.Call(0)
		return 0, true
	case wmNcDestroy:
		winMu.Lock()
		delete(wins, w.hwnd)
		winMu.Unlock()
		return defProc(w.hwnd, m, wp, lp), true
	case wmGetMinMaxInfo:
		mmi := *(**minMaxInfo)(unsafe.Pointer(&lp)) // lParam is a pointer from Windows, not Go memory
		mmi.MinTrackSize = point{int32(560 * w.scale), int32(420 * w.scale)}
		return 0, true
	case wmMouseActivate:
		w.fmu.Lock()
		no := w.noAct
		w.fmu.Unlock()
		if no {
			return maNoActivate, true
		}
	case wmSetCursor:
		if uint16(lp) == htClient {
			w.applyCursor()
			return 1, true
		}
	case wmDoCursor:
		w.refreshCursor()
		return 0, true
	case wmDoActivate:
		w.activate()
		w.fmu.Lock()
		if w.actDone != nil {
			close(w.actDone)
			w.actDone = nil
		}
		w.fmu.Unlock()
		return 0, true
	case wmSysCommand:
		if wp&0xFFF0 == scKeyMenu { // ALT or F10 alone would enter the menu loop
			return 0, true
		}
	default:
		if r, ok := w.keyboard(m, wp, lp); ok {
			return r, true
		}
		return w.mouse(m, wp, lp)
	}
	return 0, false
}

// ==========================
// ===  Keyboard          ===
// ==========================

func keyDown(vk uintptr) bool {
	r, _, _ := pGetKeyState.Call(vk)
	return int16(r) < 0
}

func mods() ui.Mods {
	var m ui.Mods
	if keyDown(vkShift) {
		m |= ui.ModShift
	}
	if keyDown(vkControl) {
		m |= ui.ModCtrl
	}
	if keyDown(vkMenu) {
		m |= ui.ModAlt
	}
	return m
}

var vkMap = map[uintptr]ui.Key{
	0x25: ui.KeyLeft, 0x26: ui.KeyUp, 0x27: ui.KeyRight, 0x28: ui.KeyDown,
	0x0D: ui.KeyEnter, 0x1B: ui.KeyEsc, 0x09: ui.KeyTab, 0x08: ui.KeyBackspace,
	0x2E: ui.KeyDelete, 0x21: ui.KeyPgUp, 0x22: ui.KeyPgDn, 0x24: ui.KeyHome,
	0x23: ui.KeyEnd, 0x20: ui.KeySpace,
}

func mapKey(vk uintptr) ui.Key {
	switch {
	case vk >= 0x70 && vk <= 0x7B:
		return ui.KeyF1 + ui.Key(vk-0x70)
	case vk >= 'A' && vk <= 'Z':
		return ui.KeyA + ui.Key(vk-'A')
	case vk >= '0' && vk <= '9':
		return ui.Key0 + ui.Key(vk-'0')
	case vk >= 0x60 && vk <= 0x69: // numpad digits with NumLock on
		return ui.Key0 + ui.Key(vk-0x60)
	}
	if k, ok := vkMap[vk]; ok {
		return k
	}
	return ui.KeyOther
}

// modifier keys alone (and their left/right variants) are not events
func isModifier(vk uintptr) bool {
	return vk == vkShift || vk == vkControl || vk == vkMenu || (vk >= 0xA0 && vk <= 0xA5)
}

func (w *Window) keyboard(m, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case wmKeyDown, wmSysKeyDown:
		if m == wmSysKeyDown && wp == vkF4 && keyDown(vkMenu) {
			return 0, false // Alt+F4: DefWindowProc turns it into WM_CLOSE
		}
		if !isModifier(wp) {
			w.post(ui.Event{Kind: ui.EvKey, Key: mapKey(wp), Mods: mods()})
		}
		return 0, true // no DefWindowProc: F10 and ALT must not reach the menu loop
	case wmKeyUp:
		return 0, true
	case wmSysKeyUp, wmSysChar:
		return 0, true // ALT release and Alt+letter: no menu, no beep
	case wmChar:
		w.char(rune(uint16(wp)))
		return 0, true
	}
	return 0, false
}

// char joins UTF-16 surrogate pairs; control characters are keys, not text.
func (w *Window) char(r rune) {
	switch {
	case r >= 0xD800 && r < 0xDC00:
		w.surr = r
		return
	case r >= 0xDC00 && r < 0xE000:
		if w.surr == 0 {
			return
		}
		r = 0x10000 + (w.surr-0xD800)<<10 + (r - 0xDC00)
	}
	w.surr = 0
	if r < 32 || r == 127 {
		return
	}
	w.post(ui.Event{Kind: ui.EvChar, Rune: r})
}

// ==========================
// ===  Mouse             ===
// ==========================

func (w *Window) mouse(m, wp, lp uintptr) (uintptr, bool) {
	x, y := loWord(lp), hiWord(lp)
	switch m {
	case wmMouseMove:
		if !w.tracking {
			t := trackMouseEvent{Flags: tmeLeave, Track: w.hwnd}
			t.Size = uint32(unsafe.Sizeof(t))
			r, _, _ := pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
			w.tracking = r != 0
		}
		w.post(ui.Event{Kind: ui.EvMouseMove, X: x, Y: y})
		return 0, true
	case wmMouseLeave:
		w.tracking = false
		w.post(ui.Event{Kind: ui.EvLeave})
		return 0, true
	case wmLButtonDown, wmRButtonDown, wmMButtonDown:
		pSetCapture.Call(w.hwnd) // drags keep reporting outside the window
		w.post(ui.Event{Kind: ui.EvMouseDown, Button: button(m), X: x, Y: y})
		return 0, true
	case wmLButtonUp, wmRButtonUp, wmMButtonUp:
		if wp&(mkLButton|mkRButton|mkMButton) == 0 {
			pReleaseCapture.Call()
		}
		w.post(ui.Event{Kind: ui.EvMouseUp, Button: button(m), X: x, Y: y})
		return 0, true
	case wmMouseWheel:
		p := point{int32(x), int32(y)} // screen coordinates in this message
		pScreenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&p)))
		w.post(ui.Event{Kind: ui.EvWheel, Wheel: float64(hiWord(wp)) / 120, X: int(p.X), Y: int(p.Y)})
		return 0, true
	}
	return 0, false
}

func button(m uintptr) ui.Button {
	switch m {
	case wmRButtonDown, wmRButtonUp:
		return ui.BtnRight
	case wmMButtonDown, wmMButtonUp:
		return ui.BtnMiddle
	}
	return ui.BtnLeft
}

// ==========================
// ===  Cursor            ===
// ==========================

var cursorIDs = map[ui.Cursor]uintptr{
	ui.CursorArrow: 32512, // IDC_ARROW
	ui.CursorHand:  32649, // IDC_HAND
	ui.CursorText:  32513, // IDC_IBEAM
	ui.CursorMove:  32646, // IDC_SIZEALL
	ui.CursorWait:  32514, // IDC_WAIT
}

func (w *Window) applyCursor() {
	w.fmu.Lock()
	c := w.cursor
	w.fmu.Unlock()
	id, ok := cursorIDs[c]
	if !ok {
		id = idcArrow
	}
	h, _, _ := pLoadCursorW.Call(0, id) // shared system cursor: never destroyed
	pSetCursor.Call(h)
}

// refreshCursor changes the pointer at once when it is over the client area,
// not on the next mouse move.
func (w *Window) refreshCursor() {
	var p point
	if r, _, _ := pGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return
	}
	if windowFromPoint(p) != w.hwnd {
		return
	}
	pScreenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&p)))
	var cr rect
	pGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&cr)))
	if p.X >= 0 && p.Y >= 0 && p.X < cr.Right && p.Y < cr.Bottom {
		w.applyCursor()
	}
}

// windowFromPoint: POINT goes by value, one register on amd64, two stack
// words on 386.
func windowFromPoint(p point) uintptr {
	var h uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		h, _, _ = pWindowFromPoint.Call(uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<16<<16)
	} else {
		h, _, _ = pWindowFromPoint.Call(uintptr(p.X), uintptr(p.Y))
	}
	return h
}
