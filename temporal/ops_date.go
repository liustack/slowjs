// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/slowjs/temporal"

import (
	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/gointl/temporal"
)

// The operations of PlainDate, PlainTime and PlainDateTime.

// dateInfo is what the calendar getters of a date answer, in this order:
// year, month, monthCode, day, era, eraYear, dayOfWeek, dayOfYear,
// weekOfYear, yearOfWeek, daysInWeek, daysInMonth, daysInYear,
// monthsInYear, inLeapYear.
func dateInfo(c *args, d temporal.PlainDate) (slowjs.Value, error) {
	r := ret(c).int(d.Year()).int(d.Month()).str(d.MonthCode()).int(d.Day())
	if era, ok := d.Era(); ok && era != "" {
		r.str(era)
	} else {
		r.undefined()
	}
	r.optInt(d.EraYear())
	r.int(d.DayOfWeek()).int(d.DayOfYear())
	r.optInt(d.WeekOfYear())
	r.optInt(d.YearOfWeek())
	r.int(d.DaysInWeek()).int(d.DaysInMonth()).int(d.DaysInYear()).int(d.MonthsInYear()).bool(d.InLeapYear())
	return r.array()
}

var dateOps = []op{
	{"pdFields", func(c *args) (slowjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := temporal.PlainDateFromFields(f, cal, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).date(d).array()
	}},
	{"pdWith", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := d.With(f, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).date(out).array()
	}},
	// pdOf is the date V8 takes from a PlainDateTime or ZonedDateTime: its
	// year, month and day in its calendar.
	{"pdOf", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := temporal.PlainDateFromFields(temporal.CalendarFields{Year: temporal.Int(d.Year()),
			Month: temporal.Int(d.Month()), Day: temporal.Int(d.Day())}, d.Calendar(), temporal.Constrain)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).date(out).array()
	}},
	{"pdAdd", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		dur, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		ov := c.overflow()
		var out temporal.PlainDate
		if c.bool() {
			out, err = d.Subtract(dur, ov)
		} else {
			out, err = d.Add(dur, ov)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).date(out).array()
	}},
	{"pdDiff", func(c *args) (slowjs.Value, error) {
		a, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		s := c.settings()
		var d temporal.Duration
		if c.bool() {
			d, err = a.Since(b, s)
		} else {
			d, err = a.Until(b, s)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"pdStr", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, d.String(temporal.DisplayCalendar(c.int())))
	}},
	{"pdInfo", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		return dateInfo(c, d)
	}},
	{"pdToYM", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		ym, err := d.ToPlainYearMonth()
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).yearMonth(ym).array()
	}},
	{"pdToMD", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		md, err := d.ToPlainMonthDay()
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).monthDay(md).array()
	}},
	{"pdToPDT", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		t, err := c.optTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		dt, err := d.ToPlainDateTime(t)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(dt).array()
	}},
	{"pdToZDT", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		tz, err := c.timeZone()
		if err != nil {
			return slowjs.Value{}, err
		}
		t, err := c.optTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		z, err := d.ToZonedDateTime(tz, t)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
}

