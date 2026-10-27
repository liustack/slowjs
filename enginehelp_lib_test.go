// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Helpers of the tests that read the engine of lib, with the same names on
// the C engine (enginehelp_c_test.go).

import (
	"testing"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
)

// vmMemory runs the GC and returns the memory usage of the runtime of m.
func vmMemory(m *VM) lib.TJSMemoryUsage {
	tls, rt := m.runtime.tls, m.runtime.cRuntime
	lib.XJS_RunGC(tls, rt)
	// The engine writes the result, so it goes in C memory.
	n := int(unsafe.Sizeof(lib.TJSMemoryUsage{}))
	p := tls.Alloc(n)
	defer tls.Free(n)
	lib.XJS_ComputeMemoryUsage(tls, rt, p)
	return *(*lib.TJSMemoryUsage)(unsafe.Pointer(p))
}

// wantNoPendingException fails the test if an exception is still pending in
// m's context. A failed call must take its exception out of the context: one
// left there is what the next caller of JS_GetException finds, and it keeps
// the thrown value alive.
func wantNoPendingException(t *testing.T, m *VM) {
	t.Helper()
	if lib.XJS_HasException(m.runtime.tls, m.cContext) != 0 {
		t.Fatal("an exception is still pending in the context")
	}
}
