package webui

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
)

// ==========================
// ===  Fast PNG writer   ===
// ==========================

// pngWriter writes opaque RGB PNGs with the Sub filter on every row. The
// standard encoder spends most of its time choosing a filter per row and
// checking alpha; a screen of flat panels compresses well with Sub alone,
// and the window ignores alpha anyway (as on Win32).
type pngWriter struct {
	row  []byte // filtered row: filter byte + RGB
	idat bytes.Buffer
	zw   *zlib.Writer
}

var pngSig = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func (p *pngWriter) encode(out *bytes.Buffer, img *image.RGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out.Write(pngSig)

	var ihdr [13]byte
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = 8 // bits per channel
	ihdr[9] = 2 // truecolour RGB
	writeChunk(out, "IHDR", ihdr[:])

	p.idat.Reset()
	if p.zw == nil {
		p.zw, _ = zlib.NewWriterLevel(&p.idat, zlib.BestSpeed)
	} else {
		p.zw.Reset(&p.idat)
	}
	if cap(p.row) < 1+w*3 {
		p.row = make([]byte, 1+w*3)
	}
	row := p.row[:1+w*3]
	row[0] = 1 // Sub: each byte minus the same channel of the left pixel
	for y := 0; y < h; y++ {
		src := img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):]
		src = src[:w*4]
		var lr, lg, lb byte
		d := row[1:]
		for len(src) >= 4 && len(d) >= 3 {
			r, g, b := src[0], src[1], src[2]
			d[0], d[1], d[2] = r-lr, g-lg, b-lb
			lr, lg, lb = r, g, b
			src, d = src[4:], d[3:]
		}
		p.zw.Write(row)
	}
	p.zw.Close()
	writeChunk(out, "IDAT", p.idat.Bytes())
	writeChunk(out, "IEND", nil)
}

func writeChunk(out *bytes.Buffer, name string, data []byte) {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(data)))
	copy(hdr[4:], name)
	out.Write(hdr[:])
	out.Write(data)
	crc := crc32.NewIEEE()
	crc.Write(hdr[4:])
	crc.Write(data)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	out.Write(sum[:])
}
