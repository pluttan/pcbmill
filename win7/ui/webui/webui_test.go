package webui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"pcbscan/ui"
)

// ==========================
// ===  Helpers           ===
// ==========================

func newTestWindow(t *testing.T) (*Window, *httptest.Server) {
	w := newWindow("Тест")
	srv := httptest.NewServer(w.handler())
	t.Cleanup(func() { w.Close(); srv.Close() })
	return w, srv
}

func post(t *testing.T, srv *httptest.Server, body string) int {
	t.Helper()
	r, err := http.Post(srv.URL+"/ev", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return r.StatusCode
}

func next(t *testing.T, w *Window) ui.Event {
	t.Helper()
	select {
	case ev := <-w.Events():
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}
	return ui.Event{}
}

type frameResp struct {
	status int
	seq    uint64
	rect   image.Rectangle
	full   bool
	img    image.Image
	hdr    http.Header
}

func getFrame(t *testing.T, srv *httptest.Server, seq, st uint64) frameResp {
	t.Helper()
	r, err := http.Get(fmt.Sprintf("%s/frame?seq=%d&st=%d&id=p1", srv.URL, seq, st))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	fr := frameResp{status: r.StatusCode, hdr: r.Header}
	if r.StatusCode != 200 {
		return fr
	}
	fr.seq, _ = strconv.ParseUint(r.Header.Get("X-Seq"), 10, 64)
	var x, y, w, h int
	fmt.Sscanf(r.Header.Get("X-Rect"), "%d,%d,%d,%d", &x, &y, &w, &h)
	fr.rect = image.Rect(x, y, x+w, y+h)
	fr.full = r.Header.Get("X-Full") == "1"
	data, _ := io.ReadAll(r.Body)
	fr.img, err = png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return fr
}

func noise(w, h int, seed int64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(seed))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(rnd.Int()), uint8(rnd.Int()), uint8(rnd.Int()), 255
	}
	return img
}

// samePixels compares want inside r with got placed at r.Min.
func samePixels(t *testing.T, want *image.RGBA, got image.Image, r image.Rectangle) {
	t.Helper()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a := want.RGBAAt(x, y)
			b := color.RGBAModel.Convert(got.At(x-r.Min.X, y-r.Min.Y)).(color.RGBA)
			if a != b {
				t.Fatalf("pixel %d,%d: want %v got %v", x, y, a, b)
			}
		}
	}
}

// ==========================
// ===  Events            ===
// ==========================

func TestEvents(t *testing.T) {
	w, srv := newTestWindow(t)
	if c := post(t, srv, `{"id":"p1","ev":[{"t":"hello","w":800,"h":600,"s":2,"f":true}]}`); c != 204 {
		t.Fatalf("hello: %d", c)
	}
	if ev := next(t, w); ev.Kind != ui.EvResize || ev.W != 800 || ev.H != 600 {
		t.Fatalf("resize: %+v", ev)
	}
	if ev := next(t, w); ev.Kind != ui.EvFocus || !ev.Focused {
		t.Fatalf("focus: %+v", ev)
	}
	if ev := next(t, w); ev.Kind != ui.EvExpose {
		t.Fatalf("expose: %+v", ev)
	}
	if ww, hh := w.Size(); ww != 800 || hh != 600 || w.Scale() != 2 {
		t.Fatalf("size %d %d scale %v", ww, hh, w.Scale())
	}

	post(t, srv, `{"id":"p1","ev":[
		{"t":"move","x":10.4,"y":20.6},
		{"t":"down","b":2,"x":11,"y":21},
		{"t":"up","b":1,"x":12,"y":22},
		{"t":"wheel","d":-1,"x":5,"y":6},
		{"t":"key","k":"ф","c":"KeyA","m":1},
		{"t":"key","k":"s","c":"KeyS","m":2},
		{"t":"key","k":"Enter","c":"Enter","m":0},
		{"t":"key","k":"F5","c":"F5","m":4},
		{"t":"key","k":" ","c":"Space","m":0},
		{"t":"leave"},
		{"t":"focus","f":false},
		{"t":"resize","w":640,"h":480,"s":1.5}]}`)
	want := []ui.Event{
		{Kind: ui.EvMouseMove, X: 10, Y: 21},
		{Kind: ui.EvMouseDown, Button: ui.BtnRight, X: 11, Y: 21},
		{Kind: ui.EvMouseUp, Button: ui.BtnMiddle, X: 12, Y: 22},
		{Kind: ui.EvWheel, Wheel: -1, X: 5, Y: 6},
		{Kind: ui.EvKey, Key: ui.KeyA, Mods: ui.ModShift},
		{Kind: ui.EvChar, Rune: 'ф', Mods: ui.ModShift},
		{Kind: ui.EvKey, Key: ui.KeyA + 18, Mods: ui.ModCtrl}, // Ctrl+S types nothing
		{Kind: ui.EvKey, Key: ui.KeyEnter},
		{Kind: ui.EvKey, Key: ui.KeyF1 + 4, Mods: ui.ModAlt},
		{Kind: ui.EvKey, Key: ui.KeySpace},
		{Kind: ui.EvChar, Rune: ' '},
		{Kind: ui.EvLeave},
		{Kind: ui.EvFocus, Focused: false},
		{Kind: ui.EvResize, W: 640, H: 480},
	}
	for i, wnt := range want {
		if ev := next(t, w); ev != wnt {
			t.Fatalf("event %d: want %+v got %+v", i, wnt, ev)
		}
	}
	if ww, hh := w.Size(); ww != 640 || hh != 480 || w.Scale() != 1.5 {
		t.Fatalf("size after resize %d %d %v", ww, hh, w.Scale())
	}
}

