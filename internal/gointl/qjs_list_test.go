// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"runtime"
	"strings"
	"testing"
)

// With empty items left out (EmptyListItems), the literals between them are
// joined into one part, each written once: adding each to the text so far
// copied the text again for each, so a list of many empty items took time
// and allocation that grow with the square of it.
func TestDropEmptyItemsLinear(t *testing.T) {
	loc, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewListFormat(loc, ListFormatOptions{Compat: EmptyListItems})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.FormatToParts([]string{"a", "", "", "b"}); len(got) != 3 || got[1].Value != ", , , and " {
		t.Fatalf("parts of a, '', '', b: %+v", got)
	}
	const n = 20000
	items := make([]string, n)
	items[0], items[n-1] = "a", "b"
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	parts := f.FormatToParts(items)
	runtime.ReadMemStats(&b)
	if len(parts) != 3 || parts[1].Value != strings.Repeat(", ", n-2)+", and " {
		t.Fatalf("parts of %d items, all empty but the ends: %d parts", n, len(parts))
	}
	// The parts of the items, each a few words, and the joined text, twice
	// as the builder grows it.
	if got, want := b.TotalAlloc-a.TotalAlloc, uint64(256*n); got > want {
		t.Errorf("%d items allocated %d bytes, want at most %d", n, got, want)
	}
}
