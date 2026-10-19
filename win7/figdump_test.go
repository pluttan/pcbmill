//go:build figs

package main

// TestFigDump writes what the CAM sees on the test board into FIG_OUT as
// JSON: the copper raster, the field of distances in a small window, the
// isolation passes, the shorts, the holes and the V-groove. The pictures of
// the article (docs/article/figures.py) are drawn from it.
//
//	go test -tags figs -run FigDump   (FIG_OUT=file, FIG_CROP=x0,y0,x1,y1 in mm)

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type figRaster struct {
	X0, Y0, Px float64
	W, H       int
	Bits       string // base64, a byte per pixel, row 0 lowest Y
}

type figField struct {
	figRaster
	Dist string // base64 of uint16 little endian, eighths of a pixel
}

func figR(r *raster) figRaster {
	return figRaster{r.x0, r.y0, r.px, r.w, r.h, base64.StdEncoding.EncodeToString(r.bits)}
}

func figPaths(op *camOp) [][][3]float64 {
	var out [][][3]float64
	for _, p := range op.paths {
		var l [][3]float64
		for _, q := range p.pts {
			l = append(l, [3]float64{q.X, q.Y, q.Z})
		}
		out = append(out, l)
	}
	return out
}

func TestFigDump(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	b, err := loadBoard(zip)
	if err != nil {
		t.Fatal(err)
	}
	c := defaultCam()
	g := b.copper[lyTop]
	out := map[string]any{"box": b.box, "outline": b.outline, "cut": c.cutWidth(c.IsoDepth)}

	full := newRaster(rect{b.box.X0, b.box.X1, b.box.Y0, b.box.Y1}, 0.05)
	for _, s := range g.shapes {
		full.draw(s)
	}
	out["copper"] = figR(full)

	iso, _ := isolation(b, lyTop, c, nil)
	out["iso"] = figPaths(iso)
	out["shorts"] = iso.marks
	out["notes"] = iso.notes
	var holes [][3]float64
	for _, h := range b.holes {
		holes = append(holes, [3]float64{h.P.X, h.P.Y, h.D})
	}
	out["holes"] = holes
	out["vcut"] = figPaths(vcut(b, c))

	var x0, y0, x1, y1 float64
	if _, err := fmt.Sscanf(os.Getenv("FIG_CROP"), "%g,%g,%g,%g", &x0, &y0, &x1, &y1); err == nil {
		r := newRaster(rect{x0, x1, y0, y1}, 0.01)
		for _, s := range g.shapes {
			r.draw(s)
		}
		d := r.distTo(1, nil)
		raw := make([]byte, 2*len(d))
		for k, v := range d {
			raw[2*k], raw[2*k+1] = byte(v), byte(v>>8)
		}
		out["crop"] = figField{figR(r), base64.StdEncoding.EncodeToString(raw)}
	}

	f := os.Getenv("FIG_OUT")
	if f == "" {
		f = "figdump.json"
	}
	data, _ := json.Marshal(out)
	if err := os.WriteFile(f, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d КБ, проходов %d, мест уже реза %d", f, len(data)/1024, len(iso.paths), len(iso.marks))
}
