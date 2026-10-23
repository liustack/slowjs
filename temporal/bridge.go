// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/slowjs/temporal"

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/gointl/temporal"
)

// maxInstantSec is how far from the epoch an Instant can be, in seconds:
// 1e8 days, as for a Date.
const maxInstantSec = 8_640_000_000_000

// The bridge between temporal.js and go-intl's temporal package: one
// Builtin, whose first argument names the operation, an index into ops.
// temporal.js reads every argument and option of a method in the order the
// spec reads them, with their conversions, and passes the values on (see
// values.go for how); Go does the computing and the checks that depend on
// it. An operation returns a string, a number, undefined or an array of
// them. A boolean comes back as 0 or 1.
//
// Where V8 throws an error of temporal_rs after reading options it reads
// after parsing a string, the operation parsing it returns the error as a
// array of its name and message, and temporal.js throws it once it
// has read the options: a deferred error.

// An op is an operation of the bridge.
type op struct {
	name string
	fn   func(c *args) (slowjs.Value, error)
}

// opNames is the json of the names of ops, in order, which temporal.js
// turns into the indices it calls them by.
var opNames = func() string {
	names := make([]string, len(ops))
	for i, o := range ops {
		names[i] = o.name
	}
	b, err := json.Marshal(names)
	if err != nil {
		panic(err)
	}
	return string(b)
}()

var bridge = slowjs.NewBuiltin("temporal", 0, func(a slowjs.Args) (slowjs.Value, error) {
	n, _ := a.Number(0)
	i := int(n)
	if float64(i) != n || i < 0 || i >= len(ops) {
		return slowjs.Value{}, errors.New("temporal: internal error: no such operation")
	}
	v, err := ops[i].fn(&args{a: a, i: 1})
	if err != nil {
		return slowjs.Value{}, jsErr(err)
	}
	return v, nil
})

func ret(c *args) *results { return newResults(c.a.VM()) }

func str(c *args, s string) (slowjs.Value, error) { return c.a.VM().NewString(s) }

func num(c *args, f float64) (slowjs.Value, error) { return ret(c).num(f).single() }

func boolean(c *args, b bool) (slowjs.Value, error) {
	if b {
		return c.a.VM().NewInt(1), nil
	}
	return c.a.VM().NewInt(0), nil
}

// single is the one value collected.
func (r *results) single() (slowjs.Value, error) { return r.vals[0], r.err }

// deferred returns err as temporal.js throws it later, or nil where err is
// nil.
func deferred(c *args, err error) (slowjs.Value, bool, error) {
	if err == nil {
		return slowjs.Value{}, false, nil
	}
	var qe *slowjs.Error
	if !errors.As(jsErr(err), &qe) {
		return slowjs.Value{}, true, err
	}
	v, err := ret(c).str(qe.Name).str(qe.Message).array()
	return v, true, err
}

func (r *results) date(d temporal.PlainDate) *results {
	return r.isoDate(d.ISO()).str(d.Calendar().ID())
}

func (r *results) dateTime(dt temporal.PlainDateTime) *results {
	iso := dt.ISO()
	return r.isoDate(iso.Date).isoTime(iso.Time).str(dt.Calendar().ID())
}

func (r *results) yearMonth(ym temporal.PlainYearMonth) *results {
	return r.isoDate(ym.ISO()).str(ym.Calendar().ID())
}

func (r *results) monthDay(md temporal.PlainMonthDay) *results {
	return r.isoDate(md.ISO()).str(md.Calendar().ID())
}

// zonedFull is zoned and the zone and calendar, for a ZonedDateTime
// temporal.js does not know these of.
func (r *results) zonedFull(z temporal.ZonedDateTime) *results {
	return r.zoned(z).str(z.TimeZone().Identifier()).str(z.Calendar().ID())
}

func (r *results) optInt(n int, ok bool) *results {
	if !ok {
		return r.undefined()
	}
	return r.int(n)
}

func (r *results) bool(b bool) *results {
	if b {
		return r.int(1)
	}
	return r.int(0)
}

// relativeTo is a relativeTo option as temporal.js keeps it: 1 and a date,
// 2 and a zoned date-time's instant, zone and calendar.
func (r *results) relativeTo(rel temporal.RelativeTo) *results {
	switch {
	case rel.Zoned != nil:
		ms, sub := epochParts(rel.Zoned.Instant())
		return r.int(2).num(ms).num(sub).str(rel.Zoned.TimeZone().Identifier()).str(rel.Zoned.Calendar().ID())
	case rel.Date != nil:
		return r.int(1).date(*rel.Date)
	}
	return r.int(0).undefined().undefined().undefined().undefined()
}

func (c *args) relativeTo() (temporal.RelativeTo, error) {
	switch c.int() {
	case 1:
		d, err := c.date()
		return temporal.RelativeTo{Date: &d}, err
	case 2:
		z, err := c.zoned()
		return temporal.RelativeTo{Zoned: &z}, err
	}
	c.i += 4
	return temporal.RelativeTo{}, nil
}

// settings are until's and since's options: largestUnit, smallestUnit,
// roundingMode and roundingIncrement.
func (c *args) settings() temporal.DifferenceSettings {
	return temporal.DifferenceSettings{LargestUnit: temporal.Unit(c.int()), SmallestUnit: temporal.Unit(c.int()),
		RoundingMode: temporal.RoundingMode(c.int()), Increment: temporal.RoundingIncrement(c.int())}
}

// rounding are round's options: smallestUnit, roundingMode and
// roundingIncrement.
func (c *args) rounding() temporal.RoundingOptions {
	return temporal.RoundingOptions{SmallestUnit: temporal.Unit(c.int()), RoundingMode: temporal.RoundingMode(c.int()),
		Increment: temporal.RoundingIncrement(c.int())}
}

