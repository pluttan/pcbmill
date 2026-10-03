package main

// Where the tool is, for the 3D views of the pages that move the machine:
// the virtual machine knows it exactly; for the real one it is the end of
// the last move typed into NcStudio.

import "sync"

var tool struct {
	sync.Mutex
	x, y, z float64
	known   bool // X and Y are known (after an absolute move)
	set     bool
	ver     int
}

func trackTool(x, y, z float64, known bool) {
	tool.Lock()
	tool.x, tool.y, tool.z, tool.known, tool.set = x, y, z, known, true
	tool.ver++
	tool.Unlock()
	if app != nil {
		app.Wake()
	}
}

// toolNow: the tool in work coordinates; ok when X and Y are known.
func toolNow() (x, y, z float64, ok bool) {
	if simM != nil {
		x, y, z, _ = simM.WorkPos()
		return x, y, z, true
	}
	tool.Lock()
	defer tool.Unlock()
	return tool.x, tool.y, tool.z, tool.known && tool.set
}

// copperZ: the height of the copper in NcStudio's Z, so the tool is drawn
// over the copper where it really is: unknown until a touch, then a guess
// of the usual 1.5 mm over it.
var copperZ struct {
	sync.Mutex
	z     float64
	known bool
}

func setCopperZ(z float64) {
	copperZ.Lock()
	copperZ.z, copperZ.known = z, true
	copperZ.Unlock()
}

// overCopper: how high the tip is over the copper under it.
func overCopper(z float64) float64 {
	copperZ.Lock()
	defer copperZ.Unlock()
	if !copperZ.known {
		return z + 1.5
	}
	return z - copperZ.z
}

// toolMoving: the virtual tool is between two points (the view animates).
func toolMoving() bool {
	if simM != nil {
		_, _, _, m := simM.WorkPos()
		return m
	}
	return false
}
