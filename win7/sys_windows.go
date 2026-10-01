//go:build windows

package main

// Everything Windows-specific with plain syscall: nothing to install on Windows 7.

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pWriteConsoleW      = kernel32.NewProc("WriteConsoleW")
	pReadConsoleW       = kernel32.NewProc("ReadConsoleW")
	pReadConsoleInputW  = kernel32.NewProc("ReadConsoleInputW")
	pFlushConsoleInput  = kernel32.NewProc("FlushConsoleInputBuffer")
	pGetConsoleWindow   = kernel32.NewProc("GetConsoleWindow")
	pGetStdHandle       = kernel32.NewProc("GetStdHandle")
	pGetCommState       = kernel32.NewProc("GetCommState")
	pSetCommState       = kernel32.NewProc("SetCommState")
	pSetCommTimeouts    = kernel32.NewProc("SetCommTimeouts")
	pPurgeComm          = kernel32.NewProc("PurgeComm")
	pEnumWindows        = user32.NewProc("EnumWindows")
	pGetWindowTextW     = user32.NewProc("GetWindowTextW")
	pIsWindowVisible    = user32.NewProc("IsWindowVisible")
	pSetForeground      = user32.NewProc("SetForegroundWindow")
	pGetForeground      = user32.NewProc("GetForegroundWindow")
	pGetAncestor        = user32.NewProc("GetAncestor")
	pSendInput          = user32.NewProc("SendInput")
	pShowWindow         = user32.NewProc("ShowWindow")
	pGetWindowThreadPid = user32.NewProc("GetWindowThreadProcessId")
	pEnumChildWindows   = user32.NewProc("EnumChildWindows")
	pGetClassNameW      = user32.NewProc("GetClassNameW")
	pGetDlgCtrlID       = user32.NewProc("GetDlgCtrlID")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
	pRegOpenKeyExW      = advapi32.NewProc("RegOpenKeyExW")
	pRegEnumValueW      = advapi32.NewProc("RegEnumValueW")
	pRegCloseKey        = advapi32.NewProc("RegCloseKey")
)

// ==========================
// ===  Console           ===
// ==========================

// say writes through WriteConsoleW: the Windows 7 console is cp866, UTF-8 bytes would be garbage.
func say(f string, a ...interface{}) {
	s := fmt.Sprintf(f, a...)
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF5)) // STD_OUTPUT_HANDLE
	u := utf16.Encode([]rune(strings.ReplaceAll(s, "\n", "\r\n")))
	if len(u) == 0 {
		return
	}
	var n uint32
	r, _, _ := pWriteConsoleW.Call(h, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&n)), 0)
	if r == 0 { // redirected to a file
		os.Stdout.WriteString(s)
	}
}

var stdinBuf = bufio.NewReader(os.Stdin)

// readLine reads key by key through ReadConsoleInputW: Russian letters arrive as
// letters, not cp866 bytes, and PageUp works without Enter (returns HotLift).
func readLine() string {
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF6)) // STD_INPUT_HANDLE
	var line []rune
	for {
		// INPUT_RECORD: EventType WORD, pad, KEY_EVENT_RECORD{bKeyDown BOOL, wRepeatCount,
		// wVirtualKeyCode, wVirtualScanCode WORD, UnicodeChar WCHAR, dwControlKeyState DWORD}
		var rec [20]byte
		var n uint32
		r, _, _ := pReadConsoleInputW.Call(h, uintptr(unsafe.Pointer(&rec[0])), 1, uintptr(unsafe.Pointer(&n)))
		if r == 0 { // not a console: piped input
			s, _ := stdinBuf.ReadString('\n')
			return strings.TrimRight(s, "\r\n")
		}
		if n == 0 || *(*uint16)(unsafe.Pointer(&rec[0])) != 1 || *(*uint32)(unsafe.Pointer(&rec[4])) == 0 {
			continue // not a key press
		}
		vk := *(*uint16)(unsafe.Pointer(&rec[10]))
		ch := rune(*(*uint16)(unsafe.Pointer(&rec[14])))
		switch {
		case vk == 0x21: // VK_PRIOR, PageUp
			say("\n")
			return HotLift
		case vk >= 0x25 && vk <= 0x28: // arrows: left, up, right, down
			say("\n")
			return [...]string{HotLeft, HotUp, HotRight, HotDown}[vk-0x25]
		case ch == '\r':
			say("\n")
			return string(line)
		case ch == '\b':
			if len(line) > 0 {
				line = line[:len(line)-1]
				say("\b \b")
			}
		case ch >= ' ':
			line = append(line, ch)
			say("%c", ch)
		}
	}
}

