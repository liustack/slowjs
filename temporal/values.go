// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"errors"
	"math"
	"math/bits"
	"strings"
	"sync"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/temporal"
)

// How the values of temporal.js cross to Go. A Temporal object keeps its
// internal slots in javascript as numbers and strings (see Slots in
// temporal.js), and a Builtin gets them as arguments, in these groups:
//
//	date      isoYear, isoMonth, isoDay, calendar
//	time      hour, minute, second, millisecond, microsecond, nanosecond
//	dateTime  a date's three numbers, a time, calendar
//	instant   epochMilliseconds (floored), the nanoseconds past it (0 to 999999)
//	zoned     an instant, timeZone, calendar
//	duration  the ten fields
//
// Go rebuilds the go-intl value from them for each call, which is cheap:
// the calendar and the zone are looked up, the rest is checked. Results go
// back as arrays of numbers (VM.NewArray), in the same groups.

// data is Temporal's calendars and zones, read once for the process.
var data = sync.OnceValues(func() (*temporal.Data, error) { return temporal.LoadData(gointl.Embedded) })

// zones caches the zones of the identifiers temporal.js keeps, which are
// those Identifier gives: a few hundred names and offsets at most.
var zones sync.Map // string -> temporal.TimeZone

// args reads the arguments of a Builtin in turn.
type args struct {
	a quickjs.Args
	i int
}

func (c *args) next() int { c.i++; return c.i - 1 }

// num is the next argument, a number.
func (c *args) num() float64 {
	f, _ := c.a.Number(c.next())
	return f
}

func (c *args) int() int { return int(c.num()) }

// opt is the next argument, a number or undefined.
func (c *args) opt() (float64, bool) { return c.a.Number(c.next()) }

// optInt is the next argument as an *int, nil for undefined.
func (c *args) optInt() *int {
	if f, ok := c.opt(); ok {
		v := int(f)
		return &v
	}
	return nil
}

// str is the next argument, a string.
func (c *args) str() string {
	s, _ := c.a.String(c.next())
	return s
}

// optStr is the next argument, a string or undefined.
func (c *args) optStr() (string, bool) { return c.a.String(c.next()) }

func (c *args) bool() bool {
	b, _ := c.a.Bool(c.next())
	return b
}

func (c *args) calendar() (*temporal.Calendar, error) {
	d, err := data()
	if err != nil {
		return nil, err
	}
	return d.Calendar(c.str())
}

func (c *args) timeZone() (temporal.TimeZone, error) { return zoneOf(c.str()) }

// zoneOf is the zone of an identifier temporal.js keeps.
func zoneOf(id string) (temporal.TimeZone, error) {
	if tz, ok := zones.Load(id); ok {
		return tz.(temporal.TimeZone), nil
	}
	d, err := data()
	if err != nil {
		return temporal.TimeZone{}, err
	}
	tz, err := d.Zones.TimeZoneFromIdentifier([]byte(id))
	if err != nil {
		return temporal.TimeZone{}, err
	}
	zones.Store(id, tz)
	return tz, nil
}

func (c *args) date() (temporal.PlainDate, error) {
	y, m, d := c.int(), c.int(), c.int()
	cal, err := c.calendar()
	if err != nil {
		return temporal.PlainDate{}, err
	}
	return temporal.NewPlainDate(y, m, d, cal, temporal.Reject)
}

func (c *args) isoTime() temporal.ISOTime {
	return temporal.ISOTime{Hour: c.int(), Minute: c.int(), Second: c.int(), Millisecond: c.int(),
		Microsecond: c.int(), Nanosecond: c.int()}
}

func (c *args) time() (temporal.PlainTime, error) {
	t := c.isoTime()
	return temporal.NewPlainTime(t.Hour, t.Minute, t.Second, t.Millisecond, t.Microsecond, t.Nanosecond, temporal.Reject)
}

// optTime is a time, or nil where the first of its arguments is undefined
// (the others are then too).
func (c *args) optTime() (*temporal.PlainTime, error) {
	if c.a.Value(c.i).IsUndefined() {
		c.i += 6
		return nil, nil
	}
	t, err := c.time()
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *args) dateTime() (temporal.PlainDateTime, error) {
	date := temporal.ISODate{Year: c.int(), Month: c.int(), Day: c.int()}
	t := c.isoTime()
	cal, err := c.calendar()
	if err != nil {
		return temporal.PlainDateTime{}, err
	}
	return temporal.NewPlainDateTime(date, t, cal, temporal.Reject)
}

func (c *args) yearMonth() (temporal.PlainYearMonth, error) {
	y, m, d := c.int(), c.int(), c.int()
	cal, err := c.calendar()
	if err != nil {
		return temporal.PlainYearMonth{}, err
	}
	return temporal.NewPlainYearMonth(y, m, &d, cal, temporal.Reject)
}

func (c *args) monthDay() (temporal.PlainMonthDay, error) {
	y, m, d := c.int(), c.int(), c.int()
	cal, err := c.calendar()
	if err != nil {
		return temporal.PlainMonthDay{}, err
	}
	return temporal.NewPlainMonthDay(m, d, cal, temporal.Reject, &y)
}

func (c *args) instant() (temporal.Instant, error) {
	ms, sub := c.num(), c.num()
	hi, lo := epochNanoseconds(int64(ms), int64(sub))
	return temporal.NewInstant(hi, lo)
}

func (c *args) zoned() (temporal.ZonedDateTime, error) {
	i, err := c.instant()
	if err != nil {
		return temporal.ZonedDateTime{}, err
	}
	tz, err := c.timeZone()
	if err != nil {
		return temporal.ZonedDateTime{}, err
	}
	cal, err := c.calendar()
	if err != nil {
		return temporal.ZonedDateTime{}, err
	}
	return temporal.NewZonedDateTimeFromInstant(i, tz, cal)
}

