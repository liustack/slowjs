//go:build gointl_all

package temporal

import (
	"strings"
	"testing"
)

// An option struct's zero value is its default: NoUnit is the zero Unit, so
// DifferenceSettings{} leaves the units unset where it had been "auto" and
// failed with "Unit was not part of the date unit group." Node's answers for
// the same calls without options (ISSUES.md API-7).
func TestZeroOptionsAreDefaults(t *testing.T) {
	if NoUnit != 0 {
		t.Fatalf("NoUnit = %d, want the zero value", NoUnit)
	}
	iso, err := NewCalendar("iso8601")
	if err != nil {
		t.Fatal(err)
	}
	from, err := NewPlainDate(2020, 1, 1, iso, Reject)
	if err != nil {
		t.Fatal(err)
	}
	to, err := NewPlainDate(2021, 3, 15, iso, Reject)
	if err != nil {
		t.Fatal(err)
	}
	// Temporal.PlainDate.from("2020-01-01").until("2021-03-15") is P439D.
	d, err := from.Until(to, DifferenceSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := d.String(DefaultToStringOptions()); err != nil || s != "P439D" {
		t.Errorf("PlainDate until: %q %v, want P439D", s, err)
	}

	t1, err := NewPlainTime(1, 2, 3, 0, 0, 0, Reject)
	if err != nil {
		t.Fatal(err)
	}
	t2, err := NewPlainTime(23, 0, 0, 0, 0, 500, Reject)
	if err != nil {
		t.Fatal(err)
	}
	// Temporal.PlainTime.from("01:02:03").until("23:00:00.0000005") is
	// PT21H57M57.0000005S.
	d, err = t1.Until(t2, DifferenceSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := d.String(DefaultToStringOptions()); err != nil || s != "PT21H57M57.0000005S" {
		t.Errorf("PlainTime until: %q %v, want PT21H57M57.0000005S", s, err)
	}

	// round({}) refuses as round() with no units does.
	if _, err := d.Round(RoundingOptions{}, RelativeTo{}); err == nil ||
		!strings.Contains(err.Error(), "smallestUnit and largestUnit cannot both be None.") {
		t.Errorf("Round with no units: %v", err)
	}
	// round({ smallestUnit: "minute" }) is PT21H58M.
	r, err := d.Round(RoundingOptions{SmallestUnit: Minute}, RelativeTo{})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := r.String(DefaultToStringOptions()); err != nil || s != "PT21H58M" {
		t.Errorf("Round to minutes: %q %v, want PT21H58M", s, err)
	}
}
