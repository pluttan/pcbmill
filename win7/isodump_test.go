package main

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

// TestIsoDump: the isolation program of a board as pcbscan makes it, for a check
// outside (hvpulse hw/pcb/mill.py counts the copper it leaves). The board is read
// as a job reads it: ISO_BOARD is a zip or a folder of Gerber files.
//
//	ISO_BOARD=board.zip ISO_PASSES=18 ISO_OUT=iso.json go test -run TestIsoDump .
//
// iso.json: the side, the cut width at the depth and at the bottom of the copper,
// the step, the notes and every path of the cut (mm, board frame, not mirrored);
// with ISO_CLEAR=<mm> the paths of the clearing by the end mill too.
func TestIsoDump(t *testing.T) {
	src := os.Getenv("ISO_BOARD")
	if src == "" {
		t.Skip("ISO_BOARD not set")
	}
	b, err := loadBoard(src)
	if err != nil {
		t.Fatal(err)
	}
	c := defaultCam()
	if n, err := strconv.Atoi(os.Getenv("ISO_PASSES")); err == nil {
		c.Passes = n
	}
	if d, err := strconv.ParseFloat(os.Getenv("ISO_DEPTH"), 64); err == nil {
		c.IsoDepth = d
	}
	if d, err := strconv.ParseFloat(os.Getenv("ISO_CLEAR"), 64); err == nil {
		c.ClearReach = d
	}
	side := lyTop
	if b.copper[lyTop] == nil {
		side = lyBottom
	}
	op, _ := isolation(b, side, c, nil)
	flat := func(ps []cpath) [][][2]float64 {
		var out [][][2]float64
		for _, p := range ps {
			var l [][2]float64
			for _, q := range p.pts {
				l = append(l, [2]float64{q.X, q.Y})
			}
			out = append(out, l)
		}
		return out
	}
	paths := flat(op.paths)
	var clear [][][2]float64
	notes := op.notes
	if c.ClearReach > 0 {
		cl := clearing(b, side, c)
		clear, notes = flat(cl.paths), append(notes, cl.notes...)
	}
	out, err := json.Marshal(map[string]interface{}{
		"side": side, "passes": c.Passes, "depth": c.IsoDepth,
		"cut": c.cutWidth(c.IsoDepth), "base": c.cutWidth(c.IsoDepth - copperT), "copper": copperT,
		"step": c.isoStep(), "notes": notes, "paths": paths, "clear": clear, "clear_tool": c.HoleTool})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ISO_OUT"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log(notes)
}
