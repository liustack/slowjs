//go:build gointl_all

package intl

import (
	"testing"
	"time"
	_ "time/tzdata"
)

// Where nothing names the host's zone, Standard reports its offset, as
// ECMA-262's SystemTimeZoneIdentifier has it, and NodeICU guesses from the
// C library's abbreviations as ICU's uprv_tzname does, both where go-intl
// had reported Etc/Unknown (HostAbbreviations, ISSUES.md RU-8). The
// expected values are ICU's algorithm and table, putil.cpp's, as a host in
// each zone would give them; Node itself is yet to be measured on such a
// Linux host.
func TestHostAbbreviations(t *testing.T) {
	for _, c := range []struct {
		zone      string
		want      hostAbbreviation
		raw       int
		guess     string
		canonical string // NodeICU's, "" for none
		standard  string
	}{
		// In the table.
		{"America/New_York", hostAbbreviation{"EST", "EDT", 1}, -18000, "US/Eastern", "America/New_York", "-05:00"},
		{"Europe/London", hostAbbreviation{"GMT", "BST", 1}, 0, "Europe/London", "Europe/London", "+00:00"},
		// 2007's abbreviations, which ICU's probes leave in tzname.
		{"Europe/Moscow", hostAbbreviation{"MSK", "MSD", 1}, 10800, "Europe/Moscow", "Europe/Moscow", "+03:00"},
		// Not in it: the standard abbreviation, a zone ICU knows at that
		// offset.
		{"Asia/Tokyo", hostAbbreviation{"JST", "JST", 0}, 32400, "JST", "Asia/Tokyo", "+09:00"},
		{"Europe/Paris", hostAbbreviation{"CET", "CEST", 1}, 3600, "CET", "Europe/Brussels", "+01:00"},
		{"America/Regina", hostAbbreviation{"CST", "CST", 0}, -21600, "CST", "America/Chicago", "-06:00"},
		{"UTC", hostAbbreviation{"UTC", "UTC", 0}, 0, "UTC", "UTC", "+00:00"},
		// Or a fixed zone of that name, which has no canonical one.
		{"Australia/Sydney", hostAbbreviation{"AEST", "AEDT", 2}, 36000, "AEST", "", "+10:00"},
		{"America/Sao_Paulo", hostAbbreviation{"-03", "-02", 2}, -10800, "-03", "", "-03:00"},
	} {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Fatal(err)
		}
		a := abbreviationsOf(loc)
		if a != c.want {
			t.Errorf("%s: %+v, want %+v", c.zone, a, c.want)
		}
		if got := guessHostZone(Embedded, a, c.raw); got != c.guess {
			t.Errorf("%s: guessed %q, want %q", c.zone, got, c.guess)
		}
		now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).UnixMilli()
		node := unnamedHostZone(Embedded, NodeICU, c.raw, now, func() (hostAbbreviation, bool) { return a, true })
		if got, _ := node.Canonical(); got != c.canonical {
			t.Errorf("%s: NodeICU %q, want %q", c.zone, got, c.canonical)
		}
		if got := node.Offset(now).Raw; got != c.raw {
			t.Errorf("%s: NodeICU's raw offset %d, want %d", c.zone, got, c.raw)
		}
		std := unnamedHostZone(Embedded, Standard, c.raw, now, nil)
		if got, _ := std.Canonical(); got != c.standard {
			t.Errorf("%s: Standard %q, want %q", c.zone, got, c.standard)
		}
	}

	// On Windows, where ICU reads no abbreviations, NodeICU is Etc/Unknown.
	now := time.Now().UnixMilli()
	none := unnamedHostZone(Embedded, NodeICU, 3600, now, func() (hostAbbreviation, bool) { return hostAbbreviation{}, false })
	if none.ID() != "Etc/Unknown" {
		t.Errorf("no abbreviations: %q, want Etc/Unknown", none.ID())
	}
	// An offset in seconds is written in whole minutes.
	if got, _ := unnamedHostZone(Embedded, Standard, -(4*3600 + 30*60 + 15), now, nil).Canonical(); got != "-04:30" {
		t.Errorf("-04:30:15: %q", got)
	}
}
