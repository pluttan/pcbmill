//go:build windows

// Package winui is the Windows 7 window of pcbscan: one top-level window per
// Open, frames drawn in software are shown as a DIB, input comes back as
// ui.Event. Plain syscall only, nothing to install on the target machine.
package winui

import "syscall"

// ==========================
// ===  DLL entry points  ===
// ==========================

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pGetMessageW              = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pSendMessageW             = user32.NewProc("SendMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pFillRect                 = user32.NewProc("FillRect")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pGetWindowRect            = user32.NewProc("GetWindowRect")
	pAdjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pSetWindowTextW           = user32.NewProc("SetWindowTextW")
	pGetWindowLongW           = user32.NewProc("GetWindowLongW")
	pSetWindowLongW           = user32.NewProc("SetWindowLongW")
	pIsIconic                 = user32.NewProc("IsIconic")
	pIsZoomed                 = user32.NewProc("IsZoomed")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pSetCursor                = user32.NewProc("SetCursor")
	pGetCursorPos             = user32.NewProc("GetCursorPos")
	pWindowFromPoint          = user32.NewProc("WindowFromPoint")
	pScreenToClient           = user32.NewProc("ScreenToClient")
	pSetCapture               = user32.NewProc("SetCapture")
	pReleaseCapture           = user32.NewProc("ReleaseCapture")
	pTrackMouseEvent          = user32.NewProc("TrackMouseEvent")
	pGetKeyState              = user32.NewProc("GetKeyState")
	pGetDC                    = user32.NewProc("GetDC")
	pReleaseDC                = user32.NewProc("ReleaseDC")
	pGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	pSystemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
	pCreateIconIndirect       = user32.NewProc("CreateIconIndirect")
	pDestroyIcon              = user32.NewProc("DestroyIcon")
	pMessageBoxW              = user32.NewProc("MessageBoxW")
	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pBringWindowToTop         = user32.NewProc("BringWindowToTop")
	pSetFocus                 = user32.NewProc("SetFocus")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pAttachThreadInput        = user32.NewProc("AttachThreadInput")
	pSendInput                = user32.NewProc("SendInput")
	pGetDeviceCaps            = gdi32.NewProc("GetDeviceCaps")
	pSetDIBitsToDevice        = gdi32.NewProc("SetDIBitsToDevice")
	pCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	pCreateDIBSection         = gdi32.NewProc("CreateDIBSection")
	pCreateBitmap             = gdi32.NewProc("CreateBitmap")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
)

// ==========================
// ===  Constants         ===
// ==========================

const (
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmSetFocus      = 0x0007
	wmKillFocus     = 0x0008
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmSetCursor     = 0x0020
	wmMouseActivate = 0x0021
	wmGetMinMaxInfo = 0x0024
	wmSetIcon       = 0x0080
	wmNcDestroy     = 0x0082
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmChar          = 0x0102
	wmSysKeyDown    = 0x0104
	wmSysKeyUp      = 0x0105
	wmSysChar       = 0x0106
	wmSysCommand    = 0x0112
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmRButtonDown   = 0x0204
	wmRButtonUp     = 0x0205
	wmMButtonDown   = 0x0207
	wmMButtonUp     = 0x0208
	wmMouseWheel    = 0x020A
	wmMouseLeave    = 0x02A3
	wmApp           = 0x8000

	// private messages: work that must run on the window thread
	wmDoClose    = wmApp + 1
	wmDoCursor   = wmApp + 2
	wmDoActivate = wmApp + 3

	wsOverlappedWindow = 0x00CF0000
	wsClipChildren     = 0x02000000
	wsExAppWindow      = 0x00040000
	wsExNoActivate     = 0x08000000
	gwlExStyle         = -20
	cwUseDefault       = 0x80000000

	swShow           = 5
	swRestore        = 9
	swShowNoActivate = 4
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpFrameChanged  = 0x0020

	sizeMinimized = 1
	maNoActivate  = 3
	htClient      = 1
	scKeyMenu     = 0xF100
	tmeLeave      = 0x00000002

	mkLButton = 0x0001
	mkRButton = 0x0002
	mkMButton = 0x0010

	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
	vkF4      = 0x73

	logPixelsX     = 88
	smCxIcon       = 11
	spiGetWorkArea = 0x0030
	smCyIcon       = 12
	smCxSmIcon     = 49
	smCySmIcon     = 50
	iconSmall      = 0
	iconBig        = 1
	dibRGBColors   = 0
	csHRedraw      = 0x0002
	csVRedraw      = 0x0001
	idcArrow       = 32512
	mbOK           = 0x00000000
	mbIconError    = 0x00000010
	mbTopMost      = 0x00040000
	mbSetFg        = 0x00010000
	inputKeyboard  = 1
	keyEventUp     = 0x0002

	bgColor = 0x002e1e1e // COLORREF of #1e1e2e: 0x00BBGGRR
)

// ==========================
// ===  Structures        ===
// ==========================

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

// bitmapInfo is BITMAPINFOHEADER plus the one colour slot BITMAPINFO declares.
type bitmapInfo struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	Colors        [4]byte
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Track     uintptr
	HoverTime uint32
}

type minMaxInfo struct {
	Reserved     point
	MaxSize      point
	MaxPosition  point
	MinTrackSize point
	MaxTrackSize point
}

type keybdInput struct {
	Vk, Scan uint16
	Flags    uint32
	Time     uint32
	Extra    uintptr
}

// input matches the C INPUT union size: 28 bytes on 386, 40 on amd64.
type input struct {
	Type uint32
	Ki   keybdInput
	pad  [8]byte
}

// ==========================
// ===  Small helpers     ===
// ==========================

func loWord(v uintptr) int { return int(int16(uint16(v))) }
func hiWord(v uintptr) int { return int(int16(uint16(v >> 16))) }

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil { // a NUL inside: show the text up to it
		for i := 0; i < len(s); i++ {
			if s[i] == 0 {
				p, _ = syscall.UTF16PtrFromString(s[:i])
				break
			}
		}
	}
	return p
}
