// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/slowjs/temporal"

import (
	"math"

	"github.com/liustack/slowjs/internal/gointl/temporal"
)

// What V8 does to the fields of a property bag once javascript has read and
// converted them (js-temporal-objects.cc), before it hands them to
// temporal_rs: clamping or checking each to the width temporal_capi takes
// it in.

// fieldArgs are a property bag's calendar fields, as temporal.js passes
// them: day, era, eraYear, month, monthCode, year, each undefined where the
// bag has none.
type fieldArgs struct {
	day, eraYear, month, year *float64
	era, monthCode            *string
}

func (c *args) fields() fieldArgs {
	var f fieldArgs
	num := func() *float64 {
		if v, ok := c.opt(); ok {
			return &v
		}
		return nil
	}
	str := func() *string {
		if s, ok := c.optStr(); ok {
			return &s
		}
		return nil
	}
	f.day = num()
	f.era = str()
	f.eraYear = num()
	f.month = num()
	f.monthCode = str()
	f.year = num()
	return f
}

// timeArgs are a property bag's time fields, hour to nanosecond, nil where
// the bag has none.
func (c *args) timeFields() [6]*float64 {
	var t [6]*float64
	for i := range t {
		if v, ok := c.opt(); ok {
			t[i] = &v
		}
	}
	return t
}

func (c *args) overflow() temporal.Overflow { return temporal.Overflow(c.int()) }

func clamp(f, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, f)) }

// regulateDate is V8's DateRecord::Regulate, and temporal_capi's conversion
// of the result.
func regulateDate(rec fieldArgs, overflow temporal.Overflow) (temporal.CalendarFields, error) {
	var f temporal.CalendarFields
	if overflow == temporal.Constrain {
		if rec.year != nil {
			f.Year = temporal.Int(int(int32(clamp(*rec.year, math.MinInt32, math.MaxInt32))))
		}
		// V8 clamps the month and day to an int8, and hands them to Rust
		// as a u8.
		if rec.month != nil {
			f.Month = temporal.Int(int(uint8(int8(clamp(*rec.month, -128, 127)))))
		}
		if rec.day != nil {
			f.Day = temporal.Int(int(uint8(int8(clamp(*rec.day, -128, 127)))))
		}
		if rec.eraYear != nil {
			f.EraYear = temporal.Int(int(int32(clamp(*rec.eraYear, math.MinInt32, math.MaxInt32))))
		}
	} else {
		check := func(v *float64, lo, hi float64) (*int, error) {
			if v == nil {
				return nil, nil
			}
			if !(*v >= lo && *v <= hi) {
				return nil, rangeErr("Temporal error: Integer out of range.")
			}
			return temporal.Int(int(*v)), nil
		}
		var err error
		if f.Year, err = check(rec.year, math.MinInt32, math.MaxInt32); err != nil {
			return f, err
		}
		if f.Month, err = check(rec.month, 0, 255); err != nil {
			return f, err
		}
		if f.Day, err = check(rec.day, 0, 255); err != nil {
			return f, err
		}
		if f.EraYear, err = check(rec.eraYear, math.MinInt32, math.MaxInt32); err != nil {
			return f, err
		}
	}
	if rec.monthCode != nil && *rec.monthCode != "" {
		if err := temporal.ParseMonthCode(*rec.monthCode); err != nil {
			return f, jsErr(err)
		}
		f.MonthCode = temporal.String(*rec.monthCode)
	}
	if rec.era != nil && *rec.era != "" {
		if len(*rec.era) > 19 || !isASCII(*rec.era) {
			return f, rangeErr("Temporal error: Invalid era code.")
		}
		f.Era = temporal.String(*rec.era)
	}
	return f, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

var timeMaxes = [6]float64{23, 59, 59, 999, 999, 999}

// regulateTime is V8's TimeRecord::Regulate.
func regulateTime(t [6]*float64, overflow temporal.Overflow) (temporal.PartialTime, error) {
	var p temporal.PartialTime
	out := [6]**int{&p.Hour, &p.Minute, &p.Second, &p.Millisecond, &p.Microsecond, &p.Nanosecond}
	if overflow == temporal.Constrain {
		for i, v := range t {
			if v != nil {
				*out[i] = temporal.Int(int(clamp(*v, 0, timeMaxes[i])))
			}
		}
		return p, nil
	}
	for i, v := range t {
		x := 0.0
		if v != nil {
			x = *v
		}
		if x < 0 || x > timeMaxes[i] {
			return p, rangeErr("Temporal error: Invalid time provided")
		}
	}
	for i, v := range t {
		if v != nil {
			*out[i] = temporal.Int(int(*v))
		}
	}
	return p, nil
}

// validISODate is V8's IsValidIsoDate.
func validISODate(y, m, d float64) bool {
	if m < 1 || m > 12 || !(y >= math.MinInt32 && y <= math.MaxInt32) {
		return false
	}
	days := [...]float64{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[int(m)-1]
	if year := int64(y); m == 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		days = 29
	}
	return d >= 1 && d <= days
}

func validTime(t [6]float64) bool {
	for i, v := range t {
		if v < 0 || v > timeMaxes[i] {
			return false
		}
	}
	return true
}
