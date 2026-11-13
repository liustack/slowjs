// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"testing"

	"github.com/liustack/quickjs/internal/hostmem"
)

// The block a VM keeps for its next call counts against its memory limit,
// is kept only when it is at most a quarter of the room left, and goes back
// to the OS after keepIdle calls that took none, and at Close.
func TestKeepCountsAgainstRoom(t *testing.T) {
	const size = 4 << 20
	held := hostmem.Blocks()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(256 << 20)
	before := m.goRoom()
	_, give, ok := m.goTake(size)
	if !ok {
		t.Fatal("goTake failed")
	}
	give()
	if m.keep.Size() != size {
		t.Fatalf("the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	if after := m.goRoom(); before-after != size {
		t.Fatalf("the room left went from %d to %d with a block of %d kept", before, after, size)
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

	m.SetMemoryLimit(uintptr(m.jsGetMemorySize()) + 3*size)
	_, give, ok = m.goTake(size)
	if !ok {
		t.Fatal("goTake failed")
	}
	give()
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("with room for 3 blocks, the VM keeps %d bytes, %d blocks held", m.keep.Size(), hostmem.Blocks()-held)
	}

	m.SetMemoryLimit(^uintptr(0))
	_, give, _ = m.goTake(size)
	give()
	if m.keep.Size() != size {
		t.Fatalf("with no memory limit, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	m.Close()
	if hostmem.Blocks() != held {
		t.Fatalf("%d blocks held after Close", hostmem.Blocks()-held)
	}
}
