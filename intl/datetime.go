// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"
	"strings"
	"time"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

const maxTimeValue = 8.64e15

// Chromium replaces narrow and thin spaces in date output.
var plainSpaces = strings.NewReplacer("\u202f", " ", "\u2009", " ")

type dateTimeFormat struct {
	Locale                 string `json:"locale"`
	Calendar               string `json:"calendar"`
	NumberingSystem        string `json:"numberingSystem"`
	TimeZone               string `json:"timeZone"`
	HourCycle              string `json:"hourCycle,omitempty"`
	Hour12                 *bool  `json:"hour12,omitempty"`
	Weekday                string `json:"weekday,omitempty"`
	Era                    string `json:"era,omitempty"`
	Year                   string `json:"year,omitempty"`
	Month                  string `json:"month,omitempty"`
	Day                    string `json:"day,omitempty"`
	DayPeriod              string `json:"dayPeriod,omitempty"`
	Hour                   string `json:"hour,omitempty"`
	Minute                 string `json:"minute,omitempty"`
	Second                 string `json:"second,omitempty"`
	FractionalSecondDigits int    `json:"fractionalSecondDigits,omitempty"`
	TimeZoneName           string `json:"timeZoneName,omitempty"`
	DateStyle              string `json:"dateStyle,omitempty"`
	TimeStyle              string `json:"timeStyle,omitempty"`

	Pattern string `json:"_pattern"`
	f       *gointl.DateTimeFormat
}

func (f *dateTimeFormat) init() error { return nil }

func resolveDateTimeFormat(req request, defaultZone string) (*dateTimeFormat, error) {
	loc, err := goResolve(gointl.ServiceDateTimeFormat, req.Locales)
	if err != nil {
		return nil, err
	}
	opts, err := dateOptions(req.Options, defaultZone)
	if err != nil {
		return nil, err
	}
	f, err := gointl.NewDateTimeFormatFrom(goData, loc, opts)
	if err != nil {
		return nil, formatOptionError(err)
	}
	r := f.ResolvedOptions()
	d := &dateTimeFormat{Locale: r.Locale, Calendar: r.Calendar, NumberingSystem: r.NumberingSystem, TimeZone: r.TimeZone, Pattern: f.ChromiumPattern(), f: f}
	if r.HourCycle != gointl.HourCycleAuto {
		d.HourCycle = r.HourCycle.String()
		d.Hour12 = &r.Hour12
	}
	if r.DateStyle != gointl.LengthNone {
		d.DateStyle = r.DateStyle.String()
	}
	if r.TimeStyle != gointl.LengthNone {
		d.TimeStyle = r.TimeStyle.String()
	}
	for _, field := range []struct {
		v   gointl.FieldWidth
		out *string
	}{
		{r.Weekday, &d.Weekday}, {r.Era, &d.Era}, {r.Year, &d.Year}, {r.Month, &d.Month}, {r.Day, &d.Day}, {r.DayPeriod, &d.DayPeriod},
		{r.Hour, &d.Hour}, {r.Minute, &d.Minute}, {r.Second, &d.Second},
	} {
		if field.v != gointl.WidthNone {
			*field.out = field.v.String()
		}
	}
	d.FractionalSecondDigits = r.FractionalSecondDigits
	if r.TimeZoneName != gointl.ZoneNone {
		d.TimeZoneName = r.TimeZoneName.String()
	}
	return d, nil
}

