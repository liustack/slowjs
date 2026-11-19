// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"math"
	"strings"
	"time"

	gointl "github.com/liustack/slowjs/internal/gointl"
)

// Intl.DateTimeFormat on go-intl's DateTimeFormat.

// maxTimeValue is the largest time value a Date holds, in milliseconds
// either side of 1970.
const maxTimeValue = 8.64e15

// plainSpaces writes the narrow no-break space and the thin space of a date
// as plain spaces, as Chromium does: "12:00 AM", where CLDR has U+202F
// before AM. Numbers keep theirs.
var plainSpaces = strings.NewReplacer(" ", " ", " ", " ")

// A dateTimeFormat is a resolved Intl.DateTimeFormat. Its json fields are
// its resolvedOptions, in their order, and the pattern it writes with, which
// resolvedOptions leaves out: two formatters whose options read alike may
// still write with different patterns, and the registry must not take one
// for the other.
type dateTimeFormat struct {
	Locale          string `json:"locale"`
	Calendar        string `json:"calendar"`
	NumberingSystem string `json:"numberingSystem"`
	TimeZone        string `json:"timeZone"`
	// The hour cycle is there only when the format writes hours.
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
	Pattern                string `json:"_pattern"`

	f *gointl.DateTimeFormat
}

// init has nothing to do: resolveDateTimeFormat made the go-intl formatter.
func (f *dateTimeFormat) init() error { return nil }

// components are the values of the _required and _defaults options, the
// arguments ECMA-402 gives CreateDateTimeFormat.
var components = map[string]gointl.DateTimeComponents{
	"date": gointl.ComponentsDate,
	"time": gointl.ComponentsTime,
	"any":  gointl.ComponentsAny,
	"all":  gointl.ComponentsAll,
}

// fieldOptions are the options that name a field, which a date or time
// style cannot be given with, in the order ECMA-402 looks for them.
var fieldOptions = []string{"weekday", "era", "year", "month", "day", "dayPeriod", "hour", "minute", "second",
	"fractionalSecondDigits", "timeZoneName"}

// resolveDateTimeFormat is the Intl.DateTimeFormat constructor once intl.js
// has read the options, in a VM whose default time zone is defaultZone.
// Besides the page's options, the request has _required and _defaults, the
// components the caller asks for (toLocaleDateString asks for the date),
// and _temporalZone when the zone is a Temporal.ZonedDateTime's.
func resolveDateTimeFormat(req request, defaultZone string) (*dateTimeFormat, error) {
	loc, err := goResolve(req.vm, gointl.ServiceDateTimeFormat, req.Locales)
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
	df := &dateTimeFormat{
		Locale:          r.Locale,
		Calendar:        r.Calendar,
		NumberingSystem: r.NumberingSystem,
		TimeZone:        r.TimeZone,
		Weekday:         width(r.Weekday),
		Era:             width(r.Era),
		Year:            width(r.Year),
		Month:           width(r.Month),
		Day:             width(r.Day),
		DayPeriod:       width(r.DayPeriod),
		Hour:            width(r.Hour),
		Minute:          width(r.Minute),
		Second:          width(r.Second),
		Pattern:         f.ChromiumPattern(),
		f:               f,
	}
	if r.HourCycle != gointl.HourCycleAuto {
		h12 := r.Hour12
		df.HourCycle, df.Hour12 = r.HourCycle.String(), &h12
	}
	df.FractionalSecondDigits = r.FractionalSecondDigits
	if r.TimeZoneName != gointl.ZoneNone {
		df.TimeZoneName = r.TimeZoneName.String()
	}
	if r.DateStyle != gointl.LengthNone {
		df.DateStyle = r.DateStyle.String()
	}
	if r.TimeStyle != gointl.LengthNone {
		df.TimeStyle = r.TimeStyle.String()
	}
	return df, nil
}

// width is the name of w, "" for a field not written.
func width(w gointl.FieldWidth) string {
	if w == gointl.WidthNone {
		return ""
	}
	return w.String()
}

