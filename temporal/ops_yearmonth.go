// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/gointl/temporal"
)

// The operations of PlainYearMonth and PlainMonthDay.

var yearMonthOps = []op{
	{"ymNew", func(c *args) (quickjs.Value, error) {
		y, m := c.num(), c.num()
		cal, err := c.calendar()
		if err != nil {
			return quickjs.Value{}, err
		}
		ref := c.num()
		if !validISODate(y, m, ref) {
			return quickjs.Value{}, rangeErr("Temporal error: Invalid ISO date.")
		}
		day := int(ref)
		ym, err := temporal.NewPlainYearMonth(int(y), int(m), &day, cal, temporal.Reject)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).yearMonth(ym).array()
	}},
	{"ymParse", func(c *args) (quickjs.Value, error) {
		p, err := temporal.ParseYearMonth(c.bytes())
		if err != nil {
			return quickjs.Value{}, err
		}
		cal, err := calendarOf(p.Calendar())
		if err != nil {
			return quickjs.Value{}, err
		}
		ym, err := temporal.PlainYearMonthFromParsed(p, cal)
		if v, ok, err := deferred(c, err); ok {
			return v, err
		}
		return ret(c).yearMonth(ym).array()
	}},
	{"ymFields", func(c *args) (quickjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return quickjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		ym, err := temporal.PlainYearMonthFromFields(f, cal, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).yearMonth(ym).array()
	}},
	{"ymWith", func(c *args) (quickjs.Value, error) {
		ym, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		out, err := ym.With(f, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).yearMonth(out).array()
	}},
	{"ymAdd", func(c *args) (quickjs.Value, error) {
		ym, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		dur, err := c.duration()
		if err != nil {
			return quickjs.Value{}, err
		}
		ov := c.overflow()
		var out temporal.PlainYearMonth
		if c.bool() {
			out, err = ym.Subtract(dur, ov)
		} else {
			out, err = ym.Add(dur, ov)
		}
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).yearMonth(out).array()
	}},
	{"ymDiff", func(c *args) (quickjs.Value, error) {
		a, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		b, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		s := c.settings()
		var d temporal.Duration
		if c.bool() {
			d, err = a.Since(b, s)
		} else {
			d, err = a.Until(b, s)
		}
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"ymStr", func(c *args) (quickjs.Value, error) {
		ym, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		return str(c, ym.String(temporal.DisplayCalendar(c.int())))
	}},
	// ymInfo answers year, month, monthCode, era, eraYear, daysInMonth,
	// daysInYear, monthsInYear and inLeapYear.
	{"ymInfo", func(c *args) (quickjs.Value, error) {
		ym, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		r := ret(c).int(ym.Year()).int(ym.Month()).str(ym.MonthCode())
		if era, ok := ym.Era(); ok && era != "" {
			r.str(era)
		} else {
			r.undefined()
		}
		r.optInt(ym.EraYear())
		return r.int(ym.DaysInMonth()).int(ym.DaysInYear()).int(ym.MonthsInYear()).bool(ym.InLeapYear()).array()
	}},
	{"ymToPD", func(c *args) (quickjs.Value, error) {
		ym, err := c.yearMonth()
		if err != nil {
			return quickjs.Value{}, err
		}
		f, err := regulateDate(c.fields(), temporal.Constrain)
		if err != nil {
			return quickjs.Value{}, err
		}
		d, err := ym.ToPlainDate(f.Day)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).date(d).array()
	}},

	{"mdNew", func(c *args) (quickjs.Value, error) {
		m, d := c.num(), c.num()
		cal, err := c.calendar()
		if err != nil {
			return quickjs.Value{}, err
		}
		ref := c.num()
		if !validISODate(ref, m, d) {
			return quickjs.Value{}, rangeErr("Temporal error: Invalid ISO date.")
		}
		year := int(ref)
		md, err := temporal.NewPlainMonthDay(int(m), int(d), cal, temporal.Reject, &year)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).monthDay(md).array()
	}},
	{"mdParse", func(c *args) (quickjs.Value, error) {
		p, err := temporal.ParseMonthDay(c.bytes())
		if err != nil {
			return quickjs.Value{}, err
		}
		cal, err := calendarOf(p.Calendar())
		if err != nil {
			return quickjs.Value{}, err
		}
		md, err := temporal.PlainMonthDayFromParsed(p, cal)
		if v, ok, err := deferred(c, err); ok {
			return v, err
		}
		return ret(c).monthDay(md).array()
	}},
	{"mdFields", func(c *args) (quickjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return quickjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		md, err := temporal.PlainMonthDayFromFields(f, cal, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).monthDay(md).array()
	}},
	{"mdWith", func(c *args) (quickjs.Value, error) {
		md, err := c.monthDay()
		if err != nil {
			return quickjs.Value{}, err
		}
		rec := c.fields()
		ov := c.overflow()
		f, err := regulateDate(rec, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		out, err := md.With(f, ov)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).monthDay(out).array()
	}},
	{"mdStr", func(c *args) (quickjs.Value, error) {
		md, err := c.monthDay()
		if err != nil {
			return quickjs.Value{}, err
		}
		return str(c, md.String(temporal.DisplayCalendar(c.int())))
	}},
	// mdInfo answers monthCode and day.
	{"mdInfo", func(c *args) (quickjs.Value, error) {
		md, err := c.monthDay()
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).str(md.MonthCode()).int(md.Day()).array()
	}},
	{"mdToPD", func(c *args) (quickjs.Value, error) {
		md, err := c.monthDay()
		if err != nil {
			return quickjs.Value{}, err
		}
		f, err := regulateDate(c.fields(), temporal.Constrain)
		if err != nil {
			return quickjs.Value{}, err
		}
		d, err := md.ToPlainDate(f.Year, f.Era, f.EraYear)
		if err != nil {
			return quickjs.Value{}, err
		}
		return ret(c).date(d).array()
	}},
}
