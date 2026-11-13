//go:build gointl_all

package temporal

import (
	"math"
	"strings"
	"testing"
)

// Duration.Round and Total refuse a unit no constant names, and Total one
// that is not a unit: a SmallestUnit past Year had divided by zero, and
// Total(UnitAuto) answered +Inf, where temporal_rs refuses "auto" as "Auto
// unit not allowed here" (ISSUES.md API-6).
func TestDurationUnitOptions(t *testing.T) {
	d, err := newDuration(0, 0, 0, 0, 1, 0, 0, 0, int128{}, int128{})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []Unit{Year + 1, Unit(-1), Unit(-5), Unit(99)} {
		if _, err := d.Round(RoundingOptions{SmallestUnit: u, LargestUnit: NoUnit}, RelativeTo{}); err == nil ||
			!strings.Contains(err.Error(), "Unit was not a valid unit.") {
			t.Errorf("Round smallest %d: %v", u, err)
		}
		if _, err := d.Round(RoundingOptions{SmallestUnit: NoUnit, LargestUnit: u}, RelativeTo{}); err == nil ||
			!strings.Contains(err.Error(), "Unit was not a valid unit.") {
			t.Errorf("Round largest %d: %v", u, err)
		}
		if v, err := d.Total(u, RelativeTo{}); err == nil || !strings.Contains(err.Error(), "Unit was not a valid unit.") {
			t.Errorf("Total %d: %v %v", u, v, err)
		}
	}
	if v, err := d.Total(UnitAuto, RelativeTo{}); err == nil || !strings.Contains(err.Error(), "Auto unit not allowed here") {
		t.Errorf("Total auto: %v %v", v, err)
	}
	if v, err := d.Total(NoUnit, RelativeTo{}); err == nil || !strings.Contains(err.Error(), "Unit is required") {
		t.Errorf("Total none: %v %v", v, err)
	}
	if v, err := d.Total(Minute, RelativeTo{}); err != nil || v != 60 || math.IsInf(v, 0) {
		t.Errorf("Total minutes: %v %v", v, err)
	}
}
