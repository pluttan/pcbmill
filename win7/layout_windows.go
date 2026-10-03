//go:build windows

package main

// The two windows side by side: NcStudio on the left half of the screen, this
// program on the right, both visible at once.

import (
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	pSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	pSetWindowPos          = user32.NewProc("SetWindowPos")
	pShowWindowAsync       = user32.NewProc("ShowWindowAsync")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

const (
	swShowNoActivate = 4 // restores a maximized or minimized window without taking focus
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpAsync         = 0x4000 // NcStudio busy: post the move, do not wait for it
	spiGetWorkArea   = 0x0030
)

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
	topSkip = appHwnd()
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
	placeApp(winRect{wa.Left + half, wa.Top, wa.Right, wa.Bottom})
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

// placeApp puts the window of this program into r.
func placeApp(r winRect) {
	if appPlace != nil {
		appPlace(int(r.Left), int(r.Top), int(r.Right-r.Left), int(r.Bottom-r.Top))
	}
}
