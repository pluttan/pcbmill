package main

// Keys: one event per key press, the same on the Windows console and in a Unix
// terminal. Screens read keys, never lines: arrows walk menus, Enter chooses,
// Esc goes back, and the manual mode moves the machine on single keys.

import (
	"io"
	"time"
	"unicode/utf8"
)

type KeyCode int

const (
	KeyNone KeyCode = iota
	KeyRune
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyEsc
	KeyTab
	KeyBackTab
	KeyBackspace
	KeyDelete
	KeyPgUp
	KeyPgDn
	KeyHome
	KeyEnd
	KeyCtrlC  // only from a pipe: in a terminal Ctrl+C is a signal
	KeyResize // the window changed size: redraw
	KeyF1
)

// KeyF returns the code of F1..F12.
func KeyF(n int) KeyCode { return KeyF1 + KeyCode(n-1) }

type Key struct {
	Code KeyCode
	R    rune // the character of a KeyRune
}

func runeKey(r rune) Key { return Key{KeyRune, r} }

// letter is the Latin key under a typed character: the Russian layout gives
// the same command as the Latin one (latin() maps the whole keyboard).
func (k Key) letter() rune {
	if k.Code != KeyRune {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(latin(string(k.R)))
	return r
}

// ==========================
// ===  Terminal bytes    ===
// ==========================

// parseKey takes one key off the front of b and says how many bytes it used.
// n == 0: b is the start of a sequence that has not fully arrived yet.
func parseKey(b []byte) (Key, int) {
	if len(b) == 0 {
		return Key{}, 0
	}
	c := b[0]
	switch {
	case c == 0x1b:
		return parseEsc(b)
	case c == '\r':
		if len(b) > 1 && b[1] == '\n' {
			return Key{Code: KeyEnter}, 2
		}
		return Key{Code: KeyEnter}, 1
	case c == '\n':
		return Key{Code: KeyEnter}, 1
	case c == 0x7f || c == 0x08:
		return Key{Code: KeyBackspace}, 1
	case c == '\t':
		return Key{Code: KeyTab}, 1
	case c == 0x03:
		return Key{Code: KeyCtrlC}, 1
	case c < 0x20:
		return Key{}, 1
	case c < 0x80:
		return runeKey(rune(c)), 1
	}
	if !utf8.FullRune(b) {
		return Key{}, 0
	}
	r, n := utf8.DecodeRune(b)
	if r == utf8.RuneError {
		return Key{}, 1
	}
	return runeKey(r), n
}

// tilde codes of ESC [ n ~
var tildeKeys = map[int]KeyCode{1: KeyHome, 7: KeyHome, 4: KeyEnd, 8: KeyEnd, 5: KeyPgUp, 6: KeyPgDn,
	3: KeyDelete, 2: KeyNone, 11: KeyF(1), 12: KeyF(2), 13: KeyF(3), 14: KeyF(4), 15: KeyF(5),
	17: KeyF(6), 18: KeyF(7), 19: KeyF(8), 20: KeyF(9), 21: KeyF(10), 23: KeyF(11), 24: KeyF(12)}

// letter finals of ESC [ x and ESC O x
var finalKeys = map[byte]KeyCode{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft,
	'H': KeyHome, 'F': KeyEnd, 'Z': KeyBackTab, 'P': KeyF(1), 'Q': KeyF(2), 'R': KeyF(3), 'S': KeyF(4)}

func parseEsc(b []byte) (Key, int) {
	if len(b) == 1 {
		return Key{}, 0 // a lone Esc or the start of a sequence: the reader waits a little
	}
	switch b[1] {
	case 'O':
		if len(b) < 3 {
			return Key{}, 0
		}
		if k, ok := finalKeys[b[2]]; ok {
			return Key{Code: k}, 3
		}
		return Key{}, 3
	case '[':
		num, first := 0, -1
		for i := 2; i < len(b) && i < 16; i++ {
			c := b[i]
			switch {
			case c >= '0' && c <= '9':
				num = num*10 + int(c-'0')
			case c == ';':
				if first < 0 {
					first = num
				}
				num = 0
			case c == '~':
				if first < 0 {
					first = num
				}
				return Key{Code: tildeKeys[first]}, i + 1
			case c >= 0x40 && c <= 0x7e:
				return Key{Code: finalKeys[c]}, i + 1 // ESC [ 1 ; 5 A is still an arrow
			default:
				return Key{}, i + 1
			}
		}
		if len(b) < 16 {
			return Key{}, 0
		}
		return Key{Code: KeyEsc}, 1
	}
	return Key{Code: KeyEsc}, 1 // Esc then a key, or Alt+key: Esc first
}

// escWait: a lone Esc is Esc when nothing follows it this soon.
const escWait = 40 * time.Millisecond

// readBytes turns a byte stream (a raw terminal, a pipe) into keys. At the end
// of the stream it sends Ctrl+C, so a scripted run ends instead of hanging.
func readBytes(r io.Reader, out chan<- Key) {
	raw := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				raw <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(raw)
				return
			}
		}
	}()
	var pend []byte
	open := true
	for open || len(pend) > 0 {
		var wait <-chan time.Time
		if len(pend) > 0 {
			wait = time.After(escWait)
		}
		select {
		case c, ok := <-raw:
			if !ok {
				open, raw = false, nil
				if len(pend) == 0 {
					break
				}
				continue
			}
			pend = append(pend, c...)
		case <-wait:
			// nothing completed the sequence: a lone Esc, or junk
			if pend[0] == 0x1b {
				out <- Key{Code: KeyEsc}
			}
			pend = pend[1:]
		}
		for len(pend) > 0 {
			k, n := parseKey(pend)
			if n == 0 {
				break
			}
			pend = pend[n:]
			if k.Code != KeyNone {
				out <- k
			}
		}
		if !open && len(pend) > 0 { // stream over: what is left will not grow
			k, n := parseKey(pend)
			if n == 0 {
				k, n = Key{Code: KeyEsc}, 1
				if pend[0] != 0x1b {
					k = Key{}
				}
			}
			pend = pend[n:]
			if k.Code != KeyNone {
				out <- k
			}
		}
	}
	out <- Key{Code: KeyCtrlC}
}

