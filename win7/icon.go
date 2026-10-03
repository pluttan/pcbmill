package main

// The icon of the program: a small lit height map, MATLAB's membrane in the
// colours of the theme, rendered by the same 3D code as the views.

import (
	"image"
	"image/color"
	"math"

	"pcbscan/r3"
)

func appIcon(size int) *image.RGBA {
	f := r3.NewFrame(size*2, size*2)
	f.Clear(theme.Surface0, theme.Crust)
	m := r3.Surface(28, 28, -1, -1, 1, 1, func(i, j int) float64 {
		x, y := -1+2*float64(i)/27, -1+2*float64(j)/27
		return 0.9*math.Exp(-3*((x-0.2)*(x-0.2)+(y+0.1)*(y+0.1))) + 0.35*math.Exp(-6*((x+0.5)*(x+0.5)+(y-0.5)*(y-0.5)))
	}, func(i, j int, z float64) color.NRGBA { return r3.Colormap(heightStops(), z/0.95) })
	cam := r3.Camera{Yaw: -math.Pi/2 + 0.6, Pitch: 0.55, FOV: 0.5}
	cam.Fit(r3.Vec{X: -1, Y: -1, Z: 0}, r3.Vec{X: 1, Y: 1, Z: 0.95}, size*2, size*2)
	f.DrawMesh(&cam, m, surfLight)
	return f.Downsample2()
}
