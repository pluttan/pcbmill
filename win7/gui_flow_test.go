package main

// The whole job in the emulation, driven like an operator: clicks on what is
// written on the screen, keys, typed yes. With PCBSCAN_SHOTS=dir every step
// leaves a picture there.

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pcbscan/ui"
)

type driver struct {
	t    *testing.T
	d    *ui.Headless
	dir  string
	n    int
	last string
}

func (dr *driver) find(s string) (image.Rectangle, bool) {
	ts := app.Texts()
	for _, x := range ts { // a button or a row is the whole text
		if x.Live && x.S == s {
			return x.R, true
		}
	}
	for _, x := range ts {
		if x.Live && strings.Contains(x.S, s) {
			return x.R, true
		}
	}
	return image.Rectangle{}, false
}

func (dr *driver) screen() string {
	var b strings.Builder
	for _, x := range app.Texts() {
		b.WriteString(x.S + " | ")
	}
	return b.String()
}

func (dr *driver) wait(s string, timeout time.Duration) {
	dr.t.Helper()
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if _, ok := dr.find(s); ok {
			time.Sleep(60 * time.Millisecond) // let the frame settle
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	dr.shot("FAIL")
	dr.t.Fatalf("нет %q на экране после %q:\n%s", s, dr.last, dr.screen())
}

// stage: a stage on the rail, by its short name when the window is narrow.
func (dr *driver) stage(s string) {
	dr.t.Helper()
	time.Sleep(100 * time.Millisecond)
	for _, x := range app.Texts() {
		if x.Live && x.S == shortTitle(s) {
			s = x.S
			break
		}
	}
	dr.click(s)
}

func (dr *driver) click(s string) {
	dr.t.Helper()
	dr.wait(s, 10*time.Second)
	r, _ := dr.find(s)
	dr.clickAt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	dr.last = "click " + s
}

func (dr *driver) clickAt(x, y int) {
	dr.d.Send(ui.Event{Kind: ui.EvMouseMove, X: x, Y: y})
	dr.d.Send(ui.Event{Kind: ui.EvMouseDown, X: x, Y: y, Button: ui.BtnLeft})
	dr.d.Send(ui.Event{Kind: ui.EvMouseUp, X: x, Y: y, Button: ui.BtnLeft})
	time.Sleep(120 * time.Millisecond)
}

func (dr *driver) key(k ui.Key) {
	dr.d.Send(ui.Event{Kind: ui.EvKey, Key: k})
	time.Sleep(80 * time.Millisecond)
	dr.last = fmt.Sprint("key ", k)
}

func (dr *driver) typ(s string) {
	for _, r := range s {
		dr.d.Send(ui.Event{Kind: ui.EvChar, Rune: r})
	}
	time.Sleep(100 * time.Millisecond)
	dr.last = "type " + s
}

func (dr *driver) shot(name string) {
	if dr.dir == "" {
		return
	}
	time.Sleep(450 * time.Millisecond) // the smoothed 3D frame after a change
	app.Wake()
	time.Sleep(150 * time.Millisecond)
	dr.n++
	dr.d.Save(filepath.Join(dr.dir, fmt.Sprintf("%02d_%s.png", dr.n, name)))
}

func copyFile(t *testing.T, src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	io.Copy(out, in)
}

func TestGUIFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий сценарий")
	}
	dir := t.TempDir()
	copyFile(t, filepath.Join("testdata", "gerbers", "2032Charger.zip"), filepath.Join(dir, "2032Charger.zip"))
	os.MkdirAll(filepath.Join(dir, "eagle-demo"), 0755)
	src := filepath.Join("..", "examples", "gerbers", "eagle")
	ents, _ := os.ReadDir(src)
	for _, e := range ents {
		if !e.IsDir() {
			copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dir, "eagle-demo", e.Name()))
		}
	}
	t.Setenv("PCBSCAN_DIR", dir)
	w, h := 1280, 800
	if s := os.Getenv("PCBSCAN_SIZE"); s != "" {
		fmt.Sscanf(s, "%dx%d", &w, &h)
	}
	o := opts{sim: true, simSpeed: 400, rapid: 1500, clear: 0.3, search: 3, below: 0.3, step: 0.05, fine: 0.01,
		mdiKey: "ctrl+shift+f9", title: "NcStudio"}
	startSim(o)
	th, err := ui.NewTheme()
	if err != nil {
		t.Skip(err)
	}
	d := ui.NewHeadless(w, h)
	theme, app = th, ui.NewApp(d, th)
	statusFn = func() Status { return statusCached(o) }
	dr := &driver{t: t, d: d, dir: os.Getenv("PCBSCAN_SHOTS")}
	if dr.dir != "" {
		os.MkdirAll(dr.dir, 0755)
	}
	done := make(chan bool)
	go func() { jobMain(o); close(done) }()

	dr.wait("Платы ещё не выбраны", 10*time.Second)
	dr.shot("start")
	dr.stage("Платы")
	dr.click("Выбрать платы")
	dr.click("Открыть")
	dr.wait("Платы рядом с pcbscan", 5*time.Second)
	dr.key(ui.KeyRight) // 2032Charger x2, the eagle board x1
	dr.key(ui.KeyRight)
	dr.key(ui.KeyDown)
	dr.key(ui.KeyRight)
	dr.shot("pick_boards")
	dr.click("Разложить")
	dr.wait("Программы листа", 120*time.Second)
	dr.shot("programs")
	dr.click("Готово")
	dr.wait("Карта высот", 10*time.Second)
	dr.shot("main_after_layout")

	// the sheet wider right in its line, laid out again: the map below runs on it
	dr.stage("Платы")
	dr.click("ширина X")
	dr.typ("150")
	dr.key(ui.KeyEnter)
	dr.wait("изменено", 5*time.Second)
	dr.shot("sheet_inline")
	dr.click("Разложить и посчитать")
	dr.click("Открыть")
	dr.wait("Программы листа", 120*time.Second)
	dr.wait("листе 150x100", 5*time.Second)
	dr.click("Готово")
	dr.wait("Карта высот", 10*time.Second)

	// the height map
	dr.stage("Карта высот")
	dr.click("Снять карту высот")
	dr.shot("map_stage")
	dr.click("Начать")
	dr.wait("Z обнулён, начать обход?", 10*time.Second)
	dr.shot("scan_question")
	dr.typ("д")
	dr.wait("Обход", 10*time.Second)
	time.Sleep(1500 * time.Millisecond)
	dr.shot("scan_running")
	dr.wait("Сдвиг первой точки", 300*time.Second)
	dr.shot("map_result")
	dr.click("Таблица чисел")
	dr.shot("map_table")
	dr.click("Готово")
	dr.click("OK")

	// milling
	dr.stage("Фрезеровка")
	dr.wait("1_iso", 10*time.Second)
	dr.shot("mill_stage")
	dr.click("Запустить")
	dr.wait("Инструмент", 10*time.Second)
	dr.shot("tool_dialog")
	dr.click("Поставил, Z0")
	dr.wait("Начать касание?", 10*time.Second)
	dr.typ("д")
	dr.wait("Обнулите Z", 60*time.Second)
	dr.shot("zero_touch")
	dr.click("OK")
	dr.wait("Напечатайте yes", 30*time.Second)
	dr.typ("нуы")
	dr.key(ui.KeyEnter)
	dr.wait("Русская раскладка", 5*time.Second)
	dr.shot("yes_hint")
	dr.typ("yes")
	dr.key(ui.KeyEnter)
	dr.wait("Станок работает", 30*time.Second)
	dr.shot("machine_works")
	dr.click("Станок закончил")
	dr.typ("д")
	dr.wait("Ожидаю дальше", 10*time.Second)
	dr.shot("done_message")
	dr.click("OK")

	// a redo in a circle, the centre by a click on the sheet
	dr.stage("Маска")
	dr.click("Повторить в круге")
	dr.click("Открыть")
	dr.wait("Сделать файл", 10*time.Second)
	dr.shot("redo_form")
	dr.clickAt(w*700/1280, h*425/800) // the pads of the left 2032Charger on the 150 mm sheet
	dr.shot("redo_clicked")
	dr.click("Сделать файл")
	dr.wait("Запустить", 10*time.Second)
	dr.typ("н")

	// manual mode
	dr.stage("Сервис")
	dr.click("Ручной режим")
	dr.click("Открыть")
	dr.typ("д")
	dr.wait("Ручной режим", 10*time.Second)
	dr.key(ui.KeyRight)
	time.Sleep(600 * time.Millisecond)
	dr.key(ui.KeyUp)
	time.Sleep(600 * time.Millisecond)
	dr.shot("manual")
	dr.click("Выход")
	dr.click("Эмуляция: станок")
	dr.click("Открыть")
	dr.wait("Неисправности", 5*time.Second)
	dr.shot("sim_panel")
	dr.click("Готово")

	// out
	dr.click("Выход")
	dr.wait("Выйти из pcbscan?", 5*time.Second)
	dr.typ("д")
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("не вышел")
	}
}
