// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"strings"
	"testing"
)

// fixture is a results file of three engines: ours and two others. The
// micro benchmark "loop" takes 3 times the median of the others, "calls" the
// same time. The V8 suite: ours scores 100 on Crypto where the others score
// 400 and 200 (a median of 300, three times as high), and the string score is
// read as a number. The workloads are 2 times the median for "react" and the
// same for the rest.
const fixture = `{
 "engines": [{"engine": "slowjs"}, {"engine": "a"}, {"engine": "b"}],
 "timed": {
  "upstream": {
   "slowjs": [{"tests": {"loop": {"ns_per_n": 30}, "calls": {"ns_per_n": 10}}}, {"tests": {"loop": {"ns_per_n": 50}, "calls": {"ns_per_n": 10}}}],
   "a": [{"tests": {"loop": {"ns_per_n": 10}, "calls": {"ns_per_n": 10}}}],
   "b": [{"tests": {"loop": {"ns_per_n": 10}, "calls": {"ns_per_n": 10}, "only_b": {"ns_per_n": 1}}}]
  },
  "v8": {
   "slowjs": [{"scores": {"Crypto": "100"}}],
   "a": [{"scores": {"Crypto": "400"}}],
   "b": [{"scores": {"Crypto": 200}}]
  },
  "react": {"slowjs": [{"ok": true, "cpu_ms_per_op": 20}, {"ok": false}], "a": [{"ok": true, "cpu_ms_per_op": 10}], "b": [{"ok": true, "cpu_ms_per_op": 10}]},
  "micro/json-parse": {"slowjs": [{"ok": true, "cpu_ms_per_op": 1}], "a": [{"ok": true, "cpu_ms_per_op": 1}], "b": [{"ok": true, "cpu_ms_per_op": 1}]},
  "micro/json-stringify": {"slowjs": [{"ok": true, "cpu_ms_per_op": 1}], "a": [{"ok": true, "cpu_ms_per_op": 1}], "b": [{"ok": true, "cpu_ms_per_op": 1}]},
  "micro/regexp": {"slowjs": [{"ok": true, "cpu_ms_per_op": 1}], "a": [{"ok": true, "cpu_ms_per_op": 1}], "b": [{"ok": true, "cpu_ms_per_op": 1}]},
  "micro/string-concat": {"slowjs": [{"ok": true, "cpu_ms_per_op": 1}], "a": [{"ok": true, "cpu_ms_per_op": 1}], "b": [{"ok": true, "cpu_ms_per_op": 1}]}
 }
}`

func TestBehind(t *testing.T) {
	rows, err := behind([]byte(fixture), 2)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, r.String())
	}
	want := []string{
		"micro    loop                          4.00  ours 40  median 10  best a 10  n=2",
		"v8       Crypto                        3.00  ours 100  median 300  best a 400  n=2",
		"workload react                         2.00  ours 20  median 10  best a 10  n=2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The module's name before v0.28.0 names the row of older results files.
func TestBehindOldName(t *testing.T) {
	old := strings.NewReplacer(`"engine": "slowjs"`, `"engine": "liustack-quickjs"`, `"slowjs":`, `"liustack-quickjs":`).Replace(fixture)
	rows, err := behind([]byte(old), 3.5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].name != "loop" {
		t.Errorf("rows = %v, want the loop only", rows)
	}
}

// A results file without a workload cannot be read as if the workload were
// fast: it is an error.
func TestBehindMissingWorkload(t *testing.T) {
	broken := strings.Replace(fixture, `"micro/regexp"`, `"micro/regexp-renamed"`, 1)
	if _, err := behind([]byte(broken), 2); err == nil {
		t.Error("no error for a results file without micro/regexp")
	}
}
