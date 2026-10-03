package r3

import (
	"image"
	"math"
)

// ==========================
// ===  Shader            ===
// ==========================

const specN = 1024 // specular lookup resolution; math.Pow per pixel is slow

type shader struct {
	lit, smooth, two bool
	L, H             Vec // light and half vector (viewer at infinity)
	amb, kd, ks      float64
	spec             *[specN + 2]float64
	tex              *image.RGBA
	tw, th           int
	active           []int
}

func (f *Frame) newShader(c *Camera, v *view, m *Mesh, l Light) *shader {
	if l == (Light{}) {
		l = c.KeyLight()
	}
	sh := &shader{lit: m.Lit, smooth: m.Lit && m.Smooth, two: m.TwoSided}
	sh.amb, sh.kd, sh.ks = l.Ambient, l.Diffuse, l.Specular
	toEye := v.f.Scale(-1)
	sh.L = l.Dir.Norm()
	if l.Headlight || sh.L == (Vec{}) {
		sh.L = toEye
	}
	sh.H = sh.L.Add(toEye).Norm()
	if sh.H == (Vec{}) {
		sh.H = toEye
	}
	shin := l.Shininess
	if !(shin >= 1) {
		shin = 1
	}
	if shin > 2000 {
		shin = 2000
	}
	if f.specSh != shin {
		for i := 0; i <= specN+1; i++ {
			f.spec[i] = math.Pow(minf(float64(i)/specN, 1), shin)
		}
		f.specSh = shin
	}
	sh.spec = &f.spec
	sh.active = []int{atR, atG, atB, atA}
	if sh.smooth {
		sh.active = append(sh.active, atNX, atNY, atNZ)
	}
	if m.Tex != nil && m.Tex.Rect.Dx() > 0 && m.Tex.Rect.Dy() > 0 {
		sh.tex = m.Tex
		sh.tw, sh.th = m.Tex.Rect.Dx(), m.Tex.Rect.Dy()
		sh.active = append(sh.active, atU, atV)
	}
	return sh
}

func (sh *shader) specular(x float64) float64 {
	if !(x > 0) {
		return 0
	}
	p := x * specN
	i := int(p)
	if i >= specN {
		return 1
	}
	fr := p - float64(i)
	return sh.spec[i] + (sh.spec[i+1]-sh.spec[i])*fr
}

// light applies the MATLAB-like model: colour*(ambient+diffuse*cos) plus a
// white highlight that only appears on the lit side.
func (sh *shader) light(c [4]float64, n Vec) [4]float64 {
	nl := n.Dot(sh.L)
	k := sh.amb
	s := 0.0
	if nl > 0 {
		k += sh.kd * nl
		s = sh.ks * 255 * sh.specular(n.Dot(sh.H))
	}
	return [4]float64{c[0]*k + s, c[1]*k + s, c[2]*k + s, c[3]}
}

// sample is a bilinear, clamp-to-edge texture fetch in 0..255.
func (sh *shader) sample(u, v float64) (r, g, b, a float64) {
	fx := u*float64(sh.tw) - 0.5
	fy := v*float64(sh.th) - 0.5
	if math.IsNaN(fx + fy) {
		return 255, 255, 255, 255
	}
	fx = clampf(fx, 0, float64(sh.tw-1))
	fy = clampf(fy, 0, float64(sh.th-1))
	x0, y0 := int(fx), int(fy)
	x1, y1 := mini(x0+1, sh.tw-1), mini(y0+1, sh.th-1)
	tx, ty := fx-float64(x0), fy-float64(y0)
	t := sh.tex
	o00 := y0*t.Stride + 4*x0
	o10 := y0*t.Stride + 4*x1
	o01 := y1*t.Stride + 4*x0
	o11 := y1*t.Stride + 4*x1
	var out [4]float64
	for c := 0; c < 4; c++ {
		top := float64(t.Pix[o00+c]) + (float64(t.Pix[o10+c])-float64(t.Pix[o00+c]))*tx
		bot := float64(t.Pix[o01+c]) + (float64(t.Pix[o11+c])-float64(t.Pix[o01+c]))*tx
		out[c] = top + (bot-top)*ty
	}
	return out[0], out[1], out[2], out[3]
}

// ==========================
// ===  Rasterizer        ===
// ==========================

