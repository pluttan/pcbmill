package r3

import "math"

// ==========================
// ===  Camera            ===
// ==========================

// Camera orbits around Target. Yaw turns around +Z, Pitch lifts the eye
// above the XY plane. FOV 0 selects an orthographic view OrthoH mm high.
type Camera struct {
	Target     Vec
	Yaw, Pitch float64
	Dist       float64
	FOV        float64
	OrthoH     float64
	Near       float64 // near clip distance in mm; 0 = automatic (Dist/200)
}

const (
	maxPitch  = 89 * math.Pi / 180
	orbitRate = 0.008 // radians per pixel of drag
	zoomStep  = 0.85  // scale per wheel notch
	minDist   = 1e-3
	maxDist   = 1e7
)

// view is the camera resolved for one frame size; every projection goes
// through it so all drawing calls agree to the last bit.
type view struct {
	eye, r, u, f Vec
	cx, cy       float64
	focal        float64 // persp: pixels per unit of tangent
	scale        float64 // ortho: pixels per mm
	ortho        bool
	near         float64
	guard        float64 // clip-space guard band, pixels from centre
	dist         float64
}

func (c *Camera) view(w, h int) view {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	yaw, pitch, dist := c.Yaw, clampf(c.Pitch, -maxPitch, maxPitch), c.Dist
	if math.IsNaN(yaw) || math.IsInf(yaw, 0) {
		yaw = 0
	}
	if math.IsNaN(pitch) {
		pitch = 0
	}
	if !(dist >= minDist) {
		dist = minDist
	}
	if dist > maxDist {
		dist = maxDist
	}
	tg := c.Target
	if tg.bad() {
		tg = Vec{}
	}
	cp := math.Cos(pitch)
	dir := Vec{cp * math.Cos(yaw), cp * math.Sin(yaw), math.Sin(pitch)}
	v := view{eye: tg.Add(dir.Scale(dist)), f: dir.Scale(-1), dist: dist}
	v.r = Vec{v.f.Y, -v.f.X, 0}.Norm() // f x Z: pitch is clamped so never zero
	v.u = v.r.Cross(v.f)
	v.cx, v.cy = float64(w)/2, float64(h)/2
	v.guard = float64(maxi(w, h)) + 64
	if c.FOV > 0 && !math.IsInf(c.FOV, 0) {
		fov := minf(c.FOV, 170*math.Pi/180)
		v.focal = v.cy / math.Tan(fov/2)
		v.near = c.Near
		if !(v.near > 0) {
			v.near = dist / 200
		}
	} else {
		v.ortho = true
		oh := c.OrthoH
		if !(oh > 1e-6) || math.IsInf(oh, 0) {
			oh = dist
		}
		v.scale = float64(h) / oh
		v.near = c.Near // ortho clips only what is behind the eye
		if !(v.near > 0) {
			v.near = 0
		}
	}
	return v
}

// toView returns screen-space x/y offsets times w, the view depth d and w.
func (v *view) toView(p Vec) (xs, ys, d, w float64) {
	q := p.Sub(v.eye)
	x, y := q.Dot(v.r), q.Dot(v.u)
	d = q.Dot(v.f)
	if v.ortho {
		return x * v.scale, -y * v.scale, d, 1
	}
	return x * v.focal, -y * v.focal, d, d
}

// key maps view depth to the value stored in the z-buffer: linear in screen
// space and larger when nearer.
func (v *view) key(d float64) float64 {
	if v.ortho {
		return -d
	}
	if d < v.near*0.5 {
		d = v.near * 0.5
	}
	return 1 / d
}

// unitsPerPixel at the Target depth.
func (v *view) unitsPerPixel() float64 {
	if v.ortho {
		return 1 / v.scale
	}
	return v.dist / v.focal
}

// Eye is the camera position in world space.
func (c *Camera) Eye() Vec {
	v := c.view(1, 1)
	return v.eye
}

// Project maps a world point to pixel coordinates (pixel i covers [i, i+1))
// and its view depth; ok is false for points behind the near plane.
func (c *Camera) Project(p Vec, w, h int) (sx, sy, depth float64, ok bool) {
	v := c.view(w, h)
	xs, ys, d, ww := v.toView(p)
	if !(d >= v.near) || (!v.ortho && d <= 0) {
		return 0, 0, d, false
	}
	return v.cx + xs/ww, v.cy + ys/ww, d, true
}

// Ray returns the world ray through a pixel position.
func (c *Camera) Ray(sx, sy float64, w, h int) (origin, dir Vec) {
	v := c.view(w, h)
	dx, dy := sx-v.cx, sy-v.cy
	if v.ortho {
		s := 1 / v.scale
		return v.eye.Add(v.r.Scale(dx * s)).Sub(v.u.Scale(dy * s)), v.f
	}
	d := v.f.Add(v.r.Scale(dx / v.focal)).Sub(v.u.Scale(dy / v.focal))
	return v.eye, d.Norm()
}

