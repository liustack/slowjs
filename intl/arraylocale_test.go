// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/tifajs/intl"

import (
	"fmt"
	"testing"
)

// TestArrayToLocaleString checks that Array.prototype.toLocaleString and
// %TypedArray%.prototype.toLocaleString hand their locales and options to the
// toLocaleString of each element, as ECMA-402 says, so that the elements are
// formatted in that locale and a bad locale throws. The expected values are
// Node 24's, but for a locale of "" after a call with no locale, which Node
// 24 takes for the default locale: ECMA-402 throws a RangeError for it.
func TestArrayToLocaleString(t *testing.T) {
	vm := newVM(t)
	catch := func(src string) string {
		return fmt.Sprintf("(() => {try {return String(%s)} catch (e) {return e.name}})()", src)
	}
	for _, test := range [][2]string{
		{`[1234.5].toLocaleString('de')`, "1.234,5"},
		{`[1234.5].toLocaleString()`, "1,234.5"},
		{`[1.5].toLocaleString('')`, "RangeError"},
		{`[1].toLocaleString('en', {style: 'bogus'})`, "RangeError"},
		{`[].toLocaleString('')`, ""},
		{`[null, undefined].toLocaleString('')`, ","},
		{`[0.25, 1].toLocaleString('en', {style: 'percent'})`, "25%,100%"},
		{`[1234.5].toLocaleString('de', {maximumFractionDigits: 0})`, "1.235"},
		{`[1, 2].toLocaleString('de', {minimumFractionDigits: 2})`, "1,00,2,00"},
		{`[1234.5, null, undefined, , 2].toLocaleString('de')`, "1.234,5,,,,2"},
		{`[[1234.5, 2], 3].toLocaleString('de')`, "1.234,5,2,3"},
		{`[1234567n].toLocaleString('de')`, "1.234.567"},
		{`[1n].toLocaleString('en', {style: 'percent'})`, "100%"},
		{`[new Date(Date.UTC(2020, 0, 2, 3, 4, 5))].toLocaleString('de', {timeZone: 'UTC'})`, "2.1.2020, 03:04:05"},
		{`[new Date(Date.UTC(2020, 0, 2, 3, 4, 5))].toLocaleString('en-US', {timeZone: 'UTC', dateStyle: 'long'})`, "January 2, 2020"},
		{`[new Date(0)].toLocaleString('')`, "RangeError"},
		{`[new Date(NaN)].toLocaleString('de')`, "Invalid Date"},
		{`[{toLocaleString(...a) {return a.length + ':' + a[0] + ':' + JSON.stringify(a[1])}}].toLocaleString('de', {x: 1})`, `2:de:{"x":1}`},
		{`[{toLocaleString(...a) {return a.length + ':' + a[0] + ':' + a[1]}}].toLocaleString()`, "2:undefined:undefined"},
		{`new Float64Array([1234.5, 2]).toLocaleString('de')`, "1.234,5,2"},
		{`new Float64Array([1.5]).toLocaleString('')`, "RangeError"},
		{`new Float64Array(0).toLocaleString('')`, ""},
		{`new Float32Array([0.5]).toLocaleString('en', {style: 'percent'})`, "50%"},
		{`new Int32Array([25]).toLocaleString('en', {style: 'percent'})`, "2,500%"},
		{`new Uint8Array([1, 200]).toLocaleString('de', {minimumFractionDigits: 1})`, "1,0,200,0"},
		{`new BigInt64Array([1234567n]).toLocaleString('de')`, "1.234.567"},
		{`new BigUint64Array([1n]).toLocaleString('')`, "RangeError"},
	} {
		if got := fmt.Sprint(eval(t, vm, catch(test[0]))); got != test[1] {
			t.Errorf("%s\n\tgot  %s\n\twant %s", test[0], got, test[1])
		}
	}
}
