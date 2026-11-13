//go:build gointl_all

package intl

import (
	"testing"
	"time"
)

// TestOffsetFromLocalAsV8 reads local times as V8's Date does, with the
// offset before a transition both where it skips the time and where it
// repeats it. The instants are Node 26's for new Date(y, m, d, h, min)
// under each zone.
func TestOffsetFromLocalAsV8(t *testing.T) {
	for _, c := range []struct {
		zone  string
		local time.Time
		want  int64
	}{
		// New York's spring gap, the minute before it and its end.
		{"America/New_York", time.Date(2026, 3, 8, 2, 30, 0, 0, time.UTC), 1772955000000},
		{"America/New_York", time.Date(2026, 3, 8, 1, 59, 0, 0, time.UTC), 1772953140000},
		{"America/New_York", time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC), 1772953200000},
		// Its repeated hour in November, read as daylight time.
		{"America/New_York", time.Date(2026, 11, 1, 1, 30, 0, 0, time.UTC), 1793511000000},
		// Dublin's, whose summer is the daylight time in ICU's data.
		{"Europe/Dublin", time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC), 1774747800000},
		{"Europe/Dublin", time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), 1792888200000},
	} {
		z, err := LoadTimeZone(Embedded, c.zone)
		if err != nil {
			t.Fatal(err)
		}
		local := c.local.UnixMilli()
		got := local - int64(z.OffsetFromLocal(local, Former, Former).Total())*1000
		if got != c.want {
			t.Errorf("%s %s = %d, want %d", c.zone, c.local.Format("2006-01-02 15:04"), got, c.want)
		}
	}
}

// TestZoneTransitionsAPI is New York's transitions either side of June
// 2026 as Temporal gives them, with the offsets ICU keeps apart.
func TestZoneTransitionsAPI(t *testing.T) {
	z, err := LoadTimeZone(Embedded, "america/new_york")
	if err != nil {
		t.Fatal(err)
	}
	if z.ID() != "America/New_York" {
		t.Errorf("ID = %q", z.ID())
	}
	june := time.Date(2026, 6, 1, 4, 0, 0, 0, time.UTC).UnixMilli()
	est, edt := ZoneOffset{Raw: -18000}, ZoneOffset{Raw: -18000, DST: 3600}
	if got := z.Offset(june); got != edt {
		t.Errorf("Offset = %+v, want %+v", got, edt)
	}
	next, ok := z.NextTransition(june)
	if want := (ZoneTransition{At: 1793512800000, Before: edt, After: est}); !ok || next != want {
		t.Errorf("NextTransition = %+v %v, want %+v", next, ok, want)
	}
	prev, ok := z.PreviousTransition(june)
	if want := (ZoneTransition{At: 1772953200000, Before: est, After: edt}); !ok || prev != want {
		t.Errorf("PreviousTransition = %+v %v, want %+v", prev, ok, want)
	}
	// Exclusive either way: the transition itself is not its own neighbour.
	if again, _ := z.NextTransition(next.At); again.At <= next.At {
		t.Errorf("NextTransition(%d) = %d", next.At, again.At)
	}
	if again, _ := z.PreviousTransition(prev.At); again.At >= prev.At {
		t.Errorf("PreviousTransition(%d) = %d", prev.At, again.At)
	}
	fixed, err := LoadTimeZone(Embedded, "+0530")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fixed.NextTransition(june); ok {
		t.Error("an offset zone has a transition")
	}
	if c, _ := fixed.Canonical(); c != "+05:30" || fixed.Offset(june).Total() != 19800 {
		t.Errorf("+0530 is %q at %d", c, fixed.Offset(june).Total())
	}
}

// TestDetectHostZone is ICU's detectHostTimeZone over what a host says:
// a name ICU spells that way is its zone; one it does not know, or a three-
// or four-letter one at another offset, is a zone of the host's offset
// with no canonical name, which V8 reports as UTC; no name is Etc/Unknown.
func TestDetectHostZone(t *testing.T) {
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC).UnixMilli()
	for _, c := range []struct {
		id        string
		raw       int
		wantID    string
		canonical string
		offset    int
	}{
		{"Asia/Kolkata", 19800, "Asia/Kolkata", "Asia/Calcutta", 19800},
		{"Etc/UTC", 0, "Etc/UTC", "UTC", 0},
		{"Etc/GMT+5", -18000, "Etc/GMT+5", "Etc/GMT+5", -18000},
		{"EST", -18000, "EST", "America/Panama", -18000},
		// An abbreviation at another offset, and a name ICU spells otherwise.
		{"EST", 3600, "EST", "", 3600},
		{"asia/kolkata", 19800, "asia/kolkata", "", 19800},
		{"Mars/Olympus", -7200, "Mars/Olympus", "", -7200},
		{"", 0, "Etc/Unknown", "Etc/Unknown", 0},
	} {
		z := detectHostZone(Embedded, c.id, c.raw, now)
		canonical, _ := z.Canonical()
		if z.ID() != c.wantID || canonical != c.canonical || z.Offset(now).Total() != c.offset {
			t.Errorf("%q at %d = %q, %q at %d; want %q, %q at %d", c.id, c.raw,
				z.ID(), canonical, z.Offset(now).Total(), c.wantID, c.canonical, c.offset)
		}
	}
}

// TestWindowsZones is CLDR's mapping as ICU reads it: the first zone
// listed for the user's region, else for 001.
func TestWindowsZones(t *testing.T) {
	for _, c := range []struct{ windows, region, want string }{
		{"Central Europe Standard Time", "CZ", "Europe/Prague"},
		{"Central Europe Standard Time", "US", "Europe/Budapest"},
		{"Central Europe Standard Time", "", "Europe/Budapest"},
		{"Eastern Standard Time", "US", "America/New_York"},
		{"Eastern Standard Time", "CA", "America/Toronto"},
		{"AUS Eastern Standard Time", "AU", "Australia/Sydney"},
		{"Alaskan Standard Time", "US", "America/Anchorage"},
	} {
		got, err := windowsZone(Embedded, c.windows, c.region)
		if err != nil || got != c.want {
			t.Errorf("%s in %q = %q, %v; want %q", c.windows, c.region, got, err, c.want)
		}
	}
	if got, err := windowsZone(Embedded, "Mars Standard Time", "US"); err == nil {
		t.Errorf("Mars Standard Time = %q", got)
	}
}
