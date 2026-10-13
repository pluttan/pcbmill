package main

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProbe: the port of a probe that answers "?" and, if up is set, "U".
type fakeProbe struct {
	mu  sync.Mutex
	out []byte
	up  string
}

func (f *fakeProbe) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range b {
		switch {
		case c == '?':
			f.out = append(f.out, "S0\n"...)
		case c == 'U' && f.up != "":
			f.out = append(f.out, f.up...)
		}
	}
	return len(b), nil
}

func (f *fakeProbe) Read(b []byte) (int, error) {
	time.Sleep(5 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	n := copy(b, f.out)
	f.out = f.out[n:]
	return n, nil
}

func (f *fakeProbe) Close() error { return nil }

var _ io.ReadWriteCloser = (*fakeProbe)(nil)

// TestBackLine: after a drop the journal tells a restart of the probe (and
// why) from a drop of USB alone; an old firmware without U says neither.
func TestBackLine(t *testing.T) {
	for _, c := range []struct{ up, want string }{
		{"UP 1500 9\n", "ПЕРЕЗАПУСКАЛСЯ (провал питания)"},
		{"UP 3600000 1\n", "оборвался только USB"},
		{"", "через 2 с"},
	} {
		m := NewMCU(&fakeProbe{up: c.up})
		start := time.Now()
		s := backLine(m, "COM5", 2*time.Second)
		if !strings.Contains(s, c.want) {
			t.Errorf("%q: %q, а надо %q", c.up, s, c.want)
		}
		if c.up == "" && time.Since(start) > 2*time.Second {
			t.Errorf("старая прошивка: ждал ответа %.1f с", time.Since(start).Seconds())
		}
		m.port.Close()
	}
}
