// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"
	"strconv"
	"strings"
)

// numberFormat is a resolved Intl.NumberFormat. Its json form is both what
// resolvedOptions() reports and the state handed back to the host on every
// format call, so the field order is the order ECMA-402 lists them in. Keys
// starting with an underscore are internal and the shim strips them before
// handing the object to javascript.
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
	MinimumSignificantDigits *int   `json:"minimumSignificantDigits,omitempty"`
	MaximumSignificantDigits *int   `json:"maximumSignificantDigits,omitempty"`
	MinimumFractionDigits    *int   `json:"minimumFractionDigits,omitempty"`
	MaximumFractionDigits    *int   `json:"maximumFractionDigits,omitempty"`
	UseGrouping              any    `json:"useGrouping"`
	Notation                 string `json:"notation"`
	CompactDisplay           string `json:"compactDisplay,omitempty"`
	SignDisplay              string `json:"signDisplay"`
	RoundingMode             string `json:"roundingMode"`
	RoundingIncrement        int    `json:"roundingIncrement"`
	TrailingZeroDisplay      string `json:"trailingZeroDisplay"`
	RoundingPriority         string `json:"roundingPriority"`

	// CompactRound records that no digit option was given for a compact
	// formatter, which selects compact notation's own rounding: an integer,
	// but never fewer than two significant digits, so 1234 is 1.2K and 12345
	// is 12K.
	CompactRound bool `json:"_compactRound,omitempty"`
}

func resolveNumberFormat(req request) (*numberFormat, error) {
	locale, err := resolveLocale(req.Locales)
	if err != nil {
		return nil, err
	}

	o := req.Options
	if o == nil {
		o = map[string]any{}
	}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	f := &numberFormat{Locale: locale, NumberingSystem: "latn", Style: "decimal"}
	if f.Style, err = optDefault(o, "style", "decimal", "decimal", "percent", "currency", "unit"); err != nil {
		return nil, err
	}

	if f.Notation, err = optDefault(o, "notation", "standard", "standard", "scientific", "engineering", "compact"); err != nil {
		return nil, err
	}

	if f.SignDisplay, err = optDefault(o, "signDisplay", "auto", "auto", "never", "always", "exceptZero", "negative"); err != nil {
		return nil, err
	}

	if f.RoundingMode, err = optDefault(o, "roundingMode", "halfExpand", "ceil", "floor", "expand", "trunc",
		"halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"); err != nil {
		return nil, err
	}

	if f.TrailingZeroDisplay, err = optDefault(o, "trailingZeroDisplay", "auto", "auto", "stripIfInteger"); err != nil {
		return nil, err
	}

	// roundingIncrement and roundingPriority are reported at their defaults,
	// which is what this implementation does. A request for anything else is
	// ignored rather than rejected - see the package documentation.
	f.RoundingIncrement, f.RoundingPriority = 1, "auto"

	if f.Notation == "compact" {
		if f.CompactDisplay, err = optDefault(o, "compactDisplay", "short", "short", "long"); err != nil {
			return nil, err
		}
	}

	currencyDigits := 2
	switch f.Style {
	case "currency":
		s, ok := o["currency"].(string)
		if !ok {
			return nil, typeError("currency must be provided when style is 'currency'")
		}

		if len(s) != 3 || !isAlpha(s) {
			return nil, rangeError("invalid currency code: %s", s)
		}

		f.Currency = strings.ToUpper(s)
		currencyDigits = currencyMinorUnits(f.Currency)
		if f.CurrencyDisplay, err = optDefault(o, "currencyDisplay", "symbol", "code", "symbol", "narrowSymbol", "name"); err != nil {
			return nil, err
		}

		if f.CurrencySign, err = optDefault(o, "currencySign", "standard", "standard", "accounting"); err != nil {
			return nil, err
		}
	case "unit":
		s, ok := o["unit"].(string)
		if !ok {
			return nil, typeError("unit must be provided when style is 'unit'")
		}

		f.Unit = s
		if f.UnitDisplay, err = optDefault(o, "unitDisplay", "short", "short", "narrow", "long"); err != nil {
			return nil, err
		}
	}

	if err := f.digitOptions(o, currencyDigits); err != nil {
		return nil, err
	}

	if f.UseGrouping, err = groupingOption(o, f.Notation); err != nil {
		return nil, err
	}

	return f, nil
}