func dateOptions(o map[string]any, defaultZone string) (gointl.DateTimeFormatOptions, error) {
	opts := gointl.DateTimeFormatOptions{TimeZone: defaultZone, Compat: goCompat &^ gointl.PlainValueZone}
	opts.ToLocaleStringTimeZone = o["_temporalZone"] == true
	var err error
	if _, err = optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return opts, err
	}
	if cal, ok := o["calendar"].(string); ok {
		if !isTypeSequence(cal) {
			return opts, rangeError("Invalid calendar: %s", cal)
		}
		opts.Calendar = cal
	}
	if opts.NumberingSystem, err = numberingSystemOption(o); err != nil {
		return opts, err
	}
	if v, ok := o["hour12"].(bool); ok {
		opts.Hour12 = &v
	}
	if opts.HourCycle, err = enumOption(o, "hourCycle", gointl.H11, gointl.H24); err != nil {
		return opts, err
	}
	if z, ok := o["timeZone"].(string); ok {
		if _, err = resolveTimeZone(z); err != nil {
			return opts, err
		}
		opts.TimeZone = z
	}
	for _, field := range []struct {
		name        string
		first, last gointl.FieldWidth
		out         *gointl.FieldWidth
	}{
		{"weekday", gointl.WidthLong, gointl.WidthNarrow, &opts.Weekday}, {"era", gointl.WidthLong, gointl.WidthNarrow, &opts.Era},
		{"year", gointl.WidthNumeric, gointl.Width2Digit, &opts.Year}, {"month", gointl.WidthNumeric, gointl.WidthNarrow, &opts.Month},
		{"day", gointl.WidthNumeric, gointl.Width2Digit, &opts.Day}, {"dayPeriod", gointl.WidthLong, gointl.WidthNarrow, &opts.DayPeriod},
		{"hour", gointl.WidthNumeric, gointl.Width2Digit, &opts.Hour}, {"minute", gointl.WidthNumeric, gointl.Width2Digit, &opts.Minute},
		{"second", gointl.WidthNumeric, gointl.Width2Digit, &opts.Second},
	} {
		if *field.out, err = enumOption(o, field.name, field.first, field.last); err != nil {
			return opts, err
		}
	}
	if opts.FractionalSecondDigits, _, err = optInt(o, "fractionalSecondDigits", 1, 3); err != nil {
		return opts, err
	}
	if opts.TimeZoneName, err = enumOption(o, "timeZoneName", gointl.ZoneShort, gointl.ZoneLongGeneric); err != nil {
		return opts, err
	}
	if _, err = optString(o, "formatMatcher", "basic", "best fit"); err != nil {
		return opts, err
	}
	if opts.DateStyle, err = enumOption(o, "dateStyle", gointl.LengthFull, gointl.LengthShort); err != nil {
		return opts, err
	}
	if opts.TimeStyle, err = enumOption(o, "timeStyle", gointl.LengthFull, gointl.LengthShort); err != nil {
		return opts, err
	}
	if opts.DateStyle != gointl.LengthNone || opts.TimeStyle != gointl.LengthNone {
		for _, key := range []string{"weekday", "era", "year", "month", "day", "dayPeriod", "hour", "minute", "second", "fractionalSecondDigits", "timeZoneName"} {
			if _, ok := o[key]; ok {
				return opts, typeError("Can't set option %s when dateStyle/timeStyle is used", key)
			}
		}
	}
	if o["_required"] == "date" && opts.TimeStyle != gointl.LengthNone {
		return opts, typeError("Invalid option : timeStyle")
	}
	if o["_required"] == "time" && opts.DateStyle != gointl.LengthNone {
		return opts, typeError("Invalid option : dateStyle")
	}
	components := map[string]gointl.DateTimeComponents{"date": gointl.ComponentsDate, "time": gointl.ComponentsTime, "any": gointl.ComponentsAny, "all": gointl.ComponentsAll}
	opts.Required, opts.Defaults = components[stringOpt(o, "_required")], components[stringOpt(o, "_defaults")]
	return opts, nil
}

func (f *dateTimeFormat) format(ms float64) ([]part, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return nil, rangeError("Invalid time value")
	}
	pieces := f.f.FormatToParts(time.UnixMilli(int64(ms)))
	parts := make([]part, len(pieces))
	for i, p := range pieces {
		parts[i] = part{Type: string(p.Kind), Value: plainSpaces.Replace(p.Value)}
	}
	return parts, nil
}

func (f *dateTimeFormat) formatString(ms float64) (string, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return "", rangeError("Invalid time value")
	}
	return plainSpaces.Replace(f.f.Format(time.UnixMilli(int64(ms)))), nil
}
