// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// TestBuiltinShape compares the functions and objects of Intl and the
// toLocale* methods, before and after the install, with Chromium's: names,
// lengths, own properties and their attributes, prototype properties,
// constructors and toString.
func TestBuiltinShape(t *testing.T) {
	src, err := os.ReadFile("testdata/chromium/shape.js")
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile("testdata/chromium/shape.json")
	if err != nil {
		t.Fatal(err)
	}

	var want map[string]any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}

	vm := newVM(t)
	var got map[string]any
	if err := json.Unmarshal([]byte(eval(t, vm, string(src)+"\nshape()").(string)), &got); err != nil {
		t.Fatal(err)
	}

	var keys []string
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		w, _ := json.Marshal(want[k])
		g, _ := json.Marshal(got[k])
		if string(g) != string(w) {
			t.Errorf("%s\n\tgot  %s\n\twant %s", k, g, w)
		}
	}
}
