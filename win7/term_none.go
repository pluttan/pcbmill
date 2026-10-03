//go:build !windows && !linux && !darwin

package main

// Other systems: no raw mode, frames are printed one after another.

import (
	"fmt"
	"os"
)

func conStart(keys chan Key) { go readBytes(os.Stdin, keys) }
func conStop()               {}
func conSize() (int, int)    { return 80, 25 }
func conClear()              {}
func conLine(row int, s string) {
	fmt.Println(strip(s))
}
func conDone()       {}
func conFlushInput() {}
