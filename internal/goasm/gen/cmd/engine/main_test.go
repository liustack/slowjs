// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"reflect"
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
