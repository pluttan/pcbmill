package r3

import (
	"image"
	"image/color"
	"math"
)

// ==========================
// ===  Mesh helpers      ===
// ==========================

// addTri appends a triangle wound so its geometric normal agrees with the
// vertex normals; builders then never need hand-checked winding.
func (m *Mesh) addTri(a, b, c int32) {
	p0, p1, p2 := m.V[a].P, m.V[b].P, m.V[c].P
	ng := p1.Sub(p0).Cross(p2.Sub(p0))
	if !(ng.Dot(ng) > 0) {
		return
	}
	n := m.V[a].N.Add(m.V[b].N).Add(m.V[c].N)
	if ng.Dot(n) < 0 {
		b, c = c, b
	}
	m.Idx = append(m.Idx, a, b, c)
}

func (m *Mesh) add(p, n Vec, col color.NRGBA) int32 {
	m.V = append(m.V, Vertex{P: p, N: n, C: col})
	return int32(len(m.V) - 1)
}

// Bounds returns the box around all finite vertices.
func (m *Mesh) Bounds() (min, max Vec) {
	inf := math.Inf(1)
	min, max = Vec{inf, inf, inf}, Vec{-inf, -inf, -inf}
	found := false
	for i := range m.V {
		p := m.V[i].P
		if p.bad() {
			continue
		}
		found = true
		min = Vec{minf(min.X, p.X), minf(min.Y, p.Y), minf(min.Z, p.Z)}
		max = Vec{maxf(max.X, p.X), maxf(max.Y, p.Y), maxf(max.Z, p.Z)}
	}
	if !found {
		return Vec{}, Vec{}
	}
	return min, max
}

// Merge concatenates meshes; flags come from the first one, the texture from
// the first that has one.
func Merge(ms ...*Mesh) *Mesh {
	out := &Mesh{}
	first := true
	for _, m := range ms {
		if m == nil {
			continue
		}
		if first {
			out.Lit, out.TwoSided, out.Smooth = m.Lit, m.TwoSided, m.Smooth
			first = false
		}
		if out.Tex == nil {
			out.Tex = m.Tex
		}
		off := int32(len(out.V))
		out.V = append(out.V, m.V...)
		for _, i := range m.Idx {
			out.Idx = append(out.Idx, i+off)
		}
	}
	return out
}

// ==========================
// ===  Solids            ===
// ==========================

// Box is an axis-aligned box with flat faces.
func Box(min, max Vec, col color.NRGBA) *Mesh {
	m := &Mesh{Lit: true}
	c := func(i int) Vec {
		p := min
		if i&1 != 0 {
			p.X = max.X
		}
		if i&2 != 0 {
			p.Y = max.Y
		}
		if i&4 != 0 {
			p.Z = max.Z
		}
		return p
	}
	faces := [6]struct {
		q [4]int
		n Vec
	}{
		{[4]int{0, 2, 3, 1}, Vec{0, 0, -1}},
		{[4]int{4, 5, 7, 6}, Vec{0, 0, 1}},
		{[4]int{0, 1, 5, 4}, Vec{0, -1, 0}},
		{[4]int{2, 6, 7, 3}, Vec{0, 1, 0}},
		{[4]int{0, 4, 6, 2}, Vec{-1, 0, 0}},
		{[4]int{1, 3, 7, 5}, Vec{1, 0, 0}},
	}
	for _, fc := range faces {
		var id [4]int32
		for k := 0; k < 4; k++ {
			id[k] = m.add(c(fc.q[k]), fc.n, col)
		}
		m.addTri(id[0], id[1], id[2])
		m.addTri(id[0], id[2], id[3])
	}
	return m
}

func ring(segs int) (cs, sn []float64) {
	cs, sn = make([]float64, segs+1), make([]float64, segs+1)
	for i := 0; i <= segs; i++ {
		a := 2 * math.Pi * float64(i) / float64(segs)
		cs[i], sn[i] = math.Cos(a), math.Sin(a)
	}
	return cs, sn
}