// digitOptions implements ECMA-402's SetNumberFormatDigitOptions.
func (f *numberFormat) digitOptions(o map[string]any, currencyDigits int) error {
	mnid, ok, err := optInt(o, "minimumIntegerDigits", 1, 21)
	if err != nil {
		return err
	}

	f.MinimumIntegerDigits = 1
	if ok {
		f.MinimumIntegerDigits = mnid
	}

	mnfd, hasMnfd, err := optInt(o, "minimumFractionDigits", 0, 100)
	if err != nil {
		return err
	}

	mxfd, hasMxfd, err := optInt(o, "maximumFractionDigits", 0, 100)
	if err != nil {
		return err
	}

	mnsd, hasMnsd, err := optInt(o, "minimumSignificantDigits", 1, 21)
	if err != nil {
		return err
	}

	mxsd, hasMxsd, err := optInt(o, "maximumSignificantDigits", 1, 21)
	if err != nil {
		return err
	}

	switch {
	case hasMnsd || hasMxsd:
		if !hasMnsd {
			mnsd = 1
		}
		if !hasMxsd {
			mxsd = 21
		}
		if mnsd > mxsd {
			return rangeError("minimumSignificantDigits is greater than maximumSignificantDigits")
		}

		f.MinimumSignificantDigits, f.MaximumSignificantDigits = &mnsd, &mxsd
	case hasMnfd || hasMxfd || f.Notation != "compact":
		mnfdDefault, mxfdDefault := 0, 3
		switch f.Style {
		case "currency":
			mnfdDefault, mxfdDefault = currencyDigits, currencyDigits
		case "percent":
			mxfdDefault = 0
		}
		if !hasMnfd {
			mnfd = mnfdDefault
		}
		if !hasMxfd {
			mxfd = mxfdDefault
			if mxfd < mnfd {
				mxfd = mnfd
			}
		}
		if mnfd > mxfd {
			return rangeError("minimumFractionDigits is greater than maximumFractionDigits")
		}

		f.MinimumFractionDigits, f.MaximumFractionDigits = &mnfd, &mxfd
	default:
		// Compact notation's own rounder, which ICU reports as one to two
		// significant digits and applies as "an integer, but never fewer than
		// two significant digits" - see compactRound.
		zero, one, two := 0, 1, 2
		f.MinimumFractionDigits, f.MaximumFractionDigits = &zero, &zero
		f.MinimumSignificantDigits, f.MaximumSignificantDigits = &one, &two
		f.RoundingPriority, f.CompactRound = "morePrecision", true
	}
	return nil
}

// groupingOption implements ECMA-402's GetBooleanOrStringNumberFormatOption for
// useGrouping, including its quirk of reading the strings "true" and "false" as
// the boolean false.
func groupingOption(o map[string]any, notation string) (any, error) {
	def := "auto"
	if notation == "compact" {
		def = "min2"
	}
	v, ok := o["useGrouping"]
	if !ok {
		return def, nil
	}

	switch x := v.(type) {
	case bool:
		if x {
			return "always", nil
		}

		return false, nil
	case string:
		switch x {
		case "min2", "auto", "always":
			return x, nil
		case "", "true", "false":
			return false, nil
		}
	case float64:
		if x == 0 {
			return false, nil
		}
	}
	return nil, rangeError("value %v out of range for Intl.NumberFormat options property useGrouping", v)
}

// optDefault is optString with a default for the absent option.
func optDefault(o map[string]any, key, def string, allowed ...string) (string, error) {
	s, err := optString(o, key, allowed...)
	if err != nil || s == "" {
		return def, err
	}

	return s, nil
}

