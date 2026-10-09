package main

// Writing the firmware of the touch probe (ESP32-C3) from pcbscan, over the
// same USB cable: the PC of the machine has nothing else to do it with. The
// talk is the one of the ROM loader of the chip, as in esptool: SLIP frames,
// a reset into the loader by DTR and RTS, then the image block by block and
// its MD5 read back. The image is firmware/touch built and merged from 0x0
// (bootloader, partitions, app); the files kept from 0x150000 stay.

import (
	"bytes"
	"crypto/md5"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

//go:embed probe/touch-c3.bin
var probeImage []byte

// linePort: a serial port whose DTR and RTS can be moved.
type linePort interface {
	io.ReadWriteCloser
	SetDTR(bool) error
	SetRTS(bool) error
}

const (
	espFlashBegin = 0x02
	espFlashData  = 0x03
	espSync       = 0x08
	espWriteReg   = 0x09
	espReadReg    = 0x0A
	espSetParams  = 0x0B
	espAttach     = 0x0D
	espFlashMD5   = 0x13

	espBlock = 0x400      // FLASH_DATA block of the ROM loader
	espFlash = 4 << 20    // the SuperMini has 4 MB
	espStore = 0x150000   // the files of the probe start here: not to be reached
	rtcCntl  = 0x60008000 // RTC_CNTL of the C3: the watchdogs
)

// espROM talks to the ROM loader over a port.
type espROM struct {
	p    linePort
	pend []byte
	buf  [256]byte
	rts  bool
}

// lines: the DTR set on Windows takes effect with the next RTS set (as esptool).
func (e *espROM) setRTS(on bool) { e.rts = on; e.p.SetRTS(on) }
func (e *espROM) setDTR(on bool) { e.p.SetDTR(on); e.p.SetRTS(e.rts) }

// resetToLoader: the sequence of the USB-Serial/JTAG of the C3 that restarts
// the chip into the ROM loader.
func (e *espROM) resetToLoader() {
	e.setRTS(false)
	e.setDTR(false)
	time.Sleep(100 * time.Millisecond)
	e.setDTR(true)
	e.setRTS(false)
	time.Sleep(100 * time.Millisecond)
	e.setRTS(true)
	e.setDTR(false)
	e.setRTS(true)
	time.Sleep(100 * time.Millisecond)
	e.setDTR(false)
	e.setRTS(false)
}

// hardReset starts the written firmware.
func (e *espROM) hardReset() {
	e.setRTS(true)
	time.Sleep(200 * time.Millisecond)
	e.setRTS(false)
	time.Sleep(200 * time.Millisecond)
}

func slip(b []byte) []byte {
	out := []byte{0xC0}
	for _, c := range b {
		switch c {
		case 0xC0:
			out = append(out, 0xDB, 0xDC)
		case 0xDB:
			out = append(out, 0xDB, 0xDD)
		default:
			out = append(out, c)
		}
	}
	return append(out, 0xC0)
}

// frame: the next whole SLIP frame, unescaped.
func (e *espROM) frame(end time.Time) ([]byte, error) {
	for {
		if i := bytes.IndexByte(e.pend, 0xC0); i >= 0 {
			if j := bytes.IndexByte(e.pend[i+1:], 0xC0); j >= 0 {
				raw := e.pend[i+1 : i+1+j]
				e.pend = e.pend[i+1+j:] // the closing C0 may open the next frame
				if len(raw) == 0 {
					continue
				}
				out := make([]byte, 0, len(raw))
				for k := 0; k < len(raw); k++ {
					if raw[k] == 0xDB && k+1 < len(raw) {
						k++
						if raw[k] == 0xDC {
							out = append(out, 0xC0)
						} else {
							out = append(out, 0xDB)
						}
						continue
					}
					out = append(out, raw[k])
				}
				return out, nil
			}
		}
		if time.Now().After(end) {
			return nil, errors.New("загрузчик ESP32 не ответил")
		}
		n, err := e.p.Read(e.buf[:])
		if err != nil {
			return nil, err
		}
		e.pend = append(e.pend, e.buf[:n]...)
	}
}

// cmd sends a command and waits its answer: the value and the data without
// the 4 status bytes of the ROM.
func (e *espROM) cmd(op byte, data []byte, chk uint32, wait time.Duration) (uint32, []byte, error) {
	h := make([]byte, 8, 8+len(data))
	h[1] = op
	binary.LittleEndian.PutUint16(h[2:], uint16(len(data)))
	binary.LittleEndian.PutUint32(h[4:], chk)
	if _, err := e.p.Write(slip(append(h, data...))); err != nil {
		return 0, nil, err
	}
	end := time.Now().Add(wait)
	for {
		f, err := e.frame(end)
		if err != nil {
			return 0, nil, err
		}
		if len(f) < 8 || f[0] != 1 || f[1] != op {
			continue // an echo of an earlier SYNC, or noise
		}
		val, body := binary.LittleEndian.Uint32(f[4:]), f[8:]
		if len(body) < 2 {
			return 0, nil, fmt.Errorf("загрузчик ESP32: короткий ответ на 0x%02X", op)
		}
		st := len(body) - 4
		if st < 0 {
			st = len(body) - 2
		}
		if body[st] != 0 {
			return 0, nil, fmt.Errorf("загрузчик ESP32: ошибка 0x%02X на команде 0x%02X", body[st+1], op)
		}
		return val, body[:st], nil
	}
}

func le(v ...uint32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return b
}

func (e *espROM) sync() error {
	d := append([]byte{0x07, 0x07, 0x12, 0x20}, bytes.Repeat([]byte{0x55}, 32)...)
	var err error
	for i := 0; i < 8; i++ {
		e.pend = nil
		if _, _, err = e.cmd(espSync, d, 0, 300*time.Millisecond); err == nil {
			end := time.Now().Add(200 * time.Millisecond) // the other answers to the same SYNC
			for {
				if _, e2 := e.frame(end); e2 != nil {
					break
				}
			}
			e.pend = nil
			return nil
		}
	}
	return err
}

func (e *espROM) writeReg(addr, v uint32) error {
	_, _, err := e.cmd(espWriteReg, le(addr, v, 0xFFFFFFFF, 0), 0, 3*time.Second)
	return err
}

// noWatchdogs: in the loader over USB the RTC watchdog of the C3 would
// restart the chip in the middle of the work.
func (e *espROM) noWatchdogs() error {
	steps := [][2]uint32{{rtcCntl + 0xA8, 0x50D83AA1}, {rtcCntl + 0x90, 0}, {rtcCntl + 0xA8, 0}}
	for _, s := range steps {
		if err := e.writeReg(s[0], s[1]); err != nil {
			return err
		}
	}
	if err := e.writeReg(rtcCntl+0xB0, 0x8F1D312A); err != nil {
		return err
	}
	v, _, err := e.cmd(espReadReg, le(rtcCntl+0xAC), 0, 3*time.Second)
	if err != nil {
		return err
	}
	if err := e.writeReg(rtcCntl+0xAC, v|1<<31); err != nil {
		return err
	}
	return e.writeReg(rtcCntl+0xB0, 0)
}

// write puts img at 0x0 and checks it by the MD5 of the flash; step reports
// the share done.
func (e *espROM) write(img []byte, step func(float64, string)) error {
	if len(img) == 0 || len(img) > espStore {
		return fmt.Errorf("образ прошивки %d байт: больше места до файлов щупа", len(img))
	}
	if _, _, err := e.cmd(espAttach, make([]byte, 8), 0, 3*time.Second); err != nil {
		return err
	}
	if _, _, err := e.cmd(espSetParams, le(0, espFlash, 64<<10, 4<<10, 256, 0xFFFF), 0, 3*time.Second); err != nil {
		return err
	}
	n := (len(img) + espBlock - 1) / espBlock
	step(0, "стираю флеш...")
	erase := time.Duration(30*float64(len(img))/float64(1<<20)*float64(time.Second)) + 5*time.Second
	if _, _, err := e.cmd(espFlashBegin, le(uint32(len(img)), uint32(n), espBlock, 0, 0), 0, erase); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		blk := bytes.Repeat([]byte{0xFF}, espBlock)
		copy(blk, img[i*espBlock:])
		chk := uint32(0xEF)
		for _, c := range blk {
			chk ^= uint32(c)
		}
		var err error
		for try := 0; try < 3; try++ {
			if _, _, err = e.cmd(espFlashData, append(le(espBlock, uint32(i), 0, 0), blk...), chk, 5*time.Second); err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("блок %d из %d: %v", i+1, n, err)
		}
		if i%16 == 0 || i == n-1 {
			step(float64(i+1)/float64(n), fmt.Sprintf("пишу %d из %d КБ", i+1, n))
		}
	}
	step(1, "сверяю MD5...")
	_, got, err := e.cmd(espFlashMD5, le(0, uint32(len(img)), 0, 0), 0, 30*time.Second)
	if err != nil {
		return err
	}
	want := md5.Sum(img)
	if len(got) < 32 || string(got[:32]) != hex.EncodeToString(want[:]) {
		return fmt.Errorf("MD5 флеша не совпал: %q", got)
	}
	return nil
}

