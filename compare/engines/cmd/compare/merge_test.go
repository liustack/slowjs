// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"encoding/json"
	"testing"
)

func TestMergeEngines(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	res := &Results{
		Engines:   []Info{{Engine: "slowjs"}, {Engine: "moejs"}},
		Features:  []FeatureRow{{ID: "a", Results: map[string]json.RawMessage{"slowjs": raw(`1`), "moejs": raw(`0`)}}},
		Timed:     map[string]map[string][]json.RawMessage{"react": {"slowjs": {raw(`1`)}, "moejs": {raw(`0`)}}},
		Disturbed: map[string]int{"slowjs react": 1, "moejs react": 4},
		Test262:   &T262Summary{Commit: "c", Engines: map[string]*T262Engine{"slowjs": {}}},
	}
	src := &Results{
		Name:      "later",
		Source:    buildInfo{Commit: "x", Describe: "v-x"},
		Engines:   []Info{{Engine: "moejs"}},
		Features:  []FeatureRow{{ID: "a", Results: map[string]json.RawMessage{"moejs": raw(`2`)}}, {ID: "b", Results: map[string]json.RawMessage{"moejs": raw(`3`)}}},
		Timed:     map[string]map[string][]json.RawMessage{"react": {"moejs": {raw(`2`), raw(`2`)}}, "v8": {"moejs": {raw(`5`)}}},
		Disturbed: map[string]int{"moejs v8": 2},
		Test262:   &T262Summary{Commit: "c", Engines: map[string]*T262Engine{"moejs": {}}},
	}
	mergeEngines(res, src)
	if len(res.Engines) != 2 || res.Engines[0].Engine != "slowjs" {
		t.Fatalf("engines = %+v", res.Engines)
	}
	if string(res.Features[0].Results["moejs"]) != "2" || string(res.Features[0].Results["slowjs"]) != "1" || len(res.Features) != 2 {
		t.Fatalf("features = %+v", res.Features)
	}
	if len(res.Timed["react"]["moejs"]) != 2 || len(res.Timed["react"]["slowjs"]) != 1 || len(res.Timed["v8"]["moejs"]) != 1 {
		t.Fatalf("timed = %+v", res.Timed)
	}
	if res.Disturbed["moejs react"] != 0 || res.Disturbed["moejs v8"] != 2 || res.Disturbed["slowjs react"] != 1 {
		t.Fatalf("disturbed = %+v", res.Disturbed)
	}
	if res.Test262.Engines["moejs"] == nil || res.Test262.Engines["slowjs"] == nil {
		t.Fatalf("test262 = %+v", res.Test262.Engines)
	}
	if len(res.Added) != 1 || res.Added[0].Run != "later" || res.Added[0].Source.Commit != "x" {
		t.Fatalf("added = %+v", res.Added)
	}
	mergeEngines(res, src)
	if len(res.Added) != 1 {
		t.Fatalf("merging again added twice: %+v", res.Added)
	}
}