// format renders v, which is whatever the shim's Number() coercion produced, as
// ECMA-402 parts. Joining their values yields the format() result.
func (f *numberFormat) format(v any) ([]part, error) {
	d, special := numberValue(v)
	neg := d.neg || special == "-Infinity"
	if special != "" {
		typ, value := "nan", "NaN"
		if special != "NaN" {
			typ, value = "infinity", "∞"
		}
		return f.wrap([]part{{typ, value}}, neg, special == "NaN"), nil
	}

	if f.Style == "percent" {
		d = d.scale(2)
	}

	var exponent, compact int
	switch f.Notation {
	case "scientific":
		exponent = d.magnitude()
		d = d.scale(-exponent)
	case "engineering":
		exponent = d.magnitude()
		exponent -= ((exponent % 3) + 3) % 3
		d = d.scale(-exponent)
	case "compact":
		compact, d = compactScale(d, 0)
	}

	minFrac := 0
	switch {
	case f.CompactRound:
		d = compactRound(d, f.RoundingMode)
		if compact != 0 && d.intDigits() > 3 {
			// 999999 rounds up to 1000K, which is 1M.
			compact, d = compactScale(d.scale(compact), compact+3)
			d = compactRound(d, f.RoundingMode)
		}
	case f.MaximumSignificantDigits != nil:
		d = d.roundTo(*f.MaximumSignificantDigits, f.RoundingMode)
		// Zero has no significant digit of its own, but the "0" written for it
		// counts as the first one, so minimumSignificantDigits 3 is "0.00".
		digits := d.exp
		if d.isZero() {
			digits = 1
		}
		if minFrac = *f.MinimumSignificantDigits - digits; minFrac < 0 {
			minFrac = 0
		}
	default:
		d = d.roundFrac(*f.MaximumFractionDigits, f.RoundingMode)
		minFrac = *f.MinimumFractionDigits
	}
	if f.TrailingZeroDisplay == "stripIfInteger" && !d.hasFraction() {
		minFrac = 0
	}
	// Rounding can carry the mantissa out of its notation's range: 999999 in
	// scientific notation rounds to 10E5, which is 1E6.
	switch mantissa := d.intDigits(); f.Notation {
	case "scientific":
		if mantissa > 1 {
			d, exponent = d.scale(1-mantissa), exponent+mantissa-1
		}
	case "engineering":
		if mantissa > 3 {
			d, exponent = d.scale(-3), exponent+3
		}
	}
	// The sign follows the rounded value, not the original: -1e-7 written with
	// three fraction digits is zero, and signDisplay "exceptZero" leaves it
	// unsigned.
	zero := d.isZero()

	ip, fp := d.render(f.MinimumIntegerDigits, minFrac)
	var r []part
	for i, g := range groupIntegerDigits(ip, f.UseGrouping) {
		if i != 0 {
			r = append(r, part{"group", ","})
		}
		r = append(r, part{"integer", g})
	}
	if fp != "" {
		r = append(r, part{"decimal", "."}, part{"fraction", fp})
	}
	switch f.Notation {
	case "scientific", "engineering":
		r = append(r, part{"exponentSeparator", "E"})
		if exponent < 0 {
			r = append(r, part{"exponentMinusSign", "-"})
			exponent = -exponent
		}
		r = append(r, part{"exponentInteger", strconv.Itoa(exponent)})
	case "compact":
		if s := compactSuffix(compact, f.CompactDisplay); s != "" {
			if f.CompactDisplay == "long" {
				r = append(r, part{"literal", " "})
			}
			r = append(r, part{"compact", s})
		}
	}
	return f.wrap(r, neg, zero), nil
}

