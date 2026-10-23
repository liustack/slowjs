// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGoIntlMigration(t *testing.T) {
	b, err := os.ReadFile("testdata/chromium/migration.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Browser string
		Cases   [][2]string
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	vm := newVM(t)
	for _, c := range corpus.Cases {
		want := c[1]
		if updated, ok := tz2026eAnswers[c[0]]; ok {
			want = updated
		}
		if got := goldenEval(t, vm, c[0]); got != want {
			t.Errorf("%s\ngot %q\nwant %q", c[0], got, want)
		}
	}
	t.Logf("%s: %d expressions", corpus.Browser, len(corpus.Cases))
}

// Chromium 156's ICU predates Winnipeg's permanent daylight time in tz2026e.
// Keep the browser recording intact and require the exact updated answers.
var tz2026eAnswers = map[string]string{
	"new Intl.DateTimeFormat(\"en\",{timeZone:\"America/Winnipeg\",year:\"numeric\",month:\"long\",day:\"numeric\",hour:\"numeric\",minute:\"numeric\",timeZoneName:\"long\"}).format(new Date(\"2026-11-15T12:00Z\"))":                        "November 15, 2026 at 7:00 AM Central Daylight Time",
	"JSON.stringify(new Intl.DateTimeFormat(\"en\",{timeZone:\"America/Winnipeg\",year:\"numeric\",month:\"long\",day:\"numeric\",hour:\"numeric\",minute:\"numeric\",timeZoneName:\"long\"}).formatToParts(new Date(\"2026-11-15T12:00Z\")))": "[{\"type\":\"month\",\"value\":\"November\"},{\"type\":\"literal\",\"value\":\" \"},{\"type\":\"day\",\"value\":\"15\"},{\"type\":\"literal\",\"value\":\", \"},{\"type\":\"year\",\"value\":\"2026\"},{\"type\":\"literal\",\"value\":\" at \"},{\"type\":\"hour\",\"value\":\"7\"},{\"type\":\"literal\",\"value\":\":\"},{\"type\":\"minute\",\"value\":\"00\"},{\"type\":\"literal\",\"value\":\" \"},{\"type\":\"dayPeriod\",\"value\":\"AM\"},{\"type\":\"literal\",\"value\":\" \"},{\"type\":\"timeZoneName\",\"value\":\"Central Daylight Time\"}]",
	"new Date(\"2026-11-15T12:00Z\").toLocaleString(\"en\",{timeZone:\"America/Winnipeg\",year:\"numeric\",month:\"long\",day:\"numeric\",hour:\"numeric\",minute:\"numeric\",timeZoneName:\"long\"})":                                         "November 15, 2026 at 7:00 AM Central Daylight Time",
}
