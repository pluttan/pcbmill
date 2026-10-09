package main

// The main screen: the stages down the left, the lines of the chosen stage
// in the middle with what the chosen line is about, the sheet in 3D on the
// right. A click chooses, a double click or the button does it; the arrows,
// Enter and Esc work as before.

import (
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strings"

	"pcbscan/gfx"
	"pcbscan/ui"
)

// mainActs: what the main screen asks the work goroutine to do.
var mainActs chan func() error

// the icons of the stages, in the order of roots()
var stageIcons = []ui.Icon{ui.IconBoards, ui.IconMap, ui.IconMill, ui.IconMask, ui.IconDrill, ui.IconVcut, ui.IconGear, ui.IconExit}

// jobMain is the program: the job screen until Выход.
func jobMain(o opts) {
	j, err := loadJob(filepath.Join(exeDir(), "job"))
	if err != nil {
		alert(err)
		j = newJob(filepath.Join(exeDir(), "job"))
	}
	u := &jobUI{o: o, j: j, c: loadCam(exeDir())}
	if err := j.loadBoards(); err != nil {
		alert(fmt.Errorf("платы задания не читаются: %v", err))
		j.Srcs, j.Places, j.boards = nil, nil, nil
	}
	mainActs = make(chan func() error, 1)
	theJob = u
	defer func() { theJob = nil }()
	if simM == nil {
		go inboxLoop(o)
	}
	u.sel[0], u.sel[1] = u.next(), 0
	for !u.quit {
		simFit(j.W, j.H) // the virtual blank follows the sheet
		cur, crumb = nil, "задание"
		app.Show(u)
		act := <-mainActs
		inboxBusy.Store(true) // the probe is the work's now
		if err := act(); err != nil && !errors.Is(err, errBack) {
			alert(err)
		}
		inboxBusy.Store(false)
		u.gen++
	}
}

// theJob: the screen, for the close button of the window.
var theJob *jobUI

func quitJob() {
	if theJob != nil && confirm("Выйти из pcbscan?") {
		theJob.quit = true
	}
}

// do sends an action to the work goroutine (from the window goroutine).
func (u *jobUI) do(act func() error) {
	if act == nil {
		return
	}
	select {
	case mainActs <- act:
	default: // one is already waiting
	}
}

func (u *jobUI) Draw(c *ui.Ctx) {
	roots, kids := u.lists()
	title := "задание"
	if u.lvl == 1 {
		title = "задание / " + roots[u.sel[0]].title
	}
	rail, mid, view, narrow := mainLayout(c, title)
	u.rail = roots // for the work pages, drawn on this goroutine
	u.drawRail(c, rail, roots, narrow, true)
	cur := roots[u.sel[0]]
	if len(kids) > 0 {
		cur = kids[u.sel[1]]
	}
	u.fieldKeys(c, kids)
	u.drawMid(c, mid, roots[u.sel[0]], kids, cur)
	u.drawView(c, view, roots[u.sel[0]], cur)
	u.keys(c, roots, kids)
}

// mainLayout: the stages, the middle column and the picture; every screen of
// the job has them in the same places.
func mainLayout(c *ui.Ctx, title string) (rail, mid, view image.Rectangle, narrow bool) {
	t := c.T
	body := ui.Inset(chrome(c, title), t.Si(16))
	narrow = body.Dx() < t.Si(980)
	railW := t.Si(236)
	if narrow {
		railW = t.Si(74)
	}
	rail = ui.CutLeft(&body, railW)
	ui.CutLeft(&body, t.Si(14))
	midW := t.Si(340)
	if body.Dx() > t.Si(960) {
		midW = t.Si(390) // room for a number with its − and +
	}
	if body.Dx() < t.Si(700) {
		midW = body.Dx() * 3 / 5 // the numbers in the lines need the room more than the picture
	}
	mid = ui.CutLeft(&body, midW)
	ui.CutLeft(&body, t.Si(14))
	return rail, mid, body, narrow
}

// shell: a work page or a form in the frame of the job screen: the stages
// on the left as they were (not clickable while the work goes on), the page
// in the middle, the picture on the right. False outside a job.
func shell(c *ui.Ctx, title string) (mid, view image.Rectangle, ok bool) {
	u := theJob
	if u == nil || len(u.rail) == 0 {
		return mid, view, false
	}
	rail, mid, view, narrow := mainLayout(c, title)
	u.drawRail(c, rail, u.rail, narrow, false)
	return mid, view, true
}

