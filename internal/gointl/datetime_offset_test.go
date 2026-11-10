//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// An offset is a time zone in ECMA-402. It is reported as "+05:30" however it
// was written, minus zero as plus, and written as its offset -- except zero,
// which ICU makes the zone "GMT" and so names as Greenwich Mean Time.
func TestOffsetTimeZones(t *testing.T) {
	when := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		zone, resolved string
		style          intl.ZoneStyle
		want           string
	}{
		{"+05:30", "+05:30", intl.ZoneShort, "20 Uhr GMT+5:30"},
		{"+0530", "+05:30", intl.ZoneLongOffset, "20 Uhr GMT+05:30"},
		{"-08", "-08:00", intl.ZoneLong, "07 Uhr GMT-08:00"},
		{"-23:59", "-23:59", intl.ZoneShortGeneric, "15 Uhr GMT-23:59"},
		{"-00:00", "+00:00", intl.ZoneLong, "15 Uhr Mittlere Greenwich-Zeit"},
		{"+00", "+00:00", intl.ZoneShort, "15 Uhr GMT+0"},
	} {
		f := newDateTime(t, "de", intl.DateTimeFormatOptions{
			TimeZone: c.zone, Hour: intl.WidthNumeric, TimeZoneName: c.style,
		})
		if got := f.ResolvedOptions().TimeZone; got != c.resolved {
			t.Errorf("%s resolves to %q, want %q", c.zone, got, c.resolved)
		}
		if got := f.Format(when); got != c.want {
			t.Errorf("%s style %d = %+q, want %+q", c.zone, c.style, got, c.want)
		}
	}
	loc, err := intl.ParseLocale("de")
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"+05:30:00", "+24:00", "+05:60", "+05:3", "Z", "+5", "05:30"} {
		if _, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{TimeZone: zone}); err == nil {
			t.Errorf("%q is accepted as a time zone", zone)
		}
	}
}
