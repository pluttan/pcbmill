// Package ui is the window of pcbscan: a platform window (Display) shows
// frames drawn by this program and sends back input events.
package ui

import "image"

// ==========================
// ===  Input events      ===
// ==========================

type Kind int

const (
	EvNone      Kind = iota
	EvKey            // a key went down (auto-repeat sends it again): Key, Mods
	EvChar           // text typed: Rune (after the keyboard layout, so Cyrillic too)
	EvMouseMove      // X, Y in client pixels
	EvMouseDown      // Button, X, Y
	EvMouseUp        // Button, X, Y
	EvWheel          // Wheel: +1 per notch away from the user, X, Y
	EvResize         // W, H: the new client size
	EvExpose         // the window needs the last frame again (nothing changed in it)
	EvFocus          // Focused: the window got or lost the keyboard
	EvClose          // the operator closes the window
	EvLeave          // the mouse left the client area
)

type Key int

const (
	KeyNone Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyEsc
	KeyTab
	KeyBackspace
	KeyDelete
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeySpace
	KeyF1  // F1..F12 are KeyF1+0 .. KeyF1+11
	KeyF12 = KeyF1 + 11
	// letters and digits for shortcuts, layout independent: KeyA+0 .. KeyA+25, Key0+0 .. Key0+9
	KeyA     = KeyF12 + 1
	KeyZ     = KeyA + 25
	Key0     = KeyZ + 1
	Key9     = Key0 + 9
	KeyOther = Key9 + 1 // anything else
)

type Mods int

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
)

type Button int

const (
	BtnLeft Button = iota
	BtnRight
	BtnMiddle
)

type Event struct {
	Kind    Kind
	Key     Key
	Mods    Mods
	Rune    rune
	X, Y    int
	Button  Button
	Wheel   float64
	W, H    int
	Focused bool
}

// ==========================
// ===  Platform window   ===
// ==========================

// Display is one top-level window. All methods are safe to call from any
// goroutine.
type Display interface {
	// Events: input in order; closed after the window is gone.
	Events() <-chan Event
	// Present shows the frame; the pixels are copied before it returns, the
	// size may differ from the window for a moment after a resize.
	Present(img *image.RGBA)
	// Size: the client area in pixels.
	Size() (w, h int)
	// Scale: pixels per 1/96 inch (1 on a usual Windows 7 screen).
	Scale() float64
	// SetCursor: the mouse pointer over the client area.
	SetCursor(c Cursor)
	// SetNoActivate: when true, clicks reach the window without taking the
	// keyboard focus from the window that has it (NcStudio while the program
	// types into it).
	SetNoActivate(on bool)
	// SetTitle: the caption.
	SetTitle(s string)
	// Close removes the window; Events is closed after that.
	Close()
}

type Cursor int

const (
	CursorArrow Cursor = iota
	CursorHand
	CursorText
	CursorMove
	CursorWait
)
