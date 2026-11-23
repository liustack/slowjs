// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import "testing"

// TestArrayToLocaleStringArguments checks what Array.prototype.toLocaleString
// and %TypedArray%.prototype.toLocaleString pass to the toLocaleString of
// each element: nothing on a VM without Intl, as ECMA-262 says, and once
// package intl gave the VM ECMA-402 (jsSetLocaleArgs, which intl.Register
// calls through vmhook.LocaleArgs), their locales and options, always two
// arguments. An element that is null or undefined is never called. Package
// intl tests what Intl makes of the arguments. The values with ECMA-402 are
// Node 24's.
func TestArrayToLocaleStringArguments(t *testing.T) {
	for _, ecma402 := range []bool{false, true} {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		defer m.Close()

		if ecma402 {
			m.jsSetLocaleArgs()
		}
		evalString(t, m, `
			var show = (a) => a.length + ':' + a.map((x) => typeof x === 'object' ? JSON.stringify(x) : String(x)).join('|')
			var seen = (tag) => ({toLocaleString(...a) { return tag + '(' + show(a) + ')' }})
			var numberTLS = Number.prototype.toLocaleString
			var bigintTLS = BigInt.prototype.toLocaleString
			var patched = (f) => {
				Number.prototype.toLocaleString = function () { return 'n' + this + '(' + show([...arguments]) + ')' }
				BigInt.prototype.toLocaleString = function () { return 'b' + this + '(' + show([...arguments]) + ')' }
				try { return f() } finally {
					Number.prototype.toLocaleString = numberTLS
					BigInt.prototype.toLocaleString = bigintTLS
				}
			}
			0`)
		// javascript, its value with ECMA-402 and its value without
		for _, test := range [][3]string{
			{`[seen('a')].toLocaleString()`, "a(2:undefined|undefined)", "a(0:)"},
			{`[seen('a'), seen('b')].toLocaleString('de')`, "a(2:de|undefined),b(2:de|undefined)", "a(0:),b(0:)"},
			{`[seen('a')].toLocaleString('de', {style: 'percent'}, 'extra')`, `a(2:de|{"style":"percent"})`, "a(0:)"},
			{`[seen('a')].toLocaleString(undefined, 7)`, "a(2:undefined|7)", "a(0:)"},
			{`(() => { const l = ['de'], o = {}; let same; [{toLocaleString(x, y) { same = x === l && y === o; return '' }}].toLocaleString(l, o); return same })()`, "true", "false"},
			{`[null, undefined, , seen('a'), null].toLocaleString('x', 'y')`, ",,,a(2:x|y),", ",,,a(0:),"},
			{`[[seen('a')], seen('b')].toLocaleString('de', 1)`, "a(2:de|1),b(2:de|1)", "a(0:),b(0:)"},
			{`Array.prototype.toLocaleString.call({length: 2, 0: seen('a'), 1: seen('b')}, 'de', 2)`, "a(2:de|2),b(2:de|2)", "a(0:),b(0:)"},
			{`(() => { try { [{toLocaleString(l) { throw new RangeError('bad ' + l) }}].toLocaleString('zz') } catch (e) { return e.name + ' ' + e.message } })()`, "RangeError bad zz", "RangeError bad undefined"},
			{`patched(() => [1.5, 2n].toLocaleString('de', {a: 1}))`, `n1.5(2:de|{"a":1}),b2(2:de|{"a":1})`, "n1.5(0:),b2(0:)"},
			{`patched(() => new Float64Array([1.5, 2]).toLocaleString('de'))`, "n1.5(2:de|undefined),n2(2:de|undefined)", "n1.5(0:),n2(0:)"},
			{`patched(() => new Uint8Array([7]).toLocaleString())`, "n7(2:undefined|undefined)", "n7(0:)"},
			{`patched(() => new BigInt64Array([3n]).toLocaleString('de', {a: 1}, 9))`, `b3(2:de|{"a":1})`, "b3(0:)"},
			{`patched(() => new Int16Array(0).toLocaleString(''))`, "", ""},
			// An element that cuts the array short or detaches the buffer:
			// the length read at the start stands, and the elements gone
			// are empty.
			{`(() => { const a = [null, seen('b'), seen('c')]; a[0] = {toLocaleString(...x) { a.length = 1; return show(x) }}; return a.toLocaleString('de') })()`, "2:de|undefined,,", "0:,,"},
			{`patched(() => { const b = new ArrayBuffer(16), t = new Float64Array(b); t[0] = 1; t[1] = 2; const f = Number.prototype.toLocaleString; Number.prototype.toLocaleString = function (...a) { b.transfer(); return f.apply(this, a) }; return t.toLocaleString('de') })`, "n1(2:de|undefined),", "n1(0:),"},
		} {
			want := test[2]
			if ecma402 {
				want = test[1]
			}
			if got := evalString(t, m, test[0]); got != want {
				t.Errorf("ECMA-402 %v: %s\n\tgot  %s\n\twant %s", ecma402, test[0], got, want)
			}
		}
	}
}
