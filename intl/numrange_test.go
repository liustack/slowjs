// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"testing"
)

// TestNumberFormatRange checks formatRange and formatRangeToParts of
// Intl.NumberFormat: the shared parts, the approximately sign, the source of
// each part, the kinds of value they take and what they throw. The answers
// are Node 24's and Chrome 157's, which agree on them. The golden set
// numrange compares many more with Chrome.
func TestNumberFormatRange(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var eur = new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR', maximumFractionDigits: 0});
var en = new Intl.NumberFormat('en');
function err(f) { try { f(); return 'no error' } catch (e) { return e.constructor.name } }
function parts(a) { return a.map(p => p.type + ':' + p.value + ':' + p.source).join('|') }
0`)
	evalTests(t, vm, [][2]string{
		{`typeof Intl.NumberFormat.prototype.formatRange + typeof Intl.NumberFormat.prototype.formatRangeToParts`, "functionfunction"},
		{`Intl.NumberFormat.prototype.formatRange.length + ',' + Intl.NumberFormat.prototype.formatRangeToParts.length`, "2,2"},
		{`eur.formatRange(80, 120)`, "€80 – €120"},
		{`parts(eur.formatRangeToParts(80, 120))`, "currency:€:startRange|integer:80:startRange|literal: – :shared|currency:€:endRange|integer:120:endRange"},
		{`eur.formatRange(3, 3) + ' ' + eur.formatRange(2.9, 3.1)`, "~€3 ~€3"},
		{`parts(en.formatRangeToParts(3, 3))`, "approximatelySign:~:shared|integer:3:shared"},
		{`new Intl.NumberFormat('de-DE', {style: 'currency', currency: 'EUR'}).formatRange(5, 10)`, "5,00–10,00\u00a0€"},
		{`new Intl.NumberFormat('en', {notation: 'compact'}).formatRange(1000, 5000)`, "1K – 5K"},
		{`new Intl.NumberFormat('en', {style: 'unit', unit: 'kilometer', unitDisplay: 'long'}).formatRange(1, 2)`, "1–2 kilometers"},
		{`en.formatRange(10n ** 20n, '1e21')`, "100,000,000,000,000,000,000–1,000,000,000,000,000,000,000"},
		{`en.formatRange(5, 3)`, "5–3"},
		{`en.formatRange({valueOf() { return 1 }}, Object(2n))`, "1–2"},
		{`err(() => en.formatRange(NaN, 1)) + err(() => en.formatRange(1, 'abc')) + err(() => en.formatRangeToParts(NaN, NaN))`, "RangeErrorRangeErrorRangeError"},
		{`err(() => en.formatRange(undefined, 1)) + err(() => en.formatRange(1)) + err(() => en.formatRangeToParts())`, "TypeErrorTypeErrorTypeError"},
		{`err(() => en.formatRange(Symbol(), 1)) + err(() => en.formatRange.call({}, 1, 2))`, "TypeErrorTypeError"},
		// Both ends are converted, start first, before NaN is refused.
		{`var seen = []; err(() => en.formatRange({valueOf() { seen.push('x'); return NaN }}, {valueOf() { seen.push('y'); return 1 }})) + seen`, "RangeErrorx,y"},
	})
}

// numrangeExprs is the golden set numrange: formatRange and
// formatRangeToParts of NumberFormat in many locales, styles and notations,
// over ends that share their parts, that round alike and that differ, and
// the values and errors of the spec's conversions.
func numrangeExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }
	options := []string{
		`{}`, `{maximumFractionDigits:0}`, `{minimumFractionDigits:2}`, `{maximumSignificantDigits:2}`,
		`{style:'percent'}`, `{style:'currency',currency:'USD'}`, `{style:'currency',currency:'EUR'}`,
		`{style:'currency',currency:'EUR',maximumFractionDigits:0}`, `{style:'currency',currency:'JPY'}`,
		`{style:'currency',currency:'CHF'}`, `{style:'currency',currency:'INR'}`,
		`{style:'currency',currency:'GBP',currencyDisplay:'code'}`, `{style:'currency',currency:'EUR',currencyDisplay:'name'}`,
		`{style:'currency',currency:'USD',currencySign:'accounting'}`,
		`{style:'unit',unit:'kilometer'}`, `{style:'unit',unit:'kilometer',unitDisplay:'long'}`,
		`{style:'unit',unit:'celsius'}`, `{style:'unit',unit:'day',unitDisplay:'long'}`,
		`{style:'unit',unit:'hour',unitDisplay:'narrow'}`, `{style:'unit',unit:'percent'}`,
		`{style:'unit',unit:'kilometer-per-hour',unitDisplay:'long'}`,
		`{notation:'compact'}`, `{notation:'compact',compactDisplay:'long'}`,
		`{notation:'compact',style:'currency',currency:'USD'}`,
		`{notation:'scientific'}`, `{notation:'engineering'}`,
		`{signDisplay:'always'}`, `{signDisplay:'exceptZero'}`, `{useGrouping:false}`, `{numberingSystem:'arab'}`,
	}
	pairs := [][2]string{
		{"1", "2"}, {"80", "120"}, {"3", "3"}, {"2.9", "3.1"}, {"-5", "5"}, {"-5", "-3"}, {"0", "1000000"},
		{"1000", "5000"}, {"1200", "1300"}, {"0.5", "0.75"}, {"5", "3"}, {"1", "1"},
	}
	for _, l := range goldenLocales {
		for _, o := range options {
			for _, p := range pairs {
				add(`new Intl.NumberFormat(%q,%s).formatRange(%s,%s)`, l, o, p[0], p[1])
			}
			add(`JSON.stringify(new Intl.NumberFormat(%q,%s).formatRangeToParts(1200,3400.5))`, l, o)
			add(`JSON.stringify(new Intl.NumberFormat(%q,%s).formatRangeToParts(-2,-2))`, l, o)
		}
	}
	// Every language Chromium has number data for.
	for _, l := range chromiumLanguages("NumberFormat") {
		for _, o := range []string{`{}`, `{style:'currency',currency:'EUR'}`, `{style:'unit',unit:'kilometer',unitDisplay:'long'}`, `{notation:'compact'}`} {
			add(`new Intl.NumberFormat(%q,%s).formatRange(3,5)`, l, o)
			add(`new Intl.NumberFormat(%q,%s).formatRange(1500,1500)`, l, o)
			add(`JSON.stringify(new Intl.NumberFormat(%q,%s).formatRangeToParts(1,2000))`, l, o)
		}
	}
	// The values the spec converts, the rounding and what throws.
	for _, o := range []string{`{}`, `{maximumFractionDigits:0}`, `{roundingIncrement:5,maximumFractionDigits:0}`,
		`{roundingMode:'floor',maximumFractionDigits:0}`, `{notation:'compact',compactDisplay:'long'}`, `{style:'currency',currency:'USD'}`} {
		for _, p := range [][2]string{
			{"1n", "2n"}, {"10n**30n", "10n**30n+1n"}, {"'1.5'", "'2.5'"}, {"'1e3'", "'0x10'"}, {"' 12 '", "''"},
			{"1", "Infinity"}, {"-Infinity", "Infinity"}, {"Infinity", "Infinity"}, {"0", "-0"}, {"-0", "-0"},
			{"1", "1.0000000001"}, {"0.1+0.2", "0.3"}, {"'0.30000000000000004'", "0.3"}, {"1n", "1"}, {"'1'", "1"},
			{"9007199254740993n", "9007199254740992"}, {"'9007199254740993'", "9007199254740993n"},
			{"1e21", "1e22"}, {"5e-7", "6e-7"}, {"true", "null"}, {"[7]", "{valueOf(){return 8}}"},
			{"NaN", "1"}, {"1", "NaN"}, {"'abc'", "1"}, {"undefined", "1"}, {"1", "undefined"}, {"Symbol()", "1"}, {"1", "{}"},
		} {
			add(`new Intl.NumberFormat('en',%s).formatRange(%s,%s)`, o, p[0], p[1])
			add(`JSON.stringify(new Intl.NumberFormat('en',%s).formatRangeToParts(%s,%s))`, o, p[0], p[1])
		}
	}
	add(`new Intl.NumberFormat('en').formatRange(1)`)
	add(`new Intl.NumberFormat('en').formatRangeToParts()`)
	add(`Intl.NumberFormat.prototype.formatRange.call({},1,2)`)
	add(`Intl.NumberFormat.prototype.formatRange.call(new Intl.DateTimeFormat('en'),1,2)`)
	add(`Intl.NumberFormat('de',{style:'currency',currency:'EUR'}).formatRange(1,2)`)
	return r
}
