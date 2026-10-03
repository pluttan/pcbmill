//go:build !windows

package main

// On a Mac or Linux the window is a page in the browser (the emulation:
// there is no NcStudio to type into, so no focus to guard).

import (
	"fmt"
	"os"

	"pcbscan/ui"
	"pcbscan/ui/webui"
)

func openDisplay(o opts) (ui.Display, error) {
	w, url, err := webui.Open("pcbscan", os.Getenv("PCBSCAN_NOBROWSER") == "")
	if err != nil {
		return nil, err
	}
	fmt.Printf("pcbscan: окно программы в браузере: %s\n", url)
	return w, nil
}

func appHwnd() uintptr { return 0 }

func backToApp() {}

func machineBusy(bool) {}

func fatal(s string) { fmt.Fprintln(os.Stderr, s) }