// Cylinder stands on base along +Z with both caps.
func Cylinder(base Vec, r, h float64, segs int, col color.NRGBA) *Mesh {
	if segs < 3 {
		segs = 3
	}
	m := &Mesh{Lit: true, Smooth: true}
	cs, sn := ring(segs)
	top := base.Add(Vec{0, 0, h})
	for i := 0; i < segs; i++ {
		n0, n1 := Vec{cs[i], sn[i], 0}, Vec{cs[i+1], sn[i+1], 0}
		b0 := m.add(base.Add(n0.Scale(r)), n0, col)
		b1 := m.add(base.Add(n1.Scale(r)), n1, col)
		t0 := m.add(top.Add(n0.Scale(r)), n0, col)
		t1 := m.add(top.Add(n1.Scale(r)), n1, col)
		m.addTri(b0, b1, t1)
		m.addTri(b0, t1, t0)
	}
	capDisc := func(c Vec, n Vec) {
		ci := m.add(c, n, col)
		for i := 0; i < segs; i++ {
			a := m.add(c.Add(Vec{cs[i] * r, sn[i] * r, 0}), n, col)
			b := m.add(c.Add(Vec{cs[i+1] * r, sn[i+1] * r, 0}), n, col)
			m.addTri(ci, a, b)
		}
	}
	capDisc(base, Vec{0, 0, -1})
	capDisc(top, Vec{0, 0, 1})
	return m
}

// Cone has its tip at tip and opens upwards to radius r at height h,
// capped on top: the shape of a V-bit.
func Cone(tip Vec, r, h float64, segs int, col color.NRGBA) *Mesh {
	if segs < 3 {
		segs = 3
	}
	m := &Mesh{Lit: true, Smooth: true}
	cs, sn := ring(segs)
	top := tip.Add(Vec{0, 0, h})
	nrm := func(c, s float64) Vec { return Vec{c * h, s * h, -r}.Norm() }
	for i := 0; i < segs; i++ {
		mc := math.Cos(2 * math.Pi * (float64(i) + 0.5) / float64(segs))
		ms := math.Sin(2 * math.Pi * (float64(i) + 0.5) / float64(segs))
		t := m.add(tip, nrm(mc, ms), col)
		a := m.add(top.Add(Vec{cs[i] * r, sn[i] * r, 0}), nrm(cs[i], sn[i]), col)
		b := m.add(top.Add(Vec{cs[i+1] * r, sn[i+1] * r, 0}), nrm(cs[i+1], sn[i+1]), col)
		m.addTri(t, a, b)
	}
	ci := m.add(top, Vec{0, 0, 1}, col)
	for i := 0; i < segs; i++ {
		a := m.add(top.Add(Vec{cs[i] * r, sn[i] * r, 0}), Vec{0, 0, 1}, col)
		b := m.add(top.Add(Vec{cs[i+1] * r, sn[i+1] * r, 0}), Vec{0, 0, 1}, col)
		m.addTri(ci, a, b)
	}
	return m
}

// Sphere is a latitude-longitude sphere with segs around and segs/2 rings.
func Sphere(c Vec, r float64, segs int, col color.NRGBA) *Mesh {
	if segs < 4 {
		segs = 4
	}
	rings := segs / 2
	m := &Mesh{Lit: true, Smooth: true}
	cs, sn := ring(segs)
	for j := 0; j <= rings; j++ {
		phi := math.Pi * float64(j) / float64(rings)
		sz, cz := math.Sin(phi), math.Cos(phi)
		for i := 0; i <= segs; i++ {
			n := Vec{cs[i] * sz, sn[i] * sz, cz}
			m.add(c.Add(n.Scale(r)), n, col)
		}
	}
	w := int32(segs + 1)
	for j := int32(0); j < int32(rings); j++ {
		for i := int32(0); i < int32(segs); i++ {
			a, b := j*w+i, j*w+i+1
			cc, d := a+w, b+w
			m.addTri(a, cc, d) // pole triangles with zero area drop out
			m.addTri(a, d, b)
		}
	}
	return m
}

// ==========================
// ===  Surfaces          ===
// ==========================