func TestLatestPageWins(t *testing.T) {
	w, srv := newTestWindow(t)
	post(t, srv, `{"id":"old","ev":[{"t":"hello","w":100,"h":100,"s":1}]}`)
	post(t, srv, `{"id":"new","ev":[{"t":"hello","w":200,"h":100,"s":1}]}`)
	for i := 0; i < 6; i++ {
		next(t, w) // two greetings
	}
	if c := post(t, srv, `{"id":"old","ev":[{"t":"move","x":1,"y":1}]}`); c != http.StatusConflict {
		t.Fatalf("old page: %d", c)
	}
	post(t, srv, `{"id":"new","ev":[{"t":"move","x":2,"y":3}]}`)
	if ev := next(t, w); ev.X != 2 || ev.Y != 3 {
		t.Fatalf("got %+v", ev)
	}
	w.Present(noise(4, 4, 1))
	r, _ := http.Get(srv.URL + "/frame?seq=0&id=old")
	r.Body.Close()
	if r.Header.Get("X-Active") != "0" {
		t.Fatal("old page not told it is passive")
	}
}

func TestMapKey(t *testing.T) {
	cases := []struct {
		key, code string
		want      ui.Key
	}{
		{"ArrowUp", "ArrowUp", ui.KeyUp}, {"PageDown", "PageDown", ui.KeyPgDn},
		{"Escape", "Escape", ui.KeyEsc}, {"Tab", "Tab", ui.KeyTab},
		{"Backspace", "Backspace", ui.KeyBackspace}, {"F12", "F12", ui.KeyF12},
		{"й", "KeyQ", ui.KeyA + 16}, {"Z", "KeyZ", ui.KeyZ}, {"!", "Digit1", ui.Key0 + 1},
		{"7", "Numpad7", ui.Key0 + 7}, {"Enter", "NumpadEnter", ui.KeyEnter},
		{"Shift", "ShiftLeft", ui.KeyOther}, {"Dead", "Quote", ui.KeyOther},
	}
	for _, c := range cases {
		if got := mapKey(c.key, c.code); got != c.want {
			t.Errorf("%q/%q: want %d got %d", c.key, c.code, c.want, got)
		}
	}
	if _, ok := typedRune("Shift", 0); ok {
		t.Error("Shift typed a char")
	}
}

func TestCloseEndsEvents(t *testing.T) {
	w, url, err := Open("Окно", false)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(page), "<title>Окно</title>") {
		t.Fatal("title missing in page")
	}
	// a parked long-poll must not delay Close
	go http.Get(url + "frame?seq=999999")
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	w.Close()
	if _, ok := <-w.Events(); ok {
		t.Fatal("Events not closed")
	}
	if time.Since(start) > time.Second {
		t.Fatal("Close waited for the long-poll")
	}
}

// ==========================
// ===  Frames            ===
// ==========================

func TestFramePNG(t *testing.T) {
	w, srv := newTestWindow(t)
	img := noise(37, 23, 7)
	w.Present(img)
	fr := getFrame(t, srv, 0, 0)
	if fr.status != 200 || fr.seq != 1 || !fr.full || fr.rect != img.Rect {
		t.Fatalf("frame %+v", fr)
	}
	samePixels(t, img, fr.img, img.Rect)
	if fr.hdr.Get("X-Title") != "%D0%A2%D0%B5%D1%81%D1%82" {
		t.Fatalf("title header %q", fr.hdr.Get("X-Title"))
	}
}

func TestSubImagePresent(t *testing.T) {
	w, srv := newTestWindow(t)
	big := noise(50, 40, 3)
	sub := big.SubImage(image.Rect(10, 5, 30, 25)).(*image.RGBA)
	w.Present(sub)
	fr := getFrame(t, srv, 0, 0)
	if fr.rect != image.Rect(0, 0, 20, 20) {
		t.Fatalf("rect %v", fr.rect)
	}
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if color.RGBAModel.Convert(fr.img.At(x, y)) != big.RGBAAt(x+10, y+5) {
				t.Fatalf("pixel %d,%d", x, y)
			}
		}
	}
}

