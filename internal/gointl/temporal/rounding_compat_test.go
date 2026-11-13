//go:build gointl_all

package temporal

import (
	"errors"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Rounding up to a calendar unit smaller than the largest, where the
// difference has none of that unit, adds none of it on the standard side
// (ECMA-262's Temporal, "Fix condition for bounding window with zero
// start"; test262's exact-multiple-of-larger-unit), and one on Node's
// (intl.RoundingWindow).
func TestRoundingWindow(t *testing.T) {
	data, err := LoadData(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	date := func(y, m, d int) PlainDate {
		p, err := NewPlainDate(y, m, d, ISOCalendar(), Reject)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	rel, err := data.ParseRelativeTo([]byte("2020-01-31"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		compat      intl.Compat
		until, year string
	}{
		{intl.Standard, "P1M", "P1Y"},
		{intl.RoundingWindow, "P1M1W", "P1Y1M"},
	} {
		d, err := date(2012, 1, 1).Until(date(2012, 2, 1), DifferenceSettings{
			LargestUnit: Month, SmallestUnit: Week, RoundingMode: Ceil, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := d.String(DefaultToStringOptions()); got != c.until {
			t.Errorf("%v: until %s, want %s", c.compat, got, c.until)
		}
		year, err := ParseDuration([]byte("P1Y"))
		if err != nil {
			t.Fatal(err)
		}
		d, err = year.Round(RoundingOptions{LargestUnit: NoUnit, SmallestUnit: Month, Compat: c.compat}, rel)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := d.String(DefaultToStringOptions()); got != c.year {
			t.Errorf("%v: round %s, want %s", c.compat, got, c.year)
		}
	}
}

// Antarctica/Casey went back from 02:00 on 2010-03-05 to 23:00 on the 4th,
// so 23:10 on the 4th is after the 5th has begun. Rounding it to the day
// takes it as the 4th's last nanosecond on the standard side (ECMA-262's
// Temporal, "Fix assertion in ZonedDateTime.round"; test262's
// same-date-starts-twice), and is refused on Node's (intl.RepeatedMidnight).
func TestRepeatedMidnight(t *testing.T) {
	data, err := LoadData(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := data.ParseRelativeTo([]byte("2010-03-04T23:10:00+08:00[Antarctica/Casey]"))
	if err != nil {
		t.Fatal(err)
	}
	z := *rel.Zoned
	for mode, want := range map[RoundingMode]string{
		Floor:      "2010-03-04T00:00:00+11:00[Antarctica/Casey]",
		HalfExpand: "2010-03-05T00:00:00+11:00[Antarctica/Casey]",
		Ceil:       "2010-03-05T00:00:00+11:00[Antarctica/Casey]",
	} {
		r, err := z.Round(RoundingOptions{LargestUnit: NoUnit, SmallestUnit: Day, RoundingMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.String(OffsetAuto, TimeZoneAuto, CalendarAuto, DefaultToStringOptions())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("mode %v: %s, want %s", mode, got, want)
		}
		_, err = z.Round(RoundingOptions{LargestUnit: NoUnit, SmallestUnit: Day, RoundingMode: mode,
			Compat: intl.RepeatedMidnight})
		if !errors.Is(err, ErrRange) || err.Error() != "RangeError: ZonedDateTime is outside the expected day bounds" {
			t.Errorf("mode %v, Node: %v", mode, err)
		}
	}
}
