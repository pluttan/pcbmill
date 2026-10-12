package main

import "testing"

// TestThroughHoles: the holes and the drills go without a touch and a map,
// the spot drill and the copper do not.
func TestThroughHoles(t *testing.T) {
	for f, want := range map[string]bool{
		`C:\job\3_holes.tap`: true, "3_drill_1.00.tap": true, "3_drill_spot.tap": false,
		"1_iso.tap": false, "2_mask.tap": false, "4_vcut.tap": false,
	} {
		if throughHoles(f) != want {
			t.Errorf("%s: %v, а надо %v", f, !want, want)
		}
	}
}
