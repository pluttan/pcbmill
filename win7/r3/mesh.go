package r3

import (
	"image"
	"image/color"
	"math"
)

// ==========================
// ===  Public types      ===
// ==========================

// Light is one directional light plus the material response. A zero Light
// means "use the camera's KeyLight".
type Light struct {
	Dir                                   Vec // towards the light, world space
	Ambient, Diffuse, Specular, Shininess float64
	Headlight                             bool // light from the camera, Dir ignored
}

// Vertex carries position, normal, colour and texture coordinates.
type Vertex struct {
	P, N Vec
	C    color.NRGBA
	U, V float64
}

// Mesh is an indexed triangle list. Front faces wind counter-clockwise seen
// from outside; back faces are culled unless TwoSided.
type Mesh struct {
	V        []Vertex
	Idx      []int32
	Tex      *image.RGBA // optional, multiplied by vertex colour
	Lit      bool        // false: vertex colours as they are
	TwoSided bool        // back faces drawn and lit with the flipped normal
	Smooth   bool        // per-pixel normals (Phong); false: Gouraud
}

// ==========================
// ===  Pipeline data     ===
// ==========================

// Attribute slots carried through clipping and interpolation.
const (
	atR = iota
	atG
	atB
	atA
	atNX
	atNY
	atNZ
	atU
	atV
	nAttr
)

const (
	subpix    = 16   // vertex snapping grid, keeps edge functions exact
	triChunk  = 2048 // triangles per setup job; fans stay below 1<<16
	vertChunk = 4096
)

// cvert is a vertex in clip space: screen offsets times w, view depth, w.
type cvert struct {
	xs, ys, d, w float64
	a            [nAttr]float64
	oc           uint8 // bit k set: outside clip plane k
}

type plane struct{ v, gx, gy float64 }

// edge function E(p) = dx*(py-ay) - dy*(px-ax); inside when E >= thr.
type edge struct{ ax, ay, dx, dy, thr float64 }

// tri is a screen triangle ready to rasterize; planes are relative to (x0,y0).
// Slot 0 is 1/w, slot 1 the depth key, then attributes times 1/w.
type tri struct {
	x0, y0                 float64
	ex                     [3]edge
	minX, maxX, minY, maxY int32
	back                   bool
	pl                     [2 + nAttr]plane
}

// ==========================
// ===  DrawMesh          ===
// ==========================

// DrawMesh rasterizes the mesh with depth test and depth write.
func (f *Frame) DrawMesh(c *Camera, m *Mesh, l Light) {
	if m == nil || len(m.V) == 0 || len(m.Idx) < 3 || c == nil {
		return
	}
	v := c.view(f.W, f.H)
	sh := f.newShader(c, &v, m, l)
	f.transform(&v, m, sh)
	nt := len(m.Idx) / 3
	nc := (nt + triChunk - 1) / triChunk
	for len(f.chunks) < nc {
		f.chunks = append(f.chunks, nil)
	}
	parallel(nc, func(k int) {
		f.chunks[k] = f.setup(&v, m, sh, k*triChunk, mini(nt, (k+1)*triChunk), f.chunks[k][:0])
	})
	nb, bh := f.bands()
	for len(f.bins) < nb {
		f.bins = append(f.bins, nil)
	}
	for b := 0; b < nb; b++ {
		f.bins[b] = f.bins[b][:0]
	}
	for k := 0; k < nc; k++ {
		for i := range f.chunks[k] {
			t := &f.chunks[k][i]
			id := uint32(k<<16 | i)
			for b := int(t.minY) / bh; b <= int(t.maxY)/bh && b < nb; b++ {
				f.bins[b] = append(f.bins[b], id)
			}
		}
	}
	parallel(nb, func(b int) {
		f.rasterBand(b*bh, mini((b+1)*bh, f.H), f.bins[b], sh)
	})
}

