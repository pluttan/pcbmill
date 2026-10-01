package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// blockingPort behaves like the Windows 7 usbser driver: Read waits until data
// exists, and Write waits while a Read is in progress (synchronous I/O on one handle).
type blockingPort struct {
	mu      sync.Mutex
	io      sync.Mutex // one I/O at a time, like a non-overlapped handle
	cond    *sync.Cond
	pending []byte
	touch   bool
	closed  bool
}

func newBlockingPort() *blockingPort {
	p := &blockingPort{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *blockingPort) Read(b []byte) (int, error) {
	p.io.Lock()
	defer p.io.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.pending) == 0 && !p.closed {
		p.cond.Wait()
	}
	if p.closed {
		return 0, errors.New("closed")
	}
	n := copy(b, p.pending)
	p.pending = p.pending[n:]
	return n, nil
}

func (p *blockingPort) Write(b []byte) (int, error) {
	p.io.Lock()
	defer p.io.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if c == '?' {
			if p.touch {
				p.pending = append(p.pending, "S1\n"...)
			} else {
				p.pending = append(p.pending, "S0\n"...)
			}
		}
	}
	p.cond.Broadcast()
	return len(b), nil
}

func (p *blockingPort) Close() error {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
	return nil
}

func TestMCUStaysAliveWithBlockingDriver(t *testing.T) {
	p := newBlockingPort()
	m := NewMCU(p)
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if err := m.Alive(); err != nil {
			t.Fatalf("через %d мс: %v", (i+1)*100, err)
		}
	}
	p.mu.Lock()
	p.touch = true
	p.mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	if !m.State() || m.Hits() != 1 {
		t.Errorf("касание не увидено: state=%v hits=%d", m.State(), m.Hits())
	}
}
