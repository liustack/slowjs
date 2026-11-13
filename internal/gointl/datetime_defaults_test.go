//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The legacy Date methods differ from Intl.DateTimeFormat only in what they
// require and what they supply when nothing is asked for. The answers are
// Node's.
func TestDateTimeRequiredAndDefaults(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	var (
		any_  = [2]intl.DateTimeComponents{intl.ComponentsAny, intl.ComponentsAll}
		date  = [2]intl.DateTimeComponents{intl.ComponentsDate, intl.ComponentsDate}
		clock = [2]intl.DateTimeComponents{intl.ComponentsTime, intl.ComponentsTime}
	)
	for _, c := range []struct {
		name   string
		method [2]intl.DateTimeComponents
		opts   intl.DateTimeFormatOptions
		want   string
	}{
		{"toLocaleString", any_, intl.DateTimeFormatOptions{}, "1/5/2024, 3:04:05 PM"},
		{"toLocaleDateString", date, intl.DateTimeFormatOptions{}, "1/5/2024"},
		{"toLocaleTimeString", clock, intl.DateTimeFormatOptions{}, "3:04:05 PM"},
		// An hour does not satisfy a method that requires a date, so the date
		// is supplied as well; and the other way about.
		{"date with an hour", date, intl.DateTimeFormatOptions{Hour: intl.WidthNumeric}, "1/5/2024, 3 PM"},
		{"time with a year", clock, intl.DateTimeFormatOptions{Year: intl.WidthNumeric}, "2024, 3:04:05 PM"},
		// Any one field satisfies "any".
		{"any with a weekday", any_, intl.DateTimeFormatOptions{Weekday: intl.WidthLong}, "Friday"},
		// A style satisfies every method that allows it.
		{"any with a date style", any_, intl.DateTimeFormatOptions{DateStyle: intl.LengthShort}, "1/5/24"},
		{"date with a date style", date, intl.DateTimeFormatOptions{DateStyle: intl.LengthLong}, "January 5, 2024"},
	} {
		opts := c.opts
		opts.TimeZone = "UTC"
		opts.Compat = intl.NodeICU
		opts.Required, opts.Defaults = c.method[0], c.method[1]
		if got := newDateTime(t, "en", opts).Format(when); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}

	// ECMA-402 throws a TypeError for a style the method cannot write.
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		name string
		opts intl.DateTimeFormatOptions
	}{
		{"toLocaleDateString with a time style", intl.DateTimeFormatOptions{
			TimeStyle: intl.LengthShort, Required: intl.ComponentsDate, Defaults: intl.ComponentsDate}},
		{"toLocaleTimeString with a date style", intl.DateTimeFormatOptions{
			DateStyle: intl.LengthShort, Required: intl.ComponentsTime, Defaults: intl.ComponentsTime}},
	} {
		if _, err := intl.NewDateTimeFormat(loc, c.opts); err == nil {
			t.Errorf("%s: built, want an error", c.name)
		}
	}
}
