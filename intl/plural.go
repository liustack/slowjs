// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	gointl "github.com/liustack/slowjs/internal/gointl"
)

type pluralFormat struct {
	Locale                   string   `json:"locale"`
	Type                     string   `json:"type"`
	Notation                 string   `json:"notation"`
	CompactDisplay           string   `json:"compactDisplay,omitempty"`
	MinimumIntegerDigits     int      `json:"minimumIntegerDigits"`
	MinimumFractionDigits    *int     `json:"minimumFractionDigits,omitempty"`
	MaximumFractionDigits    *int     `json:"maximumFractionDigits,omitempty"`
	MinimumSignificantDigits *int     `json:"minimumSignificantDigits,omitempty"`
	MaximumSignificantDigits *int     `json:"maximumSignificantDigits,omitempty"`
	PluralCategories         []string `json:"pluralCategories"`
	RoundingIncrement        int      `json:"roundingIncrement"`
	RoundingMode             string   `json:"roundingMode"`
	RoundingPriority         string   `json:"roundingPriority"`
	TrailingZeroDisplay      string   `json:"trailingZeroDisplay"`

	// Public resolved options omit fraction limits when significant digits
	// are present. Both still affect morePrecision and lessPrecision, so the
	// registry identity must also retain the complete parsed options.
	Options gointl.PluralRulesOptions `json:"_options"`

	f *gointl.PluralRules
}

func (p *pluralFormat) init() error { return nil }

func resolvePluralRules(req request) (*pluralFormat, error) {
	o := req.Options
	loc, err := goResolve(gointl.ServicePluralRules, req.Locales)
	if err != nil {
		return nil, err
	}
	if _, err = optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}
	opts := gointl.PluralRulesOptions{Compat: goCompat}
	if opts.Type, err = enumOption(o, "type", gointl.Cardinal, gointl.Ordinal); err != nil {
		return nil, err
	}
	if opts.Notation, err = enumOption(o, "notation", gointl.NotationStandard, gointl.NotationEngineering); err != nil {
		return nil, err
	}
	if opts.CompactDisplay, err = enumOption(o, "compactDisplay", gointl.CompactShort, gointl.CompactLong); err != nil {
		return nil, err
	}
	digits := gointl.NumberFormatOptions{}
	if err = numberDigits(o, &digits); err != nil {
		return nil, err
	}
	opts.MinimumIntegerDigits = digits.MinimumIntegerDigits
	opts.MinimumFractionDigits, opts.MaximumFractionDigits = digits.MinimumFractionDigits, digits.MaximumFractionDigits
	opts.MinimumSignificantDigits, opts.MaximumSignificantDigits = digits.MinimumSignificantDigits, digits.MaximumSignificantDigits
	opts.RoundingIncrement, opts.RoundingMode = digits.RoundingIncrement, digits.RoundingMode
	opts.RoundingPriority, opts.TrailingZeroDisplay = digits.RoundingPriority, digits.TrailingZeroDisplay
	f, err := gointl.NewPluralRulesFrom(chromiumData{}, loc, opts)
	if err != nil {
		return nil, formatOptionError(err)
	}
	r := f.ResolvedOptions()
	p := &pluralFormat{Locale: r.Locale, Type: r.Type.String(), Notation: r.Notation.String(),
		MinimumIntegerDigits: r.MinimumIntegerDigits, MinimumFractionDigits: r.MinimumFractionDigits, MaximumFractionDigits: r.MaximumFractionDigits,
		MinimumSignificantDigits: r.MinimumSignificantDigits, MaximumSignificantDigits: r.MaximumSignificantDigits,
		RoundingIncrement: r.RoundingIncrement, RoundingMode: r.RoundingMode.String(), RoundingPriority: r.RoundingPriority.String(), TrailingZeroDisplay: r.TrailingZeroDisplay.String(), Options: opts, f: f}
	if r.Notation == gointl.NotationCompact {
		p.CompactDisplay = r.CompactDisplay.String()
	}
	for _, c := range r.PluralCategories {
		p.PluralCategories = append(p.PluralCategories, string(c))
	}
	return p, nil
}

func (p *pluralFormat) selectNumber(v any) string {
	switch v := v.(type) {
	case float64:
		return string(p.f.Select(v))
	case int64:
		return string(p.f.Select(float64(v)))
	default:
		panic("intl: non-numeric PluralRules argument")
	}
}
func (p *pluralFormat) selectRange(x, y any) string {
	num := func(v any) float64 {
		switch v := v.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			panic("intl: non-numeric PluralRules argument")
		}
	}
	result, err := p.f.SelectRange(num(x), num(y))
	if err != nil {
		panic(err)
	}
	return string(result)
}
