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

// readLine through ReadConsoleW: Russian letters arrive as letters, not cp866 bytes.
func readLine() string {
	h, _, _ := pGetStdHandle.Call(uintptr(0xFFFFFFF6)) // STD_INPUT_HANDLE
	buf := make([]uint16, 512)
	var n uint32
	r, _, _ := pReadConsoleW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)), 0)
	if r == 0 {
		s, _ := stdinBuf.ReadString('\n')
		return s
	}
	return strings.TrimRight(string(utf16.Decode(buf[:n])), "\r\n")
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
