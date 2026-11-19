// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"testing"
)

// TestBlockWaitEndsWithTheCall pins that the wait of the collections before
// large blocks (js_trigger_gc_block_run in engine/quickjs.c) does not outlive
// the call that started it: a call that ends with out of memory in a wait,
// as a render under a tight limit does, leaves the next call to collect at
// its first block near the limit, as a VM fresh from a collection would. The
// block of the next call fits once the cycles it made before are freed.
func TestBlockWaitEndsWithTheCall(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`
function cyc(n) { for (var i = 0; i < n; i++) { var a = {i: i}; a.self = a } }
function near(k) { for (var i = 0; i < k; i++) 'x'.repeat(6000) }
function first(k) { near(k); return 'x'.repeat(1 << 20).length }
function second(n) { cyc(n); globalThis.b = 'x'.repeat(150 << 10); return b.length }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	// the bytes of one cycle, with the collections of the threshold off and
	// what the first call makes once made
	if _, err := m.Call("cyc", 1000); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	m.SetGCThreshold(1 << 40)
	before := m.MemorySize()
	if _, err := m.Call("cyc", 1000); err != nil {
		t.Fatal(err)
	}
	per := (m.MemorySize() - before) / 1000
	m.RunGC()
	if per == 0 {
		t.Fatal("1000 cycles took no memory")
	}

	// 100 blocks of 6000 bytes, each within the slack of the limit: each
	// collection leaves no room and starts a longer wait, and the call ends
	// in one, with out of memory at a block that does not fit.
	m.SetMemoryLimit(m.MemorySize() + 12000)
	m.SetGCThreshold(1 << 40)
	_, err := m.Call("first", 100)
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory {
		t.Fatalf("the first call returned %v, want out of memory", err)
	}

	// the collections of the first call set the threshold again: off, so
	// that only the block collects the cycles of the next
	m.SetGCThreshold(1 << 40)
	// 250 KiB of cycles and a block of 150 KiB, 300 KiB under the limit:
	// the block fits once the cycles are collected, and only then
	m.SetMemoryLimit(m.MemorySize() + 300<<10)
	if r, err := m.Call("second", int((250<<10)/per)); err != nil {
		t.Fatalf("the call after the one that ran out of memory: %v", err)
	} else if fmt.Sprint(r) != "153600" {
		t.Fatalf("the call after the one that ran out of memory returned %v", r)
	}
	if m.GCThreshold() == 1<<40 {
		t.Fatal("the call after the one that ran out of memory made its block with no collection")
	}
}
