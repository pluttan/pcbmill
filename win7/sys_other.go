//go:build !windows

package main

// Non-Windows stand-ins so the logic builds and tests on Linux.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

func say(f string, a ...interface{}) { fmt.Printf(f, a...) }

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
