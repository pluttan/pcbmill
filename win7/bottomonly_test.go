package main

import (
	"path/filepath"
	"testing"

	"pcbscan/ui"
)

// TestBottomOnly: a board with copper on B.Cu only is seen as such, and the
// picture of the sheet shows its copper, not a dark square.
func TestBottomOnly(t *testing.T) {
	zip, _ := filepath.Abs("../testboard/pcbmill-test.zip")
	b, err := loadBoard(zip)
	if err != nil {
		t.Skip(err)
	}
	b.copper[lyBottom], b.copper[lyTop] = b.copper[lyTop], nil
	j := newJob(filepath.Join(t.TempDir(), "job"))
	j.Srcs = []jobSrc{{zip, 1}}
	j.boards = []*board{b}
	if !j.bottomOnly() {
		t.Fatal("плата только с B.Cu не распознана")
	}
	if err := j.layout(false, false, false); err != nil {
		t.Fatal(err)
	}
	th, err := ui.NewTheme()
	if err != nil {
		t.Fatal(err)
	}
	old := theme
	theme = th
	defer func() { theme = old }()
	img, _ := newSheetScene(j, false, sheetOverlay{}).geometry()
	bright := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if int(img.Pix[i])+int(img.Pix[i+1])+int(img.Pix[i+2]) > int(th.Copper.R)+int(th.Copper.G)+int(th.Copper.B)-40 {
			bright++
		}
	}
	if bright < len(img.Pix)/4/100 {
		t.Errorf("меди на картинке %d пикселей из %d", bright, len(img.Pix)/4)
	}
}
