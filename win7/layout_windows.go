//go:build windows

package main

// Console colours and size, and the two windows side by side: NcStudio on the
// left half of the screen, this console on the right, both visible at once.

import (
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	pGetConsoleScreenBufferInfo  = kernel32.NewProc("GetConsoleScreenBufferInfo")
	pSetConsoleTextAttribute     = kernel32.NewProc("SetConsoleTextAttribute")
	pSetConsoleScreenBufferSize  = kernel32.NewProc("SetConsoleScreenBufferSize")
	pSetConsoleWindowInfo        = kernel32.NewProc("SetConsoleWindowInfo")
	pGetLargestConsoleWindowSize = kernel32.NewProc("GetLargestConsoleWindowSize")
	pGetCurrentConsoleFont       = kernel32.NewProc("GetCurrentConsoleFont")
	pSystemParametersInfoW       = user32.NewProc("SystemParametersInfoW")
	pSetWindowPos                = user32.NewProc("SetWindowPos")
	pShowWindowAsync             = user32.NewProc("ShowWindowAsync")
	pGetClientRect               = user32.NewProc("GetClientRect")
	pGetWindowLongW              = user32.NewProc("GetWindowLongW")
	pGetSystemMetrics            = user32.NewProc("GetSystemMetrics")
)

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

// conInfo is CONSOLE_SCREEN_BUFFER_INFO: all WORD-sized, no padding.
type conInfo struct {
	Size      coord
	Cursor    coord
	Attr      uint16
	Window    smallRect
	MaxWindow coord
}

type winRect struct{ Left, Top, Right, Bottom int32 }

const (
	swRestore        = 9
	swShowNoActivate = 4 // restores a maximized or minimized window without taking focus
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpAsync         = 0x4000 // NcStudio busy: post the move, do not wait for it
	wsHScroll        = 0x00100000
	wsVScroll        = 0x00200000
	smCxVScroll      = 2
	smCyHScroll      = 3
	spiGetWorkArea   = 0x0030
)

func stdout() uintptr {
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF5)) // STD_OUTPUT_HANDLE
	return h
}

func consoleInfo() (conInfo, bool) {
	var ci conInfo
	r, _, _ := pGetConsoleScreenBufferInfo.Call(stdout(), uintptr(unsafe.Pointer(&ci)))
	return ci, r != 0
}

// startAttr: the console colours before the program, what the '0' mark restores.
var startAttr = func() uint16 {
	if ci, ok := consoleInfo(); ok {
		return ci.Attr
	}
	return 0x07
}()

// curAttr is the colour say writes with now.
var curAttr = startAttr

// setInk switches the text colour ('A'+c) or the background ('a'+c); '0' goes
// back to the colours the console had at start.
func setInk(h uintptr, mark byte) {
	switch {
	case mark >= 'A' && mark < 'A'+16:
		curAttr = curAttr&0xFFF0 | uint16(mark-'A')
	case mark >= 'a' && mark < 'a'+16:
		curAttr = curAttr&0xFF0F | uint16(mark-'a')<<4
	default:
		curAttr = startAttr
	}
	pSetConsoleTextAttribute.Call(h, uintptr(curAttr))
}

func workArea() (winRect, bool) {
	var r winRect
	ok, _, _ := pSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0)
	return r, ok != 0 && r.Right > r.Left && r.Bottom > r.Top
}

// ==========================
// ===  Finding NcStudio  ===
// ==========================

// One callback for the whole run: Go keeps every syscall.NewCallback forever and
// allows about two thousand, and the status line looks for NcStudio every prompt.
var (
	topMu    sync.Mutex
	topTitle string
	topSkip  uintptr
	topHit   uintptr
	topCB    = syscall.NewCallback(func(h, _ uintptr) uintptr {
		if h == topSkip {
			return 1
		}
		if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
		if containsFold(syscall.UTF16ToString(buf), topTitle) {
			topHit = h
			return 0
		}
		return 1
	})
)

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// findTop is the visible top window with title in its caption, never this
// console: started from a folder named NcStudio, its caption has the word too.
func findTop(title string) uintptr {
	topMu.Lock()
	defer topMu.Unlock()
	topTitle, topHit = title, 0
	topSkip, _, _ = pGetConsoleWindow.Call()
	pEnumWindows.Call(topCB, 0)
	return topHit
}

// ncFound is for the status line; NcStudio started after this program is put
// on its half of the screen here, the first time it is seen.
func ncFound(title string) bool {
	h := findTop(title)
	if h != 0 {
		placeNc(h)
	}
	return h != 0
}

// ==========================
// ===  Window layout     ===
// ==========================

