//go:build gointl_all

package temporal

import (
	"fmt"
	"testing"
)

// Every option value prints as JavaScript spells it, "undefined" for an
// option not given and the type and number for a value no constant names,
// and each name reads back through the option's Parse function.
func TestOptionNames(t *testing.T) {
	for _, c := range []struct {
		v    fmt.Stringer
		want string
	}{
		{NoUnit, "undefined"}, {UnitAuto, "auto"}, {Nanosecond, "nanosecond"},
		{Microsecond, "microsecond"}, {Millisecond, "millisecond"}, {Second, "second"},
		{Minute, "minute"}, {Hour, "hour"}, {Day, "day"}, {Week, "week"}, {Month, "month"},
		{Year, "year"},
		{Constrain, "constrain"}, {Reject, "reject"},
		{RoundingModeUnset, "undefined"}, {Ceil, "ceil"}, {Floor, "floor"}, {Expand, "expand"},
		{Trunc, "trunc"}, {HalfCeil, "halfCeil"}, {HalfFloor, "halfFloor"},
		{HalfExpand, "halfExpand"}, {HalfTrunc, "halfTrunc"}, {HalfEven, "halfEven"},
		{Compatible, "compatible"}, {Earlier, "earlier"}, {Later, "later"},
		{DisambiguationReject, "reject"},
		{OffsetUnset, "undefined"}, {OffsetUse, "use"}, {OffsetPrefer, "prefer"},
		{OffsetIgnore, "ignore"}, {OffsetReject, "reject"},
		{CalendarAuto, "auto"}, {CalendarAlways, "always"}, {CalendarNever, "never"},
		{CalendarCritical, "critical"},
		{OffsetAuto, "auto"}, {OffsetNever, "never"},
		{TimeZoneAuto, "auto"}, {TimeZoneNever, "never"}, {TimeZoneCritical, "critical"},
		{PrecisionAuto, "auto"}, {PrecisionMinute, "minute"}, {Precision(0), "0"},
		{Precision(9), "9"},

		// No constant names these.
		{Unit(12), "Unit(12)"}, {Unit(-1), "Unit(-1)"}, {Overflow(2), "Overflow(2)"}, {RoundingMode(-1), "RoundingMode(-1)"},
		{Precision(10), "Precision(10)"}, {Precision(-3), "Precision(-3)"},
	} {
		if got := c.v.String(); got != c.want {
			t.Errorf("%T %d: %q, want %q", c.v, c.v, got, c.want)
		}
	}

	for u := UnitAuto; u <= Year; u++ {
		if got, ok := ParseUnit(u.String()); !ok || got != u {
			t.Errorf("ParseUnit(%q) = %v, %v", u, got, ok)
		}
	}
	for _, o := range []Overflow{Constrain, Reject} {
		if got, ok := ParseOverflow(o.String()); !ok || got != o {
			t.Errorf("ParseOverflow(%q) = %v, %v", o, got, ok)
		}
	}
	for m := Ceil; m <= HalfEven; m++ {
		if got, ok := ParseRoundingMode(m.String()); !ok || got != m {
			t.Errorf("ParseRoundingMode(%q) = %v, %v", m, got, ok)
		}
	}
	for d := Compatible; d <= DisambiguationReject; d++ {
		if got, ok := ParseDisambiguation(d.String()); !ok || got != d {
			t.Errorf("ParseDisambiguation(%q) = %v, %v", d, got, ok)
		}
	}
	for o := OffsetUse; o <= OffsetReject; o++ {
		if got, ok := ParseOffsetDisambiguation(o.String()); !ok || got != o {
			t.Errorf("ParseOffsetDisambiguation(%q) = %v, %v", o, got, ok)
		}
	}
	for d := CalendarAuto; d <= CalendarCritical; d++ {
		if got, err := ParseDisplayCalendar(d.String()); err != nil || got != d {
			t.Errorf("ParseDisplayCalendar(%q) = %v, %v", d, got, err)
		}
	}
	for d := OffsetAuto; d <= OffsetNever; d++ {
		if got, err := ParseDisplayOffset(d.String()); err != nil || got != d {
			t.Errorf("ParseDisplayOffset(%q) = %v, %v", d, got, err)
		}
	}
	for d := TimeZoneAuto; d <= TimeZoneCritical; d++ {
		if got, err := ParseDisplayTimeZone(d.String()); err != nil || got != d {
			t.Errorf("ParseDisplayTimeZone(%q) = %v, %v", d, got, err)
		}
	}
}
