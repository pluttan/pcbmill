package main

// The boards next to the exe and how many of each go on the sheet: a list
// with − and + at every board; the arrows and digits work too.

import (
	"fmt"
	"image"
	"path/filepath"

	"pcbscan/gfx"
	"pcbscan/ui"
)

type boardPick struct {
	srcs  []string
	count []int
	sel   int
	done  chan bool
	sent  bool
}

// pickCounts: how many of each board; false when the operator went back.
func pickCounts(srcs []string, count []int) ([]int, bool) {
	p := &boardPick{srcs: srcs, count: append([]int(nil), count...), done: make(chan bool, 1)}
	crumb = "задание / Платы / Выбрать"
	backToApp()
	app.Push(p)
	ok := <-p.done
	app.Pop(p)
	return p.count, ok
}

func (p *boardPick) answer(ok bool) {
	if !p.sent {
		p.sent = true
		p.done <- ok
	}
}

func (p *boardPick) Draw(c *ui.Ctx) {
	t := c.T
	rowH := t.Si(52)
	w := imin(t.Si(620), c.W-t.Si(40))
	h := imin(t.Si(24)*2+t.Si(70)+len(p.srcs)*rowH+t.Si(70), c.H-t.Si(40))
	r := ui.Center(image.Rect(0, 0, c.W, c.H), w, h)
	c.Card(r)
	in := ui.Inset(r, t.Si(24))
	c.LabelIn(ui.CutTop(&in, t.Si(30)), "Платы рядом с pcbscan", ui.Style{Kind: ui.Bold, Size: ui.SizeTitle, Color: t.Text}, 0)
	c.LabelIn(ui.CutTop(&in, t.Si(26)), "Сколько штук каждой положить на лист. Раскладка будет сама.", c.Muted(), 0)
	ui.CutTop(&in, t.Si(10))
	foot := ui.CutBottom(&in, t.Si(40))
	ui.CutBottom(&in, t.Si(12))
	list := in
	off := c.Scroll("boards", list, len(p.srcs)*rowH, false)
	total := 0
	c.WithClip(list, func() {
		for i, s := range p.srcs {
			total += p.count[i]
			rr := image.Rect(list.Min.X, list.Min.Y+i*rowH-off, list.Max.X-t.Si(6), list.Min.Y+(i+1)*rowH-off-t.Si(4))
			if c.Row(fmt.Sprintf("b/%d", i), rr, i == p.sel) {
				p.sel = i
			}
			ir := image.Rect(rr.Min.X+t.Si(12), rr.Min.Y+(rr.Dy()-t.Si(30))/2, rr.Min.X+t.Si(42), rr.Min.Y+(rr.Dy()+t.Si(30))/2)
			bg, fg := t.Surface0, t.Overlay1
			if p.count[i] > 0 {
				bg, fg = gfx.Alpha(t.Peach, 0.2), t.Peach
			}
			c.C.FillRoundRect(float64(ir.Min.X), float64(ir.Min.Y), float64(ir.Dx()), float64(ir.Dy()), t.S(7), bg)
			c.Icon(ui.IconBoards, ui.Inset(ir, t.Si(6)), fg)
			// the stepper at the right
			bs := t.Si(30)
			plus := image.Rect(rr.Max.X-t.Si(10)-bs, rr.Min.Y+(rr.Dy()-bs)/2, rr.Max.X-t.Si(10), rr.Min.Y+(rr.Dy()+bs)/2)
			numR := image.Rect(plus.Min.X-t.Si(46), plus.Min.Y, plus.Min.X-t.Si(6), plus.Max.Y)
			minus := image.Rect(numR.Min.X-t.Si(6)-bs, plus.Min.Y, numR.Min.X-t.Si(6), plus.Max.Y)
			if c.IconButton(fmt.Sprintf("b/%d/-", i), minus, ui.IconMinus, false) && p.count[i] > 0 {
				p.count[i]--
				p.sel = i
			}
			if c.IconButton(fmt.Sprintf("b/%d/+", i), plus, ui.IconPlus, false) && p.count[i] < 50 {
				p.count[i]++
				p.sel = i
			}
			col := t.Overlay1
			if p.count[i] > 0 {
				col = t.Text
			}
			c.LabelIn(numR, fmt.Sprintf("%d", p.count[i]), ui.Style{Kind: ui.Bold, Size: ui.SizeLarge, Color: col}, 1)
			c.LabelIn(image.Rect(ir.Max.X+t.Si(12), rr.Min.Y, minus.Min.X-t.Si(8), rr.Max.Y), filepath.Base(s), c.Body(), 0)
		}
	})
	c.LabelIn(image.Rect(foot.Min.X, foot.Min.Y, foot.Min.X+t.Si(200), foot.Max.Y), fmt.Sprintf("всего плат: %d", total), c.Muted(), 0)
	bs := ui.SplitX(image.Rect(foot.Max.X-t.Si(280), foot.Min.Y, foot.Max.X, foot.Max.Y), 2, t.Si(10))
	if c.ButtonX("b/ok", bs[0], "Разложить", ui.BtnPrimary, total > 0, true) {
		p.answer(true)
	}
	if c.Button("b/no", bs[1], "Отмена", ui.BtnNormal) {
		p.answer(false)
	}
	e, ok := c.Key()
	if !ok {
		return
	}
	n := len(p.srcs)
	if e.Kind == ui.EvChar {
		switch {
		case e.Rune == ' ':
			if p.count[p.sel] > 0 {
				p.count[p.sel] = 0
			} else {
				p.count[p.sel] = 1
			}
		case e.Rune >= '0' && e.Rune <= '9':
			p.count[p.sel] = int(e.Rune - '0')
		default:
			return
		}
		c.Use()
		return
	}
	switch e.Key {
	case ui.KeyUp:
		p.sel = (p.sel + n - 1) % n
	case ui.KeyDown, ui.KeyTab:
		p.sel = (p.sel + 1) % n
	case ui.KeyRight:
		if p.count[p.sel] < 50 {
			p.count[p.sel]++
		}
	case ui.KeyLeft:
		if p.count[p.sel] > 0 {
			p.count[p.sel]--
		}
	case ui.KeyEnter:
		if total > 0 {
			p.answer(true)
		}
	case ui.KeyEsc:
		p.answer(false)
	default:
		return
	}
	c.Use()
}