var (
	layoutOn bool // -nolayout leaves the windows where they are
	ncPlaced bool
)

// layoutWindows: NcStudio left, this console right, over the work area (the
// screen without the taskbar).
func layoutWindows(title string) {
	layoutOn = true
	wa, ok := workArea()
	if !ok {
		return
	}
	if h := findTop(title); h != 0 {
		placeNc(h)
	}
	half := (wa.Right - wa.Left) / 2
	placeConsole(winRect{wa.Left + half, wa.Top, wa.Right, wa.Bottom})
}

// placeNc moves NcStudio to the left half once per run. It never activates the
// window: who has the focus is the emergency stop logic, Grab decides it.
func placeNc(h uintptr) {
	if !layoutOn || ncPlaced || h == 0 {
		return
	}
	wa, ok := workArea()
	if !ok {
		return
	}
	ncPlaced = true
	half := (wa.Right - wa.Left) / 2
	pShowWindowAsync.Call(h, swShowNoActivate) // a maximized window will not move
	pSetWindowPos.Call(h, 0, uintptr(wa.Left), uintptr(wa.Top), uintptr(half), uintptr(wa.Bottom-wa.Top),
		swpNoZOrder|swpNoActivate|swpAsync)
	time.Sleep(300 * time.Millisecond) // let it redraw before keys are typed into it
}

// placeConsole fits the console into r: the buffer gets the size of the window,
// so there are no scroll bars and the full-screen frame is the whole buffer.
func placeConsole(r winRect) {
	w, _, _ := pGetConsoleWindow.Call()
	if w == 0 {
		return
	}
	out := stdout()
	ci, ok := consoleInfo()
	if !ok {
		return
	}
	pShowWindow.Call(w, swRestore)
	time.Sleep(50 * time.Millisecond)
	// frame and caption: the window rectangle less the client one; both scroll
	// bars are counted as gone, the buffer will be exactly the window
	var wr, cr winRect
	pGetWindowRect.Call(w, uintptr(unsafe.Pointer(&wr)))
	pGetClientRect.Call(w, uintptr(unsafe.Pointer(&cr)))
	gwlStyle := int32(-16)
	style, _, _ := pGetWindowLongW.Call(w, uintptr(gwlStyle))
	dx := (wr.Right - wr.Left) - (cr.Right - cr.Left)
	dy := (wr.Bottom - wr.Top) - (cr.Bottom - cr.Top)
	if style&wsVScroll != 0 {
		v, _, _ := pGetSystemMetrics.Call(smCxVScroll)
		dx -= int32(v)
	}
	if style&wsHScroll != 0 {
		v, _, _ := pGetSystemMetrics.Call(smCyHScroll)
		dy -= int32(v)
	}
	// CONSOLE_FONT_INFO: font index, then the cell size in pixels
	var font struct {
		N    uint32
		Size coord
	}
	pGetCurrentConsoleFont.Call(out, 0, uintptr(unsafe.Pointer(&font)))
	fw, fh := int32(font.Size.X), int32(font.Size.Y)
	if fw <= 0 || fh <= 0 {
		fw, fh = 8, 12
	}
	cols := (r.Right - r.Left - dx) / fw
	rows := (r.Bottom - r.Top - dy) / fh
	lg, _, _ := pGetLargestConsoleWindowSize.Call(out)
	if mx := int32(int16(lg)); mx > 0 && cols > mx {
		cols = mx
	}
	if my := int32(int16(lg >> 16)); my > 0 && rows > my {
		rows = my
	}
	if cols < 40 || rows < 10 {
		return
	}
	bufH := rows
	// the window must always fit the buffer: shrink it, set the buffer, grow it
	cw, ch := int32(ci.Window.Right-ci.Window.Left)+1, int32(ci.Window.Bottom-ci.Window.Top)+1
	if cw > cols {
		cw = cols
	}
	if ch > rows {
		ch = rows
	}
	small := smallRect{0, 0, int16(cw - 1), int16(ch - 1)}
	pSetConsoleWindowInfo.Call(out, 1, uintptr(unsafe.Pointer(&small)))
	size := uintptr(uint16(cols)) | uintptr(uint16(bufH))<<16
	if ok, _, _ := pSetConsoleScreenBufferSize.Call(out, size); ok == 0 {
		return
	}
	win := smallRect{0, 0, int16(cols - 1), int16(rows - 1)}
	pSetConsoleWindowInfo.Call(out, 1, uintptr(unsafe.Pointer(&win)))
	pSetWindowPos.Call(w, 0, uintptr(r.Left), uintptr(r.Top), uintptr(cols*fw+dx), uintptr(rows*fh+dy),
		swpNoZOrder)
}
