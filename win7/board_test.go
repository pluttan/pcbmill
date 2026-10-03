package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// boardSamples: the zips in testdata and the examples of the repository.
func boardSamples(t *testing.T) map[string]string {
	out := map[string]string{}
	zs, _ := filepath.Glob("testdata/gerbers/*.zip")
	for _, z := range zs {
		out[stem(z)] = z
	}
	for _, d := range []string{"eagle", "easyeda", "kicad-flashpads"} {
		out["ex-"+d] = filepath.Join("..", "examples", "gerbers", d)
	}
	return out
}

func stem(p string) string { b := filepath.Base(p); return b[:len(b)-len(filepath.Ext(b))] }

// layerArea: the area of a layer as filled into pixels of px mm.
func layerArea(g *gerber, px float64) (float64, rect) {
	b, _ := g.bounds()
	r := newRaster(rect{b.X0 - 1, b.X1 + 1, b.Y0 - 1, b.Y1 + 1}, px)
	for _, s := range g.shapes {
		r.draw(s)
	}
	n := 0
	for _, v := range r.bits {
		n += int(v)
	}
	return float64(n) * r.px * r.px, b
}

// TestBoardsLikeGerbonara: the areas of copper and mask, the outline and the
// holes come out as the reference reader (pcbmill, gerbonara + shapely) has them.
func TestBoardsLikeGerbonara(t *testing.T) {
	raw, err := os.ReadFile("testdata/gerbers/ref.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	for name, path := range boardSamples(t) {
		want := ref[name]
		if want == nil {
			t.Errorf("%s: нет в ref.json", name)
			continue
		}
		b, err := loadBoard(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		layers := map[string]*gerber{"top": b.copper[lyTop], "bottom": b.copper[lyBottom],
			"mask_top": b.mask[lyTopMask], "mask_bot": b.mask[lyBotMask]}
		for k, g := range layers {
			var w []json.RawMessage
			if want[k] == nil {
				if g != nil {
					t.Errorf("%s %s: лишний слой %v", name, k, b.files)
				}
				continue
			}
			json.Unmarshal(want[k], &w)
			var area float64
			json.Unmarshal(w[0], &area)
			if g == nil {
				if area > 0 {
					t.Errorf("%s %s: слой не найден", name, k)
				}
				continue
			}
			got, _ := layerArea(g, 0.01)
			if d := math.Abs(got-area) / math.Max(area, 1); d > 0.005 {
				t.Errorf("%s %s: площадь %.2f, а у эталона %.2f (%.2f%%)", name, k, got, area, d*100)
			} else {
				t.Logf("%s %s: площадь %.2f / %.2f", name, k, got, area)
			}
		}
		if want["outline"] != nil {
			var w []json.RawMessage
			json.Unmarshal(want["outline"], &w)
			var bb []float64
			json.Unmarshal(w[1], &bb)
			got := linesBox(b.outline)
			for i, v := range []float64{got.X0, got.Y0, got.X1, got.Y1} {
				if math.Abs(v-bb[i]) > 0.01 {
					t.Errorf("%s контур: %v, а у эталона %v", name, got, bb)
					break
				}
			}
			if !b.closed {
				t.Errorf("%s: контур не замкнут", name)
			}
		}
		var h []float64
		json.Unmarshal(want["holes"], &h)
		sum, slots := 0.0, 0
		for _, x := range b.holes {
			sum += x.D
			if x.Slot {
				slots++
			}
		}
		if len(b.holes) != int(h[0]) || math.Abs(sum-h[1]) > 0.01 || slots != int(h[2]) {
			t.Errorf("%s отверстия: %d (сумма d %.3f, пазов %d), у эталона %v", name, len(b.holes), sum, slots, h)
		}
	}
}