// PickPlane intersects the pixel ray with the horizontal plane Z=z.
func (c *Camera) PickPlane(sx, sy float64, w, h int, z float64) (Vec, bool) {
	o, d := c.Ray(sx, sy, w, h)
	if math.Abs(d.Z) < 1e-12 {
		return Vec{}, false
	}
	t := (z - o.Z) / d.Z
	if (c.FOV > 0 && t < 0) || math.IsNaN(t) || math.IsInf(t, 0) {
		return Vec{}, false
	}
	p := o.Add(d.Scale(t))
	p.Z = z
	return p, true
}

// ==========================
// ===  Mouse navigation  ===
// ==========================

// Orbit turns the view as if the scene were grabbed: drag right turns the
// scene right, drag down tips its top towards the viewer.
func (c *Camera) Orbit(dx, dy float64) {
	if math.IsNaN(dx+dy) || math.IsInf(dx+dy, 0) {
		return
	}
	c.Yaw = math.Remainder(c.Yaw-dx*orbitRate, 2*math.Pi)
	c.Pitch = clampf(c.Pitch+dy*orbitRate, -maxPitch, maxPitch)
	if math.IsNaN(c.Yaw) {
		c.Yaw = 0
	}
	if math.IsNaN(c.Pitch) {
		c.Pitch = 0
	}
}

// Pan moves the target so the point under the mouse (at Target depth)
// follows the mouse exactly.
func (c *Camera) Pan(dx, dy float64, w, h int) {
	if math.IsNaN(dx+dy) || math.IsInf(dx+dy, 0) {
		return
	}
	v := c.view(w, h)
	s := v.unitsPerPixel()
	c.Target = c.Target.Sub(v.r.Scale(dx * s)).Add(v.u.Scale(dy * s))
}

// Zoom by wheel notches (positive zooms in) keeping the point under the
// cursor fixed on the Target-depth plane.
func (c *Camera) Zoom(notches, x, y float64, w, h int) {
	if math.IsNaN(notches+x+y) || math.IsInf(notches+x+y, 0) {
		return
	}
	v := c.view(w, h)
	s := v.unitsPerPixel()
	p := c.Target.Add(v.r.Scale((x - v.cx) * s)).Sub(v.u.Scale((y - v.cy) * s))
	k := math.Pow(zoomStep, notches)
	if v.ortho {
		oh := float64(h) / v.scale
		noh := clampf(oh*k, 1e-4, 1e7)
		k = noh / oh
		c.OrthoH = noh
	} else {
		nd := clampf(v.dist*k, minDist, maxDist)
		k = nd / v.dist
		c.Dist = nd
	}
	c.Target = p.Add(c.Target.Sub(p).Scale(k))
}

// Fit centres the box and picks a distance (or ortho height) so all its
// corners are inside the frame with a small margin.
func (c *Camera) Fit(min, max Vec, w, h int) {
	if min.bad() || max.bad() {
		return
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	ctr := min.Add(max).Scale(0.5)
	rad := max.Sub(min).Len() / 2
	if rad < 1e-3 {
		rad = 1e-3
	}
	c.Target = ctr
	c.Dist = rad * 3
	v := c.view(w, h)
	hw, hh := float64(w)/2*0.9, float64(h)/2*0.9
	need := 0.0
	ext := 0.0
	for i := 0; i < 8; i++ {
		p := Vec{min.X, min.Y, min.Z}
		if i&1 != 0 {
			p.X = max.X
		}
		if i&2 != 0 {
			p.Y = max.Y
		}
		if i&4 != 0 {
			p.Z = max.Z
		}
		q := p.Sub(ctr)
		x, y, z := math.Abs(q.Dot(v.r)), math.Abs(q.Dot(v.u)), q.Dot(v.f)
		if v.ortho {
			ext = maxf(ext, maxf(x/hw, y/hh))
			continue
		}
		need = maxf(need, maxf(x*v.focal/hw, y*v.focal/hh)-z)
	}
	if v.ortho {
		c.OrthoH = maxf(ext*float64(h), 1e-3)
		c.Dist = rad*4 + 1
		return
	}
	c.Dist = clampf(maxf(need, rad*1.05), minDist, maxDist)
}

// KeyLight is a MATLAB-like "material shiny" light placed upper front left
// of the current view; call it again after orbiting to keep it there.
func (c *Camera) KeyLight() Light {
	v := c.view(2, 2)
	dir := v.r.Scale(-0.55).Add(v.u.Scale(0.75)).Sub(v.f.Scale(0.7))
	return Light{Dir: dir.Norm(), Ambient: 0.3, Diffuse: 0.7, Specular: 0.5, Shininess: 48}
}