// flashProbe: the whole work on a port that open gives (again after a reset,
// if the chip came back as a new USB device).
func flashProbe(open func() (linePort, error), img []byte, step func(float64, string)) error {
	p, err := open()
	if err != nil {
		return err
	}
	e := &espROM{p: p}
	defer func() { e.p.Close() }()
	step(0, "перезапускаю щуп в загрузчик...")
	e.resetToLoader()
	time.Sleep(300 * time.Millisecond)
	if err = e.sync(); err != nil {
		e.p.Close()
		for t := 0; t < 20; t++ { // the port comes back after the USB reconnects
			time.Sleep(500 * time.Millisecond)
			if p, err = open(); err == nil {
				break
			}
		}
		if err != nil {
			e.p = nopPort{}
			return fmt.Errorf("порт не вернулся после сброса: %v", err)
		}
		e = &espROM{p: p}
		if err = e.sync(); err != nil {
			return fmt.Errorf("щуп не вошёл в загрузчик: %v", err)
		}
	}
	if err := e.noWatchdogs(); err != nil {
		return err
	}
	if err := e.write(img, step); err != nil {
		return err
	}
	step(1, "запускаю новую прошивку...")
	e.hardReset()
	return nil
}

type nopPort struct{}

func (nopPort) Read([]byte) (int, error)  { return 0, io.EOF }
func (nopPort) Write([]byte) (int, error) { return 0, io.EOF }
func (nopPort) Close() error              { return nil }
func (nopPort) SetDTR(bool) error         { return nil }
func (nopPort) SetRTS(bool) error         { return nil }

