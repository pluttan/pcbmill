package main

// The probe drops off USB now and then (a reset, a loose cable, Windows
// renumbering the port). The work does not end for that: the machine stands
// where it is, pcbscan opens the port again until the MCU answers, and the
// step goes on. Only the Стоп button of the page ends the wait.

import (
	"fmt"
	"sync"
	"time"
)

// reTouch: the touch of the work, the same across reconnections; the count
// of rising edges goes on from where the old MCU left it.
type reTouch struct {
	mu   sync.Mutex
	t    Touch
	port string
	p    *page
	base int // the edges of the MCUs before this one
	last int // the last count read from this one
	find func(port string) (Touch, error)
}

func newReTouch(t Touch, port string, p *page) Touch {
	if simM != nil {
		return t // the emulation has its own faults to show
	}
	return &reTouch{t: t, port: port, p: p, find: func(port string) (Touch, error) {
		m, _, err := findMCU(port)
		if err != nil {
			return nil, err
		}
		return m, nil
	}}
}

func (r *reTouch) cur() Touch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

// Alive waits through a lost MCU: once a second it looks for the port again.
func (r *reTouch) Alive() error {
	err := r.cur().Alive()
	if err == nil {
		return nil
	}
	logLine("▲ МК не отвечает: станок стоит, жду, когда щуп снова ответит. Стоп завершает работу.")
	start := time.Now()
	for {
		if r.p != nil {
			if id, ok := r.p.pressed(); ok && (id == "stop" || id == "cancel") {
				r.p.send(id) // the step that asked sees it too
				return fail("остановлено, пока МК не отвечал")
			}
			r.p.set(func() { r.p.progText = fmt.Sprintf("жду МК: %.0f с", time.Since(start).Seconds()) })
		}
		time.Sleep(time.Second)
		if r.cur().Alive() == nil { // came back by itself
			break
		}
		if t, e := r.find(r.port); e == nil && t.Alive() == nil {
			r.mu.Lock()
			r.base += r.last
			r.last = 0
			r.t = t
			r.mu.Unlock()
			break
		}
	}
	logLine(fmt.Sprintf("√ МК снова на связи через %.0f с, продолжаю.", time.Since(start).Seconds()))
	return nil
}

func (r *reTouch) State() bool { return r.cur().State() }

func (r *reTouch) Hits() int {
	h := r.cur().Hits()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last = h
	return r.base + h
}
