package ui

// Catppuccin Mocha and the fonts of the system: Segoe UI and Consolas on
// Windows 7, Tahoma or Arial on a Mac, Liberation or DejaVu on Linux.

import (
	"fmt"
	"image/color"
	"math"

	"pcbscan/gfx"
)

type Theme struct {
	scale float64

	Base, Mantle, Crust                 color.NRGBA
	Surface0, Surface1, Surface2        color.NRGBA
	Overlay0, Overlay1, Overlay2        color.NRGBA
	Subtext0, Subtext1, Text            color.NRGBA
	Lavender, Blue, Sapphire, Sky, Teal color.NRGBA
	Green, Yellow, Peach, Maroon, Red   color.NRGBA
	Mauve, Pink, Flamingo, Rosewater    color.NRGBA
	Dim                                 color.NRGBA // under a dialog
	Copper, CopperDark                  color.NRGBA // the sheet in 3D

	regular, bold, mono *gfx.Font
	fallback            *gfx.Font
	faces               map[faceKey]*gfx.Face
}

type faceKey struct {
	kind FontKind
	px   float64
}

type FontKind int

const (
	Regular FontKind = iota
	Bold
	Mono
)

// NewTheme loads the fonts; without a font there is no text, so it fails.
func NewTheme() (*Theme, error) {
	h := gfx.Hex
	t := &Theme{scale: 1,
		Base: h("#000000"), Mantle: h("#0b0b10"), Crust: h("#000000"), // black backgrounds, asked for
		Surface0: h("#313244"), Surface1: h("#45475a"), Surface2: h("#585b70"),
		Overlay0: h("#6c7086"), Overlay1: h("#7f849c"), Overlay2: h("#9399b2"),
		Subtext0: h("#a6adc8"), Subtext1: h("#bac2de"), Text: h("#cdd6f4"),
		Lavender: h("#b4befe"), Blue: h("#89b4fa"), Sapphire: h("#74c7ec"), Sky: h("#89dceb"), Teal: h("#94e2d5"),
		Green: h("#a6e3a1"), Yellow: h("#f9e2af"), Peach: h("#fab387"), Maroon: h("#eba0ac"), Red: h("#f38ba8"),
		Mauve: h("#cba6f7"), Pink: h("#f5c2e7"), Flamingo: h("#f2cdcd"), Rosewater: h("#f5e0dc"),
		faces: map[faceKey]*gfx.Face{},
	}
	t.Dim = gfx.Alpha(t.Crust, 0.55)
	t.Copper = gfx.Mix(t.Peach, t.Yellow, 0.2)
	t.CopperDark = gfx.Mix(gfx.Mix(t.Peach, t.Maroon, 0.35), h("#000000"), 0.55)
	load := func(names ...string) *gfx.Font {
		p := gfx.FindFont(names...)
		if p == "" {
			return nil
		}
		f, err := gfx.LoadFont(p)
		if err != nil {
			return nil
		}
		return f
	}
	t.regular = load("segoeui.ttf", "Tahoma.ttf", "tahoma.ttf", "Arial.ttf", "arial.ttf",
		"LiberationSans-Regular.ttf", "DejaVuSans.ttf", "NotoSans-Regular.ttf")
	if t.regular == nil {
		return nil, fmt.Errorf("нет системного шрифта (Segoe UI, Tahoma, Arial)")
	}
	t.bold = load("seguisb.ttf", "segoeuib.ttf", "Tahoma Bold.ttf", "tahomabd.ttf", "Arial Bold.ttf", "arialbd.ttf",
		"LiberationSans-Bold.ttf", "DejaVuSans-Bold.ttf", "NotoSans-Bold.ttf")
	if t.bold == nil {
		t.bold = t.regular
	}
	t.mono = load("consola.ttf", "Courier New.ttf", "cour.ttf", "LiberationMono-Regular.ttf",
		"DejaVuSansMono.ttf", "NotoSansMono-Regular.ttf")
	if t.mono == nil {
		t.mono = t.regular
	}
	t.fallback = load("Arial Unicode.ttf", "DejaVuSans.ttf", "arial.ttf", "Arial.ttf", "segoeui.ttf")
	return t, nil
}

func (t *Theme) setScale(s float64) {
	if s < 0.75 || math.IsNaN(s) {
		s = 1
	}
	if s != t.scale {
		t.scale = s
		t.faces = map[faceKey]*gfx.Face{}
	}
}

// S: a length of the design (at 96 dpi) in pixels of this screen.
func (t *Theme) S(v float64) float64 { return v * t.scale }

// Si: the same, rounded.
func (t *Theme) Si(v float64) int { return int(math.Round(v * t.scale)) }

// Face: a font of the theme at a design size.
func (t *Theme) Face(kind FontKind, size float64) *gfx.Face {
	k := faceKey{kind, size}
	if f, ok := t.faces[k]; ok {
		return f
	}
	src := t.regular
	switch kind {
	case Bold:
		src = t.bold
	case Mono:
		src = t.mono
	}
	f := src.Face(t.S(size))
	if t.fallback != nil && t.fallback != src {
		f.Fallback = t.fallback.Face(t.S(size))
	}
	t.faces[k] = f
	return f
}

// The sizes of the text, at 96 dpi.
const (
	SizeSmall = 12.0
	SizeBody  = 14.0
	SizeLarge = 16.0
	SizeTitle = 20.0
	SizeHuge  = 30.0
)