func (c *args) duration() (temporal.Duration, error) {
	var f [10]float64
	for i := range f {
		f[i] = c.num()
	}
	return temporal.DurationFromNumbers(f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9])
}

// epochNanoseconds is ms × 10⁶ + sub as the two words of an int128.
func epochNanoseconds(ms, sub int64) (hi int64, lo uint64) {
	neg := ms < 0
	m := uint64(ms)
	if neg {
		m = uint64(-ms)
	}
	h, l := bits.Mul64(m, 1_000_000)
	if neg {
		l = ^l + 1
		h = ^h
		if l == 0 {
			h++
		}
	}
	l2, carry := bits.Add64(l, uint64(sub), 0)
	return int64(h + carry), l2
}

// epochParts is an instant as temporal.js keeps it.
func epochParts(i temporal.Instant) (ms, sub float64) {
	m := i.EpochMilliseconds()
	_, lo := i.EpochNanoseconds()
	return float64(m), float64(int64(lo - uint64(m)*1_000_000))
}

// results collects the values a Builtin returns as an array.
type results struct {
	vm   *quickjs.VM
	vals []quickjs.Value
	err  error
}

func newResults(vm *quickjs.VM) *results { return &results{vm: vm, vals: make([]quickjs.Value, 0, 16)} }

func (r *results) num(f float64) *results {
	if f == math.Trunc(f) && f >= math.MinInt32 && f <= math.MaxInt32 && (f != 0 || !math.Signbit(f)) {
		r.vals = append(r.vals, r.vm.NewInt(int(f)))
	} else {
		r.vals = append(r.vals, r.vm.NewFloat64(f))
	}
	return r
}

func (r *results) int(n int) *results { return r.num(float64(n)) }

func (r *results) undefined() *results {
	r.vals = append(r.vals, quickjs.UndefinedValue)
	return r
}

func (r *results) str(s string) *results {
	if r.err != nil {
		return r
	}
	v, err := r.vm.NewString(s)
	if err != nil {
		r.err = err
		return r
	}
	r.vals = append(r.vals, v)
	return r
}

func (r *results) isoDate(d temporal.ISODate) *results { return r.int(d.Year).int(d.Month).int(d.Day) }

func (r *results) isoTime(t temporal.ISOTime) *results {
	return r.int(t.Hour).int(t.Minute).int(t.Second).int(t.Millisecond).int(t.Microsecond).int(t.Nanosecond)
}

func (r *results) instant(i temporal.Instant) *results {
	ms, sub := epochParts(i)
	return r.num(ms).num(sub)
}

// zoned is a zoned date-time's instant, offset in nanoseconds and as a
// string, and wall clock, which temporal.js keeps along with its zone and
// calendar.
func (r *results) zoned(z temporal.ZonedDateTime) *results {
	iso := z.ToPlainDateTime().ISO()
	return r.instant(z.Instant()).num(float64(z.OffsetNanoseconds())).str(z.Offset()).isoDate(iso.Date).isoTime(iso.Time)
}

func (r *results) duration(d temporal.Duration) *results {
	return r.num(float64(d.Years())).num(float64(d.Months())).num(float64(d.Weeks())).num(float64(d.Days())).
		num(float64(d.Hours())).num(float64(d.Minutes())).num(float64(d.Seconds())).num(float64(d.Milliseconds())).
		num(d.Microseconds()).num(d.Nanoseconds())
}

// array is the results as a javascript array.
func (r *results) array() (quickjs.Value, error) {
	defer func() {
		for i := range r.vals {
			// UndefinedValue belongs to no VM, and undefined needs no
			// freeing.
			if !r.vals[i].IsUndefined() {
				r.vals[i].Free()
			}
		}
	}()
	if r.err != nil {
		return quickjs.Value{}, r.err
	}
	return r.vm.NewArray(r.vals...)
}

// jsBytes is the bytes V8 hands temporal_rs for a string to parse
// (temporal.JSStringBytes): a string of Latin-1 characters byte for byte,
// any other as UTF-8. A lone surrogate came over as U+FFFD, which no
// Temporal string has, so it fails to parse as it does in V8.
func jsBytes(s string) []byte {
	latin1 := true
	for _, r := range s {
		if r > 0xFF {
			latin1 = false
			break
		}
	}
	if !latin1 {
		return []byte(s)
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		b = append(b, byte(r))
	}
	return b
}

// jsErr is err as the error javascript throws, as V8 throws an error of
// temporal_rs: a RangeError or a TypeError with its message after
// "Temporal error: ", anything else an Error saying it is internal.
func jsErr(err error) error {
	if err == nil {
		return nil
	}
	var qe *quickjs.Error
	if errors.As(err, &qe) {
		return err
	}
	msg := err.Error()
	switch {
	case errors.Is(err, temporal.ErrRange):
		return &quickjs.Error{Name: "RangeError", Message: "Temporal error: " + strings.TrimPrefix(msg, "RangeError: ")}
	case errors.Is(err, temporal.ErrType):
		return &quickjs.Error{Name: "TypeError", Message: "Temporal error: " + strings.TrimPrefix(msg, "TypeError: ")}
	case errors.Is(err, temporal.ErrInternal):
		return &quickjs.Error{Name: "Error", Message: "Temporal error: Internal error: " + strings.TrimPrefix(msg, "Error: ") + "."}
	}
	return err
}

func rangeErr(msg string) error { return &quickjs.Error{Name: "RangeError", Message: msg} }
func typeErr(msg string) error  { return &quickjs.Error{Name: "TypeError", Message: msg} }
