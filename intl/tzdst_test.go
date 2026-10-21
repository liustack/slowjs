// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// testdata/tzdata holds a few zones in both forms of the tz data, compiled
// from the tzdb release of the table by the generator.
var tzdataZones = []string{"Africa/Casablanca", "Africa/Windhoek", "America/Nome", "Europe/Dublin", "Europe/Minsk"}

func loadTestZone(t *testing.T, form, name string) *time.Location {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "tzdata", form, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}

	loc, err := time.LoadLocationFromTZData(name, b)
	if err != nil {
		t.Fatal(err)
	}

	return loc
}

// testPeriods returns the periods of loc before 2038 as the generator
// finds them.
func testPeriods(t *testing.T, loc *time.Location) (r []dstPeriod) {
	limit := time.Date(2038, 1, 1, 0, 0, 0, 0, time.UTC)
	at := time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC).In(loc)
	start := int64(math.MinInt64)
	for {
		_, off := at.Zone()
		if n := len(r); n == 0 || r[n-1].offset != off || r[n-1].dst != at.IsDST() {
			r = append(r, dstPeriod{start, off, at.IsDST()})
		}
		_, end := at.ZoneBounds()
		if end.IsZero() || !end.Before(limit) {
			return r
		}

		if !end.After(at) {
			t.Fatalf("%s: the period at %v ends at %v", loc, at, end)
		}

		at, start = end, end.Unix()
	}
}

// The table holds a zone exactly when its two forms differ in the daylight
// saving flag, and then holds the rearguard form's periods.
func TestDSTTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "tzdata", "version"))
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(b)); got != tzdbVersion {
		t.Fatalf("testdata/tzdata is tzdb %s, the table %s: run the generator", got, tzdbVersion)
	}

	for _, name := range tzdataZones {
		main, rearguard := testPeriods(t, loadTestZone(t, "main", name)), testPeriods(t, loadTestZone(t, "rearguard", name))
		// The offsets of the two forms are the same, so their periods
		// differ only where the flag does.
		differ := !slices.Equal(main, rearguard)
		got, ok := rearguardDST[name]
		switch {
		case ok != differ:
			t.Errorf("%s: in the table %v, the two forms differ %v", name, ok, differ)
		case ok && !slices.Equal(got, rearguard):
			t.Errorf("%s: the table has %d periods that are not the %d of the rearguard form", name, len(got), len(rearguard))
		}
	}
}

// isDST answers as the rearguard form does, whichever form the zone was read
// from: at the start, the middle and the last second of every period of
// either form from 1970 to 2038, and every ten days from then to 2100, where
// Go works the periods out from the zone's rule.
func TestDaylightSavingIsTheRearguardOne(t *testing.T) {
	from, to := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), time.Date(2038, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	for _, name := range tzdataZones {
		main, rearguard := loadTestZone(t, "main", name), loadTestZone(t, "rearguard", name)
		starts := []int64{from, to}
		for _, loc := range []*time.Location{main, rearguard} {
			for _, p := range testPeriods(t, loc) {
				if p.start > from && p.start < to {
					starts = append(starts, p.start)
				}
			}
		}
		slices.Sort(starts)
		starts = slices.Compact(starts)
		var samples []int64
		for i, s := range starts[:len(starts)-1] {
			next := starts[i+1]
			samples = append(samples, s, s+(next-s)/2, next-1)
		}
		for at := to; at < time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix(); at += 10 * 24 * 3600 {
			samples = append(samples, at)
		}
		differ := 0
		for _, s := range samples {
			at := time.Unix(s, 0)
			want := at.In(rearguard).IsDST()
			if at.In(main).IsDST() != want {
				differ++
			}
			if got := isDST(at.In(main)); got != want {
				t.Errorf("%s %s: isDST in the main form = %v, want %v", name, at.UTC().Format(time.DateTime), got, want)
			}
			if got := isDST(at.In(rearguard)); got != want {
				t.Errorf("%s %s: isDST in the rearguard form = %v, want %v", name, at.UTC().Format(time.DateTime), got, want)
			}
			// dstNear finds the changes of the flag the table has, also where
			// the data in the main form has no transition, as Windhoek in
			// October 2017, when only the flag changed.
			if got, want := dstNear(at.In(main)), dstNear(at.In(rearguard)); got != want {
				t.Errorf("%s %s: dstNear in the main form = %v, in the rearguard form %v", name, at.UTC().Format(time.DateTime), got, want)
			}
		}
		if _, ok := rearguardDST[name]; ok != (differ != 0) {
			t.Errorf("%s: in the table %v, but the forms differ at %d of %d times", name, ok, differ, len(samples))
		}
	}
}

// Zone data of another tzdb release than the table's can give a time of a
// zone in the table an offset the table does not have for it then. The flag
// is then the one of the zone's last period with that offset.
func TestDaylightSavingOfAnotherRelease(t *testing.T) {
	for _, test := range []struct {
		zone   string
		offset int
		at     time.Time
		want   bool
	}{
		// The table has +01 as daylight saving time from 2026-03-22, and
		// +00 as standard time before.
		{"Africa/Casablanca", 0, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), false},
		// Past the table, summer time is the +01 of 2037.
		{"Europe/Dublin", 3600, time.Date(2040, 7, 1, 0, 0, 0, 0, time.UTC), true},
		{"Europe/Dublin", 0, time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC), false},
		// Dublin kept +01 as its standard time from 1968 to 1971.
		{"Europe/Dublin", 3600, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), false},
	} {
		at := test.at.In(time.FixedZone(test.zone, test.offset))
		if got := isDST(at); got != test.want {
			t.Errorf("%s at %v: isDST = %v, want %v", test.zone, at, got, test.want)
		}
	}
}