// rasterBand fills rows [y0,y1) with the binned triangles in order, so the
// result does not depend on how bands were spread over cores.
func (f *Frame) rasterBand(y0, y1 int, ids []uint32, sh *shader) {
	W := f.W
	pix := f.Img.Pix
	stride := f.Img.Stride
	zb := f.Z
	var row [2 + nAttr]float64
	for _, id := range ids {
		t := &f.chunks[id>>16][id&0xffff]
		ya, yb := maxi(int(t.minY), y0), mini(int(t.maxY), y1-1)
		for y := ya; y <= yb; y++ {
			yc := float64(y) + 0.5
			xl, xr := float64(t.minX), float64(t.maxX)
			var cc [3]float64
			empty := false
			for k := 0; k < 3; k++ {
				e := &t.ex[k]
				cc[k] = e.dx*(yc-e.ay) + e.dy*e.ax // E = cc - dy*px
				switch {
				case e.dy == 0:
					if cc[k] < e.thr {
						empty = true
					}
				case e.dy > 0:
					xr = minf(xr, floorIn((cc[k]-e.thr)/e.dy-0.5, xl, xr)+1)
				default:
					xl = maxf(xl, floorIn((cc[k]-e.thr)/e.dy-0.5, xl, xr))
				}
			}
			if empty || !(xl <= xr) {
				continue
			}
			ix0, ix1 := int(xl), int(xr)
			px0 := float64(ix0) + 0.5
			e0 := cc[0] - t.ex[0].dy*px0
			e1 := cc[1] - t.ex[1].dy*px0
			e2 := cc[2] - t.ex[2].dy*px0
			d0, d1, d2 := t.ex[0].dy, t.ex[1].dy, t.ex[2].dy
			r0, r1, r2 := t.ex[0].thr, t.ex[1].thr, t.ex[2].thr
			ox, oy := px0-t.x0, yc-t.y0
			row[0] = t.pl[0].v + t.pl[0].gx*ox + t.pl[0].gy*oy
			row[1] = t.pl[1].v + t.pl[1].gx*ox + t.pl[1].gy*oy
			for _, k := range sh.active {
				p := &t.pl[2+k]
				row[2+k] = p.v + p.gx*ox + p.gy*oy
			}
			zrow := zb[y*W : y*W+W]
			prow := pix[y*stride : y*stride+4*W]
			for x := ix0; x <= ix1; x, e0, e1, e2 = x+1, e0-d0, e1-d1, e2-d2 {
				if e0 < r0 || e1 < r1 || e2 < r2 {
					continue
				}
				dx := float64(x - ix0)
				key := float32(row[1] + t.pl[1].gx*dx)
				if !(key > zrow[x]) {
					continue
				}
				q := row[0] + t.pl[0].gx*dx
				if !(q > 0) {
					continue
				}
				w := 1 / q
				pl := &t.pl
				r := (row[2+atR] + pl[2+atR].gx*dx) * w
				g := (row[2+atG] + pl[2+atG].gx*dx) * w
				b := (row[2+atB] + pl[2+atB].gx*dx) * w
				a := (row[2+atA] + pl[2+atA].gx*dx) * w
				if sh.tex != nil {
					tr, tg, tb, ta := sh.sample((row[2+atU]+pl[2+atU].gx*dx)*w, (row[2+atV]+pl[2+atV].gx*dx)*w)
					r, g, b, a = r*tr/255, g*tg/255, b*tb/255, a*ta/255
				}
				if sh.smooth {
					nx := (row[2+atNX] + pl[2+atNX].gx*dx) * w
					ny := (row[2+atNY] + pl[2+atNY].gx*dx) * w
					nz := (row[2+atNZ] + pl[2+atNZ].gx*dx) * w
					l := nx*nx + ny*ny + nz*nz
					if l > 1e-30 {
						s := 1 / math.Sqrt(l)
						if t.back {
							s = -s
						}
						nx, ny, nz = nx*s, ny*s, nz*s
					}
					nl := nx*sh.L.X + ny*sh.L.Y + nz*sh.L.Z
					k := sh.amb
					sp := 0.0
					if nl > 0 {
						k += sh.kd * nl
						sp = sh.ks * 255 * sh.specular(nx*sh.H.X+ny*sh.H.Y+nz*sh.H.Z)
					}
					r, g, b = r*k+sp, g*k+sp, b*k+sp
				}
				o := 4 * x
				if a >= 254.5 {
					prow[o], prow[o+1], prow[o+2], prow[o+3] = to8(r), to8(g), to8(b), 255
					zrow[x] = key
					continue
				}
				if a < 0.5 {
					continue
				}
				blend(prow[o:o+4], r, g, b, a/255)
				if a >= 127.5 {
					zrow[x] = key
				}
			}
		}
	}
}

// floorIn floors v after clamping it to a little beyond [lo, hi]; the
// clamp keeps the int conversion safe and math.Floor is a call on 386.
func floorIn(v, lo, hi float64) float64 {
	v = clampf(v, lo-2, hi+2)
	i := float64(int(v))
	if i > v {
		i--
	}
	return i
}

// blend composites a straight colour over a premultiplied pixel.
func blend(p []uint8, r, g, b, a float64) {
	k := 1 - a
	p[0] = to8(r*a + float64(p[0])*k)
	p[1] = to8(g*a + float64(p[1])*k)
	p[2] = to8(b*a + float64(p[2])*k)
	p[3] = to8(255*a + float64(p[3])*k)
}
