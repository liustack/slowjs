//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

func newRelative(t *testing.T, tag string, opts intl.RelativeTimeFormatOptions) *intl.RelativeTimeFormat {
	t.Helper()
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		t.Fatalf("ParseLocale(%q): %v", tag, err)
	}
	f, err := intl.NewRelativeTimeFormat(loc, opts)
	if err != nil {
		t.Fatalf("NewRelativeTimeFormat(%q): %v", tag, err)
	}
	return f
}

// The corpus covers only the long style and seven of the eight units, so these
// go where it does not. The expectations are node's.
func TestRelativeTimeBeyondTheCorpus(t *testing.T) {
	for _, c := range []struct {
		loc  string
		opts intl.RelativeTimeFormatOptions
		v    float64
		unit string
		want string
	}{
		{"en", intl.RelativeTimeFormatOptions{Numeric: intl.RelativeAuto,
			Style: intl.RelativeShort}, -1, "day", "yesterday"},
		{"en", intl.RelativeTimeFormatOptions{Style: intl.RelativeShort},
			-1, "day", "1 day ago"},
		// The quarter, which the corpus never asks for.
		{"en", intl.RelativeTimeFormatOptions{Numeric: intl.RelativeAuto,
			Style: intl.RelativeNarrow}, 3, "quarter", "in 3q"},
		// Zero looks forward rather than back.
		{"en", intl.RelativeTimeFormatOptions{}, 0, "second", "in 0 seconds"},
		{"de", intl.RelativeTimeFormatOptions{Numeric: intl.RelativeAuto,
			Style: intl.RelativeShort}, 2, "week", "in 2 Wochen"},
		// Polish "many" and Russian "many", which English has no equivalent of.
		{"pl", intl.RelativeTimeFormatOptions{}, 5, "month", "za 5 miesięcy"},
		{"ru", intl.RelativeTimeFormatOptions{Style: intl.RelativeShort},
			21, "hour", "через 21 ч"},
		// Arabic has a dual, so two days is one word rather than a count.
		{"ar-EG", intl.RelativeTimeFormatOptions{}, 2, "day", "خلال يومين"},
	} {
		f := newRelative(t, c.loc, c.opts)
		unit, ok := intl.ParseRelativeTimeUnit(c.unit)
		if !ok {
			t.Fatalf("%q is not a unit", c.unit)
		}
		if got := f.Format(c.v, unit); got != c.want {
			t.Errorf("%s %v %s = %q, want %q", c.loc, c.v, c.unit, got, c.want)
		}
	}
}

// The pieces say which of them is the count, so a caller can style it.
func TestRelativeTimeParts(t *testing.T) {
	f := newRelative(t, "en", intl.RelativeTimeFormatOptions{})
	unit, _ := intl.ParseRelativeTimeUnit("day")
	got := f.FormatToParts(-3, unit)
	want := []intl.RelativePart{
		{Kind: intl.PartInteger, Unit: "day", Value: "3"},
		{Kind: intl.PartLiteral, Value: " days ago"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// A unit may be written singular or plural, which is what ECMA-402 accepts.
func TestParseRelativeTimeUnit(t *testing.T) {
	for _, name := range []string{"day", "days", "quarter", "quarters", "second"} {
		if _, ok := intl.ParseRelativeTimeUnit(name); !ok {
			t.Errorf("%q was not recognized", name)
		}
	}
	for _, name := range []string{"", "fortnight", "decade"} {
		if _, ok := intl.ParseRelativeTimeUnit(name); ok {
			t.Errorf("%q was recognized", name)
		}
	}
}
