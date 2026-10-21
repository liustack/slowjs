// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
)

// cycles defines garbage(n), which leaves n pairs of objects that point at
// each other: reference counting never frees them, only the GC does.
const cycles = `globalThis.garbage = n => {
	for (let i = 0; i < n; i++) {
		const a = {}, b = {a};
		a.b = b;
	}
}`

func newGCVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if _, err := m.Eval(cycles, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	return m
}

// mallocSize is malloc_size as JS_ComputeMemoryUsage reports it, without a GC.
func mallocSize(m *VM) uintptr {
	tls, rt := m.runtime.tls, m.runtime.cRuntime
	n := int(unsafe.Sizeof(lib.TJSMemoryUsage{}))
	p := tls.Alloc(n)
	defer tls.Free(n)
	lib.XJS_ComputeMemoryUsage(tls, rt, p)
	return uintptr((*lib.TJSMemoryUsage)(unsafe.Pointer(p)).Fmalloc_size)
}

// TestRunGC pins that RunGC frees objects in reference cycles, that
// MemorySize follows what the runtime takes, and that RunGC leaves the GC
// threshold alone.
func TestRunGC(t *testing.T) {
	m := newGCVM(t)
	off := ^uintptr(0) // no GC but the one RunGC runs
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
		t.Fatalf("RunGC set the GC threshold to %d, expected %d", got, off)
	}
}

// TestGCThreshold pins that GCThreshold returns what SetGCThreshold set until
// the engine runs a GC by itself, which sets the threshold to 1.5 times what
// the GC left.
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

	// Each GC the engine runs leaves about live bytes, so it sets the
	// threshold to about 1.5 live, below set.
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
