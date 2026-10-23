// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/slowjs/temporal"

import (
	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/gointl/temporal"
)

// The operations of Instant, ZonedDateTime and Duration.

var instantOps = []op{
	{"iParse", func(c *args) (slowjs.Value, error) {
		i, err := temporal.ParseInstant(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).instant(i).array()
	}},
	{"iAdd", func(c *args) (slowjs.Value, error) {
		i, err := c.instant()
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		if c.bool() {
			i, err = i.Subtract(d)
		} else {
			i, err = i.Add(d)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).instant(i).array()
	}},
	{"iDiff", func(c *args) (slowjs.Value, error) {
		a, err := c.instant()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.instant()
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
	{"iRound", func(c *args) (slowjs.Value, error) {
		i, err := c.instant()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := i.Round(c.rounding())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).instant(out).array()
	}},
	{"iStr", func(c *args) (slowjs.Value, error) {
		i, err := c.instant()
		if err != nil {
			return slowjs.Value{}, err
		}
		var tz *temporal.TimeZone
		if id, ok := c.optStr(); ok {
			z, err := zoneOf(id)
			if err != nil {
				return slowjs.Value{}, err
			}
			tz = &z
		}
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		s, err := i.String(d.Zones, tz, c.toStringOptions())
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, s)
	}},
}

var zonedOps = []op{
	{"zMake", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
	// zParse checks a string's syntax, which ZonedDateTime.from does before
	// it reads its options: zFromString then makes the value.
	{"zParse", func(c *args) (slowjs.Value, error) {
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		_, err = d.Zones.ParseZonedDateTime(c.bytes())
		return slowjs.UndefinedValue, err
	}},
	{"zFromString", func(c *args) (slowjs.Value, error) {
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		p, err := d.Zones.ParseZonedDateTime(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		dis := temporal.Disambiguation(c.int())
		off := temporal.OffsetDisambiguation(c.int())
		cal, err := calendarOf(p.Calendar())
		if err != nil {
			return slowjs.Value{}, err
		}
		z, err := temporal.ZonedDateTimeFromParsed(p, cal, dis, off)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
	{"zFields", func(c *args) (slowjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		var dis temporal.Disambiguation
		var off temporal.OffsetDisambiguation
		var ov temporal.Overflow
		p, tz, err := c.regulateZoned(func() temporal.Overflow {
			dis, off, ov = temporal.Disambiguation(c.int()), temporal.OffsetDisambiguation(c.int()), c.overflow()
			return ov
		})
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		z, err := d.Zones.ZonedDateTimeFromFields(p, tz, cal, ov, dis, off)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
	{"zWith", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		var dis temporal.Disambiguation
		var off temporal.OffsetDisambiguation
		var ov temporal.Overflow
		p, _, err := c.regulateZoned(func() temporal.Overflow {
			dis, off, ov = temporal.Disambiguation(c.int()), temporal.OffsetDisambiguation(c.int()), c.overflow()
			return ov
		})
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := z.With(p, dis, off, ov)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(out).array()
	}},
	{"zAdd", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		ov := c.overflow()
		if c.bool() {
			z, err = z.Subtract(d, ov)
		} else {
			z, err = z.Add(d, ov)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(z).array()
	}},
	{"zDiff", func(c *args) (slowjs.Value, error) {
		a, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.zoned()
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
	{"zRound", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := z.Round(c.rounding())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(out).array()
	}},
	// zStr takes the toString options, then calendarName, offset and
	// timeZoneName.
	{"zStr", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		o := c.toStringOptions()
		showCal := temporal.DisplayCalendar(c.int())
		showOffset := temporal.DisplayOffset(c.int())
		showZone := temporal.DisplayTimeZone(c.int())
		s, err := z.String(showOffset, showZone, showCal, o)
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, s)
	}},
	{"zWithTime", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		t, err := c.optTime()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := z.WithPlainTime(t)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(out).array()
	}},
	{"zStartOfDay", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := z.StartOfDay()
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).zonedFull(out).array()
	}},
	{"zTransition", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, ok, err := z.TimeZoneTransition(c.bool())
		if err != nil {
			return slowjs.Value{}, err
		}
		if !ok {
			return slowjs.UndefinedValue, nil
		}
		return ret(c).zonedFull(out).array()
	}},
	{"zHoursInDay", func(c *args) (slowjs.Value, error) {
		z, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		h, err := z.HoursInDay()
		if err != nil {
			return slowjs.Value{}, err
		}
		return num(c, h)
	}},
	{"zEquals", func(c *args) (slowjs.Value, error) {
		a, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.zoned()
		if err != nil {
			return slowjs.Value{}, err
		}
		return boolean(c, a.Equals(b))
	}},
	{"relParse", func(c *args) (slowjs.Value, error) {
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		rel, err := d.ParseRelativeTo(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).relativeTo(rel).array()
	}},
	// relFields is the relativeTo option of a property bag: a ZonedDateTime
	// where it has a zone, else a PlainDate.
	{"relFields", func(c *args) (slowjs.Value, error) {
		cal, err := c.calendar()
		if err != nil {
			return slowjs.Value{}, err
		}
		p, tz, err := c.regulateZoned(func() temporal.Overflow { return temporal.Constrain })
		if err != nil {
			return slowjs.Value{}, err
		}
		if tz == nil {
			pd, err := temporal.PlainDateFromFields(p.Date, cal, temporal.Constrain)
			if err != nil {
				return slowjs.Value{}, err
			}
			return ret(c).relativeTo(temporal.RelativeTo{Date: &pd}).array()
		}
		d, err := data()
		if err != nil {
			return slowjs.Value{}, err
		}
		z, err := d.Zones.ZonedDateTimeFromFields(p, tz, cal, temporal.Constrain, temporal.Compatible, temporal.OffsetReject)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).relativeTo(temporal.RelativeTo{Zoned: &z}).array()
	}},
}

