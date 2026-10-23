// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"testing"

	"github.com/liustack/slowjs"
)

func TestPluralRulesRoundingRegistry(t *testing.T) {
	for _, separate := range []bool{false, true} {
		for _, order := range [][2]int{{0, 2}, {2, 0}} {
			t.Run(fmt.Sprintf("separateVMs=%t/first=%d", separate, order[0]), func(t *testing.T) {
				first := newVM(t)
				vms := []*slowjs.VM{first, first}
				if separate {
					vms[1] = newVM(t)
				}
				// Evict earlier tests' formatters through the public API so each
				// order starts without either rounding configuration cached.
				evalTests(t, first, [][2]string{{floodJS, floodWant}})
				for i, digits := range order {
					eval(t, vms[i], fmt.Sprintf(`var pr%d = new Intl.PluralRules('en', {
						maximumSignificantDigits: 1, roundingPriority: 'morePrecision',
						maximumFractionDigits: %d
					}); 0`, i, digits))
				}
				check := func() {
					for i, digits := range order {
						want := "one"
						if digits == 2 {
							want = "other"
						}
						evalTests(t, vms[i], [][2]string{
							{fmt.Sprintf(`pr%d.select(1.1)`, i), want},
							// Cache identity must not change public resolved options.
							{fmt.Sprintf(`Object.keys(pr%d.resolvedOptions()).some(k => k.startsWith('_'))`, i), "false"},
						})
					}
				}
				check()
				evalTests(t, first, [][2]string{{floodJS, floodWant}})
				check()
			})
		}
	}
}
