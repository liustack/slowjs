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
