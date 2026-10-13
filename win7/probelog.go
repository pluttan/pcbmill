package main

// The journal of the probe link. Every drop of the MCU and every return go
// into probe-log.txt next to the exe, with how long the probe had been up and
// why it last started: a restart (power dip, crash, watchdog) and a USB drop
// alone look the same to Windows (error 31), the journal tells them apart.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pcbscan/ui"
)

const probeLogFile = "probe-log.txt"

var (
	probeLogMu    sync.Mutex
	probeDrops    int    // drops since the start of pcbscan
	probeRestarts int    // of them, the probe itself restarted
	probeLast     string // the last line of the journal
)

// probeLog adds a line to the journal and to the log of the page.
func probeLog(s string) {
	logLine("▲ щуп: " + s)
	probeLogMu.Lock()
	defer probeLogMu.Unlock()
	probeLast = time.Now().Format("15:04:05") + "  " + s
	f, err := os.OpenFile(filepath.Join(exeDir(), probeLogFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05"), s)
}

// resetWhy: esp_reset_reason() of the ESP32-C3 in words.
func resetWhy(r int) string {
	switch r {
	case 1:
		return "включение питания"
	case 2:
		return "кнопка RESET"
	case 3:
		return "программный перезапуск"
	case 4:
		return "сбой прошивки"
	case 5, 6, 7:
		return "сторожевой таймер"
	case 9:
		return "провал питания"
	case 11:
		return "сброс по USB"
	case 12:
		return "сброс по JTAG"
	case 14:
		return "помеха по питанию"
	}
	return fmt.Sprintf("причина %d", r)
}

// uptime asks the probe ("U", firmware since 12.10.2026) how long it has run
// and why it started; ok is false for an older firmware or a Nano.
func (m *MCU) uptime() (up time.Duration, why int, ok bool) {
	err := m.do(func(io *mcuIO) error {
		if err := io.write("U"); err != nil {
			return err
		}
		for {
			s, err := io.line(400 * time.Millisecond)
			if err != nil {
				return err
			}
			f := strings.Fields(s)
			if len(f) == 3 && f[0] == "UP" {
				ms, e1 := strconv.ParseInt(f[1], 10, 64)
				r, e2 := strconv.Atoi(f[2])
				if e1 == nil && e2 == nil {
					up, why, ok = time.Duration(ms)*time.Millisecond, r, true
				}
				return nil
			}
		}
	})
	if err != nil {
		return 0, 0, false
	}
	return up, why, ok
}

// backLine: what the journal says when the probe answers again.
func backLine(m *MCU, name string, gone time.Duration) string {
	s := fmt.Sprintf("снова на связи (%s) через %.0f с", name, gone.Seconds())
	if up, why, ok := m.uptime(); ok {
		if up < gone+5*time.Second {
			probeLogMu.Lock()
			probeRestarts++
			probeLogMu.Unlock()
			s += fmt.Sprintf(": щуп ПЕРЕЗАПУСКАЛСЯ (%s), работает %.0f с", resetWhy(why), up.Seconds())
		} else {
			s += fmt.Sprintf(": щуп не перезапускался, работает %.0f мин; оборвался только USB", up.Minutes())
		}
	}
	return s
}

// cmdProbeLink: Сервис / Проверка связи щупа. The port is held and polled as
// in work; every drop and return is counted and goes into the journal. Run it
// with the VFD off, on, and the spindle turning: the counts tell the cause.
func cmdProbeLink(o opts) error {
	title := "сервис / Проверка связи щупа"
	p := newPage(title, "Ищу щуп на COM-портах...")
	if _, _, err := findTouch(o.port); err != nil {
		return err
	}
	probeLogMu.Lock()
	d0, r0 := probeDrops, probeRestarts
	probeLogMu.Unlock()
	start := time.Now()
	probeLog("проверка связи: начало")
	p.drain()
	p.buttons(pageBtn{id: "ok", label: "Готово", kind: ui.BtnPrimary, key: ui.KeyEnter})
	backToApp()
	sum := func() string {
		probeLogMu.Lock()
		defer probeLogMu.Unlock()
		return fmt.Sprintf("%.0f мин: обрывов %d, из них щуп перезапускался %d",
			time.Since(start).Minutes(), probeDrops-d0, probeRestarts-r0)
	}
	for {
		if _, ok := p.pressed(); ok {
			break
		}
		time.Sleep(500 * time.Millisecond)
		mcuMu.Lock()
		if mcuCache == nil || mcuCache.Alive() != nil {
			findMCUlocked(o.port) // a drop is counted and the port opened again there
		}
		mcuMu.Unlock()
		probeLogMu.Lock()
		last := probeLast
		probeLogMu.Unlock()
		p.set(func() {
			p.head = "Проверка связи щупа"
			p.info = []string{sum(), "Последнее: " + last,
				"Журнал: " + probeLogFile + " рядом с pcbscan. Готово завершает проверку."}
		})
	}
	s := sum()
	probeLog("проверка связи: конец, " + s)
	message("Проверка связи щупа", s, "Подробности в "+probeLogFile+" рядом с pcbscan.")
	return nil
}