// drawShellView: the picture of a page in the frame: its own scene, else the
// one the job screen showed last (so it stays put, camera and all).
func drawShellView(c *ui.Ctx, r image.Rectangle, sc ui.Scene, own bool) ui.ViewHit {
	t := c.T
	c.Card(r)
	in := ui.Inset(r, t.Si(1))
	if sc == nil && !own && theJob != nil {
		sc = theJob.scene
	}
	if sc == nil {
		c.Icon(ui.IconBoards, ui.Center(in, t.Si(64), t.Si(64)), t.Surface2)
		return ui.ViewHit{}
	}
	id := "main/sheet" // one camera with the job screen
	if _, ok := sc.(*mapScene); ok {
		id = "main/map"
	}
	return c.View3D(id, in, sc)
}

// drawRail: the stages; live: a click chooses one.
func (u *jobUI) drawRail(c *ui.Ctx, r image.Rectangle, roots []jnode, narrow, live bool) {
	t := c.T
	c.Panel(r, t.Crust)
	in := ui.Inset(r, t.Si(8))
	rowH := t.Si(56)
	if narrow {
		rowH = t.Si(64)
	}
	for i, n := range roots {
		rr := image.Rect(in.Min.X, in.Min.Y+i*(rowH+t.Si(4)), in.Max.X, in.Min.Y+i*(rowH+t.Si(4))+rowH)
		if i == len(roots)-1 && rr.Max.Y < in.Max.Y { // Выход at the bottom
			rr = image.Rect(in.Min.X, in.Max.Y-rowH, in.Max.X, in.Max.Y)
		}
		if !live {
			if i == u.sel[0] {
				x, y, w, h := float64(rr.Min.X), float64(rr.Min.Y), float64(rr.Dx()), float64(rr.Dy())
				c.C.FillRoundRect(x, y, w, h, t.S(7), t.Surface0)
				c.C.FillRoundRect(x+t.S(3), y+h*0.22, t.S(3), h*0.56, t.S(1.5), t.Lavender)
			}
		} else if c.Row(fmt.Sprintf("rail/%d", i), rr, i == u.sel[0]) {
			u.sel[0], u.sel[1] = i, 0
			u.lvl = 1
			if !n.branch && n.act != nil {
				u.lvl = 0
				u.do(n.act)
			}
		}
		ic := ui.IconDot
		if i < len(stageIcons) {
			ic = stageIcons[i]
		}
		bg, fg := t.Surface0, t.Subtext1
		switch n.icon {
		case "√":
			bg, fg = gfx.Alpha(t.Green, 0.18), t.Green
		case "►":
			bg, fg = gfx.Alpha(t.Blue, 0.22), t.Blue
		}
		if n.title == "Выход" {
			bg, fg = gfx.Alpha(t.Surface0, 0.6), t.Overlay2
		}
		box := t.Si(36)
		var ir image.Rectangle
		if narrow {
			ir = image.Rect(rr.Min.X+(rr.Dx()-box)/2, rr.Min.Y+t.Si(6), rr.Min.X+(rr.Dx()+box)/2, rr.Min.Y+t.Si(6)+box)
		} else {
			ir = image.Rect(rr.Min.X+t.Si(12), rr.Min.Y+(rr.Dy()-box)/2, rr.Min.X+t.Si(12)+box, rr.Min.Y+(rr.Dy()+box)/2)
		}
		c.C.FillRoundRect(float64(ir.Min.X), float64(ir.Min.Y), float64(box), float64(box), t.S(9), bg)
		c.Icon(ic, ui.Inset(ir, t.Si(8)), fg)
		if n.icon == "√" { // a small tick in the corner says done
			cx, cy := float64(ir.Max.X)-t.S(2), float64(ir.Min.Y)+t.S(2)
			c.C.FillCircle(cx, cy, t.S(7), t.Green)
			c.Icon(ui.IconCheck, image.Rect(int(cx-t.S(5)), int(cy-t.S(5)), int(cx+t.S(5)), int(cy+t.S(5))), t.Crust)
		}
		if narrow {
			c.LabelIn(image.Rect(rr.Min.X, ir.Max.Y+t.Si(2), rr.Max.X, rr.Max.Y), shortTitle(n.title),
				ui.Style{Kind: ui.Regular, Size: ui.SizeSmall - 1, Color: t.Subtext0}, 1)
			continue
		}
		tx := ir.Max.X + t.Si(12)
		c.LabelIn(image.Rect(tx, rr.Min.Y+t.Si(8), rr.Max.X-t.Si(8), rr.Min.Y+t.Si(30)), n.title, ui.Style{Kind: ui.Bold, Size: ui.SizeBody, Color: t.Text}, 0)
		if n.right != "" {
			col := t.Overlay2
			if n.icon == "►" {
				col = t.Blue
			}
			c.LabelIn(image.Rect(tx, rr.Min.Y+t.Si(28), rr.Max.X-t.Si(8), rr.Max.Y-t.Si(6)), n.right, ui.Style{Kind: ui.Regular, Size: ui.SizeSmall, Color: col}, 0)
		}
	}
}

