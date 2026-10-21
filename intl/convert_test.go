// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"testing"
)

// TestConversions checks that options, values and locale lists are converted
// with ToString, ToNumber and ToLength, which throw for a Symbol or a BigInt
// where String() and Number() do not. The expected values are Node 24's.
func TestConversions(t *testing.T) {
	vm := newVM(t)
	catch := func(src string) string {
		return fmt.Sprintf("(() => {try {return String(%s)} catch (e) {return e.name}})()", src)
	}
	for _, test := range [][2]string{
		{`new Intl.NumberFormat('en', {roundingMode: Symbol()})`, "TypeError"},
		{`new Intl.NumberFormat('en', {minimumFractionDigits: 1n})`, "TypeError"},
		{`new Intl.DateTimeFormat('en', {timeZone: Symbol()})`, "TypeError"},
		{`new Intl.DateTimeFormat('en', {fractionalSecondDigits: 1n})`, "TypeError"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'UTC'}).format(0n)`, "TypeError"},
		{`new Intl.PluralRules('en').select(1n)`, "TypeError"},
		{`new Intl.PluralRules('en').selectRange(1n, 2)`, "TypeError"},
		{`Intl.getCanonicalLocales({length: -Math.pow(2, 32) + 1, get 0() { throw new Error('read') }}).length`, "0"},
		{`Intl.getCanonicalLocales({length: 1.7, 0: 'de'}).join()`, "de"},
		{`Intl.getCanonicalLocales({length: '2', 0: 'de', 1: 'en'}).join()`, "de,en"},
		{`Intl.getCanonicalLocales({length: 1n, 0: 'de'}).join()`, "TypeError"},
		{`new Intl.NumberFormat('en', {useGrouping: Symbol()})`, "TypeError"},
		{`new Intl.NumberFormat('en', {useGrouping: null}).resolvedOptions().useGrouping`, "false"},
		{`new Intl.NumberFormat('en', {useGrouping: 0}).resolvedOptions().useGrouping`, "false"},
		{`new Intl.NumberFormat('en', {useGrouping: ''}).resolvedOptions().useGrouping`, "false"},
		{`new Intl.NumberFormat('en', {useGrouping: true}).resolvedOptions().useGrouping`, "always"},
		{`new Intl.NumberFormat('en', {useGrouping: 'min2'}).resolvedOptions().useGrouping`, "min2"},
		{`new Intl.NumberFormat('en', {style: {toString() { return 'percent' }}}).format(0.5)`, "50%"},
		{`new Intl.NumberFormat('en').format({valueOf() { return 3 }})`, "3"},
	} {
		src, want := catch(test[0]), test[1]
		if got := fmt.Sprint(eval(t, vm, src)); got != want {
			t.Errorf("%s\n\tgot  %s\n\twant %s", test[0], got, want)
		}
	}
}

// TestOptionOrder checks the order the constructors read their options in,
// and the rounding options they check. The expected values are Node 24's,
// but for PluralRules' order, which is test262's: Node 24 has no notation
// for PluralRules and reads neither it nor compactDisplay.
func TestOptionOrder(t *testing.T) {
	vm := newVM(t)
	reads := func(c string) string {
		return `(() => { const r = []; new Intl.` + c + `('en', new Proxy({}, {get(o, k) { r.push(k) }})); return r.join(' ') })()`
	}
	catch := func(src string) string {
		return fmt.Sprintf("(() => {try {return String(%s)} catch (e) {return e.name}})()", src)
	}
	evalTests(t, vm, [][2]string{
		{reads("NumberFormat"), "localeMatcher numberingSystem style currency currencyDisplay currencySign unit unitDisplay notation " +
			"minimumIntegerDigits minimumFractionDigits maximumFractionDigits minimumSignificantDigits maximumSignificantDigits " +
			"roundingIncrement roundingMode roundingPriority trailingZeroDisplay compactDisplay useGrouping signDisplay"},
		{reads("PluralRules"), "localeMatcher type notation compactDisplay minimumIntegerDigits minimumFractionDigits maximumFractionDigits " +
			"minimumSignificantDigits maximumSignificantDigits roundingIncrement roundingMode roundingPriority trailingZeroDisplay"},
		{reads("DateTimeFormat"), "localeMatcher calendar numberingSystem hour12 hourCycle timeZone weekday era year month day dayPeriod " +
			"hour minute second fractionalSecondDigits timeZoneName formatMatcher dateStyle timeStyle"},
		{catch(`new Intl.NumberFormat('en', {roundingPriority: 'x'})`), "RangeError"},
		{catch(`new Intl.PluralRules('en', {roundingPriority: 'x'})`), "RangeError"},
		{catch(`new Intl.NumberFormat('en', {compactDisplay: 'x'})`), "RangeError"},
		{catch(`new Intl.NumberFormat('en', {roundingIncrement: 0})`), "RangeError"},
		{catch(`new Intl.NumberFormat('en', {roundingIncrement: 3})`), "RangeError"},
		{catch(`new Intl.PluralRules('en', {roundingIncrement: 5001})`), "RangeError"},
		{catch(`new Intl.NumberFormat('en', {roundingIncrement: 'abc'})`), "RangeError"},
		{catch(`new Intl.NumberFormat('en', {roundingPriority: 'morePrecision'}).format(1)`), "1"},
	})
}
