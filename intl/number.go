// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"errors"
	"strconv"

	gointl "github.com/liustack/slowjs/internal/gointl"
)

type numberFormat struct {
	Locale                   string `json:"locale"`
	NumberingSystem          string `json:"numberingSystem"`
	Style                    string `json:"style"`
	Currency                 string `json:"currency,omitempty"`
	CurrencyDisplay          string `json:"currencyDisplay,omitempty"`
	CurrencySign             string `json:"currencySign,omitempty"`
	Unit                     string `json:"unit,omitempty"`
	UnitDisplay              string `json:"unitDisplay,omitempty"`
	MinimumIntegerDigits     int    `json:"minimumIntegerDigits"`
	MinimumFractionDigits    *int   `json:"minimumFractionDigits,omitempty"`
	MaximumFractionDigits    *int   `json:"maximumFractionDigits,omitempty"`
	MinimumSignificantDigits *int   `json:"minimumSignificantDigits,omitempty"`
	MaximumSignificantDigits *int   `json:"maximumSignificantDigits,omitempty"`
	UseGrouping              any    `json:"useGrouping"`
	Notation                 string `json:"notation"`
	CompactDisplay           string `json:"compactDisplay,omitempty"`
	SignDisplay              string `json:"signDisplay"`
	RoundingIncrement        int    `json:"roundingIncrement"`
	RoundingMode             string `json:"roundingMode"`
	RoundingPriority         string `json:"roundingPriority"`
	TrailingZeroDisplay      string `json:"trailingZeroDisplay"`

	f *gointl.NumberFormat
}

func (f *numberFormat) init() error { return nil }

func resolveNumberFormat(req request) (*numberFormat, error) {
	o := req.Options
	loc, err := goResolve(gointl.ServiceNumberFormat, req.Locales)
	if err != nil {
		return nil, err
	}
	opts, err := numberOptions(o)
	if err != nil {
		return nil, err
	}
	f, err := gointl.NewNumberFormatFrom(goData, loc, opts)
	if err != nil {
		return nil, formatOptionError(err)
	}
	r := f.ResolvedOptions()
	n := &numberFormat{
		Locale: r.Locale, NumberingSystem: r.NumberingSystem, Style: r.Style.String(),
		MinimumIntegerDigits: r.MinimumIntegerDigits, MinimumFractionDigits: r.MinimumFractionDigits,
		MaximumFractionDigits: r.MaximumFractionDigits, MinimumSignificantDigits: r.MinimumSignificantDigits,
		MaximumSignificantDigits: r.MaximumSignificantDigits, UseGrouping: r.UseGrouping.String(),
		Notation: r.Notation.String(), SignDisplay: r.SignDisplay.String(), RoundingIncrement: r.RoundingIncrement,
		RoundingMode: r.RoundingMode.String(), RoundingPriority: r.RoundingPriority.String(), TrailingZeroDisplay: r.TrailingZeroDisplay.String(), f: f,
	}
	if r.UseGrouping == gointl.GroupingNever {
		n.UseGrouping = false
	}
	if r.Style == gointl.StyleCurrency {
		n.Currency = r.Currency
		n.CurrencyDisplay = r.CurrencyDisplay.String()
		n.CurrencySign = r.CurrencySign.String()
	}
	if r.Style == gointl.StyleUnit {
		n.Unit = r.Unit
		n.UnitDisplay = r.UnitDisplay.String()
	}
	if r.Notation == gointl.NotationCompact {
		n.CompactDisplay = r.CompactDisplay.String()
	}
	return n, nil
}

func numberOptions(o map[string]any) (gointl.NumberFormatOptions, error) {
	opts := gointl.NumberFormatOptions{Compat: goCompat}
	var err error
	if _, err = optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return opts, err
	}
	if opts.NumberingSystem, err = numberingSystemOption(o); err != nil {
		return opts, err
	}
	if opts.Style, err = enumOption(o, "style", gointl.StyleDecimal, gointl.StyleUnit); err != nil {
		return opts, err
	}
	if currency, ok := o["currency"].(string); ok {
		if len(currency) != 3 || !isAlpha(currency) {
			return opts, rangeError("Invalid currency code: %s", currency)
		}
		opts.Currency = currency
	}
	if opts.Style == gointl.StyleCurrency && opts.Currency == "" {
		return opts, typeError("Currency code is required with currency style.")
	}
	if opts.CurrencyDisplay, err = enumOption(o, "currencyDisplay", gointl.CurrencySymbol, gointl.CurrencyName); err != nil {
		return opts, err
	}
	if opts.CurrencySign, err = enumOption(o, "currencySign", gointl.CurrencySignStandard, gointl.CurrencySignAccounting); err != nil {
		return opts, err
	}
	if unit, ok := o["unit"].(string); ok {
		if !gointl.HasUnit(unit) {
			return opts, rangeError("Invalid unit argument: %s", unit)
		}
		opts.Unit = unit
	}
	if opts.Style == gointl.StyleUnit && opts.Unit == "" {
		return opts, typeError("Unit is required with unit style.")
	}
	if opts.UnitDisplay, err = enumOption(o, "unitDisplay", gointl.UnitShort, gointl.UnitNarrow); err != nil {
		return opts, err
	}
	if opts.Notation, err = enumOption(o, "notation", gointl.NotationStandard, gointl.NotationEngineering); err != nil {
		return opts, err
	}
	if err = numberDigits(o, &opts); err != nil {
		return opts, err
	}
	if opts.CompactDisplay, err = enumOption(o, "compactDisplay", gointl.CompactShort, gointl.CompactLong); err != nil {
		return opts, err
	}
	switch v, ok := o["useGrouping"]; {
	case !ok:
	case v == false || v == "":
		opts.UseGrouping = gointl.GroupingNever
	case v == true:
		opts.UseGrouping = gointl.GroupingAlways
	case v == "true" || v == "false":
	default:
		if opts.UseGrouping, err = enumOption(o, "useGrouping", gointl.GroupingAuto, gointl.GroupingMin2); err != nil {
			return opts, err
		}
	}
	if opts.SignDisplay, err = enumOption(o, "signDisplay", gointl.SignAuto, gointl.SignNegative); err != nil {
		return opts, err
	}
	return opts, nil
}

