//go:build windows

package main

// The native window on Windows, and the focus between it and NcStudio: while
// the program types into NcStudio a click on this window must not take the
// keyboard (that would stop the work), so the window is in no-activate mode.

import (
	"pcbscan/ui"
	"pcbscan/ui/winui"
)

var appWin *winui.Window

// appPlace puts the window on its half of the screen.
var appPlace func(x, y, w, h int)

func openDisplay(o opts) (ui.Display, error) {
	w, err := winui.Open("pcbscan", 1280, 800)
	if err != nil {
		return nil, err
	}
	appWin = w
	appPlace = func(x, y, wd, h int) { w.SetBounds(x, y, wd, h) }
	w.SetIcon(appIcon(64))
	return w, nil
}

func appHwnd() uintptr {
	if appWin == nil {
		return 0
	}
	return appWin.Hwnd()
}

// backToApp gives the keyboard back to this window after NcStudio had it.
func backToApp() {
	if appWin == nil {
		return
	}
	appWin.SetNoActivate(false)
	appWin.Activate()
}

// machineBusy: on while keys go to NcStudio, so clicks here keep its focus.
func machineBusy(on bool) {
	if appWin != nil {
		appWin.SetNoActivate(on)
	}
}

// fatal shows a crash in a message box: there is no console to print to.
func fatal(s string) { winui.Fatal("pcbscan", s) }
