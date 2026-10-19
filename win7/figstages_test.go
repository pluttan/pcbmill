//go:build figs

package main

// TestFigStages walks the test board through the job in emulation and saves
// a frame of every stage into PCBSCAN_SHOTS: the sheet laid out, the height
// map, then the files of milling, mask, drilling and the V-groove. The
// pictures of the article are these frames.
//
//	go test -tags figs -run FigStages   (PCBSCAN_SHOTS=dir, PCBSCAN_SIZE=WxH)

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pcbscan/ui"
)

func TestFigStages(t *testing.T) {
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "testboard", "pcbmill-test.zip"), filepath.Join(dir, "pcbmill-test.zip"))
	t.Setenv("PCBSCAN_DIR", dir)
	w, h := 1280, 800
	fmt.Sscanf(os.Getenv("PCBSCAN_SIZE"), "%dx%d", &w, &h)
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
	os.MkdirAll(dr.dir, 0755)
	done := make(chan bool)
	go func() { jobMain(o); close(done) }()

	dr.wait("Платы ещё не выбраны", 10*time.Second)
	dr.stage("Платы")
	dr.click("pcbmill-test")
	dr.key(ui.KeyRight)
	dr.wait("изменено", 5*time.Second)
	dr.click("Разложить и посчитать")
	dr.click("Открыть")
	dr.wait("Программы листа", 120*time.Second)
	dr.shot("programs")
	dr.click("Готово")
	dr.wait("Карта высот", 10*time.Second)
	dr.shot("sheet")

	dr.stage("Карта высот")
	dr.click("Снять карту высот")
	dr.click("Начать")
	dr.wait("Z обнулён, начать обход?", 10*time.Second)
	dr.typ("д")
	dr.wait("Углы сняты", 120*time.Second)
	dr.shot("scan_corners")
	dr.click("Продолжить обход")
	time.Sleep(2 * time.Second)
	dr.shot("scan_running")
	dr.wait("Сдвиг первой точки", 300*time.Second)
	dr.shot("map_result")
	dr.click("Готово")
	dr.click("OK")

	for _, st := range []string{"Фрезеровка", "Маска", "Сверловка", "V-рез"} {
		dr.stage(st)
		time.Sleep(300 * time.Millisecond)
		if st == "Маска" { // the first row says to put the mask on; the files follow
			dr.key(ui.KeyDown)
			dr.key(ui.KeyDown)
		}
		dr.shot("stage_" + st)
	}

	dr.click("Выход")
	dr.wait("Выйти из pcbscan?", 5*time.Second)
	dr.typ("д")
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("не вышел")
	}
}
