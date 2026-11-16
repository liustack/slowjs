// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestPartTypeNames checks the json intl.js makes its list of part types
// from: the keys of an object, in the order of partTypes, and the list
// itself, null until the first formatToParts.
func TestPartTypeNames(t *testing.T) {
	var lists map[string]json.RawMessage
	if err := json.Unmarshal([]byte(optionLists), &lists); err != nil {
		t.Fatal(err)
	}
	if string(lists["partTypes"]) != "null" {
		t.Fatalf("partTypes is %s", lists["partTypes"])
	}
	var names string
	if err := json.Unmarshal(lists["partTypeNames"], &names); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(names))
	var keys []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if k, ok := tok.(string); ok {
			keys = append(keys, k)
		}
	}
	if !slices.Equal(keys, partTypes) {
		t.Fatalf("partTypeNames has the keys\n%q\nwant\n%q", keys, partTypes)
	}
}

// TestPartTypesRead checks that formatToParts gives the types and values,
// those Go gives as an index and those it gives as a string, and a type
// a page can use as a key.
func TestPartTypesRead(t *testing.T) {
	vm := newVM(t)
	for _, tc := range [][2]string{
		{`JSON.stringify(new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR'}).formatToParts(-1234.5))`,
			`[{"type":"minusSign","value":"-"},{"type":"currency","value":"€"},{"type":"integer","value":"1"},{"type":"group","value":","},{"type":"integer","value":"234"},{"type":"decimal","value":"."},{"type":"fraction","value":"50"}]`},
		{`JSON.stringify(new Intl.DateTimeFormat('en', {dateStyle: 'medium', timeStyle: 'short', hourCycle: 'h23', timeZone: 'UTC'}).formatToParts(0))`,
			`[{"type":"month","value":"Jan"},{"type":"literal","value":" "},{"type":"day","value":"1"},{"type":"literal","value":", "},{"type":"year","value":"1970"},{"type":"literal","value":", "},{"type":"hour","value":"00"},{"type":"literal","value":":"},{"type":"minute","value":"00"}]`},
		{`(() => { const pos = {year: 0, month: 1, day: 2}; return new Intl.DateTimeFormat('en', {timeZone: 'UTC'}).formatToParts(0).filter(p => p.type in pos).map(p => pos[p.type] + p.value).join(); })()`,
			`11,21,01970`},
		{`(() => { const f = new Intl.NumberFormat('en'); f.formatToParts(1); return JSON.stringify(f.formatToParts(-0.5)); })()`,
			`[{"type":"minusSign","value":"-"},{"type":"integer","value":"0"},{"type":"decimal","value":"."},{"type":"fraction","value":"5"}]`},
	} {
		if got := goldenEval(t, vm, tc[0]); got != tc[1] {
			t.Errorf("%s\n\tgot  %s\n\twant %s", tc[0], got, tc[1])
		}
	}
}
