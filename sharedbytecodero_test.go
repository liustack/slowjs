// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"runtime/debug"
	"testing"
	"unsafe"
)

// On the C engine the bytes of a SharedBytecode are a read-only mapping that
// every VM reads in place: a write to them faults, while VMs that loaded them
// run.
func TestSharedBytecodeReadOnly(t *testing.T) {
	code := compileShared(t)
	shared := newSharedBytecode(t, code)
	defer shared.Close()
	m := newTestVM(t)
	if _, err := m.EvalSharedBytecode(shared); err != nil {
		t.Fatal(err)
	}
	if !writeFaults(shared.p) {
		t.Fatal("a write to the bytes of a SharedBytecode did not fault")
	}
	if got, want := evalString(t, m, sharedProbe), evalString(t, newTestVMWith(t, code), sharedProbe); got != want {
		t.Fatalf("shared bytecode shows\n%s\nwant\n%s", got, want)
	}
}

// writeFaults reports whether writing the byte at p faults.
func writeFaults(p uintptr) (faulted bool) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		_, faulted = recover().(interface{ Addr() uintptr })
	}()
	b := (*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
	*b ^= noChange
	return false
}

// noChange is what writeFaults writes over the byte with: a variable, so
// that the compiler keeps the write.
var noChange byte