func TestDirtyRect(t *testing.T) {
	w, srv := newTestWindow(t)
	img := noise(200, 100, 1)
	w.Present(img)
	client := image.NewRGBA(img.Rect)
	fr := getFrame(t, srv, 0, 0)
	drawAt(client, fr)

	// two small changes, the page fetches only after both
	img.SetRGBA(30, 40, color.RGBA{1, 2, 3, 255})
	w.Present(img)
	img.SetRGBA(70, 45, color.RGBA{4, 5, 6, 255})
	w.Present(img)
	fr = getFrame(t, srv, 1, 0)
	if fr.seq != 3 || fr.full || fr.rect != image.Rect(30, 40, 71, 46) {
		t.Fatalf("diff frame seq %d full %v rect %v", fr.seq, fr.full, fr.rect)
	}
	drawAt(client, fr)
	samePixels(t, img, client, img.Rect)

	// an identical frame is not news
	w.Present(img)
	if fr := getFrame(t, srv, 0, 0); fr.seq != 3 {
		t.Fatalf("identical frame bumped seq to %d", fr.seq)
	}
	// a new size is always whole
	w.Present(noise(120, 80, 2))
	if fr := getFrame(t, srv, 3, 0); !fr.full || fr.rect.Dx() != 120 {
		t.Fatalf("resized frame %+v", fr.rect)
	}
}

func drawAt(dst *image.RGBA, fr frameResp) {
	b := fr.img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(fr.rect.Min.X+x, fr.rect.Min.Y+y, fr.img.At(x, y))
		}
	}
}

func TestLongPollAndStaleDrop(t *testing.T) {
	w, srv := newTestWindow(t)
	frames := []*image.RGBA{noise(16, 16, 1), noise(16, 16, 2), noise(16, 16, 3)}
	for _, f := range frames {
		w.Present(f)
	}
	// only the last one is served
	fr := getFrame(t, srv, 0, 0)
	if fr.seq != 3 {
		t.Fatalf("seq %d", fr.seq)
	}
	samePixels(t, frames[2], fr.img, frames[2].Rect)

	st, _ := strconv.ParseUint(fr.hdr.Get("X-State"), 10, 64)
	done := make(chan frameResp, 1)
	go func() { done <- getFrame(t, srv, 3, st) }()
	select {
	case <-done:
		t.Fatal("long-poll returned without a new frame")
	case <-time.After(150 * time.Millisecond):
	}
	f4 := noise(16, 16, 4)
	w.Present(f4)
	select {
	case fr := <-done:
		if fr.seq != 4 {
			t.Fatalf("seq %d", fr.seq)
		}
		samePixels(t, f4, fr.img, f4.Rect)
	case <-time.After(2 * time.Second):
		t.Fatal("long-poll did not wake")
	}
}

func TestStateWakesPoll(t *testing.T) {
	w, srv := newTestWindow(t)
	w.Present(noise(8, 8, 1))
	fr := getFrame(t, srv, 0, 0)
	st, _ := strconv.ParseUint(fr.hdr.Get("X-State"), 10, 64)
	done := make(chan frameResp, 1)
	go func() { done <- getFrame(t, srv, fr.seq, st) }()
	time.Sleep(50 * time.Millisecond)
	w.SetCursor(ui.CursorHand)
	select {
	case fr := <-done:
		if fr.status != 204 || fr.hdr.Get("X-Cursor") != "pointer" {
			t.Fatalf("status %d cursor %q", fr.status, fr.hdr.Get("X-Cursor"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cursor change did not wake the poll")
	}
}

func TestPollTimeout(t *testing.T) {
	old := pollTimeout
	pollTimeout = 100 * time.Millisecond
	defer func() { pollTimeout = old }()
	w, srv := newTestWindow(t)
	w.Present(noise(8, 8, 1))
	fr := getFrame(t, srv, 0, 0)
	st, _ := strconv.ParseUint(fr.hdr.Get("X-State"), 10, 64)
	if fr := getFrame(t, srv, 1, st); fr.status != 204 {
		t.Fatalf("status %d", fr.status)
	}
}

// ==========================
// ===  Speed             ===
// ==========================

// demoFrame looks like a real screen: flat panels, some lines, text-sized
// detail, not noise.
func demoFrame(w, h, shift int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{30, 30, 46, 255}
			if x < 260 {
				c = color.RGBA{24, 24, 37, 255}
			}
			if (x+shift)%40 == 0 || y%40 == 0 {
				c = color.RGBA{69, 71, 90, 255}
			}
			if y%18 < 12 && x < 240 && x > 16 && (x*7+y*3)%5 < 2 {
				c = color.RGBA{205, 214, 244, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func BenchmarkFullFrame1400x900(b *testing.B) {
	var f frameStore
	f.init()
	a, c := demoFrame(1400, 900, 0), demoFrame(1400, 900, 7)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			f.present(a)
		} else {
			f.present(c)
		}
		e := f.fetch(0)
		b.SetBytes(int64(len(e.data)))
	}
}

func BenchmarkSmallChange1400x900(b *testing.B) {
	var f frameStore
	f.init()
	img := demoFrame(1400, 900, 0)
	f.present(img)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := 100 + i%1000
		img.SetRGBA(x, 300, color.RGBA{255, 0, 0, 255})
		f.present(img)
		f.fetch(f.seq - 1)
	}
}
