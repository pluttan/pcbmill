package main

// Files from the touch probe. The PC of the machine has no network and no
// flash drive; the ESP32 of the probe keeps files put on it over Wi-Fi
// (firmware/touch) and gives them out over its COM port. pcbscan looks at the
// list now and then, takes the new ones: boards go next to the exe, a new
// pcbscan.exe replaces the program after a yes.

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ==========================
// ===  Jobs on the port  ===
// ==========================

// mcuJob runs in the goroutine of the MCU between two polls: the Windows 7
// driver takes reads and writes only from one goroutine.
type mcuJob struct {
	fn   func(*mcuIO) error
	done chan error
}

// do runs fn on the port and waits for it.
func (m *MCU) do(fn func(*mcuIO) error) error {
	j := mcuJob{fn, make(chan error, 1)}
	select {
	case m.jobs <- j:
	case <-time.After(2 * time.Second):
		return errors.New("МК занят")
	}
	return <-j.done
}

// runJob: called by run between polls.
func (m *MCU) runJob() {
	select {
	case j := <-m.jobs:
		j.done <- j.fn(&mcuIO{m: m})
	default:
	}
}

// mcuIO reads lines and raw bytes; touch lines on the way go to the state.
type mcuIO struct {
	m    *MCU
	pend []byte
	buf  [512]byte
}

func (io *mcuIO) write(s string) error {
	_, err := io.m.port.Write([]byte(s))
	return err
}

func (io *mcuIO) fill(end time.Time) error {
	if time.Now().After(end) {
		return errors.New("МК не ответил")
	}
	n, err := io.m.port.Read(io.buf[:])
	if err != nil {
		return err
	}
	io.pend = append(io.pend, io.buf[:n]...)
	return nil
}

// line: the next line that is not a touch report.
func (io *mcuIO) line(wait time.Duration) (string, error) {
	end := time.Now().Add(wait)
	for {
		if i := bytes.IndexByte(io.pend, '\n'); i >= 0 {
			s := strings.TrimRight(string(io.pend[:i]), "\r")
			io.pend = io.pend[i+1:]
			io.m.mu.Lock()
			io.m.lastSeen = time.Now()
			io.m.mu.Unlock()
			switch s {
			case "T1", "T0", "S1", "S0", "PCBTOUCH 1", "":
				io.m.handle(s)
				continue
			}
			return s, nil
		}
		if err := io.fill(end); err != nil {
			return "", err
		}
	}
}

// bytes: exactly n raw bytes.
func (io *mcuIO) bytes(n int, wait time.Duration) ([]byte, error) {
	end := time.Now().Add(wait)
	for len(io.pend) < n {
		if err := io.fill(end); err != nil {
			return nil, err
		}
	}
	b := io.pend[:n:n]
	io.pend = io.pend[n:]
	return b, nil
}

// ==========================
// ===  Files in the probe ===
// ==========================

type probeFile struct {
	n    int
	size int
	md5  string
	name string
}

const fileChunk = 4096

// probeFiles: the list the probe keeps.
func (m *MCU) probeFiles() ([]probeFile, error) {
	var out []probeFile
	err := m.do(func(io *mcuIO) error {
		if err := io.write("L"); err != nil {
			return err
		}
		for {
			s, err := io.line(time.Second)
			if err != nil {
				return err
			}
			if s == "END" {
				return nil
			}
			f := strings.SplitN(s, " ", 5)
			if len(f) < 5 || f[0] != "E" {
				continue
			}
			n, e1 := strconv.Atoi(f[1])
			size, e2 := strconv.Atoi(f[2])
			if e1 == nil && e2 == nil {
				out = append(out, probeFile{n, size, f[3], f[4]})
			}
		}
	})
	return out, err
}

