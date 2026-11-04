// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"testing"
)

func TestIslamicCalendarInstant(t *testing.T) {
	// Node 24.13.0 answers, around the January 2020 lunar month boundary.
	// The local day and the moon's age at the actual instant are both needed.
	cases := []struct{ zone, instant, want string }{
		{"Pacific/Kiritimati", "2020-01-24T09:59:59Z", "5/29/1441 AH"},
		{"Pacific/Kiritimati", "2020-01-24T10:00:00Z", "5/30/1441 AH"},
		{"Pacific/Kiritimati", "2020-01-24T12:00:00Z", "5/30/1441 AH"},
		{"Pacific/Kiritimati", "2020-01-25T00:00:00Z", "6/1/1441 AH"},
		{"Etc/GMT+12", "2020-01-25T00:00:00Z", "5/29/1441 AH"},
		{"Etc/GMT+12", "2020-01-25T11:59:59Z", "5/29/1441 AH"},
		{"Etc/GMT+12", "2020-01-25T12:00:00Z", "6/1/1441 AH"},
		{"Etc/GMT+12", "2020-01-26T12:00:00Z", "6/2/1441 AH"},
	}
	for _, calendar := range []string{"islamic", "islamic-rgsa"} {
		for _, tc := range cases {
			t.Run(calendar+"/"+tc.zone+"/"+tc.instant, func(t *testing.T) {
				vm := newVM(t)
				eval(t, vm, fmt.Sprintf(`var f = new Intl.DateTimeFormat('en', {
					calendar: %q, timeZone: %q, year: 'numeric', month: 'numeric', day: 'numeric'
				}); var d = new Date(%q); 0`, calendar, tc.zone, tc.instant))
				evalTests(t, vm, [][2]string{
					{`f.format(d)`, tc.want},
					{`f.formatToParts(d).map(p => p.value).join('')`, tc.want},
					{`f.formatRange(d, d)`, tc.want},
					{`f.formatRangeToParts(d, d).map(p => p.value).join('')`, tc.want},
				})
			})
		}
	}
}
