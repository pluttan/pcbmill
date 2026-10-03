//go:build !windows

package main

// Non-Windows stand-ins so the logic builds and tests on Linux.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// say drops the colour marks; PCBSCAN_ANSI=1 shows them as terminal colours.
func say(f string, a ...interface{}) {
	s := fmt.Sprintf(f, a...)
	if os.Getenv("PCBSCAN_ANSI") == "" {
		fmt.Print(strip(s))
		return
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != esc || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if s[i] == '0' {
			b.WriteString("\x1b[0m")
			continue
		}
		c := int(s[i] - 'A')
		// console order is blue-green-red, ANSI is red-green-blue
		fg := [8]int{0, 4, 2, 6, 1, 5, 3, 7}[c&7] + 30
		if c >= 8 {
			fg += 60
		}
		fmt.Fprintf(&b, "\x1b[%dm", fg)
	}
	fmt.Print(b.String())
}

func conWidth() int { return 80 }

func layoutWindows(string) {}

func ncFound(string) bool { return false }

func ComPorts() []string { return nil }

type Port struct{ io.ReadWriteCloser }

func OpenPort(name string) (*Port, error) { return nil, errors.New("только Windows") }

type WinKeys struct {
	Title, MDIKey string
	Delay         interface{}
}

func (k *WinKeys) Grab() error       { return errors.New("только Windows") }
func (k *WinKeys) OpenMDI() error    { return errors.New("только Windows") }
func (k *WinKeys) Line(string) error { return errors.New("только Windows") }

func ValidKeys(string) error { return nil }

var stdinBuf = bufio.NewReader(os.Stdin)

func readLine() string { s, _ := stdinBuf.ReadString('\n'); return s }

func backToConsole() {}

type WinInfo struct {
	H                      uintptr
	Class, Text            string
	ID                     int
	Left, Top, Right, Bott int32
}

func NcWindows(string) ([]WinInfo, error) { return nil, errors.New("только Windows") }
