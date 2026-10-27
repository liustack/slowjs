// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// Helpers of the tests that read the C engine, with the names and the
// results of the ones of lib (enginehelp_lib_test.go).

import (
	"testing"

	"github.com/liustack/slowjs/internal/cengine"
)

// engineMemoryUsage holds the fields of JSMemoryUsage the tests read, under
// the names of lib.TJSMemoryUsage.
type engineMemoryUsage struct {
	Fmalloc_size, Fatom_count, Fshape_count int64
}

// vmMemory runs the GC and returns the memory usage of the runtime of m.
func vmMemory(m *VM) engineMemoryUsage {
	m.RunGC()
	u := m.MemoryUsage()
	return engineMemoryUsage{Fmalloc_size: u.MallocSize, Fatom_count: u.AtomCount, Fshape_count: u.ShapeCount}
}

// wantNoPendingException fails the test if an exception is still pending in
// m's context.
func wantNoPendingException(t *testing.T, m *VM) {
	t.Helper()
	var pending bool
	m.outer(func() { pending = int32(cengine.JS_HasException(m.sp, m.ctx)) != 0 })
	if pending {
		t.Fatal("an exception is still pending in the context")
	}
}