// fetch reads a file of the probe and checks its md5; stop says to give up
// (the operator started work that needs the probe).
func (m *MCU) fetch(f probeFile, stop func() bool) ([]byte, error) {
	data := make([]byte, 0, f.size)
	err := m.do(func(io *mcuIO) error {
		if err := io.write("R" + string(rune('0'+f.n))); err != nil {
			return err
		}
		for len(data) < f.size {
			if stop != nil && stop() {
				return errors.New("прервано")
			}
			if err := io.write("N"); err != nil {
				return err
			}
			k := f.size - len(data)
			if k > fileChunk {
				k = fileChunk
			}
			b, err := io.bytes(k, 3*time.Second)
			if err != nil {
				return err
			}
			data = append(data, b...)
			io.m.mu.Lock()
			io.m.lastSeen = time.Now() // a long file is no silence
			io.m.mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sum := md5.Sum(data)
	if hex.EncodeToString(sum[:]) != f.md5 {
		return nil, fmt.Errorf("%s пришёл с ошибкой (md5)", f.name)
	}
	return data, nil
}

// ==========================
// ===  Taking them in    ===
// ==========================

// inboxBusy is set while the operator's work runs: the probe is not to wait
// for a file then.
var inboxBusy atomic.Bool

// inboxSeen: the md5 of every file taken, in inbox.txt next to the exe.
func inboxSeen(dir string) map[string]bool {
	seen := map[string]bool{}
	b, _ := os.ReadFile(filepath.Join(dir, "inbox.txt"))
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			seen[f[0]] = true
		}
	}
	return seen
}

func inboxMark(dir string, f probeFile) {
	o, err := os.OpenFile(filepath.Join(dir, "inbox.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(o, "%s %s %s\r\n", f.md5, time.Now().Format("2006-01-02 15:04"), f.name)
	o.Close()
}

// unpack: the name to keep and the content, ungzipped when the page gzipped it.
func unpack(name string, data []byte) (string, []byte, error) {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if !strings.HasSuffix(strings.ToLower(name), ".gz") {
		return name, data, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", nil, err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return "", nil, err
	}
	return name[:len(name)-3], out, nil
}

// isProgram: the file replaces pcbscan itself.
func isProgram(name string) bool {
	return runtime.GOOS == "windows" && strings.EqualFold(name, "pcbscan.exe")
}

const newExe = "pcbscan.new.exe"

// takeInbox takes the new files of the probe into dir; says what came.
func takeInbox(m *MCU, dir string, stop func() bool) (boards []string, program bool, err error) {
	files, err := m.probeFiles()
	if err != nil {
		return nil, false, err
	}
	seen := inboxSeen(dir)
	for _, f := range files {
		if seen[f.md5] {
			continue
		}
		data, err := m.fetch(f, stop)
		if err != nil {
			return boards, program, err
		}
		name, out, err := unpack(f.name, data)
		if err != nil {
			return boards, program, fmt.Errorf("%s: %v", f.name, err)
		}
		dst := filepath.Join(dir, name)
		if isProgram(name) {
			dst = filepath.Join(dir, newExe)
		}
		if err := os.WriteFile(dst+".part", out, 0o755); err != nil {
			return boards, program, err
		}
		os.Remove(dst)
		if err := os.Rename(dst+".part", dst); err != nil {
			return boards, program, err
		}
		inboxMark(dir, f)
		seen[f.md5] = true
		if isProgram(name) {
			program = true
		} else {
			boards = append(boards, name)
		}
	}
	return boards, program, nil
}

// inboxLoop looks at the probe every few seconds while nothing else uses it.
func inboxLoop(o opts) {
	dir := exeDir()
	os.Remove(filepath.Join(dir, "pcbscan.old.exe")) // the program before the last update
	var lastFind time.Time
	for {
		time.Sleep(5 * time.Second)
		if inboxBusy.Load() {
			continue
		}
		mcuMu.Lock()
		m := mcuCache
		if m == nil && time.Since(lastFind) > 30*time.Second {
			lastFind = time.Now()
			m, _, _ = findMCUlocked(o.port)
		}
		mcuMu.Unlock()
		if m == nil || m.Alive() != nil {
			continue
		}
		boards, program, err := takeInbox(m, dir, inboxBusy.Load)
		if err != nil && len(boards) == 0 && !program {
			continue
		}
		if len(boards) > 0 {
			post(func() error {
				message("Пришли файлы со щупа", append([]string{"Они лежат рядом с pcbscan:"}, boards...)...)
				return nil
			})
		}
		if program {
			post(func() error {
				if confirm("Пришла новая версия pcbscan", "Перезапустить программу сейчас?",
					"Задание и карта высот сохранятся.") {
					return selfUpdate()
				}
				return nil
			})
		}
	}
}

// post puts an action to the work goroutine, waiting for it to be free.
func post(act func() error) {
	for mainActs == nil || inboxBusy.Load() {
		time.Sleep(time.Second)
	}
	mainActs <- act
}

// selfUpdate puts pcbscan.new.exe in place of the running exe (Windows lets
// a running exe be renamed, not overwritten) and starts it.
func selfUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	old := filepath.Join(dir, "pcbscan.old.exe")
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("pcbscan.exe не переименовать: %v", err)
	}
	if err := os.Rename(filepath.Join(dir, newExe), exe); err != nil {
		os.Rename(old, exe)
		return fmt.Errorf("новая версия не встала на место: %v", err)
	}
	mcuMu.Lock()
	if mcuCache != nil {
		mcuCache.Close() // the new program opens the port
	}
	mcuMu.Unlock()
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("новая версия не запустилась: %v", err)
	}
	os.Exit(0)
	return nil
}

// mcuMu guards the one connection to the probe: the work and the inbox both
// look for it.
var mcuMu sync.Mutex
