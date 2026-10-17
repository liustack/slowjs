// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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

