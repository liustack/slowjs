// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
)

// mallocSize is malloc_size as JS_ComputeMemoryUsage reports it, without a GC.
func mallocSize(m *VM) uintptr {
	return uintptr(m.MemoryUsage().MallocSize)
}

// TestRunGC pins that RunGC frees objects in reference cycles, that
// MemorySize follows what the runtime takes, and that RunGC leaves alone a
// threshold that turns the GC of the engine off.
func TestRunGC(t *testing.T) {
	m := newGCVM(t)
	off := ^uintptr(0) // no GC but the one RunGC runs, and RunGC keeps it so
	m.SetGCThreshold(off)
	m.RunGC()
	live := m.MemorySize()
	if live == 0 || live != mallocSize(m) {
		t.Fatalf("MemorySize %d, malloc_size %d", live, mallocSize(m))
	}

	if _, err := m.Eval("garbage(20000)", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	grown := m.MemorySize()
	if grown != mallocSize(m) {
		t.Fatalf("MemorySize %d, malloc_size %d", grown, mallocSize(m))
	}
	if grown < live+1<<20 {
		t.Fatalf("20000 cycles took %d bytes, expected at least 1 MiB", grown-live)
	}

	m.RunGC()
	if got := m.MemorySize(); got > live+64<<10 {
		t.Fatalf("after RunGC the runtime takes %d bytes, %d before the cycles", got, live)
	}
	if got := m.GCThreshold(); got != off {
		t.Fatalf("RunGC set the GC threshold to %d, expected %d that turns the GC off", got, off)
	}
}

// collectable is the memory a GC sets the threshold from: MemorySize without
// the register stack (JS_RegvmStat 9), which counts under the limit but
// which no GC can free.
func collectable(m *VM) uintptr {
	return m.MemorySize() - uintptr(regvmStat(m, 9))
}

// TestRunGCSetsTheThreshold pins that RunGC sets the GC threshold to 1.5 times
// what it left, without the register stack, as the GC the engine runs by
// itself does, whatever the threshold was.
func TestRunGCSetsTheThreshold(t *testing.T) {
	m := newGCVM(t)
	for _, before := range []uintptr{1, 1 << 20, 1 << 40} {
		m.SetGCThreshold(before)
		m.RunGC()
		live := collectable(m)
		if got, want := m.GCThreshold(), live+live/2; got != want {
			t.Fatalf("after RunGC with the threshold at %d: GCThreshold %d, want %d, 1.5 times the %d bytes left", before, got, want, live)
		}
	}
}

// TestGCThreshold pins that GCThreshold returns what SetGCThreshold set until
// the engine runs a GC by itself, which sets the threshold to 1.5 times what
// the GC left, without the register stack.
func TestGCThreshold(t *testing.T) {
	m := newGCVM(t)
	m.RunGC()
	live := m.MemorySize()
	set := live + 1<<20
	m.SetGCThreshold(set)
	if got := m.GCThreshold(); got != set {
		t.Fatalf("GCThreshold %d, SetGCThreshold set %d", got, set)
	}
	// No object, no GC.
	if _, err := m.Eval("1 + 1", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if got := m.GCThreshold(); got != set {
		t.Fatalf("GCThreshold %d after a script that made no object, SetGCThreshold set %d", got, set)
	}

	// Each GC the engine runs leaves about live bytes besides the register
	// stack, so it sets the threshold to about 1.5 live, below set. live is
	// taken after a GC: before one, MemorySize also counts the blocks a
	// script freed into the allocator's front cache, which every GC gives
	// back (8.8 KB after "1 + 1"), so that a GC may leave less than it.
	m.RunGC()
	live = collectable(m)
	m.SetGCThreshold(set)
	if _, err := m.Eval("garbage(100000)", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	got := m.GCThreshold()
	if got == set {
		t.Fatal("the engine ran no GC over 100000 cycles past the threshold")
	}
	if lo, hi := live+live/2, live+live/2+(live+live/2)/4; got < lo || got > hi {
		t.Fatalf("after the engine's GC the threshold is %d, expected 1.5 times what was left (%d to %d)", got, lo, hi)
	}
}

// TestGCAfterClose pins that the GC methods, like SetGCThreshold, panic after
// Close.
func TestGCAfterClose(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*VM)
	}{
		{"RunGC", (*VM).RunGC},
		{"MemorySize", func(m *VM) { m.MemorySize() }},
		{"GCThreshold", func(m *VM) { m.GCThreshold() }},
		{"SetGCThreshold", func(m *VM) { m.SetGCThreshold(1 << 20) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if recover() == nil {
					t.Fatalf("%s did not panic after Close", tc.name)
				}
			}()
			tc.call(m)
		})
	}
}