// ==========================
// ===  Input field       ===
// ==========================

// field is a one-line editor: characters, Backspace, Delete, arrows, Home/End.
type field struct {
	text []rune
	pos  int
}

func newField(s string) *field { r := []rune(s); return &field{text: r, pos: len(r)} }

func (f *field) String() string { return string(f.text) }

// key edits; done on Enter, cancel on Esc.
func (f *field) key(k Key) (done, cancel bool) {
	switch k.Code {
	case KeyEnter:
		return true, false
	case KeyEsc, KeyCtrlC:
		return false, true
	case KeyLeft:
		if f.pos > 0 {
			f.pos--
		}
	case KeyRight:
		if f.pos < len(f.text) {
			f.pos++
		}
	case KeyHome:
		f.pos = 0
	case KeyEnd:
		f.pos = len(f.text)
	case KeyBackspace:
		if f.pos > 0 {
			f.text = append(f.text[:f.pos-1], f.text[f.pos:]...)
			f.pos--
		}
	case KeyDelete:
		if f.pos < len(f.text) {
			f.text = append(f.text[:f.pos], f.text[f.pos+1:]...)
		}
	case KeyRune:
		if k.R >= ' ' && len(f.text) < 200 {
			f.text = append(f.text[:f.pos], append([]rune{k.R}, f.text[f.pos:]...)...)
			f.pos++
		}
	}
	return false, false
}

// view is the field w wide with the cursor drawn as an inverted cell: the
// console cursor itself stays hidden, so it never flickers over the screen.
func (f *field) view(w int) string {
	if w < 2 {
		w = 2
	}
	start := 0
	if f.pos >= w {
		start = f.pos - w + 1
	}
	var b []rune
	for i := start; i < len(f.text) && i < start+w; i++ {
		b = append(b, f.text[i])
	}
	s := ""
	for i, r := range b {
		if start+i == f.pos {
			s += hilite(string(r))
		} else {
			s += string(r)
		}
	}
	if f.pos-start == len(b) {
		s += hilite(" ")
	}
	return padRight(s, w)
}