// wrap adds the sign, the currency symbol and the percent or unit marker around
// the digits in num.
func (f *numberFormat) wrap(num []part, neg, zero bool) (r []part) {
	sign, signType := "", "minusSign"
	switch f.SignDisplay {
	case "never":
		// nop
	case "always":
		if sign, signType = "+", "plusSign"; neg {
			sign, signType = "-", "minusSign"
		}
	case "exceptZero":
		if !zero {
			if sign, signType = "+", "plusSign"; neg {
				sign, signType = "-", "minusSign"
			}
		}
	case "negative":
		if neg && !zero {
			sign = "-"
		}
	default: // auto
		if neg {
			sign = "-"
		}
	}
	accounting := f.Style == "currency" && f.CurrencySign == "accounting" && sign == "-"
	if accounting {
		sign = ""
		r = append(r, part{"literal", "("})
	}
	if sign != "" {
		r = append(r, part{signType, sign})
	}
	currency, currencyLast := "", false
	if f.Style == "currency" {
		if currency, currencyLast = currencyLabel(f.Currency, f.CurrencyDisplay); !currencyLast {
			r = append(r, part{"currency", currency})
			if currencySpace(currency, num[0].Value) {
				r = append(r, part{"literal", "\u00a0"})
			}
		}
	}
	r = append(r, num...)
	switch f.Style {
	case "percent":
		r = append(r, part{"percentSign", "%"})
	case "unit":
		label, space := unitLabel(f.Unit, f.UnitDisplay, isOne(num))
		if space {
			r = append(r, part{"literal", " "})
		}
		r = append(r, part{"unit", label})
	}
	if currencyLast {
		r = append(r, part{"literal", " "}, part{"currency", currency})
	}
	if accounting {
		r = append(r, part{"literal", ")"})
	}
	return r
}

// isOne reports whether the digits in num read exactly "1", the only case where
// English uses a singular unit name.
func isOne(num []part) bool {
	return len(num) == 1 && num[0].Type == "integer" && num[0].Value == "1"
}

// numberValue converts a value arriving from javascript to a dec. QuickJS tags
// integers that fit as such, so those never take the float64 detour.
func numberValue(v any) (dec, string) {
	var f float64
	switch x := v.(type) {
	case int:
		return decFromInt(int64(x)), ""
	case int64:
		return decFromInt(x), ""
	case float64:
		f = x
	case string:
		// A string arrives from a BigInt or from format("123.45"). Parsing its
		// digits keeps it exact, which a detour through a float64 would not do
		// for a BigInt of more than 17 of them.
		d, ok := decFromDecimalString(strings.TrimSpace(x))
		if !ok {
			return dec{}, "NaN"
		}

		return d, ""
	case bool:
		if x {
			return decFromInt(1), ""
		}

		return dec{}, ""
	case nil:
		return dec{}, ""
	default:
		return dec{}, "NaN"
	}
	switch {
	case math.IsNaN(f):
		return dec{}, "NaN"
	case math.IsInf(f, 1):
		return dec{}, "Infinity"
	case math.IsInf(f, -1):
		return dec{}, "-Infinity"
	}
	return decFromFloat(f), ""
}

// compactScale divides d by the largest power of a thousand not greater than
// it, at least min, and returns that power's exponent along with the result.
func compactScale(d dec, min int) (int, dec) {
	m := d.magnitude() / 3 * 3
	switch {
	case d.magnitude() < 3 || m < min:
		m = min
	case m > 12:
		m = 12
	}
	return m, d.scale(-m)
}

// compactRound rounds to an integer but keeps two significant digits, which is
// what compact notation does when no digit option overrides it.
func compactRound(d dec, mode string) dec {
	return d.roundTo(max(d.exp, 2), mode)
}

func compactSuffix(magnitude int, display string) string {
	i := magnitude/3 - 1
	if i < 0 || i > 3 {
		return ""
	}

	if display == "long" {
		return [...]string{"thousand", "million", "billion", "trillion"}[i]
	}

	return [...]string{"K", "M", "B", "T"}[i]
}

// groupIntegerDigits splits the integer digits into the groups en-US separates
// with a comma. The grouping mode is ECMA-402's resolved useGrouping value.
func groupIntegerDigits(ip string, useGrouping any) []string {
	n := len(ip)
	switch useGrouping {
	case false:
		return []string{ip}
	case "min2":
		if n < 5 {
			return []string{ip}
		}
	default:
		if n < 4 {
			return []string{ip}
		}
	}
	var r []string
	if k := n % 3; k != 0 {
		r = append(r, ip[:k])
		ip = ip[k:]
	}
	for ; ip != ""; ip = ip[3:] {
		r = append(r, ip[:3])
	}
	return r
}
