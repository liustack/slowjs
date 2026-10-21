// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"testing"
)

// TestLegacyConstructor calls Intl.NumberFormat and Intl.DateTimeFormat as
// functions on an object made from their prototype, which ECMA-402's
// normative optional legacy mode (ChainNumberFormat, UnwrapNumberFormat)
// turns into a formatter through a symbol property. The expected values are
// Node 24's.
func TestLegacyConstructor(t *testing.T) {
	vm := newVM(t)
	catch := func(src string) string {
		return fmt.Sprintf("(() => {try {return String(%s)} catch (e) {return e.name}})()", src)
	}
	for _, test := range [][2]string{
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); return Intl.NumberFormat.call(o, 'de') === o })()`, "true"},
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); Intl.NumberFormat.call(o); return Object.getOwnPropertySymbols(o).map((s) => s.description).join() })()`, "IntlLegacyConstructedSymbol"},
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); Intl.NumberFormat.call(o, 'de'); return Intl.NumberFormat.prototype.resolvedOptions.call(o).locale + ' ' + o.format(1234.5) })()`, "de 1.234,5"},
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); Intl.NumberFormat.call(o); return Intl.NumberFormat.prototype.formatToParts.call(o, 1) })()`, "TypeError"},
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); Intl.NumberFormat.call(o); const d = Object.getOwnPropertyDescriptor(o, Object.getOwnPropertySymbols(o)[0]); return [d.writable, d.enumerable, d.configurable, d.value instanceof Intl.NumberFormat].join() })()`, "false,false,false,true"},
		{`(() => { const o = {}; const r = Intl.NumberFormat.call(o); return (r !== o) + ' ' + (r instanceof Intl.NumberFormat) + ' ' + Object.getOwnPropertySymbols(o).length })()`, "true true 0"},
		{`(() => { const o = new Intl.NumberFormat('en'); return Intl.NumberFormat.call(o, 'de') === o && o.format(1234.5) })()`, "1,234.5"},
		{`(() => { const o = Object.create(Intl.DateTimeFormat.prototype); Intl.DateTimeFormat.call(o, 'de', {timeZone: 'UTC'}); return o.format(0) + ' ' + Intl.DateTimeFormat.prototype.resolvedOptions.call(o).locale })()`, "1.1.1970 de"},
		{`(() => { const o = Object.create(Intl.DateTimeFormat.prototype); Intl.DateTimeFormat.call(o); return Intl.DateTimeFormat.prototype.formatToParts.call(o, 0) })()`, "TypeError"},
		{`(() => { const o = Object.create(Intl.PluralRules.prototype); return Intl.PluralRules.call(o) })()`, "TypeError"},
		{`(() => { const o = Object.create(Intl.NumberFormat.prototype); Intl.NumberFormat.call(o); const p = new Proxy(o, {}); return Intl.NumberFormat.prototype.resolvedOptions.call(p).locale })()`, "en-US"},
		{`Intl.NumberFormat.call(undefined) instanceof Intl.NumberFormat`, "true"},
	} {
		if got := fmt.Sprint(eval(t, vm, catch(test[0]))); got != test[1] {
			t.Errorf("%s\n\tgot  %s\n\twant %s", test[0], got, test[1])
		}
	}
}
