// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"strings"
	"testing"
)

func TestNumberFormatBigIntBeyondNumberRange(t *testing.T) {
	for _, sign := range []string{"", "-"} {
		t.Run("sign="+sign, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `var n = `+sign+`(10n ** 310n); var f = new Intl.NumberFormat('en', {useGrouping: false}); 0`)
			want := sign + "1" + strings.Repeat("0", 310)
			wantParts := `[{"type":"integer","value":"` + want + `"}]`
			if sign != "" {
				wantParts = `[{"type":"minusSign","value":"-"},{"type":"integer","value":"` + want[1:] + `"}]`
			}
			for _, value := range []string{"n", "Object(n)", "({[Symbol.toPrimitive](hint) {if (hint !== 'number') throw Error(hint); return n}})", "({valueOf() {return n}})"} {
				evalTests(t, vm, [][2]string{
					{fmt.Sprintf(`f.format(%s)`, value), want},
					{fmt.Sprintf(`JSON.stringify(f.formatToParts(%s))`, value), wantParts},
				})
			}
			evalTests(t, vm, [][2]string{
				{`n.toLocaleString('en', {useGrouping: false})`, want},
				{`Object(n).toLocaleString('en', {useGrouping: false})`, want},
				// Numeric strings are restricted to Number's magnitude, unlike BigInt.
				{`f.format(String(n))`, sign + "∞"},
				{`f.formatToParts(String(n)).map(p => p.type + ':' + p.value).join('|')`, map[bool]string{false: "infinity:∞", true: "minusSign:-|infinity:∞"}[sign != ""]},
				{`f.format({[Symbol.toPrimitive]() {return String(n)}})`, sign + "∞"},
			})
		})
	}
}
