package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpeedAfterMap: 8000 when the programs were written, the map, then
// 24000 (after a restart, no Пересчитать line): the stage runs with 24000,
// the map stays.
func TestSpeedAfterMap(t *testing.T) {
	u := &jobUI{j: testJob(t), c: defaultCam()}
	u.c.MaskTest = true // as testJob laid it out
	u.c.Spindle = 8000
	ops, err := u.j.cam(u.c, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := u.j.writeProgs(ops); err != nil {
		t.Fatal(err)
	}
	u.j.markCam(u.c)
	os.WriteFile(u.j.mapPath(), []byte("map"), 0o644)
	u.c.Spindle = 24000
	if err := u.freshProgs(); err != nil {
		t.Fatal(err)
	}
	f := u.j.stageFiles("1_")[0]
	if b, _ := os.ReadFile(f); !strings.Contains(string(b), "S24000M3") {
		t.Errorf("%s без S24000", filepath.Base(f))
	}
	if b, _ := os.ReadFile(u.j.mapPath()); string(b) != "map" {
		t.Error("карта пропала")
	}
}