// dateOptions checks the options of a DateTimeFormat, in the order ECMA-402
// reads them.
func dateOptions(o map[string]any, defaultZone string) (gointl.DateTimeFormatOptions, error) {
	opts := gointl.DateTimeFormatOptions{
		TimeZone:               defaultZone,
		Compat:                 goCompat &^ gointl.PlainValueZone,
		ToLocaleStringTimeZone: o["_temporalZone"] == true,
	}
	var err error
	if _, err = optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return opts, err
	}

	if s, ok := o["calendar"].(string); ok {
		if !isTypeSequence(s) {
			return opts, rangeError("Invalid calendar: %s", s)
		}
		opts.Calendar = s
	}

	if s, ok := o["numberingSystem"].(string); ok {
		if !isTypeSequence(s) {
			return opts, rangeError("Invalid numberingSystem: %s", s)
		}
		opts.NumberingSystem = s
	}

	if b, ok := optBool(o, "hour12"); ok {
		opts.Hour12 = &b
	}

	if opts.HourCycle, err = enumOption(o, "hourCycle", gointl.H11, gointl.H24); err != nil {
		return opts, err
	}

	if s, ok := o["timeZone"].(string); ok {
		if _, err := resolveTimeZone(s); err != nil {
			return opts, err
		}
		opts.TimeZone = s
	}

	fields := []struct {
		key         string
		into        *gointl.FieldWidth
		first, last gointl.FieldWidth
	}{
		{"weekday", &opts.Weekday, gointl.WidthLong, gointl.WidthNarrow},
		{"era", &opts.Era, gointl.WidthLong, gointl.WidthNarrow},
		{"year", &opts.Year, gointl.WidthNumeric, gointl.Width2Digit},
		{"month", &opts.Month, gointl.WidthNumeric, gointl.WidthNarrow},
		{"day", &opts.Day, gointl.WidthNumeric, gointl.Width2Digit},
		{"dayPeriod", &opts.DayPeriod, gointl.WidthLong, gointl.WidthNarrow},
		{"hour", &opts.Hour, gointl.WidthNumeric, gointl.Width2Digit},
		{"minute", &opts.Minute, gointl.WidthNumeric, gointl.Width2Digit},
		{"second", &opts.Second, gointl.WidthNumeric, gointl.Width2Digit},
	}
	for _, f := range fields {
		if *f.into, err = enumOption(o, f.key, f.first, f.last); err != nil {
			return opts, err
		}
	}

	n, ok, err := optInt(o, "fractionalSecondDigits", 1, 3)
	if err != nil {
		return opts, err
	}
	if ok {
		opts.FractionalSecondDigits = n
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
		for _, k := range fieldOptions {
			if _, ok := o[k]; ok {
				return opts, typeError("Can't set option %s when dateStyle/timeStyle is used", k)
			}
		}
	}

	// These two messages are V8's, word for word: Temporal's tests compare
	// them.
	required, _ := o["_required"].(string)
	switch {
	case required == "date" && opts.TimeStyle != gointl.LengthNone:
		return opts, typeError("Invalid option : timeStyle")
	case required == "time" && opts.DateStyle != gointl.LengthNone:
		return opts, typeError("Invalid option : dateStyle")
	}

	defaults, _ := o["_defaults"].(string)
	opts.Required, opts.Defaults = components[required], components[defaults]
	return opts, nil
}

// format writes the time value ms, in milliseconds since 1970, as parts.
func (f *dateTimeFormat) format(ms float64) ([]part, error) {
	t, err := timeValue(ms)
	if err != nil {
		return nil, err
	}

	ps := f.f.FormatToParts(t)
	r := make([]part, len(ps))
	for i, p := range ps {
		r[i] = part{Type: string(p.Kind), Value: plainSpaces.Replace(p.Value)}
	}
	return r, nil
}

// formatString is format, joined.
func (f *dateTimeFormat) formatString(ms float64) (string, error) {
	t, err := timeValue(ms)
	if err != nil {
		return "", err
	}
	return plainSpaces.Replace(f.f.Format(t)), nil
}

// timeValue is ms as the instant a Date with that time value is, or the
// RangeError of a value no Date has.
func timeValue(ms float64) (time.Time, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return time.Time{}, rangeError("Invalid time value")
	}
	return time.UnixMilli(int64(ms)).UTC(), nil
}
