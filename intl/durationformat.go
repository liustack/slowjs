// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"errors"
	"math"
	"strconv"
	"strings"

	gointl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/gointl/temporal"
)

// durationFormat is a resolved Intl.DurationFormat. Its json form is the
// resolvedOptions() result, in the order Chromium reports it.
type durationFormat struct {
	Locale              string `json:"locale"`
	NumberingSystem     string `json:"numberingSystem"`
	Style               string `json:"style"`
	Years               string `json:"years"`
	YearsDisplay        string `json:"yearsDisplay"`
	Months              string `json:"months"`
	MonthsDisplay       string `json:"monthsDisplay"`
	Weeks               string `json:"weeks"`
	WeeksDisplay        string `json:"weeksDisplay"`
	Days                string `json:"days"`
	DaysDisplay         string `json:"daysDisplay"`
	Hours               string `json:"hours"`
	HoursDisplay        string `json:"hoursDisplay"`
	Minutes             string `json:"minutes"`
	MinutesDisplay      string `json:"minutesDisplay"`
	Seconds             string `json:"seconds"`
	SecondsDisplay      string `json:"secondsDisplay"`
	Milliseconds        string `json:"milliseconds"`
	MillisecondsDisplay string `json:"millisecondsDisplay"`
	Microseconds        string `json:"microseconds"`
	MicrosecondsDisplay string `json:"microsecondsDisplay"`
	Nanoseconds         string `json:"nanoseconds"`
	NanosecondsDisplay  string `json:"nanosecondsDisplay"`
	FractionalDigits    *int   `json:"fractionalDigits,omitempty"`

	f *gointl.DurationFormat
}

// durationUnits are the units of a duration, largest first, as
// gointl.DurationYears and the rest index them.
var durationUnits = [gointl.DurationUnits]string{"years", "months", "weeks", "days", "hours", "minutes", "seconds",
	"milliseconds", "microseconds", "nanoseconds"}

var (
	durationStyles     = []string{"long", "short", "narrow", "digital"}
	durationUnitStyles = []string{"", "long", "short", "narrow", "numeric", "2-digit"}
	durationDisplays   = []string{"", "auto", "always"}
)

// resolveDurationFormat is the Intl.DurationFormat constructor but for
// reading the options, which intl.js does.
func resolveDurationFormat(req request) (*durationFormat, error) {
	o := req.Options
	opts := gointl.DurationFormatOptions{Compat: goCompat}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	if s, ok := o["numberingSystem"].(string); ok {
		if !isTypeSequence(s) {
			return nil, rangeError("Invalid numberingSystem : %s", s)
		}

		opts.NumberingSystem = s
	}
	loc, err := goResolve(gointl.ServiceDurationFormat, req.Locales)
	if err != nil {
		return nil, err
	}

	style, err := optString(o, "style", durationStyles...)
	if err != nil {
		return nil, err
	}

	if style == "" {
		style = "short"
	}
	opts.Style = []gointl.DurationStyle{gointl.DurationLong, gointl.DurationShort, gointl.DurationNarrow,
		gointl.DurationDigital}[indexOf(durationStyles, style)]
	for i, unit := range durationUnits {
		allowed := []string{"long", "short", "narrow"}
		switch {
		case i >= gointl.DurationHours && i <= gointl.DurationSeconds:
			allowed = append(allowed, "numeric", "2-digit")
		case i > gointl.DurationSeconds:
			allowed = append(allowed, "numeric")
		}
		s, err := optString(o, unit, allowed...)
		if err != nil {
			return nil, err
		}

		opts.Units[i] = gointl.DurationUnitStyle(indexOf(durationUnitStyles, s))
		if s, err = optString(o, unit+"Display", "auto", "always"); err != nil {
			return nil, err
		}

		opts.Display[i] = gointl.DurationDisplay(indexOf(durationDisplays, s))
	}
	// GetNumberOption, which checks the range before it takes the floor.
	if v, ok := o["fractionalDigits"]; ok {
		f, _ := v.(float64)
		if math.IsNaN(f) || f < 0 || f > 9 {
			return nil, rangeError("fractionalDigits value is out of range.")
		}

		d := int(f)
		opts.FractionalDigits = &d
	}
	f, err := gointl.NewDurationFormatFrom(goData, loc, opts)
	switch {
	case errors.Is(err, gointl.ErrNotFound):
		return nil, err
	case err != nil:
		// The units' styles do not go together.
		return nil, rangeError("%s", strings.TrimPrefix(err.Error(), "intl: "))
	}

	r := f.ResolvedOptions()
	d := &durationFormat{
		Locale:           r.Locale,
		NumberingSystem:  r.NumberingSystem,
		Style:            durationStyles[indexOf([]gointl.DurationStyle{gointl.DurationLong, gointl.DurationShort, gointl.DurationNarrow, gointl.DurationDigital}, r.Style)],
		FractionalDigits: r.FractionalDigits,
		f:                f,
	}
	units := [...]*[2]*string{
		{&d.Years, &d.YearsDisplay}, {&d.Months, &d.MonthsDisplay}, {&d.Weeks, &d.WeeksDisplay}, {&d.Days, &d.DaysDisplay},
		{&d.Hours, &d.HoursDisplay}, {&d.Minutes, &d.MinutesDisplay}, {&d.Seconds, &d.SecondsDisplay},
		{&d.Milliseconds, &d.MillisecondsDisplay}, {&d.Microseconds, &d.MicrosecondsDisplay},
		{&d.Nanoseconds, &d.NanosecondsDisplay},
	}
	for i, u := range units {
		*u[0] = durationUnitStyles[r.Units[i]]
		*u[1] = durationDisplays[r.Display[i]]
	}
	return d, nil
}

