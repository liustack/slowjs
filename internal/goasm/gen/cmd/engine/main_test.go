// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestFirstGroups: a function two files define goes with the first one, as
// llvm-link keeps the first one's name, whichever compile finished last.
func TestFirstGroups(t *testing.T) {
	got := firstGroups([][]string{{"a", "dup"}, {"dup", "b"}, {"dup"}}, []string{"one", "two", "three"})
	want := map[string]string{"a": "one", "dup": "one", "b": "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestKeepNarrowFromEnv: only 1 turns the negative build on. A variable that
// reads as set but not as 1 (0, true, a space) is an error, not a build that
// treats 0 as on.
func TestKeepNarrowFromEnv(t *testing.T) {
	for v, want := range map[string]bool{"": false, "1": true} {
		if got, err := keepNarrowFromEnv(v); err != nil || got != want {
			t.Errorf("%q: %v, %v, want %v", v, got, err, want)
		}
	}
	for _, v := range []string{"0", "true", "yes", "2", " 1", "1 ", "off"} {
		if got, err := keepNarrowFromEnv(v); err == nil || got || !strings.Contains(err.Error(), "GEN_KEEP_NARROW") {
			t.Errorf("%q: accepted as %v, %v", v, got, err)
		}
	}
}