// ==========================
// ===  Menu items        ===
// ==========================

// cmdFlashProbe: Сервис / Перепрошить щуп.
func cmdFlashProbe(o opts) error {
	sum := md5.Sum(probeImage)
	if !askYes("Перепрошить щуп?",
		"pcbscan запишет в ESP32 щупа прошивку, которая встроена в программу.",
		fmt.Sprintf("Образ %d КБ, md5 %s.", len(probeImage)/1024, hex.EncodeToString(sum[:4])),
		"Файлы в хранилище щупа останутся.",
		"Не отключайте кабель около минуты.") {
		return nil
	}
	p := newPage("Перепрошить щуп", "Ищу щуп...")
	mcuMu.Lock()
	defer mcuMu.Unlock()
	name := o.port
	if name == "" {
		if _, n, err := findMCUlocked(""); err == nil {
			name = n
		} else if mcuName != "" {
			name = mcuName
		} else {
			return err
		}
	}
	if mcuCache != nil {
		mcuCache.Close()
		mcuCache = nil
		time.Sleep(500 * time.Millisecond)
	}
	open := func() (linePort, error) {
		var err error
		for i := 0; i < 5; i++ {
			var pt *Port
			if pt, err = OpenPort(name); err == nil {
				return pt, nil
			}
			time.Sleep(300 * time.Millisecond)
		}
		return nil, err
	}
	p.set(func() { p.head = "Прошивка щупа на " + name; p.progress = 0 })
	start := time.Now()
	err := flashProbe(open, probeImage, func(f float64, s string) {
		logLine(s)
		p.set(func() { p.progress, p.progText = f, s })
	})
	if err != nil {
		return fmt.Errorf("прошивка щупа: %v", err)
	}
	time.Sleep(2 * time.Second) // the new firmware starts, USB comes back
	var m *MCU
	for i := 0; i < 5 && m == nil; i++ {
		m, _, err = findMCUlocked(name)
		if m == nil {
			time.Sleep(time.Second)
		}
	}
	about := fmt.Sprintf("Записано за %.0f с, MD5 совпал.", time.Since(start).Seconds())
	if m == nil {
		message("Щуп перепрошит", about, "Щуп пока не ответил: переподключите кабель.")
		return nil
	}
	message("Щуп перепрошит", about, "Wi-Fi щупа теперь выключен.",
		"Включает его кнопка BOOT на плате или Сервис / Wi-Fi щупа на 10 минут.")
	return nil
}

// cmdProbeWiFi: Сервис / Wi-Fi щупа на 10 минут.
func cmdProbeWiFi(o opts) error {
	m, _, err := findMCU(o.port)
	if err != nil {
		return err
	}
	var st string
	err = m.do(func(io *mcuIO) error {
		if err := io.write("AW"); err != nil {
			return err
		}
		for {
			s, err := io.line(3 * time.Second)
			if err != nil {
				return err
			}
			if len(s) > 5 && s[:5] == "WIFI " {
				st = s
				return nil
			}
		}
	})
	if err != nil {
		return fmt.Errorf("щуп не ответил про Wi-Fi: %v (старая прошивка? Сервис / Перепрошить щуп)", err)
	}
	message("Wi-Fi щупа", st, "Сеть pcbscan, пароль pcbmill1, адрес http://192.168.4.1",
		"Точка выключится сама через 10 минут без подключений.")
	return nil
}
