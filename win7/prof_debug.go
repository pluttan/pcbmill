//go:build prof

package main

// A CPU profile of the running program: go build -tags prof, then
// PCBSCAN_CPUPROF=file writes the first PCBSCAN_CPUPROF_S seconds (default 20).

import (
	"os"
	"runtime/pprof"
	"strconv"
	"time"
)

func init() {
	out := os.Getenv("PCBSCAN_CPUPROF")
	if out == "" {
		return
	}
	f, err := os.Create(out)
	if err != nil {
		return
	}
	s, _ := strconv.Atoi(os.Getenv("PCBSCAN_CPUPROF_S"))
	if s <= 0 {
		s = 20
	}
	pprof.StartCPUProfile(f)
	time.AfterFunc(time.Duration(s)*time.Second, func() { pprof.StopCPUProfile(); f.Close() })
}
