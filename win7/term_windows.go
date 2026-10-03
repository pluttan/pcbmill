//go:build windows

package main

// The full screen in the Windows 7 console: the frame is written in place with
// SetConsoleCursorPosition, the cursor is hidden, keys come from
// ReadConsoleInputW one event at a time.

import (
	"os"
	"unsafe"
)

var (
	pSetConsoleCursorPosition    = kernel32.NewProc("SetConsoleCursorPosition")
	pGetConsoleCursorInfo        = kernel32.NewProc("GetConsoleCursorInfo")
	pSetConsoleCursorInfo        = kernel32.NewProc("SetConsoleCursorInfo")
	pFillConsoleOutputCharacterW = kernel32.NewProc("FillConsoleOutputCharacterW")
	pFillConsoleOutputAttribute  = kernel32.NewProc("FillConsoleOutputAttribute")
)

// CONSOLE_CURSOR_INFO: DWORD dwSize, BOOL bVisible.
type cursorInfo struct {
	Size    uint32
	Visible int32
}

var (
	savedCursor cursorInfo
	haveCursor  bool
)

// coordArg packs a COORD for a by-value argument: X in the low word, Y in the high one.
func coordArg(x, y int) uintptr { return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16 }

func setCursorVisible(on bool) {
	if !haveCursor {
		return
	}
	ci := savedCursor
	if !on {
		ci.Visible = 0
	}
	pSetConsoleCursorInfo.Call(stdout(), uintptr(unsafe.Pointer(&ci)))
}

func conStart(keys chan Key) {
	if r, _, _ := pGetConsoleCursorInfo.Call(stdout(), uintptr(unsafe.Pointer(&savedCursor))); r != 0 {
		haveCursor = true
	}
	setCursorVisible(false)
	go conKeys(keys)
}

// conStop: the cursor back, the start colours, an empty window, cursor on top.
func conStop() {
	curAttr = startAttr
	pSetConsoleTextAttribute.Call(stdout(), uintptr(startAttr))
	conClear()
	if ci, ok := consoleInfo(); ok {
		pSetConsoleCursorPosition.Call(stdout(), coordArg(int(ci.Window.Left), int(ci.Window.Top)))
	}
	setCursorVisible(true)
}

// conSize is the visible window (srWindow), not the buffer: the buffer may be
// taller when -nolayout leaves the console as it was.
func conSize() (int, int) {
	ci, ok := consoleInfo()
	if !ok {
		return 80, 25
	}
	return int(ci.Window.Right-ci.Window.Left) + 1, int(ci.Window.Bottom-ci.Window.Top) + 1
}

// conClear fills the visible rows with spaces in the start colours.
func conClear() {
	ci, ok := consoleInfo()
	if !ok {
		return
	}
	n := uint32(ci.Size.X) * uint32(ci.Window.Bottom-ci.Window.Top+1)
	var done uint32
	at := coordArg(0, int(ci.Window.Top))
	pFillConsoleOutputCharacterW.Call(stdout(), uintptr(' '), uintptr(n), at, uintptr(unsafe.Pointer(&done)))
	pFillConsoleOutputAttribute.Call(stdout(), uintptr(startAttr), uintptr(n), at, uintptr(unsafe.Pointer(&done)))
}

// conLine writes one row of the frame; the frame lines are padded to the width,
// so the old text under them is overwritten without clearing (no flicker).
func conLine(row int, s string) {
	ci, ok := consoleInfo()
	if !ok {
		say("%s\n", s)
		return
	}
	pSetConsoleCursorPosition.Call(stdout(), coordArg(int(ci.Window.Left), int(ci.Window.Top)+row))
	say("%s", s)
}

func conDone() {}

func conFlushInput() {
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF6)) // STD_INPUT_HANDLE
	pFlushConsoleInput.Call(h)
}

// virtual key codes of the keys screens use
var vkKeys = map[uint16]KeyCode{
	0x26: KeyUp, 0x28: KeyDown, 0x25: KeyLeft, 0x27: KeyRight,
	0x0D: KeyEnter, 0x1B: KeyEsc, 0x09: KeyTab, 0x08: KeyBackspace, 0x2E: KeyDelete,
	0x21: KeyPgUp, 0x22: KeyPgDn, 0x24: KeyHome, 0x23: KeyEnd,
}

const shiftPressed = 0x0010

// conKeys reads key presses with ReadConsoleInputW: Russian letters arrive as
// letters, not cp866 bytes. Releases, mouse and focus events are skipped.
// Ctrl+C never gets here: the console turns it into a signal.
func conKeys(out chan<- Key) {
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF6))
	for {
		// INPUT_RECORD: EventType WORD, pad, KEY_EVENT_RECORD{bKeyDown BOOL, wRepeatCount,
		// wVirtualKeyCode, wVirtualScanCode WORD, UnicodeChar WCHAR, dwControlKeyState DWORD}
		var rec [20]byte
		var n uint32
		r, _, _ := pReadConsoleInputW.Call(h, uintptr(unsafe.Pointer(&rec[0])), 1, uintptr(unsafe.Pointer(&n)))
		if r == 0 { // not a console: piped input
			readBytes(os.Stdin, out)
			return
		}
		if n == 0 || *(*uint16)(unsafe.Pointer(&rec[0])) != 1 || *(*uint32)(unsafe.Pointer(&rec[4])) == 0 {
			continue
		}
		vk := *(*uint16)(unsafe.Pointer(&rec[10]))
		ch := rune(*(*uint16)(unsafe.Pointer(&rec[14])))
		state := *(*uint32)(unsafe.Pointer(&rec[16]))
		var k Key
		switch {
		case vk == 0x09 && state&shiftPressed != 0:
			k = Key{Code: KeyBackTab}
		case vk >= 0x70 && vk <= 0x7B:
			k = Key{Code: KeyF(int(vk-0x70) + 1)}
		case vkKeys[vk] != KeyNone:
			k = Key{Code: vkKeys[vk]}
		case ch >= ' ' && (ch < 0xD800 || ch > 0xDFFF):
			k = runeKey(ch)
		default:
			continue // Shift, Ctrl, Alt alone and the like
		}
		out <- k
	}
}