// transform moves every vertex into clip space once; Gouraud lighting is
// done here because it only depends on the vertex.
func (f *Frame) transform(v *view, m *Mesh, sh *shader) {
	n := len(m.V)
	if cap(f.verts) < n {
		f.verts = make([]cvert, n)
	}
	f.verts = f.verts[:n]
	gouraud := m.Lit && !m.Smooth
	if gouraud && m.TwoSided {
		if cap(f.back) < n {
			f.back = make([][4]float64, n)
		}
		f.back = f.back[:n]
	}
	parallel((n+vertChunk-1)/vertChunk, func(k int) {
		for i := k * vertChunk; i < n && i < (k+1)*vertChunk; i++ {
			src := &m.V[i]
			cv := &f.verts[i]
			if src.P.bad() {
				cv.d = math.NaN()
				continue
			}
			cv.xs, cv.ys, cv.d, cv.w = v.toView(src.P)
			cv.oc = 0
			for k := 0; k < 5; k++ {
				if !(clipVal(v, cv, k) >= 0) {
					cv.oc |= 1 << uint(k)
				}
			}
			base := [4]float64{float64(src.C.R), float64(src.C.G), float64(src.C.B), float64(src.C.A)}
			nrm := src.N.Norm()
			switch {
			case gouraud:
				lc := sh.light(base, nrm)
				copy(cv.a[:4], lc[:])
				if m.TwoSided {
					f.back[i] = sh.light(base, nrm.Scale(-1))
				}
			default:
				copy(cv.a[:4], base[:])
			}
			cv.a[atNX], cv.a[atNY], cv.a[atNZ] = nrm.X, nrm.Y, nrm.Z
			cv.a[atU], cv.a[atV] = src.U, src.V
		}
	})
}

// clipVal is the signed distance to clip plane k (>= 0 inside): near plane,
// then a guard band around the screen so fixed-point setup cannot overflow.
func clipVal(v *view, c *cvert, k int) float64 {
	switch k {
	case 0:
		return c.d - v.near
	case 1:
		return v.guard*c.w - c.xs
	case 2:
		return v.guard*c.w + c.xs
	case 3:
		return v.guard*c.w - c.ys
	}
	return v.guard*c.w + c.ys
}

func lerpCV(a, b *cvert, t float64) cvert {
	r := cvert{
		xs: a.xs + (b.xs-a.xs)*t, ys: a.ys + (b.ys-a.ys)*t,
		d: a.d + (b.d-a.d)*t, w: a.w + (b.w-a.w)*t,
	}
	for k := 0; k < nAttr; k++ {
		r.a[k] = a.a[k] + (b.a[k]-a.a[k])*t
	}
	return r
}

// setup turns triangles [t0,t1) into screen triangles appended to out.
func (f *Frame) setup(v *view, m *Mesh, sh *shader, t0, t1 int, out []tri) []tri {
	nv := uint32(len(m.V))
	toEye := v.f.Scale(-1)
	var poly, tmp [9]cvert
	for t := t0; t < t1; t++ {
		i0, i1, i2 := uint32(m.Idx[3*t]), uint32(m.Idx[3*t+1]), uint32(m.Idx[3*t+2])
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		vp := [3]*cvert{&f.verts[i0], &f.verts[i1], &f.verts[i2]}
		if math.IsNaN(vp[0].d + vp[1].d + vp[2].d) {
			continue
		}
		// Outcodes: reject when all three are outside one plane, skip the
		// clipper entirely when all are inside.
		and := vp[0].oc & vp[1].oc & vp[2].oc
		or := vp[0].oc | vp[1].oc | vp[2].oc
		if and != 0 {
			continue
		}
		// Facing from world geometry: robust even when the triangle
		// straddles the near plane.
		p0 := m.V[i0].P
		ng := m.V[i1].P.Sub(p0).Cross(m.V[i2].P.Sub(p0))
		var s float64
		if v.ortho {
			s = ng.Dot(toEye)
		} else {
			s = ng.Dot(v.eye.Sub(p0))
		}
		if !(s != 0) {
			continue
		}
		back := s < 0
		if back && !m.TwoSided {
			continue
		}
		backCol := back && sh.lit && !sh.smooth && m.TwoSided
		if or == 0 && !backCol {
			out = f.emit(v, sh, vp[0], vp[1], vp[2], back, out)
			continue
		}
		poly[0], poly[1], poly[2] = *vp[0], *vp[1], *vp[2]
		if backCol {
			copy(poly[0].a[:4], f.back[i0][:])
			copy(poly[1].a[:4], f.back[i1][:])
			copy(poly[2].a[:4], f.back[i2][:])
		}
		n := 3
		if or != 0 {
			n = clipPoly(v, &poly, &tmp, or)
		}
		for j := 1; j+1 < n; j++ {
			out = f.emit(v, sh, &poly[0], &poly[j], &poly[j+1], back, out)
		}
	}
	return out
}

