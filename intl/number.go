// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"strconv"

	"github.com/liustack/slowjs"
	gointl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/intlhook"
)

// Intl.NumberFormat on go-intl's NumberFormat.

// sanctionedUnits are the units ECMA-402 lets a NumberFormat take.
var sanctionedUnits = gointl.SanctionedUnits()

// A numberFormat is a resolved Intl.NumberFormat. Its json fields are its
// resolvedOptions, in their order, and what the registry tells formatters
// apart by.
type numberFormat struct {
	Locale          string `json:"locale"`
	NumberingSystem string `json:"numberingSystem"`
	Style           string `json:"style"`
	// Only a currency style has the currency fields, and only a unit style
	// the unit ones.
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
	// UseGrouping is false for no grouping, and else the name of the
	// grouping as a string.
	UseGrouping         any    `json:"useGrouping"`
	Notation            string `json:"notation"`
	CompactDisplay      string `json:"compactDisplay,omitempty"`
	SignDisplay         string `json:"signDisplay"`
	RoundingIncrement   int    `json:"roundingIncrement"`
	RoundingMode        string `json:"roundingMode"`
	RoundingPriority    string `json:"roundingPriority"`
	TrailingZeroDisplay string `json:"trailingZeroDisplay"`

	f *gointl.NumberFormat
}

// init has nothing to do: resolveNumberFormat made the go-intl formatter.
func (f *numberFormat) init() error { return nil }

// bigintDecimal is the decimal string of a BigInt, which is read exactly,
// however large, where a numeric string is rounded to the range of a
// Number first.
type bigintDecimal string

// resolveNumberFormat is the Intl.NumberFormat constructor once intl.js has
// read the options: the locale first, so that a bad tag is the error before
// a bad option, then the options in the order ECMA-402 reads them.
func resolveNumberFormat(req request) (*numberFormat, error) {
	loc, err := goResolve(req.vm, gointl.ServiceNumberFormat, req.Locales)
	if err != nil {
		return nil, err
	}

	opts, err := numberOptions(req.Options)
	if err != nil {
		return nil, err
	}

	f, err := gointl.NewNumberFormatFrom(goData, loc, opts)
	if err != nil {
		return nil, formatOptionError(err)
	}

	r := f.ResolvedOptions()
	nf := &numberFormat{
		Locale:                   r.Locale,
		NumberingSystem:          r.NumberingSystem,
		Style:                    r.Style.String(),
		MinimumIntegerDigits:     r.MinimumIntegerDigits,
		MinimumFractionDigits:    r.MinimumFractionDigits,
		MaximumFractionDigits:    r.MaximumFractionDigits,
		MinimumSignificantDigits: r.MinimumSignificantDigits,
		MaximumSignificantDigits: r.MaximumSignificantDigits,
		UseGrouping:              r.UseGrouping.String(),
		Notation:                 r.Notation.String(),
		SignDisplay:              r.SignDisplay.String(),
		RoundingIncrement:        r.RoundingIncrement,
		RoundingMode:             r.RoundingMode.String(),
		RoundingPriority:         r.RoundingPriority.String(),
		TrailingZeroDisplay:      r.TrailingZeroDisplay.String(),
		f:                        f,
	}
	switch r.Style {
	case gointl.StyleCurrency:
		nf.Currency, nf.CurrencyDisplay, nf.CurrencySign = r.Currency, r.CurrencyDisplay.String(), r.CurrencySign.String()
	case gointl.StyleUnit:
		nf.Unit, nf.UnitDisplay = r.Unit, r.UnitDisplay.String()
	}
	if r.UseGrouping == gointl.GroupingNever {
		nf.UseGrouping = false
	}
	if r.Notation == gointl.NotationCompact {
		nf.CompactDisplay = r.CompactDisplay.String()
	}
	return nf, nil
}