// ==========================
// ===  Serial port       ===
// ==========================

type dcb struct {
	DCBlength  uint32
	BaudRate   uint32
	Flags      uint32
	wReserved  uint16
	XonLim     uint16
	XoffLim    uint16
	ByteSize   byte
	Parity     byte
	StopBits   byte
	XonChar    byte
	XoffChar   byte
	ErrorChar  byte
	EofChar    byte
	EvtChar    byte
	wReserved1 uint16
}

type commTimeouts struct {
	ReadIntervalTimeout         uint32
	ReadTotalTimeoutMultiplier  uint32
	ReadTotalTimeoutConstant    uint32
	WriteTotalTimeoutMultiplier uint32
	WriteTotalTimeoutConstant   uint32
}

type Port struct{ h syscall.Handle }

func OpenPort(name string) (*Port, error) {
	path, _ := syscall.UTF16PtrFromString(`\\.\` + name)
	h, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, err
	}
	var d dcb
	d.DCBlength = uint32(unsafe.Sizeof(d))
	pGetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d)))
	d.BaudRate = 115200
	d.ByteSize, d.Parity, d.StopBits = 8, 0, 0
	d.Flags = 1 | 1<<4 | 1<<12 // binary, DTR on, RTS on
	if r, _, e := pSetCommState.Call(uintptr(h), uintptr(unsafe.Pointer(&d))); r == 0 {
		syscall.CloseHandle(h)
		return nil, e
	}
	// return as soon as a byte arrives, or after 50 ms
	t := commTimeouts{0xFFFFFFFF, 0xFFFFFFFF, 100, 0, 500}
	pSetCommTimeouts.Call(uintptr(h), uintptr(unsafe.Pointer(&t)))
	pPurgeComm.Call(uintptr(h), 0xF)
	return &Port{h}, nil
}

func (p *Port) Read(b []byte) (int, error) {
	var n uint32
	err := syscall.ReadFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *Port) Write(b []byte) (int, error) {
	var n uint32
	err := syscall.WriteFile(p.h, b, &n, nil)
	return int(n), err
}

func (p *Port) Close() error { return syscall.CloseHandle(p.h) }

// ComPorts lists HKLM\HARDWARE\DEVICEMAP\SERIALCOMM.
func ComPorts() []string {
	key, _ := syscall.UTF16PtrFromString(`HARDWARE\DEVICEMAP\SERIALCOMM`)
	var hk syscall.Handle
	if r, _, _ := pRegOpenKeyExW.Call(uintptr(0x80000002), uintptr(unsafe.Pointer(key)), 0, 0x20019,
		uintptr(unsafe.Pointer(&hk))); r != 0 {
		return nil
	}
	defer pRegCloseKey.Call(uintptr(hk))
	var out []string
	for i := 0; ; i++ {
		name := make([]uint16, 256)
		data := make([]uint16, 64)
		nl, dl := uint32(len(name)), uint32(len(data)*2)
		var typ uint32
		r, _, _ := pRegEnumValueW.Call(uintptr(hk), uintptr(i), uintptr(unsafe.Pointer(&name[0])),
			uintptr(unsafe.Pointer(&nl)), 0, uintptr(unsafe.Pointer(&typ)),
			uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&dl)))
		if r != 0 {
			break
		}
		out = append(out, syscall.UTF16ToString(data))
	}
	return out
}

// ==========================
// ===  Keystrokes        ===
// ==========================

type keybdInput struct {
	Vk, Scan uint16
	Flags    uint32
	Time     uint32
	Extra    uintptr
}

// input matches the C INPUT union size: 28 bytes on 386, 40 on amd64
type input struct {
	Type uint32
	Ki   keybdInput
	pad  [8]byte
}

const (
	keyUp      = 0x2
	keyUnicode = 0x4
)

var vkNames = map[string]uint16{"ctrl": 0x11, "shift": 0x10, "alt": 0x12, "enter": 0x0D, "esc": 0x1B,
	"tab": 0x09, "space": 0x20, "left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28,
	"home": 0x24, "end": 0x23, "del": 0x2E, "backspace": 0x08,
	"f1": 0x70, "f2": 0x71, "f3": 0x72, "f4": 0x73, "f5": 0x74, "f6": 0x75, "f7": 0x76, "f8": 0x77,
	"f9": 0x78, "f10": 0x79, "f11": 0x7A, "f12": 0x7B}

type WinKeys struct {
	Title  string
	MDIKey string
	hwnd   uintptr
	Delay  time.Duration
}

func sendKey(vk, scan uint16, flags uint32) {
	in := input{Type: 1, Ki: keybdInput{Vk: vk, Scan: scan, Flags: flags}}
	pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func (k *WinKeys) find() error {
	var found uintptr
	cb := syscall.NewCallback(func(h, _ uintptr) uintptr {
		if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 256)
		if strings.Contains(strings.ToLower(syscall.UTF16ToString(buf)), strings.ToLower(k.Title)) {
			found = h
			return 0
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	if found == 0 {
		return fmt.Errorf("окно %s не найдено: запустите NcStudio", k.Title)
	}
	k.hwnd = found
	return nil
}

func pidOf(h uintptr) uint32 {
	var pid uint32
	pGetWindowThreadPid.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// focused: any window of the NcStudio process counts, its MDI dialog included.
func (k *WinKeys) focused() bool {
	fg, _, _ := pGetForeground.Call()
	if fg == 0 || k.hwnd == 0 {
		return false
	}
	root, _, _ := pGetAncestor.Call(fg, 3)
	return fg == k.hwnd || root == k.hwnd || pidOf(fg) == pidOf(k.hwnd)
}

// focus only checks: once an operation has started, the operator clicking any
// other window is the emergency stop, so focus is never taken back silently.
func (k *WinKeys) focus() error {
	if !k.focused() {
		return fmt.Errorf("остановлено: окно NcStudio потеряло фокус (щелчок мышью или другое окно)")
	}
	return nil
}

// backToConsole returns focus to this program after a manual-mode move, so the
// next arrow or PageUp lands here; keys pressed meanwhile (autorepeat) are dropped.
func backToConsole() {
	w, _, _ := pGetConsoleWindow.Call()
	if fg, _, _ := pGetForeground.Call(); w == 0 || fg == w {
		return // already here: keep whatever the operator typed ahead
	}
	sendKey(vkNames["alt"], 0, 0)
	sendKey(vkNames["alt"], 0, keyUp)
	pSetForeground.Call(w)
	time.Sleep(100 * time.Millisecond)
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF6))
	pFlushConsoleInput.Call(h)
}

// Grab brings NcStudio forward at the start of an operation, after the operator answered.
func (k *WinKeys) Grab() error {
	if k.hwnd == 0 {
		if err := k.find(); err != nil {
			return err
		}
	}
	if k.focused() {
		return nil
	}
	// Windows only hands focus over right after a key event from this process
	sendKey(vkNames["alt"], 0, 0)
	sendKey(vkNames["alt"], 0, keyUp)
	pShowWindow.Call(k.hwnd, 9) // SW_RESTORE
	pSetForeground.Call(k.hwnd)
	time.Sleep(250 * time.Millisecond)
	if !k.focused() {
		return fmt.Errorf("NcStudio не на переднем плане: не трогайте мышь и клавиатуру во время работы")
	}
	return nil
}

func vkOf(name string) uint16 {
	if v, ok := vkNames[name]; ok {
		return v
	}
	if len(name) == 1 {
		c := strings.ToUpper(name)[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint16(c)
		}
	}
	return 0
}

// ValidKeys reports a key name the program does not know, before anything is pressed.
func ValidKeys(seq string) error {
	for _, step := range strings.Fields(seq) {
		for _, p := range strings.Split(strings.ToLower(step), "+") {
			if vkOf(p) == 0 {
				return fmt.Errorf("неизвестная клавиша %q в %q", p, seq)
			}
		}
	}
	return nil
}

func (k *WinKeys) combo(keys string) {
	var vks []uint16
	for _, p := range strings.Split(strings.ToLower(keys), "+") {
		vks = append(vks, vkOf(p))
	}
	for _, v := range vks {
		sendKey(v, 0, 0)
	}
	for i := len(vks) - 1; i >= 0; i-- {
		sendKey(vks[i], 0, keyUp)
	}
}

func (k *WinKeys) OpenMDI() error {
	if err := k.focus(); err != nil {
		return err
	}
	// "ctrl+shift+f9" is one chord; "alt o e" is a menu walk, pressed one after another
	for _, step := range strings.Fields(k.MDIKey) {
		k.combo(step)
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	return k.focus()
}

func (k *WinKeys) Line(s string) error {
	if err := k.focus(); err != nil {
		return err
	}
	// unicode packets: the Russian layout cannot turn G into П
	for _, r := range s {
		if !k.focused() {
			return k.focus()
		}
		sendKey(0, uint16(r), keyUnicode)
		sendKey(0, uint16(r), keyUnicode|keyUp)
		time.Sleep(k.Delay)
	}
	k.combo("enter")
	return nil
}

// ==========================
// ===  NcStudio windows  ===
// ==========================

// WinInfo is one window of the NcStudio process as another program sees it.
type WinInfo struct {
	H                      uintptr
	Class, Text            string
	ID                     int
	Left, Top, Right, Bott int32
}

// windowText asks the window itself (WM_GETTEXT): GetWindowText does not reach
// controls of another process. A hung window gives up after 200 ms.
func windowText(h uintptr) string {
	buf := make([]uint16, 512)
	var res uintptr
	r, _, _ := pSendMessageTimeout.Call(h, 0x000D, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])),
		0x0002, 200, uintptr(unsafe.Pointer(&res))) // WM_GETTEXT, SMTO_ABORTIFHUNG
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// NcWindows lists every window and control of the NcStudio process with its text.
func NcWindows(title string) ([]WinInfo, error) {
	k := &WinKeys{Title: title}
	if err := k.find(); err != nil {
		return nil, err
	}
	var pid uint32
	pGetWindowThreadPid.Call(k.hwnd, uintptr(unsafe.Pointer(&pid)))
	var out []WinInfo
	add := func(h uintptr) {
		var w WinInfo
		w.H = h
		cls := make([]uint16, 128)
		pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&cls[0])), 128)
		w.Class = syscall.UTF16ToString(cls)
		id, _, _ := pGetDlgCtrlID.Call(h)
		w.ID = int(int32(id))
		var rc [4]int32
		pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&rc[0])))
		w.Left, w.Top, w.Right, w.Bott = rc[0], rc[1], rc[2], rc[3]
		w.Text = windowText(h)
		out = append(out, w)
	}
	child := syscall.NewCallback(func(h, _ uintptr) uintptr { add(h); return 1 })
	var tops []uintptr
	top := syscall.NewCallback(func(h, _ uintptr) uintptr {
		var p uint32
		pGetWindowThreadPid.Call(h, uintptr(unsafe.Pointer(&p)))
		if p == pid {
			tops = append(tops, h)
		}
		return 1
	})
	pEnumWindows.Call(top, 0)
	for _, h := range tops {
		add(h)
		pEnumChildWindows.Call(h, child, 0) // all descendants, not only direct children
	}
	return out, nil
}
