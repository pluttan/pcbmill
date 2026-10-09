package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// espPort answers like firmware/touch on the ESP32: ?, L, R<n>, N; reads
// return what there is or nothing after a while, like the Windows driver.
type espPort struct {
	mu      sync.Mutex
	pending []byte
	files   []espFile
	rd      []byte
	rAt     int
	waitR   bool
	closed  bool
	touch   bool
	breakAt int // corrupt the byte at this offset of every read, 0: never
}

type espFile struct {
	name string
	data []byte
}

func (p *espPort) Read(b []byte) (int, error) {
	for i := 0; i < 10; i++ {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return 0, errors.New("closed")
		}
		if len(p.pending) > 0 {
			n := copy(b, p.pending)
			p.pending = p.pending[n:]
			p.mu.Unlock()
			return n, nil
		}
		p.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	return 0, nil
}

func (p *espPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if p.waitR {
			p.waitR = false
			p.rd, p.rAt = append([]byte(nil), p.files[c-'0'].data...), 0
			if p.breakAt > 0 && p.breakAt < len(p.rd) {
				p.rd[p.breakAt] ^= 0xFF
			}
			continue
		}
		switch c {
		case '?':
			if p.touch {
				p.pending = append(p.pending, "S1\n"...)
			} else {
				p.pending = append(p.pending, "S0\n"...)
			}
		case 'L':
			p.touch = true
			p.pending = append(p.pending, "T1\n"...) // a touch lands in the middle
			for i, f := range p.files {
				s := md5.Sum(f.data)
				p.pending = append(p.pending, fmt.Sprintf("E %d %d %s %s\n", i, len(f.data), hex.EncodeToString(s[:]), f.name)...)
			}
			p.pending = append(p.pending, "END\n"...)
		case 'R':
			p.waitR = true
		case 'N':
			k := len(p.rd) - p.rAt
			if k > fileChunk {
				k = fileChunk
			}
			p.pending = append(p.pending, p.rd[p.rAt:p.rAt+k]...)
			p.rAt += k
		}
	}
	return len(b), nil
}

func (p *espPort) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

func gz(b []byte) []byte {
	var w bytes.Buffer
	z := gzip.NewWriter(&w)
	z.Write(b)
	z.Close()
	return w.Bytes()
}

// TestInbox: the files of the probe come whole, ungzipped, once each; a
// touch report in the middle of the list goes to the state.
func TestInbox(t *testing.T) {
	board := bytes.Repeat([]byte("gerber "), 3000) // several chunks
	prog := bytes.Repeat([]byte{0x4d, 0x5a, 0, 1}, 5000)
	port := &espPort{files: []espFile{{"test board.zip.gz", gz(board)}, {"pcbscan.exe.gz", gz(prog)}, {"notes.txt", []byte("raw")}}}
	m := NewMCU(port)
	defer m.Close()
	dir := t.TempDir()
	boards, program, err := takeInbox(m, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.State() {
		t.Error("касание посреди списка потерялось")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "test board.zip"))
	if !bytes.Equal(got, board) {
		t.Errorf("плата пришла %d байт, ждали %d", len(got), len(board))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "notes.txt")); string(b) != "raw" {
		t.Errorf("файл без gzip: %q", b)
	}
	exe := "pcbscan.exe"
	if isProgram(exe) {
		exe = newExe
	}
	if b, _ := os.ReadFile(filepath.Join(dir, exe)); !bytes.Equal(b, prog) {
		t.Errorf("программа пришла %d байт в %s", len(b), exe)
	}
	if isProgram("pcbscan.exe") != program {
		t.Errorf("программа: %v", program)
	}
	t.Logf("платы %v, программа %v", boards, program)
	again, program2, err := takeInbox(m, dir, nil)
	if err != nil || len(again) > 0 || program2 {
		t.Errorf("второй раз пришло снова: %v %v %v", again, program2, err)
	}
	// a broken byte: md5 catches it, nothing is written
	port.mu.Lock()
	port.files = append(port.files, espFile{"bad.zip", bytes.Repeat([]byte("x"), 9000)})
	port.breakAt = 5000
	port.mu.Unlock()
	if _, _, err := takeInbox(m, dir, nil); err == nil {
		t.Error("испорченный файл прошёл")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.zip")); err == nil {
		t.Error("испорченный файл записан")
	}
}
