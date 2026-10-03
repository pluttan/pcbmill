package webui

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"pcbscan/ui"
)

// ==========================
// ===  HTTP routes       ===
// ==========================

// pollTimeout keeps long-polls under common proxy and browser limits.
var pollTimeout = 25 * time.Second

func (w *Window) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", w.servePage)
	mux.HandleFunc("/frame", w.serveFrame)
	mux.HandleFunc("/ev", w.serveEvents)
	return mux
}

func (w *Window) servePage(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(rw, r)
		return
	}
	w.mu.Lock()
	title := w.title
	w.mu.Unlock()
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(rw, strings.Replace(pageHTML, "{{TITLE}}", html.EscapeString(title), 1))
}

// serveFrame: GET /frame?seq=N&st=M&id=P. Blocks until a frame newer than
// N (200 + PNG) or a state newer than M (204), or times out (204).
func (w *Window) serveFrame(rw http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	seq, _ := strconv.ParseUint(q.Get("seq"), 10, 64)
	st, _ := strconv.ParseUint(q.Get("st"), 10, 64)
	id := q.Get("id")

	ctx, cancel := contextWithTimeoutDone(r.Context(), pollTimeout, w.done)
	defer cancel()

	stateNewer := func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.stateVer != st
	}
	got := w.frames.waitNewer(ctx, seq, stateNewer)

	h := rw.Header()
	h.Set("Cache-Control", "no-store")
	w.mu.Lock()
	h.Set("X-Cursor", cursorCSS(w.cursor))
	h.Set("X-Title", url.PathEscape(w.title))
	h.Set("X-State", strconv.FormatUint(w.stateVer, 10))
	if id == "" || w.active == "" || id == w.active {
		h.Set("X-Active", "1")
	} else {
		h.Set("X-Active", "0")
	}
	w.mu.Unlock()
	if !got {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	e := w.frames.fetch(seq)
	h.Set("Content-Type", "image/png")
	h.Set("Content-Length", strconv.Itoa(len(e.data)))
	h.Set("X-Seq", strconv.FormatUint(e.seq, 10))
	h.Set("X-Rect", fmt.Sprintf("%d,%d,%d,%d", e.rect.Min.X, e.rect.Min.Y, e.rect.Dx(), e.rect.Dy()))
	h.Set("X-Size", fmt.Sprintf("%d,%d", e.fullW, e.fullH))
	h.Set("X-Full", boolStr(e.full))
	h.Set("X-Encode-Ms", strconv.FormatFloat(e.encodeTime, 'f', 1, 64))
	rw.Write(e.data)
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ==========================
// ===  Input from page   ===
// ==========================

// pageEvent is one item of the batch the page posts.
type pageEvent struct {
	T string  `json:"t"` // hello move down up wheel key resize focus leave
	X float64 `json:"x"`
	Y float64 `json:"y"`
	B int     `json:"b"` // DOM button: 0 left, 1 middle, 2 right
	D float64 `json:"d"` // wheel notches, + away from the user
	K string  `json:"k"` // KeyboardEvent.key
	C string  `json:"c"` // KeyboardEvent.code
	M int     `json:"m"` // ui.Mods bits
	W int     `json:"w"`
	H int     `json:"h"`
	S float64 `json:"s"` // devicePixelRatio
	F bool    `json:"f"`
}

type pageBatch struct {
	ID string      `json:"id"`
	Ev []pageEvent `json:"ev"`
}

func (w *Window) serveEvents(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var b pageBatch
	if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 1<<20)).Decode(&b); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	if !w.beginSend() {
		http.Error(rw, "closed", http.StatusGone)
		return
	}
	defer w.pending.Done()
	rw.Header().Set("Cache-Control", "no-store")
	for _, pe := range b.Ev {
		if pe.T == "hello" {
			if w.hello(b.ID, pe) != nil {
				return
			}
			continue
		}
		w.mu.Lock()
		mine := b.ID == w.active
		w.mu.Unlock()
		if !mine {
			// an older tab: the newest page drives input
			rw.WriteHeader(http.StatusConflict)
			return
		}
		for _, ev := range w.convert(pe) {
			if w.send(ev) != nil {
				return
			}
		}
	}
	rw.WriteHeader(http.StatusNoContent)
}

// hello makes the page the active one and tells the program its size;
// EvExpose asks for a redraw since the page has nothing on screen.
func (w *Window) hello(id string, pe pageEvent) error {
	w.mu.Lock()
	w.active = id
	w.mu.Unlock()
	w.frames.wakeAll() // older tabs learn they are passive
	for _, ev := range w.convert(pageEvent{T: "resize", W: pe.W, H: pe.H, S: pe.S}) {
		if err := w.send(ev); err != nil {
			return err
		}
	}
	if err := w.send(ui.Event{Kind: ui.EvFocus, Focused: pe.F}); err != nil {
		return err
	}
	return w.send(ui.Event{Kind: ui.EvExpose})
}

