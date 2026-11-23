// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package cengine

import "testing"

// TestLeakedCounts: Leaked grows by the blocks a runtime still had in use
// when Close freed it, and a VM that freed all moves it by nothing, so a test
// of a package that only sees *tifajs.VM finds a leak of the engine.
func TestLeakedCounts(t *testing.T) {
	before := Leaked()
	clean := newVM(t, Options{})
	if _, err := clean.Eval(`[1, 2, 3].map(String).join()`, "x.js"); err != nil {
		t.Fatal(err)
	}
	closeVM(t, clean)
	if got := Leaked(); got != before {
		t.Fatalf("a VM that freed all moved Leaked from %d to %d", before, got)
	}

	v := newVM(t, Options{})
	// an object the VM never frees
	v.enter(func() { JS_NewObject(v.sp, v.ctx) })
	n, err := v.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("Close found no block in use")
	}
	if got := Leaked(); got != before+uint64(n) {
		t.Fatalf("Leaked is %d after %d blocks in use, %d before", got, n, before)
	}
}
