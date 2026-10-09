package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"testing"
)

// romPort answers like the ROM loader of the C3 once the reset sequence came.
type romPort struct {
	mu      sync.Mutex
	pending []byte
	in      []byte
	flash   []byte
	loader  bool
	dtr     bool
	ops     []byte
}

func (p *romPort) SetDTR(on bool) error { p.mu.Lock(); p.dtr = on; p.mu.Unlock(); return nil }
func (p *romPort) SetRTS(on bool) error {
	p.mu.Lock()
	if on && !p.dtr {
		p.loader = true // reset with the line low: our sequence ends in the loader
	}
	p.mu.Unlock()
	return nil
}
func (p *romPort) Close() error { return nil }

func (p *romPort) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := copy(b, p.pending)
	p.pending = p.pending[n:]
	return n, nil
}

func (p *romPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.in = append(p.in, b...)
	for {
		i := bytes.IndexByte(p.in, 0xC0)
		if i < 0 {
			break
		}
		j := bytes.IndexByte(p.in[i+1:], 0xC0)
		if j < 0 {
			break
		}
		raw := p.in[i+1 : i+1+j]
		p.in = p.in[i+2+j:]
		raw = bytes.ReplaceAll(bytes.ReplaceAll(raw, []byte{0xDB, 0xDC}, []byte{0xC0}), []byte{0xDB, 0xDD}, []byte{0xDB})
		if p.loader && len(raw) >= 8 {
			p.answer(raw[1], raw[8:])
		}
	}
	return len(b), nil
}

func (p *romPort) answer(op byte, d []byte) {
	p.ops = append(p.ops, op)
	var body []byte
	switch op {
	case espFlashData:
		seq := binary.LittleEndian.Uint32(d[4:])
		copy(p.flash[seq*espBlock:], d[16:])
	case espFlashMD5:
		addr, size := binary.LittleEndian.Uint32(d), binary.LittleEndian.Uint32(d[4:])
		s := md5.Sum(p.flash[addr : addr+size])
		body = []byte(hex.EncodeToString(s[:]))
	}
	f := make([]byte, 8)
	f[0], f[1] = 1, op
	f = append(f, body...)
	f = append(f, 0, 0, 0, 0)
	binary.LittleEndian.PutUint16(f[2:], uint16(len(f)-8))
	p.pending = append(p.pending, slip(f)...)
}

// TestFlashProbe: the image lands in the flash whole, from 0x0, checked by MD5.
func TestFlashProbe(t *testing.T) {
	img := make([]byte, 3*espBlock+123)
	for i := range img {
		img[i] = byte(i*7) ^ 0xC0 // SLIP bytes inside the data
	}
	port := &romPort{flash: bytes.Repeat([]byte{0xFF}, 1<<20)}
	err := flashProbe(func() (linePort, error) { return port, nil }, img, func(float64, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(port.flash[:len(img)], img) {
		t.Error("во флеше не тот образ")
	}
	if port.flash[len(img)] != 0xFF {
		t.Error("хвост блока не 0xFF")
	}
	if !bytes.Contains(port.ops, []byte{espFlashBegin}) || port.ops[0] != espSync {
		t.Errorf("команды %x", port.ops)
	}
	if len(probeImage) < 0x10000 || len(probeImage) > espStore {
		t.Errorf("встроенный образ %d байт", len(probeImage))
	}
}
