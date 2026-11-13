//go:build gointl_all && windows

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// On Windows, where ICU does not read TZ, Node sets ICU's default zone from
// it when it starts, with createTimeZone: a name exactly as ICU spells it, a
// custom ID in any of ICU's forms, or else Etc/Unknown. The expectations are
// Node 26's, run with TZ set.
func TestHostTimeZoneFromTZ(t *testing.T) {
	for tz, want := range map[string]struct{ id, canonical string }{
		"Europe/Paris": {"Europe/Paris", "Europe/Paris"},
		"europe/paris": {"Etc/Unknown", "Etc/Unknown"},
		"GMT+05:30":    {"GMT+05:30", "+05:30"},
		"gmt-5":        {"GMT-05:00", "-05:00"},
		"GMT+0530":     {"GMT+05:30", "+05:30"},
		"GMT+5:30:15":  {"GMT+05:30:15", "+05:30:15"},
		"GMT+24":       {"Etc/Unknown", "Etc/Unknown"},
		"UTC":          {"UTC", "UTC"},
		"Etc/UTC":      {"Etc/UTC", "UTC"},
		"bogus":        {"Etc/Unknown", "Etc/Unknown"},
		"EST":          {"EST", "America/Panama"},
	} {
		t.Setenv("TZ", tz)
		z := intl.HostTimeZone(intl.Embedded, intl.NodeICU)
		canonical, _ := z.Canonical()
		if z.ID() != want.id || canonical != want.canonical {
			t.Errorf("TZ=%s: %s, %s; want %s, %s", tz, z.ID(), canonical, want.id, want.canonical)
		}
	}
	t.Setenv("TZ", "GMT+5:30:15")
	if got := intl.HostTimeZone(intl.Embedded, intl.NodeICU).Offset(0).Total(); got != 5*3600+30*60+15 {
		t.Errorf("GMT+5:30:15 is %d seconds east", got)
	}
}

// The ID "GMT", the zone's or the custom zone of offset zero's, is what V8
// names "+00:00" in a DateTimeFormat and "UTC" for Temporal.Now, and ICU
// names it Greenwich Mean Time. go-intl had given "UTC" and "+00:00" the
// other way round for TZ=GMT, and named TZ=GMT+00 "GMT+00:00". Node 26's
// answers, run with TZ set (ISSUES.md TZ-1, TZ-2).
func TestHostTimeZoneGMT(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tz, canonical, temporal, name string }{
		{"GMT", "+00:00", "UTC", "Greenwich Mean Time"},
		{"GMT+00", "+00:00", "UTC", "Greenwich Mean Time"},
		{"GMT+0", "UTC", "UTC", "Greenwich Mean Time"},
		{"GMT0", "UTC", "UTC", "Greenwich Mean Time"},
		{"UTC", "UTC", "UTC", "Coordinated Universal Time"},
		{"GMT+01:00", "+01:00", "+01:00", "GMT+01:00"},
		{"UTC+00", "Etc/Unknown", "Etc/Unknown", "GMT+00:00"},
	} {
		t.Setenv("TZ", c.tz)
		z := intl.HostTimeZone(intl.Embedded, intl.NodeICU)
		canonical, _ := z.Canonical()
		temporal := intl.DefaultTimeZone(intl.Embedded, intl.NodeICU)
		name, err := intl.TimeZoneDisplayName(intl.Embedded, en, z, false, time.Unix(0, 0))
		if err != nil {
			t.Fatal(err)
		}
		if canonical != c.canonical || temporal != c.temporal || name != c.name {
			t.Errorf("TZ=%s: %q, %q, %q; want %q, %q, %q", c.tz, canonical, temporal, name,
				c.canonical, c.temporal, c.name)
		}
	}
}
