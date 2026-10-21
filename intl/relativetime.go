// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// relativeTimeFormat is a resolved Intl.RelativeTimeFormat.
type relativeTimeFormat struct {
	Locale          string `json:"locale"`
	Style           string `json:"style"`
	Numeric         string `json:"numeric"`
	NumberingSystem string `json:"numberingSystem"`

	f *gointl.RelativeTimeFormat
}

// resolveRelativeTimeFormat is the Intl.RelativeTimeFormat constructor but
// for reading the options, which intl.js does.
func resolveRelativeTimeFormat(req request) (*relativeTimeFormat, error) {
	o := req.Options
	opts := gointl.RelativeTimeFormatOptions{Compat: goCompat}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	if s, ok := o["numberingSystem"].(string); ok {
		if !isTypeSequence(s) {
			return nil, rangeError("Invalid numberingSystem : %s", s)
		}

		opts.NumberingSystem = s
	}
	loc, err := goResolve(gointl.ServiceRelativeTimeFormat, req.Locales)
	if err != nil {
		return nil, err
	}

	switch s, err := optString(o, "style", "long", "short", "narrow"); {
	case err != nil:
		return nil, err
	case s == "short":
		opts.Style = gointl.RelativeShort
	case s == "narrow":
		opts.Style = gointl.RelativeNarrow
	}
	switch s, err := optString(o, "numeric", "always", "auto"); {
	case err != nil:
		return nil, err
	case s == "auto":
		opts.Numeric = gointl.RelativeAuto
	}
	f, err := gointl.NewRelativeTimeFormatFrom(goData, loc, opts)
	if err != nil {
		return nil, err
	}

	r := f.ResolvedOptions()
	return &relativeTimeFormat{
		Locale:          r.Locale,
		Style:           [...]string{"long", "short", "narrow"}[r.Style],
		Numeric:         [...]string{"always", "auto"}[r.Numeric],
		NumberingSystem: r.NumberingSystem,
		f:               f,
	}, nil
}

// init has nothing to do: go-intl made the formatter while resolving it.
func (f *relativeTimeFormat) init() error { return nil }

// unit is the unit named u, for value v: a RangeError when v is not finite
// or u names no unit, as PartitionRelativeTimePattern has it.
func (f *relativeTimeFormat) unit(v float64, u string) (gointl.RelativeTimeUnit, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, rangeError("Invalid time value")
	}

	unit, ok := gointl.ParseRelativeTimeUnit(u)
	if !ok {
		return 0, rangeError("Invalid unit argument for format() '%s'", u)
	}

	return unit, nil
}