// Surface builds an nx*ny heightfield over [x0,x1]x[y0,y1]: vertex (i,j)
// sits at x index i, y index j. NaN heights are holes; cells with three
// valid corners keep one triangle so hole edges stay tight.
func Surface(nx, ny int, x0, y0, x1, y1 float64, z func(i, j int) float64, col func(i, j int, z float64) color.NRGBA) *Mesh {
	m := &Mesh{Lit: true, Smooth: true, TwoSided: true}
	if nx < 2 || ny < 2 || z == nil {
		return m
	}
	zs := make([]float64, nx*ny)
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			zs[j*nx+i] = z(i, j)
		}
	}
	dx := (x1 - x0) / float64(nx-1)
	dy := (y1 - y0) / float64(ny-1)
	ok := func(i, j int) bool {
		if i < 0 || j < 0 || i >= nx || j >= ny {
			return false
		}
		v := zs[j*nx+i]
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	// slope by central differences, one-sided next to edges and holes
	slope := func(i, j, di, dj int, step float64) float64 {
		c := zs[j*nx+i]
		hasP, hasM := ok(i+di, j+dj), ok(i-di, j-dj)
		switch {
		case hasP && hasM:
			return (zs[(j+dj)*nx+i+di] - zs[(j-dj)*nx+i-di]) / (2 * step)
		case hasP:
			return (zs[(j+dj)*nx+i+di] - c) / step
		case hasM:
			return (c - zs[(j-dj)*nx+i-di]) / step
		}
		return 0
	}
	m.V = make([]Vertex, nx*ny)
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			vx := &m.V[j*nx+i]
			vx.P = Vec{x0 + dx*float64(i), y0 + dy*float64(j), 0}
			vx.U, vx.V = float64(i)/float64(nx-1), 1-float64(j)/float64(ny-1)
			if !ok(i, j) {
				vx.P.Z = math.NaN()
				continue
			}
			zz := zs[j*nx+i]
			vx.P.Z = zz
			gx, gy := 0.0, 0.0
			if dx != 0 {
				gx = slope(i, j, 1, 0, dx)
			}
			if dy != 0 {
				gy = slope(i, j, 0, 1, dy)
			}
			vx.N = Vec{-gx, -gy, 1}.Norm()
			if col != nil {
				vx.C = col(i, j, zz)
			} else {
				vx.C = color.NRGBA{200, 200, 200, 255}
			}
		}
	}
	m.Idx = make([]int32, 0, (nx-1)*(ny-1)*6)
	for j := 0; j+1 < ny; j++ {
		for i := 0; i+1 < nx; i++ {
			a, b := int32(j*nx+i), int32(j*nx+i+1)
			c, d := a+int32(nx), b+int32(nx)
			va, vb, vc, vd := ok(i, j), ok(i+1, j), ok(i, j+1), ok(i+1, j+1)
			switch {
			case va && vb && vc && vd:
				m.addTri(a, b, d)
				m.addTri(a, d, c)
			case vb && vc && vd:
				m.addTri(b, d, c)
			case va && vc && vd:
				m.addTri(a, d, c)
			case va && vb && vd:
				m.addTri(a, b, d)
			case va && vb && vc:
				m.addTri(a, b, c)
			}
		}
	}
	return m
}

// Plane is a flat rectangle at height z facing +Z; UV runs 0..1 with the
// texture's top row at y1 so an image looks upright from above.
func Plane(x0, y0, x1, y1, z float64, tex *image.RGBA, col color.NRGBA) *Mesh {
	m := &Mesh{Lit: true, TwoSided: true, Tex: tex}
	n := Vec{0, 0, 1}
	m.V = []Vertex{
		{P: Vec{x0, y0, z}, N: n, C: col, U: 0, V: 1},
		{P: Vec{x1, y0, z}, N: n, C: col, U: 1, V: 1},
		{P: Vec{x1, y1, z}, N: n, C: col, U: 1, V: 0},
		{P: Vec{x0, y1, z}, N: n, C: col, U: 0, V: 0},
	}
	m.addTri(0, 1, 2)
	m.addTri(0, 2, 3)
	return m
}

// ==========================
// ===  Colormap          ===
// ==========================

// Colormap interpolates evenly spaced stops at t in 0..1, blending in
// linear light so gradients do not muddy in the middle.
func Colormap(stops []color.NRGBA, t float64) color.NRGBA {
	if len(stops) == 0 {
		return color.NRGBA{A: 255}
	}
	if len(stops) == 1 || math.IsNaN(t) {
		return stops[0]
	}
	t = clampf(t, 0, 1) * float64(len(stops)-1)
	i := int(t)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	fr := t - float64(i)
	fr = fr*fr*(3-2*fr)*0.35 + fr*0.65 // soften the kinks at stops
	a, b := stops[i], stops[i+1]
	mix := func(x, y uint8) uint8 {
		lx, ly := math.Pow(float64(x)/255, 2.2), math.Pow(float64(y)/255, 2.2)
		return to8(math.Pow(lx+(ly-lx)*fr, 1/2.2) * 255)
	}
	return color.NRGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B),
		to8(float64(a.A) + (float64(b.A)-float64(a.A))*fr)}
}