func shortTitle(s string) string {
	switch s {
	case "Карта высот":
		return "Карта"
	case "Фрезеровка":
		return "Фрезер."
	case "Сверловка":
		return "Сверл."
	}
	return s
}

func (u *jobUI) drawMid(c *ui.Ctx, r image.Rectangle, root jnode, kids []jnode, cur jnode) {
	t := c.T
	c.Card(r)
	in := ui.Inset(r, t.Si(16))
	c.LabelIn(ui.CutTop(&in, t.Si(28)), root.title, ui.Style{Kind: ui.Bold, Size: ui.SizeTitle, Color: t.Text}, 0)
	if len(root.about) > 0 {
		h := drawLines(c, image.Rect(in.Min.X, in.Min.Y+t.Si(4), in.Max.X, in.Max.Y), in.Min.Y+t.Si(4), root.about[:1], c.Muted()) - in.Min.Y
		ui.CutTop(&in, h+t.Si(10))
	}
	// the lines of the stage
	rowH := t.Si(40)
	listH := len(kids) * rowH
	aboutMin := t.Si(150)
	if listH > in.Dy()-aboutMin {
		listH = imax(rowH*3, in.Dy()-aboutMin)
	}
	list := ui.CutTop(&in, listH)
	off := c.Scroll("mid/list", list, len(kids)*rowH, false)
	if u.lvl == 1 { // keep the chosen line in sight
		y := u.sel[1] * rowH
		if y < off {
			c.SetScroll("mid/list", y)
		} else if y+rowH > off+list.Dy() {
			c.SetScroll("mid/list", y+rowH-list.Dy())
		}
	}
	c.WithClip(list, func() {
		for i, k := range kids {
			rr := image.Rect(list.Min.X, list.Min.Y+i*rowH-off, list.Max.X-t.Si(6), list.Min.Y+(i+1)*rowH-off)
			if rr.Max.Y < list.Min.Y || rr.Min.Y > list.Max.Y {
				continue
			}
			if k.head {
				drawHead(c, rr, k.title)
				continue
			}
			if c.Row(fmt.Sprintf("kid/%d/%s", i, k.title), rr, u.lvl == 1 && i == u.sel[1]) {
				if i != u.sel[1] {
					u.msg = ""
					u.applyEdit()
				}
				u.sel[1], u.lvl = i, 1
				if c.Double() && k.act != nil {
					u.do(k.act)
				}
			}
			if k.field != nil {
				u.drawField(c, i, k, rr, i == u.sel[1])
				continue
			}
			ir := image.Rect(rr.Min.X+t.Si(12), rr.Min.Y+(rr.Dy()-t.Si(18))/2, rr.Min.X+t.Si(30), rr.Min.Y+(rr.Dy()+t.Si(18))/2)
			switch k.icon {
			case "√":
				c.Icon(ui.IconCheck, ir, t.Green)
			case "○":
				c.Icon(ui.IconRing, ir, t.Overlay1)
			case "·":
				c.Icon(ui.IconNext, ir, t.Overlay1)
			}
			tx := rr.Min.X + t.Si(40)
			rw := 0
			if k.right != "" {
				rs := ui.Style{Kind: ui.Regular, Size: ui.SizeSmall, Color: t.Overlay2}
				if k.hot {
					rs = ui.Style{Kind: ui.Bold, Size: ui.SizeSmall, Color: t.Yellow}
				}
				rw = imin(c.Measure(k.right, rs), rr.Dx()/2)
				c.LabelIn(image.Rect(rr.Max.X-t.Si(10)-rw, rr.Min.Y, rr.Max.X-t.Si(10), rr.Max.Y), k.right, rs, 2)
			}
			st := c.Body()
			if k.icon == " " {
				st = c.Muted()
			}
			if k.hot {
				st = ui.Style{Kind: ui.Bold, Size: ui.SizeBody, Color: t.Yellow}
			}
			c.LabelIn(image.Rect(tx, rr.Min.Y, rr.Max.X-t.Si(20)-rw, rr.Max.Y), k.title, st, 0)
		}
	})
	if len(kids) == 0 {
		return
	}
	ui.CutTop(&in, t.Si(10))
	c.HLine(in.Min.X, in.Max.X, in.Min.Y)
	ui.CutTop(&in, t.Si(12))
	// what the chosen line is, and its button
	if cur.act != nil {
		foot := ui.CutBottom(&in, t.Si(40))
		label := "Открыть"
		switch {
		case cur.icon == "○" || cur.icon == "√":
			label = "Запустить"
		case strings.HasPrefix(cur.title, "Снять"):
			label = "Начать"
		}
		if c.ButtonX("mid/act", foot, label, ui.BtnPrimary, true, u.lvl == 1) {
			u.do(cur.act)
		}
		ui.CutBottom(&in, t.Si(10))
	}
	about := cur.about
	if u.msg != "" {
		about = append([]string{"■ " + u.msg}, about...)
	}
	c.WithClip(in, func() {
		drawLines(c, in, in.Min.Y, about, c.Body())
	})
}