var durationOps = []op{
	{"dNew", func(c *args) (slowjs.Value, error) {
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"dParse", func(c *args) (slowjs.Value, error) {
		d, err := temporal.ParseDuration(c.bytes())
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	// dPartial is a duration of the fields of a property bag, undefined
	// where it has none, which temporal.js has checked are integers and the
	// first eight within an int64.
	{"dPartial", func(c *args) (slowjs.Value, error) {
		var p temporal.PartialDuration
		ints := []**int64{&p.Years, &p.Months, &p.Weeks, &p.Days, &p.Hours, &p.Minutes, &p.Seconds, &p.Milliseconds}
		for _, f := range ints {
			if v, ok := c.opt(); ok {
				if !(v >= -0x1p63 && v < 0x1p63) {
					return slowjs.Value{}, rangeErr("Temporal error: Duration field out of range.")
				}
				n := int64(v)
				*f = &n
			}
		}
		for _, f := range []**float64{&p.Microseconds, &p.Nanoseconds} {
			if v, ok := c.opt(); ok {
				*f = &v
			}
		}
		d, err := temporal.DurationFromPartial(p)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(d).array()
	}},
	{"dAdd", func(c *args) (slowjs.Value, error) {
		a, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		if c.bool() {
			a, err = a.Subtract(b)
		} else {
			a, err = a.Add(b)
		}
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(a).array()
	}},
	// dRound takes largestUnit, smallestUnit, roundingMode,
	// roundingIncrement and relativeTo.
	{"dRound", func(c *args) (slowjs.Value, error) {
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		o := temporal.RoundingOptions{LargestUnit: temporal.Unit(c.int()), SmallestUnit: temporal.Unit(c.int()),
			RoundingMode: temporal.RoundingMode(c.int()), Increment: temporal.RoundingIncrement(c.int())}
		rel, err := c.relativeTo()
		if err != nil {
			return slowjs.Value{}, err
		}
		out, err := d.Round(o, rel)
		if err != nil {
			return slowjs.Value{}, err
		}
		return ret(c).duration(out).array()
	}},
	{"dTotal", func(c *args) (slowjs.Value, error) {
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		u := temporal.Unit(c.int())
		rel, err := c.relativeTo()
		if err != nil {
			return slowjs.Value{}, err
		}
		f, err := d.Total(u, rel)
		if err != nil {
			return slowjs.Value{}, err
		}
		return c.a.VM().NewFloat64(f), nil
	}},
	{"dCompare", func(c *args) (slowjs.Value, error) {
		a, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		b, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		rel, err := c.relativeTo()
		if err != nil {
			return slowjs.Value{}, err
		}
		n, err := a.Compare(b, rel)
		if err != nil {
			return slowjs.Value{}, err
		}
		return c.a.VM().NewInt(n), nil
	}},
	{"dStr", func(c *args) (slowjs.Value, error) {
		d, err := c.duration()
		if err != nil {
			return slowjs.Value{}, err
		}
		s, err := d.String(c.toStringOptions())
		if err != nil {
			return slowjs.Value{}, err
		}
		return str(c, s)
	}},
}
