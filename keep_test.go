// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"testing"

	"github.com/liustack/quickjs/internal/hostmem"
)

// Only a VM with no memory limit keeps the block given back for its next
// call. It goes back to the OS after keepIdle calls that took none, when the
// VM is given a limit, and at Close. A VM with a limit gives each block back
// to the OS, so its memory stays within the limit.
func TestKeepOnlyWithNoLimit(t *testing.T) {
	const size = 4 << 20
	held := hostmem.Blocks()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	take := func() {
		t.Helper()
		_, give, ok := m.goTake(size)
		if !ok {
			t.Fatal("goTake failed")
		}
		give()
	}
	take()
	if m.keep.Size() != size {
		t.Fatalf("with no memory limit, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	for i := range keepIdle {
		if _, err := m.Eval("1", EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if kept := m.keep.Size() != 0; kept != (i < keepIdle-1) {
			t.Fatalf("after %d calls that took no block, the VM keeps %d bytes", i+1, m.keep.Size())
		}
	}
	if hostmem.Blocks() != held {
		t.Fatalf("%d blocks held after the kept one went back", hostmem.Blocks()-held)
	}

	take()
	m.SetMemoryLimit(256 << 20)
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after SetMemoryLimit, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
	}
	take()
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("with a memory limit, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
	}

	m.SetMemoryLimit(^uintptr(0))
	take()
	if m.keep.Size() != size {
		t.Fatalf("with the limit taken off, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	m.Close()
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after Close, the VM keeps %d bytes, %d blocks held", m.keep.Size(), hostmem.Blocks()-held)
	}
}
