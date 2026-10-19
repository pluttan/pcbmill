//go:build figs

package main

// TestFigMap draws a height map from a CSV the way the screen of pcbscan
// draws it (the 3D view with its walls, numbers and colour bar) and saves
// the frame as PNG. The article shows the first real map of the machine.
//
//	go test -tags figs -run FigMap   (FIG_MAP=x,y,z csv, FIG_OUT=png, FIG_SIZE=WxH)

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"pcbscan/ui"
)

func TestFigMap(t *testing.T) {
	f, err := os.Open(os.Getenv("FIG_MAP"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var pts []Point
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		var p Point
		if _, err := fmt.Sscanf(l, "%g,%g,%g", &p.X, &p.Y, &p.Z); err != nil {
			t.Fatal(err)
		}
		pts = append(pts, p)
	}
	hm, err := MapFromPoints(pts)
	if err != nil {
		t.Fatal(err)
	}
	w, h := 1400, 900
	fmt.Sscanf(os.Getenv("FIG_SIZE"), "%dx%d", &w, &h)
	th, err := ui.NewTheme()
	if err != nil {
		t.Skip(err)
	}
	theme = th
	d := ui.NewHeadless(w, h)
	a := ui.NewApp(d, th)
	s := newMapScene(hm, pts, nil, false)
	a.Show(ui.LayerFunc(func(c *ui.Ctx) {
		c.View3D("fig/map", image.Rect(0, 0, c.W, c.H), s)
	}))
	time.Sleep(3 * time.Second) // the view at rest is drawn again with 2x supersampling
	a.Wake()
	time.Sleep(time.Second)
	if err := d.Save(os.Getenv("FIG_OUT")); err != nil {
		t.Fatal(err)
	}
}
