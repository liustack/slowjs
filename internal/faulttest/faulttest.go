// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package faulttest is for the tests that make a memory fault on purpose: a
// read through a wild pointer, or a write to a guard page or to read-only
// memory.
//
// On windows such a fault may break the memory of the test process. The
// system writes the state of the CPU at the fault onto the goroutine's stack,
// and Go keeps 4 KiB below each stack for it. On CPUs whose state is larger,
// such as Intel with AMX, which the GitHub windows runners have, the state
// takes about 11.7 KiB, and the write goes past the stack into the heap
// (golang/go#81238). The tests skip their faults there.
package faulttest

import (
	"runtime"
	"testing"
)

// Unsafe reports whether a fault on purpose may break the memory of the
// process: on windows.
const Unsafe = runtime.GOOS == "windows"

// Skip skips the test where a fault on purpose is unsafe.
func Skip(t testing.TB) {
	t.Helper()
	if Unsafe {
		t.Skip("a fault on purpose may break the memory of the process on windows (golang/go#81238)")
	}
}
