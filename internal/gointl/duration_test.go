//go:build gointl_all

package intl_test

import (
	"math"
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// Valid is ECMA-402's IsValidDuration: whole numbers of one sign, the
// calendar units below 2**32, the rest below 2**53 seconds in all.
func TestDurationValid(t *testing.T) {
	for _, c := range []struct {
		d    intl.Duration
		want bool
	}{
		{intl.Duration{}, true},
		{intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: 46}, true},
		{intl.Duration{intl.DurationHours: -1, intl.DurationMinutes: -46}, true},
		{intl.Duration{intl.DurationHours: -1, intl.DurationMinutes: 46}, false},
		{intl.Duration{intl.DurationSeconds: 1.5}, false},
		{intl.Duration{intl.DurationDays: math.NaN()}, false},
		{intl.Duration{intl.DurationDays: math.Inf(1)}, false},
		{intl.Duration{intl.DurationYears: 1<<32 - 1}, true},
		{intl.Duration{intl.DurationYears: 1 << 32}, false},
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1}, true},
		{intl.Duration{intl.DurationSeconds: 1 << 53}, false},
		// 2**53 seconds less a nanosecond, counted exactly.
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1, intl.DurationNanoseconds: 999999999}, true},
		{intl.Duration{intl.DurationSeconds: 1<<53 - 1, intl.DurationNanoseconds: 1e9}, false},
	} {
		if got := c.d.Valid(); got != c.want {
			t.Errorf("%v.Valid() = %v, want %v", c.d, got, c.want)
		}
	}
}

// An invalid duration is an error rather than a guess.
func TestDurationFormatRefusesInvalid(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	f, err := intl.NewDurationFormat(loc, intl.DurationFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Format(intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: -1}); err == nil {
		t.Error("a duration of mixed signs was formatted")
	}
	got, err := f.Format(intl.Duration{intl.DurationHours: 1, intl.DurationMinutes: 46, intl.DurationSeconds: 40})
	if err != nil || got != "1 hr, 46 min, 40 sec" {
		t.Errorf("Format = %q, %v", got, err)
	}
}

func BenchmarkNewDurationFormat(b *testing.B) {
	loc, _ := intl.ParseLocale("de")
	for i := 0; i < b.N; i++ {
		if _, err := intl.NewDurationFormat(loc, intl.DurationFormatOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestDurationOverflowProfiles holds both profiles to what they say of a
// fraction of a second past 2**63 nanoseconds: the exact sum, and V8's
// int64 overflowed to INT64_MIN.
func TestDurationOverflowProfiles(t *testing.T) {
	en, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat intl.Compat
		opts   intl.DurationFormatOptions
		d      intl.Duration
		want   string
	}{
		{intl.Standard, intl.DurationFormatOptions{Style: intl.DurationDigital},
			intl.Duration{intl.DurationNanoseconds: 1e20}, "0:00:100000000000"},
		{intl.NodeICU, intl.DurationFormatOptions{Style: intl.DurationDigital},
			intl.Duration{intl.DurationNanoseconds: 1e20}, "0:00:9223372036.854775808"},
		{intl.Standard, intl.DurationFormatOptions{Units: [intl.DurationUnits]intl.DurationUnitStyle{intl.DurationSeconds: intl.DurationUnitNumeric}},
			intl.Duration{intl.DurationNanoseconds: 1e20}, "100000000000"},
		{intl.NodeICU, intl.DurationFormatOptions{Units: [intl.DurationUnits]intl.DurationUnitStyle{intl.DurationSeconds: intl.DurationUnitNumeric}},
			intl.Duration{intl.DurationNanoseconds: 1e20}, "-9223372036.854775808"},
		// Within range the profiles agree.
		{intl.NodeICU, intl.DurationFormatOptions{Style: intl.DurationDigital},
			intl.Duration{intl.DurationSeconds: 5, intl.DurationMilliseconds: 250, intl.DurationNanoseconds: 7}, "0:00:05.250000007"},
	} {
		c.opts.Compat = c.compat
		f, err := intl.NewDurationFormat(en, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Format(c.d)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%v: got %q, want %q", c.compat, got, c.want)
		}
	}
}
