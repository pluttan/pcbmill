// Package gfx is a small software 2D renderer: antialiased shapes, paths
// and TrueType text drawn into an *image.RGBA without any dependencies.
package gfx

import (
	"encoding/binary"
	"image/color"
	"math"
)

// ==========================
// ===  Color helpers     ===
// ==========================

// Hex parses "#rgb", "#rrggbb" or "#rrggbbaa"; malformed input gives
// opaque magenta so that mistakes are visible instead of silent.
func Hex(s string) color.NRGBA {
	bad := color.NRGBA{255, 0, 255, 255}
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	var v [8]uint8
	for i := 0; i < len(s); i++ {
		d, ok := hexDigit(s[i])
		if !ok || i >= 8 {
			return bad
		}
		v[i] = d
	}
	switch len(s) {
	case 3:
		return color.NRGBA{v[0] * 17, v[1] * 17, v[2] * 17, 255}
	case 6:
		return color.NRGBA{v[0]<<4 | v[1], v[2]<<4 | v[3], v[4]<<4 | v[5], 255}
	case 8:
		return color.NRGBA{v[0]<<4 | v[1], v[2]<<4 | v[3], v[4]<<4 | v[5], v[6]<<4 | v[7]}
	}
	return bad
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// Alpha multiplies the alpha of c by a (0..1).
func Alpha(c color.NRGBA, a float64) color.NRGBA {
	c.A = clamp8(float64(c.A) * a)
	return c
}

// Mix interpolates every channel linearly: t=0 gives a, t=1 gives b.
func Mix(a, b color.NRGBA, t float64) color.NRGBA {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	f := func(x, y uint8) uint8 { return clamp8(float64(x) + (float64(y)-float64(x))*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

func clamp8(v float64) uint8 {
	if v <= 0 || math.IsNaN(v) {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// ==========================
// ===  Pixel blending    ===
// ==========================

// div255 is an exact rounded x/255 for x <= 255*255.
func div255(x uint32) uint32 {
	x += 128
	return (x + x>>8) >> 8
}

// blendPix composites a straight color with effective alpha a (0..255)
// over one premultiplied pixel p (4 bytes).
func blendPix(p []uint8, col color.NRGBA, a uint32) {
	if a == 0 {
		return
	}
	if a == 255 {
		p[0], p[1], p[2], p[3] = col.R, col.G, col.B, 255
		return
	}
	ia := 255 - a
	p[0] = uint8(div255(uint32(col.R)*a + uint32(p[0])*ia))
	p[1] = uint8(div255(uint32(col.G)*a + uint32(p[1])*ia))
	p[2] = uint8(div255(uint32(col.B)*a + uint32(p[2])*ia))
	p[3] = uint8(div255(255*a + uint32(p[3])*ia))
}

// blendSpan paints n pixels starting at p with a constant alpha. Pixels
// are processed as two 16-bit lanes per uint32 (R|B and G|A), halving the
// multiplies; lanes never overflow since 255*255+255+128 < 1<<16.
func blendSpan(p []uint8, n int, col color.NRGBA, a uint32) {
	if a == 0 || n <= 0 {
		return
	}
	p = p[:n*4]
	if a == 255 {
		// Opaque: seed one pixel and double it with memmove.
		p[0], p[1], p[2], p[3] = col.R, col.G, col.B, 255
		for k := 4; k < len(p); k *= 2 {
			copy(p[k:], p[:k])
		}
		return
	}
	ia := 255 - a
	srb := uint32(col.R)*a | uint32(col.B)*a<<16
	sag := uint32(col.G)*a | 255*a<<16
	for i := 0; i < len(p); i += 4 {
		d := binary.LittleEndian.Uint32(p[i : i+4])
		rb := (d&0x00FF00FF)*ia + srb + 0x00800080
		ag := (d>>8&0x00FF00FF)*ia + sag + 0x00800080
		rb = (rb + rb>>8&0x00FF00FF) >> 8 & 0x00FF00FF
		ag = (ag + ag>>8&0x00FF00FF) & 0xFF00FF00
		binary.LittleEndian.PutUint32(p[i:i+4], rb|ag)
	}
}

// blendPremul composites a premultiplied source pixel scaled by k (0..255).
func blendPremul(p []uint8, sr, sg, sb, sa, k uint32) {
	if k != 255 {
		sr, sg, sb, sa = div255(sr*k), div255(sg*k), div255(sb*k), div255(sa*k)
	}
	if sa == 0 {
		return
	}
	ia := 255 - sa
	p[0] = sat8(sr + div255(uint32(p[0])*ia))
	p[1] = sat8(sg + div255(uint32(p[1])*ia))
	p[2] = sat8(sb + div255(uint32(p[2])*ia))
	p[3] = sat8(sa + div255(uint32(p[3])*ia))
}

// sat8 keeps malformed (non-premultiplied) sources from wrapping around.
func sat8(v uint32) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// blendMask composites col through a row of 8-bit coverage. Runs of equal
// coverage (shape interiors, blurred plateaus) go through blendSpan,
// which is several times cheaper than per-pixel blending.
func blendMask(pix []uint8, mask []uint8, col color.NRGBA, ca uint32) {
	n := len(mask)
	pix = pix[:n*4]
	cr, cg, cb := uint32(col.R), uint32(col.G), uint32(col.B)
	for i := 0; i < n; {
		m := mask[i]
		if m == 0 {
			i++
			continue
		}
		j := i + 1
		for j < n && mask[j] == m {
			j++
		}
		a := div255(uint32(m) * ca)
		if j-i > 1 || a == 255 {
			blendSpan(pix[i*4:], j-i, col, a)
			i = j
			continue
		}
		// Single pixel: same packed arithmetic as blendSpan, inlined.
		ia := 255 - a
		q := pix[i*4 : i*4+4]
		d := binary.LittleEndian.Uint32(q)
		rb := (d&0x00FF00FF)*ia + (cr|cb<<16)*a + 0x00800080
		ag := (d>>8&0x00FF00FF)*ia + (cg|255<<16)*a + 0x00800080
		rb = (rb + rb>>8&0x00FF00FF) >> 8 & 0x00FF00FF
		ag = (ag + ag>>8&0x00FF00FF) & 0xFF00FF00
		binary.LittleEndian.PutUint32(q, rb|ag)
		i++
	}
}
