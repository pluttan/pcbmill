package main

import (
	"strings"
	"testing"
	"time"

	"pcbscan/ui"
)

// testApp: the window without a screen for one test; the globals come back after.
func testApp(t *testing.T) *ui.Headless {
	t.Helper()
	th, err := ui.NewTheme()
	if err != nil {
		t.Skip(err)
	}
	oldApp, oldTheme := app, theme
	d := ui.NewHeadless(1000, 700)
	theme, app = th, ui.NewApp(d, th)
	t.Cleanup(func() { d.Close(); app, theme = oldApp, oldTheme })
	return d
}

// waitText waits until s is on the top layer of the screen.
func waitText(t *testing.T, s string) {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		for _, x := range app.Texts() {
			if x.Live && strings.Contains(x.S, s) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("нет %q на экране", s)
}

// sendKeys: strings are typed, ui.Key values pressed.
func sendKeys(d *ui.Headless, ks ...interface{}) {
	for _, k := range ks {
		switch v := k.(type) {
		case string:
			for _, r := range v {
				d.Send(ui.Event{Kind: ui.EvChar, Rune: r})
			}
		case ui.Key:
			d.Send(ui.Event{Kind: ui.EvKey, Key: v})
		}
		time.Sleep(30 * time.Millisecond)
	}
}
