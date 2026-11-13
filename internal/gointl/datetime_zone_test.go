//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The names ICU's TimeZoneFormat gives zones, in the cases go-intl once got
// wrong: an offset of zero written as the locale's "GMT" rather than
// "GMT+0"; a specific name falling back to a generic one; a generic name that
// is really a location ("United Kingdom Time") or a standard name ("India
// Standard Time"); and a locale ICU keeps no zone bundle for, which falls
// back by ICU's resource rules rather than CLDR's.
func TestZoneNamesAsICU(t *testing.T) {
	winter := time.UnixMilli(1704467045000)
	for _, c := range []struct {
		tag, zone string
		style     intl.ZoneStyle
		want      string
	}{
		{"en", "UTC", intl.ZoneShortOffset, "GMT+0"},
		{"en", "UTC", intl.ZoneLongOffset, "GMT+00:00"},
		{"en", "UTC", intl.ZoneShort, "UTC"},
		{"en", "UTC", intl.ZoneLongGeneric, "GMT+00:00"},
		{"fr", "Europe/London", intl.ZoneShort, "UTC+0"},
		{"af", "Europe/London", intl.ZoneShort, "GMT+0"},
		{"en", "Europe/London", intl.ZoneShort, "GMT"},
		{"en", "Europe/London", intl.ZoneShortGeneric, "United Kingdom Time"},
		{"de", "Europe/London", intl.ZoneLongGeneric, "Vereinigtes Königreich (Ortszeit)"},
		{"en", "Africa/Abidjan", intl.ZoneLongGeneric, "Greenwich Mean Time"},
		{"de", "Africa/Abidjan", intl.ZoneShortGeneric, "Côte d’Ivoire (Ortszeit)"},
		{"en", "Asia/Kolkata", intl.ZoneLongGeneric, "India Standard Time"},
		{"en", "America/New_York", intl.ZoneShortGeneric, "ET"},
		{"en", "Australia/Lord_Howe", intl.ZoneShortGeneric, "Lord Howe Island Time"},
		{"en", "Etc/GMT+5", intl.ZoneShort, "GMT-5"},
		{"sr-Cyrl-ME", "America/New_York", intl.ZoneLong, "Severnoameričko istočno standardno vreme"},
	} {
		f := newDateTime(t, c.tag, intl.DateTimeFormatOptions{
			TimeZone: c.zone, Hour: intl.WidthNumeric, TimeZoneName: c.style,
		})
		var got string
		for _, p := range f.FormatToParts(winter) {
			if p.Kind == intl.PartTimeZoneName {
				got = p.Value
			}
		}
		if got != c.want {
			t.Errorf("%s %s style %d = %+q, want %+q", c.tag, c.zone, c.style, got, c.want)
		}
	}
}

// ICU writes the noon period only when the time as written is exactly noon:
// a pattern without seconds calls 12:00:30 noon, one with them does not. It
// never writes midnight, which it finds ambiguous, and calls the hour by its
// part of the day instead.
func TestDayPeriodNoon(t *testing.T) {
	noon := time.UnixMilli(1704456000000)
	noonAndABit := time.UnixMilli(1704456030000)
	midnight := time.UnixMilli(1704412800000)
	long := intl.DateTimeFormatOptions{TimeZone: "UTC", DayPeriod: intl.WidthLong}
	seconds := intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric,
		Minute: intl.Width2Digit, Second: intl.Width2Digit, DayPeriod: intl.WidthShort}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		when time.Time
		want string
	}{
		{"en", long, noon, "noon"},
		{"en", long, noonAndABit, "noon"},
		{"en", long, midnight, "in the morning"},
		{"en", seconds, noon, "12:00:00 noon"},
		{"en", seconds, noonAndABit, "12:00:30 in the afternoon"},
		{"am", long, noon, "ቀትር"},
		{"az", intl.DateTimeFormatOptions{TimeZone: "UTC", DayPeriod: intl.WidthNarrow}, noon, "g"},
	} {
		if got := newDateTime(t, c.tag, c.opts).Format(c.when); got != c.want {
			t.Errorf("%s %v = %+q, want %+q", c.tag, c.when, got, c.want)
		}
	}
}

// ICU's time zone update 2026c, which Node runs, moves Casablanca to the
// Western European metazone from 2026-09-20; the data archive's 2026a has
// it in none. The answers are Node's.
func TestCasablancaMetazone2026c(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{TimeZone: "Africa/Casablanca",
		TimeZoneName: intl.ZoneLong, Hour: intl.WidthNumeric, Minute: intl.WidthNumeric})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		when time.Time
		want string
	}{
		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "1:00 AM GMT+01:00"},
		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "12:00 AM Western European Standard Time"},
	} {
		if got := f.Format(c.when); got != c.want {
			t.Errorf("%s = %q, want %q", c.when.Format("2006-01-02"), got, c.want)
		}
	}
}