func (u *jobUI) drawView(c *ui.Ctx, r image.Rectangle, root, cur jnode) {
	t := c.T
	c.Card(r)
	in := ui.Inset(r, t.Si(1))
	if len(u.j.Places) == 0 {
		u.scene = nil
		c.Icon(ui.IconBoards, ui.Center(in, t.Si(64), t.Si(64)), t.Surface2)
		c.LabelIn(image.Rect(in.Min.X, in.Min.Y+in.Dy()/2+t.Si(44), in.Max.X, in.Min.Y+in.Dy()/2+t.Si(70)),
			"Платы ещё не выбраны", ui.Style{Kind: ui.Bold, Size: ui.SizeLarge, Color: t.Subtext0}, 1)
		c.LabelIn(image.Rect(in.Min.X, in.Min.Y+in.Dy()/2+t.Si(72), in.Max.X, in.Min.Y+in.Dy()/2+t.Si(96)),
			"Платы / Платы на листе: сколько штук каждой", c.Muted(), 1)
		return
	}
	if (cur.mapView || (root.mapView && u.lvl == 0)) && u.hm != nil {
		u.scene = newMapScene(u.hm, u.hmPts, u.j, u.mirror())
		c.View3D("main/map", in, u.scene)
		return
	}
	var ov sheetOverlay
	if cur.ov != nil {
		ov = cur.ov()
	} else if root.ov != nil {
		ov = root.ov()
	}
	sc := newSheetScene(u.j, u.mirror(), ov)
	if _, err := u.jmap(); err == nil && u.hm != nil && root.title != "Платы" {
		sc.hm = nil // the map has its own view; the sheet stays flat and true
	}
	u.scene = sc
	c.View3D("main/sheet", in, sc)
}

// keys: the arrows, Enter and Esc as on the old screen.
func (u *jobUI) keys(c *ui.Ctx, roots, kids []jnode) {
	e, ok := c.Key()
	if !ok || e.Kind != ui.EvKey {
		return
	}
	n := len(roots)
	if u.lvl == 1 {
		n = len(kids)
	}
	move := func(d int) {
		if n == 0 {
			return
		}
		u.msg = ""
		u.applyEdit()
		for k := 0; k < n; k++ { // past the headings
			u.sel[u.lvl] = (u.sel[u.lvl] + d + n) % n
			if u.lvl == 0 || !kids[u.sel[1]].head {
				break
			}
		}
		if u.lvl == 0 {
			u.sel[1] = 0
		}
	}
	switch e.Key {
	case ui.KeyUp:
		move(-1)
	case ui.KeyDown, ui.KeyTab:
		move(1)
	case ui.KeyHome:
		u.sel[u.lvl] = 0
	case ui.KeyEnd:
		u.sel[u.lvl] = n - 1
	case ui.KeyRight, ui.KeyEnter:
		if u.lvl == 0 {
			r := roots[u.sel[0]]
			if r.branch {
				u.lvl, u.sel[1] = 1, 0
			} else if e.Key == ui.KeyEnter {
				u.do(r.act)
			}
		} else if len(kids) > 0 && e.Key == ui.KeyEnter {
			u.do(kids[u.sel[1]].act)
		}
	case ui.KeyLeft, ui.KeyEsc:
		if u.lvl == 1 {
			u.lvl = 0
		} else if e.Key == ui.KeyEsc {
			u.do(roots[len(roots)-1].act)
		}
	default:
		return
	}
	c.Use()
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