// numberDigits validates only the counts SetNumberFormatDigitOptions uses.
// go-intl receives absent pointers for fraction counts ignored by auto priority.
func numberDigits(o map[string]any, opts *gointl.NumberFormatOptions) error {
	var err error
	opts.MinimumIntegerDigits, _, err = optInt(o, "minimumIntegerDigits", 1, 21)
	if err != nil {
		return err
	}
	opts.RoundingIncrement, _, err = optInt(o, "roundingIncrement", 1, 5000)
	if err != nil {
		return err
	}
	if opts.RoundingMode, err = enumOption(o, "roundingMode", gointl.HalfExpand, gointl.HalfEven); err != nil {
		return err
	}
	if opts.RoundingPriority, err = enumOption(o, "roundingPriority", gointl.PriorityAuto, gointl.LessPrecision); err != nil {
		return err
	}
	if opts.TrailingZeroDisplay, err = enumOption(o, "trailingZeroDisplay", gointl.TrailingZeroAuto, gointl.TrailingZeroStripIfInteger); err != nil {
		return err
	}
	for _, field := range []struct {
		name string
		out  **int
	}{
		{"minimumSignificantDigits", &opts.MinimumSignificantDigits}, {"maximumSignificantDigits", &opts.MaximumSignificantDigits},
	} {
		n, ok, err := optInt(o, field.name, 1, 21)
		if err != nil {
			return err
		}
		if ok {
			*field.out = &n
		}
	}
	hasSignificant := opts.MinimumSignificantDigits != nil || opts.MaximumSignificantDigits != nil
	if opts.RoundingIncrement > 1 && (hasSignificant || opts.RoundingPriority != gointl.PriorityAuto) {
		return typeError("Rounding increment requires fraction digits and auto priority")
	}
	if !hasSignificant || opts.RoundingPriority != gointl.PriorityAuto {
		for _, field := range []struct {
			name string
			out  **int
		}{
			{"minimumFractionDigits", &opts.MinimumFractionDigits}, {"maximumFractionDigits", &opts.MaximumFractionDigits},
		} {
			n, ok, err := optInt(o, field.name, 0, 100)
			if err != nil {
				return err
			}
			if ok {
				*field.out = &n
			}
		}
	}
	return nil
}

// bigintDecimal holds digits produced by JS BigInt conversion, whose
// magnitude stays exact even beyond the range allowed for numeric strings.
type bigintDecimal string

func (f *numberFormat) format(v any) []part {
	var pieces []gointl.Part
	switch v := v.(type) {
	case float64:
		pieces = f.f.FormatToParts(v)
	case int64:
		pieces = f.f.FormatDecimalToParts(gointl.ParseExactDecimal(strconv.FormatInt(v, 10)))
	case string:
		pieces = f.f.FormatDecimalToParts(gointl.ParseDecimal(v))
	case bigintDecimal:
		pieces = f.f.FormatDecimalToParts(gointl.ParseExactDecimal(string(v)))
	default:
		panic("intl: unexpected number bridge value")
	}
	return numberParts(pieces)
}

func (f *numberFormat) formatString(v any) string {
	switch v := v.(type) {
	case float64:
		return f.f.Format(v)
	case int64:
		return f.f.FormatDecimal(gointl.ParseExactDecimal(strconv.FormatInt(v, 10)))
	case string:
		return f.f.FormatDecimal(gointl.ParseDecimal(v))
	case bigintDecimal:
		return f.f.FormatDecimal(gointl.ParseExactDecimal(string(v)))
	default:
		panic("intl: unexpected number bridge value")
	}
}

func numberParts(pieces []gointl.Part) []part {
	parts := make([]part, len(pieces))
	for i, p := range pieces {
		parts[i] = part{Type: string(p.Kind), Value: p.Value}
	}
	return parts
}

func formatOptionError(err error) error {
	if errors.Is(err, gointl.ErrOption) {
		return rangeError("%s", err)
	}
	return err
}

func enumOption[T interface {
	~int
	String() string
}](o map[string]any, key string, first, last T) (T, error) {
	v, ok := o[key]
	if !ok {
		return 0, nil
	}
	for n := first; n <= last; n++ {
		if v == n.String() {
			return n, nil
		}
	}
	return 0, rangeError("value %v out of range for Intl options property %s", v, key)
}

func numberingSystemOption(o map[string]any) (string, error) {
	s, ok := o["numberingSystem"].(string)
	if !ok {
		return "", nil
	}
	if !isTypeSequence(s) {
		return "", rangeError("Invalid numberingSystem: %s", s)
	}
	return s, nil
}

func optDefault(o map[string]any, key, def string, allowed ...string) (string, error) {
	s, err := optString(o, key, allowed...)
	if err != nil || s == "" {
		return def, err
	}

	return s, nil
}

var sanctionedUnits = gointl.SanctionedUnits()