var timeOps = []op{
	{"ptNew", func(c *args) (slowjs.Value, error) {
		var t [6]float64
		for i := range t {
			t[i] = c.num()
		}
		if !validTime(t) {
			return slowjs.Value{}, rangeErr("Temporal error: Invalid time")
		}
		pt, err := temporal.NewPlainTime(int(t[0]), int(t[1]), int(t[2]), int(t[3]), int(t[4]), int(t[5]), temporal.Reject)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).isoTime(pt.ISO()).array()
	}},
	{"ptParse", func(c *args) (slowjs.Value, error) {
		t, err := temporal.ParsePlainTime(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).isoTime(t.ISO()).array()
	}},
	// ptFields is a time of a property bag's fields, those it lacks 0.
	{"ptFields", func(c *args) (slowjs.Value, error) {
		t := c.timeFields()
		ov := c.overflow()
		p, err := regulateTime(t, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		pt, err := temporal.PlainTimeFromPartial(p, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).isoTime(pt.ISO()).array()
	}},
	{"ptWith", func(c *args) (slowjs.Value, error) {
		t, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		f := c.timeFields()
		ov := c.overflow()
		p, err := regulateTime(f, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := t.With(p, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).isoTime(out.ISO()).array()
	}},
	{"ptAdd", func(c *args) (slowjs.Value, error) {
		t, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		if c.bool() {
			t = t.Subtract(d)
		} else {
			t = t.Add(d)
		}
		return ret(c).isoTime(t.ISO()).array()
	}},
	{"ptDiff", func(c *args) (slowjs.Value, error) {
		a, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		s := c.settings()
		var d temporal.Duration
		if c.bool() {
			d, err = a.Since(b, s)
		} else {
			d, err = a.Until(b, s)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"ptRound", func(c *args) (slowjs.Value, error) {
		t, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := t.Round(c.rounding())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).isoTime(out.ISO()).array()
	}},
	{"ptStr", func(c *args) (slowjs.Value, error) {
		t, err := c.time()
		if err != nil {
			return slowjs.Value{}, err
		}
		s, err := t.String(c.toStringOptions())
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, s)
	}},
}

var dateTimeOps = []op{
	{"pdtNew", func(c *args) (slowjs.Value, error) {
		y, m, d := c.num(), c.num(), c.num()
		var t [6]float64
		for i := range t {
			t[i] = c.num()
		}
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		if !validISODate(y, m, d) {
			return slowjs.Value{}, rangeErr("Temporal error: Invalid ISO date.")
		}
		if !validTime(t) {
			return slowjs.Value{}, rangeErr("Temporal error: Invalid time")
		}
		dt, err := temporal.NewPlainDateTime(temporal.ISODate{Year: int(y), Month: int(m), Day: int(d)},
			temporal.ISOTime{Hour: int(t[0]), Minute: int(t[1]), Second: int(t[2]), Millisecond: int(t[3]),
				Microsecond: int(t[4]), Nanosecond: int(t[5])}, cal, temporal.Reject)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(dt).array()
	}},
	{"pdtParse", func(c *args) (slowjs.Value, error) {
		p, err := temporal.ParseDateTime(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		cal, err := calendarOf(p.Calendar())
		if err != nil {
			return slowjs.Value{}, err
		}
		dt, err := temporal.PlainDateTimeFromParsed(p, cal)
		if v, ok, err := deferred(c, err); ok {
			return v, err
		}
		return ret(c).dateTime(dt).array()
	}},
	{"pdtFields", func(c *args) (slowjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		rec := c.fields()
		t := c.timeFields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		pt, err := regulateTime(t, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		dt, err := temporal.PlainDateTimeFromFields(f, pt, cal, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(dt).array()
	}},
	{"pdtWith", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		rec := c.fields()
		t := c.timeFields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		pt, err := regulateTime(t, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := dt.With(f, pt, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(out).array()
	}},
	// pdtOf is the date-time V8 makes of a PlainDate or a ZonedDateTime: the
	// date's year, month and day in its calendar, and the time, midnight
	// where undefined.
	{"pdtOf", func(c *args) (slowjs.Value, error) {
		d, err := c.date()
		if err != nil {
			return slowjs.Value{}, err
		}
		var p temporal.PartialTime
		if t, err := c.optTime(); err != nil {
			return slowjs.Value{}, err
		} else if t != nil {
			it := t.ISO()
			p = temporal.PartialTime{Hour: temporal.Int(it.Hour), Minute: temporal.Int(it.Minute),
				Second: temporal.Int(it.Second), Millisecond: temporal.Int(it.Millisecond),
				Microsecond: temporal.Int(it.Microsecond), Nanosecond: temporal.Int(it.Nanosecond)}
		}
		dt, err := temporal.PlainDateTimeFromFields(temporal.CalendarFields{Year: temporal.Int(d.Year()),
			Month: temporal.Int(d.Month()), Day: temporal.Int(d.Day())}, p, d.Calendar(), c.overflow())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(dt).array()
	}},
	{"pdtAdd", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		dur, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		ov := c.overflow()
		var out temporal.PlainDateTime
		if c.bool() {
			out, err = dt.Subtract(dur, ov)
		} else {
			out, err = dt.Add(dur, ov)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(out).array()
	}},
	{"pdtDiff", func(c *args) (slowjs.Value, error) {
		a, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		s := c.settings()
		var d temporal.Duration
		if c.bool() {
			d, err = a.Since(b, s)
		} else {
			d, err = a.Until(b, s)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"pdtRound", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := dt.Round(c.rounding())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(out).array()
	}},
	{"pdtStr", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		o := c.toStringOptions()
		s, err := dt.String(o, temporal.DisplayCalendar(c.int()))
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, s)
	}},
	{"pdtWithTime", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		t, err := c.optTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := dt.WithPlainTime(t)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).dateTime(out).array()
	}},
	{"pdtToZDT", func(c *args) (slowjs.Value, error) {
		dt, err := c.dateTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		tz, err := c.timeZone()
		if err != nil {
			return slowjs.Value{}, err
		}
		z, err := dt.ToZonedDateTime(tz, temporal.Disambiguation(c.int()))
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
}
