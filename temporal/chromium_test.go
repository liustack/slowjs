// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"encoding/json"
	"os"
	"testing"
)

// differFromChromium are the cases of testdata/corpus.js whose answers are
// not Chromium's, and why.
var differFromChromium = map[string]string{
	`Temporal.PlainDate.from("2024-01-05").toLocaleString("en-US", {era: "short"})`:                                     eraAlone,
	`Temporal.PlainDateTime.from("2024-01-05T13:05:07").toLocaleString("en-US", {era: "short"})`:                        eraAlone,
	`Temporal.ZonedDateTime.from("2024-01-05T13:05:07[America/New_York]").toLocaleString("en-US", {era: "short"})`:      eraAlone,
	`Temporal.Instant.from("2024-01-05T13:05:07Z").toLocaleString("en-US", {era: "short"})`:                             eraAlone,
	`Temporal.PlainYearMonth.from({year: 2024, month: 1, calendar: "gregory"}).toLocaleString("en-US", {era: "short"})`: eraAlone,
	`Temporal.PlainYearMonth.from({year: 2024, month: 1, calendar: "gregory"}).toLocaleString("zh-CN", {month: "long", day: "numeric"})`: "ECMA-402 drops the day and formats the month alone, " +
		"as Chromium's Intl.DateTimeFormat does with {month: 'long'}, V8 takes the pattern of month and day and drops the day from it",
	`Temporal.PlainDate.from("2024-01-05[u-ca=japanese]").toLocaleString("ja-JP-u-ca-japanese")`: "package intl formats in the Gregorian calendar only",
}

const eraAlone = "ECMA-402 adds the date to an era alone (GetDateTimeFormat, needDefaults), V8 formats the era only"

// TestChromium runs the cases of testdata/corpus.js and compares their
// answers, values or errors with their messages, byte for byte with
// Chromium 156's (testdata/chromium.json).
func TestChromium(t *testing.T) {
	src, err := os.ReadFile("testdata/corpus.js")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("testdata/chromium.json")
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Answers map[string]string
	}
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}

	vm := newVM(t)
	var got struct {
		Cases   []string
		Answers []string
	}
	js := string(src) + "\nJSON.stringify({cases: cases, answers: cases.map(answer)})"
	if err := json.Unmarshal([]byte(eval(t, vm, js).(string)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Cases) != len(want.Answers) {
		t.Errorf("%d cases, Chromium answered %d", len(got.Cases), len(want.Answers))
	}
	same := 0
	for i, c := range got.Cases {
		w, ok := want.Answers[c]
		switch {
		case !ok:
			t.Errorf("%s: Chromium has no answer", c)
		case got.Answers[i] == w:
			same++
			if _, ok := differFromChromium[c]; ok {
				t.Errorf("%s: answers as Chromium does, but is listed as different", c)
			}
		case differFromChromium[c] != "":
			t.Logf("%s:\n\tgot      %q\n\tChromium %q\n\t(%s)", c, got.Answers[i], w, differFromChromium[c])
		default:
			t.Errorf("%s:\n\tgot      %q\n\tChromium %q", c, got.Answers[i], w)
		}
	}
	t.Logf("%d of %d cases answer as Chromium does", same, len(got.Cases))
}