// convert maps a page event to program events (a key may also type a char).
func (w *Window) convert(pe pageEvent) []ui.Event {
	x, y := int(math.Round(pe.X)), int(math.Round(pe.Y))
	switch pe.T {
	case "move":
		return []ui.Event{{Kind: ui.EvMouseMove, X: x, Y: y}}
	case "down", "up":
		k := ui.EvMouseDown
		if pe.T == "up" {
			k = ui.EvMouseUp
		}
		return []ui.Event{{Kind: k, Button: domButton(pe.B), X: x, Y: y, Mods: ui.Mods(pe.M) & ui.ModShift}}
	case "wheel":
		return []ui.Event{{Kind: ui.EvWheel, Wheel: pe.D, X: x, Y: y}}
	case "leave":
		return []ui.Event{{Kind: ui.EvLeave}}
	case "focus":
		return []ui.Event{{Kind: ui.EvFocus, Focused: pe.F}}
	case "resize":
		if pe.W <= 0 || pe.H <= 0 {
			return nil
		}
		w.mu.Lock()
		w.w, w.h = pe.W, pe.H
		if pe.S > 0 {
			w.scale = pe.S
		}
		w.mu.Unlock()
		return []ui.Event{{Kind: ui.EvResize, W: pe.W, H: pe.H}}
	case "key":
		mods := ui.Mods(pe.M) & (ui.ModShift | ui.ModCtrl | ui.ModAlt)
		evs := []ui.Event{{Kind: ui.EvKey, Key: mapKey(pe.K, pe.C), Mods: mods}}
		if r, ok := typedRune(pe.K, mods); ok {
			evs = append(evs, ui.Event{Kind: ui.EvChar, Rune: r, Mods: mods})
		}
		return evs
	}
	return nil
}

func domButton(b int) ui.Button {
	switch b {
	case 1:
		return ui.BtnMiddle
	case 2:
		return ui.BtnRight
	}
	return ui.BtnLeft
}

// ==========================
// ===  Keys              ===
// ==========================

var namedKeys = map[string]ui.Key{
	"ArrowUp": ui.KeyUp, "ArrowDown": ui.KeyDown, "ArrowLeft": ui.KeyLeft, "ArrowRight": ui.KeyRight,
	"Enter": ui.KeyEnter, "Escape": ui.KeyEsc, "Tab": ui.KeyTab, "Backspace": ui.KeyBackspace,
	"Delete": ui.KeyDelete, "PageUp": ui.KeyPgUp, "PageDown": ui.KeyPgDn, "Home": ui.KeyHome,
	"End": ui.KeyEnd, " ": ui.KeySpace,
	// old browsers
	"Up": ui.KeyUp, "Down": ui.KeyDown, "Left": ui.KeyLeft, "Right": ui.KeyRight,
	"Esc": ui.KeyEsc, "Del": ui.KeyDelete, "Spacebar": ui.KeySpace,
}

// mapKey: letters and digits come from the physical key (code) so
// shortcuts work in any layout; the rest from the key name.
func mapKey(key, code string) ui.Key {
	if len(code) == 4 && strings.HasPrefix(code, "Key") && code[3] >= 'A' && code[3] <= 'Z' {
		return ui.KeyA + ui.Key(code[3]-'A')
	}
	if len(code) == 6 && strings.HasPrefix(code, "Digit") && code[5] >= '0' && code[5] <= '9' {
		return ui.Key0 + ui.Key(code[5]-'0')
	}
	if code == "Space" {
		return ui.KeySpace
	}
	if code == "NumpadEnter" {
		return ui.KeyEnter
	}
	if k, ok := namedKeys[key]; ok {
		return k
	}
	if len(key) >= 2 && len(key) <= 3 && key[0] == 'F' {
		if n, err := strconv.Atoi(key[1:]); err == nil && n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1)
		}
	}
	// numpad digits with NumLock, keys without a code
	if utf8.RuneCountInString(key) == 1 {
		c := key[0]
		switch {
		case c >= '0' && c <= '9':
			return ui.Key0 + ui.Key(c-'0')
		case c >= 'a' && c <= 'z':
			return ui.KeyA + ui.Key(c-'a')
		case c >= 'A' && c <= 'Z':
			return ui.KeyA + ui.Key(c-'A')
		}
	}
	return ui.KeyOther
}

// typedRune: the text a key types; Ctrl combinations type nothing (the page
// clears Ctrl for AltGr, which types normally).
func typedRune(key string, mods ui.Mods) (rune, bool) {
	if mods&ui.ModCtrl != 0 || utf8.RuneCountInString(key) != 1 {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(key)
	if r < 0x20 || r == 0x7f {
		return 0, false
	}
	return r, true
}
