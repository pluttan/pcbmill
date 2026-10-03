//go:build !windows

package main

// Non-Windows stand-ins so the logic builds and tests on Linux.

import (
	"errors"
	"fmt"
	"io"
)

func say(f string, a ...interface{}) { fmt.Printf(f, a...) }

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
func (k *WinKeys) Load(string) error { return errors.New("только Windows") }

func ValidKeys(string) error { return nil }

type WinInfo struct {
	H                      uintptr
	Class, Text            string
	ID                     int
	Left, Top, Right, Bott int32
}

func NcWindows(string) ([]WinInfo, error) { return nil, errors.New("только Windows") }