func indexOf[T comparable](l []T, v T) int {
	for i, x := range l {
		if x == v {
			return i
		}
	}
	return 0
}

// init has nothing to do: go-intl made the formatter while resolving it.
func (f *durationFormat) init() error { return nil }

// durationOf is ToTemporalDuration, as V8 carries it out for
// Intl.DurationFormat, after javascript has read the fields: s is either a
// string of Temporal's duration syntax, as WTF-8, or, when fields, the
// fields of an object in the order of durationUnits, each a number or empty
// where the object has none, separated by commas.
func durationOf(s string, fields bool) (gointl.Duration, error) {
	var d temporal.Duration
	var err error
	if !fields {
		var b []byte
		if b, err = temporal.JSStringBytes(utf16Of(s)); err == nil {
			d, err = temporal.ParseDuration(b)
		}
	} else {
		d, err = partialDuration(s)
	}
	switch {
	case errors.Is(err, temporal.ErrType):
		return gointl.Duration{}, typeError("%s", strings.TrimPrefix(err.Error(), "TypeError: "))
	case err != nil:
		return gointl.Duration{}, rangeError("%s", strings.TrimPrefix(err.Error(), "RangeError: "))
	}

	return gointl.Duration{
		float64(d.Years()), float64(d.Months()), float64(d.Weeks()), float64(d.Days()),
		float64(d.Hours()), float64(d.Minutes()), float64(d.Seconds()), float64(d.Milliseconds()),
		d.Microseconds(), d.Nanoseconds(),
	}, nil
}

// partialDuration is ToTemporalPartialDurationRecord and the duration it
// makes, for the fields as durationOf takes them.
func partialDuration(s string) (temporal.Duration, error) {
	var p temporal.PartialDuration
	ints := [...]**int64{&p.Years, &p.Months, &p.Weeks, &p.Days, &p.Hours, &p.Minutes, &p.Seconds, &p.Milliseconds}
	for i, field := range strings.Split(s, ",") {
		if field == "" {
			continue
		}

		f, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return temporal.Duration{}, err
		}

		switch {
		case i < len(ints):
			n, err := temporal.IntegerInRange64(f)
			if err != nil {
				return temporal.Duration{}, err
			}

			*ints[i] = &n
		case i == gointl.DurationMicroseconds:
			p.Microseconds = &f
		default:
			p.Nanoseconds = &f
		}
	}
	return temporal.DurationFromPartial(p)
}
