package ui

// Cutting rectangles: a screen is laid out by taking strips off the sides
// of the space that is left, like cutting a sheet.

import "image"

func CutTop(r *image.Rectangle, h int) image.Rectangle {
	if h > r.Dy() {
		h = r.Dy()
	}
	out := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h)
	r.Min.Y += h
	return out
}

func CutBottom(r *image.Rectangle, h int) image.Rectangle {
	if h > r.Dy() {
		h = r.Dy()
	}
	out := image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
	r.Max.Y -= h
	return out
}

func CutLeft(r *image.Rectangle, w int) image.Rectangle {
	if w > r.Dx() {
		w = r.Dx()
	}
	out := image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y)
	r.Min.X += w
	return out
}

func CutRight(r *image.Rectangle, w int) image.Rectangle {
	if w > r.Dx() {
		w = r.Dx()
	}
	out := image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y)
	r.Max.X -= w
	return out
}

// Inset shrinks r by d on every side.
func Inset(r image.Rectangle, d int) image.Rectangle {
	out := image.Rect(r.Min.X+d, r.Min.Y+d, r.Max.X-d, r.Max.Y-d)
	if out.Dx() < 0 {
		out.Max.X = out.Min.X
	}
	if out.Dy() < 0 {
		out.Max.Y = out.Min.Y
	}
	return out
}

// InsetXY shrinks r by dx left and right, dy top and bottom.
func InsetXY(r image.Rectangle, dx, dy int) image.Rectangle {
	out := image.Rect(r.Min.X+dx, r.Min.Y+dy, r.Max.X-dx, r.Max.Y-dy)
	if out.Dx() < 0 {
		out.Max.X = out.Min.X
	}
	if out.Dy() < 0 {
		out.Max.Y = out.Min.Y
	}
	return out
}

// Center: a w x h rectangle in the middle of r.
func Center(r image.Rectangle, w, h int) image.Rectangle {
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + (r.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// SplitX cuts r into n columns with gap between them.
func SplitX(r image.Rectangle, n, gap int) []image.Rectangle {
	if n < 1 {
		return nil
	}
	w := (r.Dx() - gap*(n-1)) / n
	var out []image.Rectangle
	x := r.Min.X
	for i := 0; i < n; i++ {
		x1 := x + w
		if i == n-1 {
			x1 = r.Max.X
		}
		out = append(out, image.Rect(x, r.Min.Y, x1, r.Max.Y))
		x = x1 + gap
	}
	return out
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