// clipPoly clips the triangle in p against the planes flagged in mask
// (Sutherland-Hodgman in clip space) and returns the vertex count.
func clipPoly(v *view, p, tmp *[9]cvert, mask uint8) int {
	n := 3
	for k := 0; k < 5 && n >= 3; k++ {
		if mask&(1<<uint(k)) == 0 {
			continue
		}
		m := 0
		for i := 0; i < n; i++ {
			a, b := &p[i], &p[(i+1)%n]
			da, db := clipVal(v, a, k), clipVal(v, b, k)
			if da >= 0 && m < 9 {
				tmp[m] = *a
				m++
			}
			if (da >= 0) != (db >= 0) && m < 9 {
				tmp[m] = lerpCV(a, b, da/(da-db))
				m++
			}
		}
		*p = *tmp
		n = m
	}
	return n
}

// snap rounds to the sub-pixel grid; the guard band keeps x small enough
// for an int conversion, which is much cheaper than math.Floor on 386.
func snap(x float64) float64 {
	v := x*subpix + 0.5
	i := float64(int(v))
	if i > v {
		i--
	}
	return i / subpix
}

// emit projects one clipped triangle and builds its edges and planes.
func (f *Frame) emit(v *view, sh *shader, a, b, c *cvert, back bool, out []tri) []tri {
	cv := [3]*cvert{a, b, c}
	var xs, ys, q, key [3]float64
	for i, p := range cv {
		if !(p.w > 0) {
			return out
		}
		xs[i] = snap(v.cx + p.xs/p.w)
		ys[i] = snap(v.cy + p.ys/p.w)
		q[i] = 1 / p.w
		key[i] = v.key(p.d)
	}
	area := (xs[1]-xs[0])*(ys[2]-ys[0]) - (xs[2]-xs[0])*(ys[1]-ys[0])
	if !(area != 0) || math.IsInf(area, 0) {
		return out
	}
	if area < 0 {
		area = -area
		xs[1], xs[2] = xs[2], xs[1]
		ys[1], ys[2] = ys[2], ys[1]
		q[1], q[2] = q[2], q[1]
		key[1], key[2] = key[2], key[1]
		cv[1], cv[2] = cv[2], cv[1]
	}
	minx, maxx := minf(xs[0], minf(xs[1], xs[2])), maxf(xs[0], maxf(xs[1], xs[2]))
	miny, maxy := minf(ys[0], minf(ys[1], ys[2])), maxf(ys[0], maxf(ys[1], ys[2]))
	if maxx < 0 || maxy < 0 || minx > float64(f.W) || miny > float64(f.H) {
		return out
	}
	x0, x1 := maxi(0, int(math.Floor(minx))), mini(f.W-1, int(math.Ceil(maxx)))
	y0, y1 := maxi(0, int(math.Floor(miny))), mini(f.H-1, int(math.Ceil(maxy)))
	if x0 > x1 || y0 > y1 {
		return out
	}
	// grow in place: every field read later is written below, so the
	// slot needs no zeroing (the struct is large)
	if len(out) < cap(out) {
		out = out[:len(out)+1]
	} else {
		out = append(out, tri{})
	}
	t := &out[len(out)-1]
	t.minX, t.maxX, t.minY, t.maxY = int32(x0), int32(x1), int32(y0), int32(y1)
	t.back = back
	t.x0, t.y0 = xs[0], ys[0]
	for k := 0; k < 3; k++ {
		j := (k + 1) % 3
		e := edge{ax: xs[k], ay: ys[k], dx: xs[j] - xs[k], dy: ys[j] - ys[k]}
		// Top-left rule for this winding (y down): top edges run right,
		// left edges run up. Others need strictly positive E.
		if !((e.dy == 0 && e.dx > 0) || e.dy < 0) {
			e.thr = 0.5 / (subpix * subpix)
		}
		t.ex[k] = e
	}
	x10, y10 := xs[1]-xs[0], ys[1]-ys[0]
	x20, y20 := xs[2]-xs[0], ys[2]-ys[0]
	set := func(slot int, f0, f1, f2 float64) {
		t.pl[slot] = plane{
			v:  f0,
			gx: ((f1-f0)*y20 - (f2-f0)*y10) / area,
			gy: ((f2-f0)*x10 - (f1-f0)*x20) / area,
		}
	}
	set(0, q[0], q[1], q[2])
	set(1, key[0], key[1], key[2])
	for _, k := range sh.active {
		set(2+k, cv[0].a[k]*q[0], cv[1].a[k]*q[1], cv[2].a[k]*q[2])
	}
	return out
}