// TestDublinSeasonsFromICU is Ireland as ICU's zone data has it, which is
// the tz database's rearguard form: summer is the daylight time, Irish
// Standard Time, and winter Greenwich Mean Time. Go's own copy of the tz
// database has the winter as a negative daylight saving, and go-intl had
// called January Irish Standard Time. In 1970 Ireland kept one offset all
// year, which is standard time, and the zone's metazone then has no name
// for it. The expectations are Node 26's.
func TestDublinSeasonsFromICU(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		style intl.ZoneStyle
		when  time.Time
		want  string
	}{
		{intl.ZoneLong, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "1/15/2026, Greenwich Mean Time"},
		{intl.ZoneShort, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "1/15/2026, GMT"},
		{intl.ZoneLong, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "7/15/2026, Irish Standard Time"},
		{intl.ZoneShort, time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "7/15/2026, GMT+1"},
		{intl.ZoneLongGeneric, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "1/15/2026, Ireland Time"},
		{intl.ZoneLong, time.Date(1970, 1, 15, 12, 0, 0, 0, time.UTC), "1/15/1970, GMT+01:00"},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{TimeZone: "Europe/Dublin", TimeZoneName: c.style})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(c.when); got != c.want {
			t.Errorf("%v at %s = %q, want %q", c.style, c.when.Format("2006-01-02"), got, c.want)
		}
	}
}

// TestZoneResolvedAsICUCanonical is the zone resolvedOptions reports with
// ZoneIdentifiers: ICU's canonical name, whatever the case of the one given,
// and "UTC" for Etc/UTC and Etc/GMT, as V8 reports them. The expectations
// are Node 26's.
func TestZoneResolvedAsICUCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"Asia/Kolkata":          "Asia/Calcutta",
		"europe/kyiv":           "Europe/Kiev",
		"US/PACIFIC":            "America/Los_Angeles",
		"EST":                   "America/Panama",
		"etc/gmt+5":             "Etc/GMT+5",
		"GMT":                   "UTC",
		"Etc/GMT0":              "UTC",
		"Antarctica/South_Pole": "Antarctica/McMurdo",
		"america/port_of_spain": "America/Port_of_Spain",
		"SYSTEMV/AST4ADT":       "SystemV/AST4ADT",
		"+0530":                 "+05:30",
		"-00:00":                "+00:00",
	} {
		f, err := intl.NewDateTimeFormat(intl.Locale{}, intl.DateTimeFormatOptions{
			TimeZone: in, Compat: intl.ZoneIdentifiers,
		})
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got := f.ResolvedOptions().TimeZone; got != want {
			t.Errorf("%s resolved as %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"Etc/Unknown", "Factory", "Europe/", "Europe//Paris", "UTC ", "Etc/GMT+15", "+24:00", "../tz/utc"} {
		if _, err := intl.NewDateTimeFormat(intl.Locale{}, intl.DateTimeFormatOptions{TimeZone: in}); err == nil {
			t.Errorf("%q is a time zone, want an error", in)
		}
	}
}

// The zones V8 makes UTC before ICU sees them are written with UTC's names,
// as Node writes them, and resolve as UTC with ZoneIdentifiers; the other
// links to Etc/GMT keep Greenwich's.
func TestUTCAliasNames(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for zone, want := range map[string]string{
		"GMT": "Coordinated Universal Time", "Etc/GMT": "Coordinated Universal Time",
		"etc/utc": "Coordinated Universal Time", "Etc/UCT": "Coordinated Universal Time",
		"GMT0": "Coordinated Universal Time", "GMT+0": "Coordinated Universal Time",
		"GMT-0": "Coordinated Universal Time", "UTC": "Coordinated Universal Time",
		"Etc/GMT0": "Greenwich Mean Time", "Etc/GMT-0": "Greenwich Mean Time",
		"Greenwich": "Greenwich Mean Time", "Etc/Greenwich": "Greenwich Mean Time",
	} {
		for _, compat := range []intl.Compat{intl.Standard, intl.ZoneIdentifiers} {
			f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
				TimeZone: zone, TimeZoneName: intl.ZoneLong, Compat: compat,
			})
			if err != nil {
				t.Fatal(err)
			}
			var got string
			for _, p := range f.FormatToParts(time.UnixMilli(0)) {
				if p.Kind == intl.PartTimeZoneName {
					got = p.Value
				}
			}
			if got != want {
				t.Errorf("%s %v: %q, want %q", zone, compat, got, want)
			}
			if r := f.ResolvedOptions().TimeZone; compat == intl.ZoneIdentifiers && r != "UTC" {
				t.Errorf("%s: resolved %s, want UTC", zone, r)
			}
		}
	}
}
