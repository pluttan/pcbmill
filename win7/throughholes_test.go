package main

import "testing"

// TestThroughHoles: the holes and the drills go without a touch and a map,
// the spot drill and the copper do not.
func TestThroughHoles(t *testing.T) {
	for f, want := range map[string]bool{
		`C:\pcbscan\job\sheet_3_holes.tap`: true, "sheet_3_holes_redo1.tap": true, "sheet_3_drill_1.00.tap": true,
		"sheet_3_drill_spot.tap": false, "sheet_1_iso.tap": false, "sheet_2_mask.tap": false, "sheet_4_vcut.tap": false,
	} {
		if throughHoles(f) != want {
			t.Errorf("%s: %v, а надо %v", f, !want, want)
		}
	}
}

// TestThroughHolesFiles: the same on the files a job writes.
func TestThroughHolesFiles(t *testing.T) {
	j := testJob(t)
	ops, err := j.cam(defaultCam(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.writeProgs(ops); err != nil {
		t.Fatal(err)
	}
	holes := 0
	for _, f := range j.stageFiles("3_") {
		if throughHoles(f) {
			holes++
		}
	}
	for _, p := range []string{"1_", "2_", "4_"} {
		for _, f := range j.stageFiles(p) {
			if throughHoles(f) {
				t.Errorf("%s без касания", f)
			}
		}
	}
	if holes == 0 {
		t.Errorf("ни один файл сверловки не пойдёт без касания: %v", j.stageFiles("3_"))
	}
}
