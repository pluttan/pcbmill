package main

import (
	"errors"
	"testing"
)

type fakeTouch struct {
	dead bool
	hits int
}

func (f *fakeTouch) Alive() error {
	if f.dead {
		return errors.New("МК не отвечает")
	}
	return nil
}
func (f *fakeTouch) State() bool { return false }
func (f *fakeTouch) Hits() int   { return f.hits }

// TestReTouch: a lost MCU is waited for and replaced by the one found again;
// the count of edges does not go back, so travel sees no false touch.
func TestReTouch(t *testing.T) {
	old := &fakeTouch{hits: 7}
	fresh := &fakeTouch{hits: 0}
	tries := 0
	r := &reTouch{t: old, find: func(string) (Touch, error) {
		if tries++; tries < 2 {
			return nil, errors.New("порта нет")
		}
		return fresh, nil
	}}
	before := r.Hits()
	old.dead = true
	if err := r.Alive(); err != nil {
		t.Fatal(err)
	}
	if r.cur() != fresh || tries != 2 {
		t.Fatalf("МК не заменён: попыток %d", tries)
	}
	if r.Hits() != before {
		t.Errorf("касаний было %d, стало %d", before, r.Hits())
	}
	fresh.hits = 1
	if r.Hits() != before+1 {
		t.Errorf("новое касание не видно: %d", r.Hits())
	}
}