// numberOptions checks the options of a NumberFormat.
func numberOptions(o map[string]any) (gointl.NumberFormatOptions, error) {
	opts := gointl.NumberFormatOptions{Compat: goCompat}
	var err error
	if _, err = optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return opts, err
	}

	if s, ok := o["numberingSystem"].(string); ok {
		if !isTypeSequence(s) {
			return opts, rangeError("Invalid numberingSystem: %s", s)
		}
		opts.NumberingSystem = s
	}

	if opts.Style, err = enumOption(o, "style", gointl.StyleDecimal, gointl.StyleUnit); err != nil {
		return opts, err
	}

	if s, ok := o["currency"].(string); ok {
		if len(s) != 3 || !isAlpha(s) {
			return opts, rangeError("Invalid currency code: %s", s)
		}
		opts.Currency = s
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

	if s, ok := o["unit"].(string); ok {
		if !gointl.HasUnit(s) {
			return opts, rangeError("Invalid unit argument: %s", s)
		}
		opts.Unit = s
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

	if opts.UseGrouping, err = grouping(o); err != nil {
		return opts, err
	}

	opts.SignDisplay, err = enumOption(o, "signDisplay", gointl.SignAuto, gointl.SignNegative)
	return opts, err
}

// grouping is the useGrouping option, which intl.js passes as true, false
// or a string (GetBooleanOrStringNumberFormatOption): false and "" are no
// grouping, true is always, and the strings "true" and "false" leave the
// default.
func grouping(o map[string]any) (gointl.Grouping, error) {
	switch v, ok := o["useGrouping"]; {
	case !ok:
		return gointl.GroupingDefault, nil
	case v == false || v == "":
		return gointl.GroupingNever, nil
	case v == true:
		return gointl.GroupingAlways, nil
	case v == "true" || v == "false":
		return gointl.GroupingDefault, nil
	}
	return enumOption(o, "useGrouping", gointl.GroupingAuto, gointl.GroupingMin2)
}

// numberDigits checks the digit options of SetNumberFormatDigitOptions into
// opts. The defaults, which depend on the style, the currency and the
// notation, are go-intl's to settle from the options given: those not given
// stay nil. The fraction digits are not read next to significant digits that
// decide alone, so that a fraction digit option out of range there is no
// error.
func numberDigits(o map[string]any, opts *gointl.NumberFormatOptions) error {
	n, ok, err := optInt(o, "minimumIntegerDigits", 1, 21)
	if err != nil {
		return err
	}
	if ok {
		opts.MinimumIntegerDigits = n
	}

	if n, ok, err = optInt(o, "roundingIncrement", 1, 5000); err != nil {
		return err
	}
	if ok {
		opts.RoundingIncrement = n
	}

	if opts.RoundingMode, err = enumOption(o, "roundingMode", gointl.HalfExpand, gointl.HalfEven); err != nil {
		return err
	}

	if opts.RoundingPriority, err = enumOption(o, "roundingPriority", gointl.PriorityAuto, gointl.LessPrecision); err != nil {
		return err
	}

	if opts.TrailingZeroDisplay, err = enumOption(o, "trailingZeroDisplay", gointl.TrailingZeroAuto,
		gointl.TrailingZeroStripIfInteger); err != nil {
		return err
	}

	if opts.MinimumSignificantDigits, err = digitPointer(o, "minimumSignificantDigits", 1, 21); err != nil {
		return err
	}

	if opts.MaximumSignificantDigits, err = digitPointer(o, "maximumSignificantDigits", 1, 21); err != nil {
		return err
	}

	significant := opts.MinimumSignificantDigits != nil || opts.MaximumSignificantDigits != nil
	auto := opts.RoundingPriority == gointl.PriorityAuto
	if opts.RoundingIncrement > 1 && (significant || !auto) {
		return typeError("Rounding increment requires fraction digits and auto priority")
	}

	if significant && auto {
		return nil
	}

	if opts.MinimumFractionDigits, err = digitPointer(o, "minimumFractionDigits", 0, 100); err != nil {
		return err
	}

	opts.MaximumFractionDigits, err = digitPointer(o, "maximumFractionDigits", 0, 100)
	return err
}

// digitPointer is optInt as the pointer go-intl takes, nil when the option
// was not given.
func digitPointer(o map[string]any, key string, min, max int) (*int, error) {
	n, ok, err := optInt(o, key, min, max)
	if err != nil || !ok {
		return nil, err
	}
	return &n, nil
}

// Most bytes Go may take for each digit of a number it writes, for a format
// and for parts. Measured, the most were 112 and 323.
const (
	bytesPerDigit     = 144
	bytesPerPartDigit = 400
)

// reserve refuses d when f would write it with more than intlhook.MaxDigits
// digits, and else counts on door the room Go needs to write them.
func (f *numberFormat) reserve(door *intlhook.Door, d gointl.Decimal, parts bool) {
	n := f.f.Output(d)
	if n > intlhook.MaxDigits {
		intlhook.RefuseDigits()
	}
	per := int64(bytesPerDigit)
	if parts {
		per = bytesPerPartDigit
	}
	door.Reserve(n * per)
}

// admit makes v, a value of numberArg or a bigintDecimal, the value format
// takes: a string becomes the Decimal it reads as, checked for its length
// once written. A Number is short whatever its value and stays as it is.
func (f *numberFormat) admit(door *intlhook.Door, v any, parts bool) any {
	var d gointl.Decimal
	switch x := v.(type) {
	case string:
		d = gointl.ParseDecimal(x)
	case bigintDecimal:
		d = gointl.ParseExactDecimal(string(x))
	default:
		return v
	}
	f.reserve(door, d, parts)
	return d
}

// rangeEnd is argument i of a formatRange call as a Decimal: a Number as V8
// hands one to ICU, or a string, exact when argument i+1 says it is the
// digits of a BigInt.
func (f *numberFormat) rangeEnd(a slowjs.Args, door *intlhook.Door, i int) gointl.Decimal {
	if x, ok := a.Number(i); ok {
		return gointl.DecimalFromFloat(x)
	}

	s, _ := door.Short(i, intlhook.KindNumber)
	var d gointl.Decimal
	if exact, _ := a.Bool(i + 1); exact {
		d = gointl.ParseExactDecimal(s)
	} else {
		d = gointl.ParseDecimal(s)
	}
	f.reserve(door, d, true)
	return d
}

// format writes v, a float64, an int64 or a Decimal from admit, as parts.
func (f *numberFormat) format(v any) []part {
	var ps []gointl.Part
	switch x := v.(type) {
	case float64:
		ps = f.f.FormatToParts(x)
	case int64:
		ps = f.f.FormatDecimalToParts(intDecimal(x))
	case gointl.Decimal:
		ps = f.f.FormatDecimalToParts(x)
	default:
		panic("intl: a NumberFormat was given a value of the wrong type")
	}

	r := make([]part, len(ps))
	for i, p := range ps {
		r[i] = part{Type: string(p.Kind), Value: p.Value}
	}
	return r
}

// formatString is format, joined.
func (f *numberFormat) formatString(v any) string {
	switch x := v.(type) {
	case float64:
		return f.f.Format(x)
	case int64:
		return f.f.FormatDecimal(intDecimal(x))
	case gointl.Decimal:
		return f.f.FormatDecimal(x)
	}
	panic("intl: a NumberFormat was given a value of the wrong type")
}

// intDecimal is n as the exact Decimal of its digits.
func intDecimal(n int64) gointl.Decimal {
	var b [20]byte
	return gointl.ParseExactDecimal(string(strconv.AppendInt(b[:0], n, 10)))
}

// A part is one piece of the output of formatToParts.
type part struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
