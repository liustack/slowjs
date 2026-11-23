// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package faulttest is for the tests that make a memory fault on purpose: a
// read through a wild pointer, or a write to a guard page or to read-only
// memory.
//
// On windows such a fault may break the memory of the test process. The
// system writes the state of the CPU at the fault below the stack pointer of
// the thread, which is the goroutine's stack, and Go keeps 4 KiB below each
// stack for it. On CPUs whose state is larger, such as Intel with AMX, which
// the GitHub windows runners have, the state and the frames of the exception
// dispatch take about 12.8 KiB, and the write goes past the stack into the
// heap (golang/go#81238). On windows/arm64 the write takes about 2.5 KiB
// with 128-bit SVE, which fits, and internal/faultstate refuses a machine
// where it may not.
//
// The generated engine makes the room for it at the start of every function
// on windows/amd64 (docs/internals/changes/c-engine.md, When the engine fails), so a
// fault in the engine is safe, and the engine does not run on a windows/arm64
// machine faultstate refuses. A test whose fault is in Go code calls Skip
// first, which skips it on such a machine, and calls Room right before the
// access that faults. A fault in Go code behind a call the test cannot put
// Room after, as in a function of the host that faults inside, is unsafe on
// windows/amd64 and on a refused machine, and the test skips it there
// (Unsafe).
package faulttest

import (
	"runtime"
	"testing"

	"github.com/liustack/tifajs/internal/faultstate"
)

// Skip skips t where a fault in Go code may write past the stack even with
// Room: on a windows/arm64 machine internal/faultstate refuses.
func Skip(t testing.TB) {
	t.Helper()
	if err := faultstate.Check(); err != nil {
		t.Skip(err)
	}
}

// Unsafe reports whether a fault on purpose in Go code that Room cannot come
// right before may break the memory of the process: on windows/amd64, and on
// a windows/arm64 machine internal/faultstate refuses.
func Unsafe() bool {
	return runtime.GOOS == "windows" && runtime.GOARCH == "amd64" || faultstate.Check() != nil
}
