// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"testing"

	"github.com/liustack/quickjs/internal/hostmem"
)

// A VM keeps the block given back for its next call when it fits under the
// memory limit with the VM's memory, and the limit counts it. It goes back to
// the OS after keepIdle calls that took none, when SetMemoryLimit sets a
// limit it no longer fits under, when an allocation of the engine needs its
// room, and at Close.
func TestKeepWithinLimit(t *testing.T) {
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
	within := func() {
		t.Helper()
		if m.memoryLimit != 0 && m.MemorySize()+m.keep.Size() > m.memoryLimit {
			t.Fatalf("the VM uses %d bytes and keeps %d, past its limit of %d", m.MemorySize(), m.keep.Size(), m.memoryLimit)
		}
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

	// A limit with room for the block keeps it.
	take()
	m.SetMemoryLimit(m.MemorySize() + 3*size)
	if m.keep.Size() != size {
		t.Fatalf("after SetMemoryLimit with room, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	within()
	take()
	if m.keep.Size() != size {
		t.Fatalf("with a memory limit with room, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	within()

	// An allocation that needs the room of the block gets it.
	if _, err := m.Eval(`globalThis.s = 'y'.repeat(9 << 20); s.length`, EvalGlobal); err != nil {
		t.Fatalf("an allocation that fits once the block goes back: %v", err)
	}
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after an allocation that needed its room, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
	}
	within()
	// No room now: the block goes back to the OS.
	take()
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("with no room under the limit, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
	}
	if _, err := m.Eval(`delete globalThis.s; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()

	// A limit the block no longer fits under gives it back.
	take()
	if m.keep.Size() != size {
		t.Fatalf("with room again, the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	m.SetMemoryLimit(m.MemorySize() + size/2)
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after SetMemoryLimit with no room, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
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

// A block given back in the unwind of a panic out of the engine goes back to
// the OS: the engine is not asked its size then, which would panic over the
// first panic and lose the block.
func TestKeepInEngineFault(t *testing.T) {
	held := hostmem.Blocks()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(256 << 20)
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		_, give, ok := m.goTake(4 << 20)
		if !ok {
			t.Error("goTake failed")
			return m.NewInt(0), nil
		}
		defer give()
		withoutContext(m, func() { m.NewObjectValue() })
		return m.NewInt(0), nil
	}))
	lowFault(t, evalRecover(m, `inner()`))
	wantBroken(t, m, "a fault at a low address in a Go function holding a block")
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after the fault, the VM keeps %d bytes, %d blocks held, want 0", m.keep.Size(), hostmem.Blocks()-held)
	}
	m.Close()
}