// toStringOptions are fractionalSecondDigits (-1 for auto), smallestUnit
// and roundingMode.
func (c *args) toStringOptions() temporal.ToStringRoundingOptions {
	return temporal.ToStringRoundingOptions{Precision: temporal.Precision(c.int()), SmallestUnit: temporal.Unit(c.int()),
		RoundingMode: temporal.RoundingMode(c.int())}
}

func (c *args) bytes() []byte { return jsBytes(c.str()) }

// regulateZoned is V8's CombinedRecord::Regulate for a
// PartialZonedDateTime: the fields, the time fields and the offset.
func (c *args) regulateZoned(overflow func() temporal.Overflow) (temporal.PartialZonedDateTime, *temporal.TimeZone, error) {
	f := c.fields()
	t := c.timeFields()
	off, hasOff := c.optStr()
	var tz *temporal.TimeZone
	if id, ok := c.optStr(); ok {
		z, err := zoneOf(id)
		if err != nil {
			return temporal.PartialZonedDateTime{}, nil, err
		}
		tz = &z
	}
	ov := overflow()
	var p temporal.PartialZonedDateTime
	var err error
	if p.Date, err = regulateDate(f, ov); err != nil {
		return p, nil, err
	}
	if p.Time, err = regulateTime(t, ov); err != nil {
		return p, nil, err
	}
	if hasOff {
		ns, err := temporal.ParseOffset([]byte(off))
		if err != nil {
			return p, nil, err
		}
		p.Offset = &ns
	}
	return p, tz, nil
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

func calendarOf(id string) (*temporal.Calendar, error) {
	d, err := data()
	if err != nil {
		return nil, err
	}
	return d.Calendar(id)
}

var coreOps = []op{
	// ---- Calendars, zones and offsets.
	{"calCanon", func(c *args) (slowjs.Value, error) {
		s := c.str()
		id, ok := temporal.CalendarID(lowerASCII(s))
		if !ok {
			return slowjs.Value{}, rangeErr("Temporal error: Unknown calendar type " + s + ".")
		}
		cal, err := calendarOf(id)
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, cal.ID())
	}},
	{"calParse", func(c *args) (slowjs.Value, error) {
		id, ok := temporal.ParseCalendarString([]byte(c.str()))
		if !ok {
			return slowjs.Value{}, rangeErr("Temporal error: Invalid calendar string")
		}
		cal, err := calendarOf(id)
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, cal.ID())
	}},
	{"calEras", func(c *args) (slowjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		return boolean(c, cal.HasEras())
	}},
	{"tzParse", func(c *args) (slowjs.Value, error) {
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		tz, err := d.Zones.TimeZoneFromString([]byte(c.str()))
		if err != nil {
			return slowjs.Value{}, err
		}
		id := tz.Identifier()
		zones.LoadOrStore(id, tz)
		return str(c, id)
	}},
	{"tzIdent", func(c *args) (slowjs.Value, error) {
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		tz, err := d.Zones.TimeZoneFromIdentifier([]byte(c.str()))
		if err != nil {
			return slowjs.Value{}, err
		}
		id := tz.Identifier()
		zones.LoadOrStore(id, tz)
		return str(c, id)
	}},
	{"tzEquals", func(c *args) (slowjs.Value, error) {
		a, err := c.timeZone()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.timeZone()
		if err != nil {
			return slowjs.Value{}, err
		}
		return boolean(c, a.Equals(b))
	}},
	{"offsetCheck", func(c *args) (slowjs.Value, error) {
		ns, err := temporal.ParseOffset([]byte(c.str()))
		if err != nil {
			return slowjs.Value{}, err
		}
		return num(c, float64(ns))
	}},
	{"now", func(c *args) (slowjs.Value, error) {
		// Seconds and nanoseconds, not UnixNano, which holds only the years
		// 1678 to 2262. An Instant is at most 8.64e12 s from the epoch.
		t := clockOf(c.a.VM())()
		sec, nsec := t.Unix(), int64(t.Nanosecond())
		if sec < -maxInstantSec || sec > maxInstantSec || sec == maxInstantSec && nsec > 0 {
			return slowjs.Value{}, rangeErr("Temporal.Now: the clock reads " + t.UTC().Format(time.RFC3339Nano) + ", outside the range of Temporal.Instant")
		}
		ms := sec*1000 + nsec/1_000_000
		return ret(c).num(float64(ms)).num(float64(nsec % 1_000_000)).array()
	}},

	// ---- PlainDate.
	{"pdNew", func(c *args) (slowjs.Value, error) {
		y, m, d := c.num(), c.num(), c.num()
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		if !validISODate(y, m, d) {
			return slowjs.Value{}, rangeErr("Temporal error: Invalid ISO date.")
		}
		pd, err := temporal.NewPlainDate(int(y), int(m), int(d), cal, temporal.Reject)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).date(pd).array()
	}},
	{"pdParse", func(c *args) (slowjs.Value, error) {
		p, err := temporal.ParseDate(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		cal, err := calendarOf(p.Calendar())
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := temporal.PlainDateFromParsed(p, cal)
		if v, ok, err := deferred(c, err); ok {
			return v, err
		}
		return ret(c).date(d).array()
	}},
}

// ops are the operations, in the order of their indices.
var ops = concat(coreOps, dateOps, timeOps, dateTimeOps, yearMonthOps, instantOps, zonedOps, durationOps)

func concat(lists ...[]op) []op {
	var r []op
	for _, l := range lists {
		r = append(r, l...)
	}
	return r
}
