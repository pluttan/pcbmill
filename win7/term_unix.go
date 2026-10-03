//go:build linux || darwin

package main

// The full screen in a Unix terminal (to look at the interface on a Mac or
// Linux): raw mode through ioctl, the alternate screen, ANSI colours.

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	savedTermios syscall.Termios
	rawMode      bool
	outBuf       bytes.Buffer
)

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// conStart: keys one by one without echo; Ctrl+C stays a signal, so the
// terminal is restored on it (conTerm.start catches it).
func conStart(keys chan Key) {
	if ioctl(0, ioctlGetTermios, unsafe.Pointer(&savedTermios)) == nil {
		raw := savedTermios
		raw.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.INLCR | syscall.IGNCR
		raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.IEXTEN
		raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 1, 0
		if ioctl(0, ioctlSetTermios, unsafe.Pointer(&raw)) == nil {
			rawMode = true
		}
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l\x1b[0m\x1b[2J")
	go readBytes(os.Stdin, keys)
	win := make(chan os.Signal, 1)
	signal.Notify(win, syscall.SIGWINCH)
	go func() {
		for range win {
			select {
			case keys <- Key{Code: KeyResize}:
			default:
			}
		}
	}()
}

func conStop() {
	os.Stdout.WriteString("\x1b[0m\x1b[?25h\x1b[?1049l")
	if rawMode {
		ioctl(0, ioctlSetTermios, unsafe.Pointer(&savedTermios))
		rawMode = false
	}
}

// conSize from TIOCGWINSZ; COLUMNS and LINES, or 80x25, when stdout is not a terminal.
func conSize() (int, int) {
	var ws struct{ Row, Col, X, Y uint16 }
	if ioctl(1, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)) == nil && ws.Col > 0 && ws.Row > 0 {
		return int(ws.Col), int(ws.Row)
	}
	w, _ := strconv.Atoi(os.Getenv("COLUMNS"))
	h, _ := strconv.Atoi(os.Getenv("LINES"))
	if w <= 0 || h <= 0 {
		return 80, 25
	}
	return w, h
}

func conClear() { outBuf.WriteString("\x1b[0m\x1b[2J") }

func conLine(row int, s string) {
	fmt.Fprintf(&outBuf, "\x1b[%d;1H%s\x1b[0m\x1b[K", row+1, ansiOf(s))
}

func conDone() {
	os.Stdout.Write(outBuf.Bytes())
	outBuf.Reset()
}

// conFlushInput: keys still in the terminal are read into the channel and
// dropped there; the terminal side needs nothing.
func conFlushInput() {}

// Terminals on a Mac or Linux have the rounded corners; the raster font of a
// Windows 7 console does not (cp866), so there the corners stay square.
func init() { outer = frame{"╭", "╮", "╰", "╯", "─", "│", "├", "┤"} }
