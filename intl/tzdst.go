// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"cmp"
	"slices"
	"time"
)

// dstPeriod is a time of one zone with one offset and one daylight saving
// flag, from start (Unix seconds) to the start of the next one.
type dstPeriod struct {
	start  int64
	offset int
	dst    bool
}

// isDST reports whether t is in daylight saving time as ICU says it. ICU
// builds from the rearguard form of the tz data, where daylight saving time
// is always ahead of standard time. Go reports what the data it reads says,
// and in the main form, which Linux distributions and Go's time/tzdata ship,
// a few zones have it behind: Europe/Dublin keeps IST in summer as its
// standard time and GMT in winter as daylight saving time. Those zones take
// the flag from rearguardDST, which tzdb.go in the generator makes.
func isDST(t time.Time) bool {
	periods, i, ok := tablePeriod(t)
	if !ok {
		return t.IsDST()
	}

	// The data Go reads can be of another tzdb release than the table and
	// give t another offset than the table has for it, and the table ends in
	// 2038. Then the flag is the one of the zone's last period with t's
	// offset.
	_, offset := t.Zone()
	for ; i >= 0; i-- {
		if periods[i].offset == offset {
			return periods[i].dst
		}
	}
	return t.IsDST()
}

// tablePeriod returns the periods rearguardDST has for t's zone and the
// index of the one t is in, -1 before the first. ok is false for a zone not
// in the table.
func tablePeriod(t time.Time) (periods []dstPeriod, i int, ok bool) {
	periods, ok = rearguardDST[t.Location().String()]
	if !ok {
		return nil, 0, false
	}

	at := t.Unix()
	i, found := slices.BinarySearchFunc(periods, at, func(p dstPeriod, at int64) int {
		return cmp.Compare(p.start, at)
	})
	if !found {
		i--
	}
	return periods, i, true
}

// periodBounds is ZoneBounds with the changes of the flag the table has for
// t's zone, which the data in the main form does not have where the offset
// stays: Windhoek turned its winter time from daylight saving time into
// standard time in October 2017 without a transition there. A start or end
// is used only when the table gives t the offset Go gives it.
func periodBounds(t time.Time) (start, end time.Time) {
	start, end = t.ZoneBounds()
	if !end.IsZero() && !end.After(t) {
		// Go before 1.28 can end a period before t on the last day of a leap
		// year past the zone's last transition (golang/go#81583). A day on,
		// it finds the bounds again.
		end = t.Add(24 * time.Hour)
	}
	periods, i, ok := tablePeriod(t)
	if _, offset := t.Zone(); !ok || i < 0 || periods[i].offset != offset {
		return start, end
	}
	if s := time.Unix(periods[i].start, 0).In(t.Location()); s.After(start) {
		start = s
	}
	if i+1 < len(periods) {
		if e := time.Unix(periods[i+1].start, 0).In(t.Location()); end.IsZero() || e.Before(end) {
			end = e
		}
	}
	return start, end
}
