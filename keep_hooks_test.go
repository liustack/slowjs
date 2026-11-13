// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package quickjs

import (
	"testing"

	"github.com/liustack/quickjs/internal/gohost"
	"github.com/liustack/quickjs/internal/hostmem"
)

// keptVM is a VM with no memory limit that keeps a block of size bytes, at
// the address it returns.
func keptVM(t *testing.T, size int) (*VM, uintptr) {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	p, give, ok := m.goTake(size)
	if !ok {
		t.Fatal("goTake failed")
	}
	give()
	if m.keep.Size() != uintptr(size) {
		t.Fatalf("the VM keeps %d bytes, want %d", m.keep.Size(), size)
	}
	return m, uintptr(p)
}

// recovering calls f and returns what it panicked with.
func recovering(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

// When the OS refuses the block kept, a SetMemoryLimit it no longer fits
// under panics and leaves the VM as it was: with no limit, keeping the block.
// A SetMemoryLimit again gives the limit and the block back.
func TestSetMemoryLimitKeepRefused(t *testing.T) {
	const size = 4 << 20
	held := hostmem.Blocks()
	m, at := keptVM(t, size)
	defer m.Close()
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" && addr == at {
			return errRefused
		}
		return nil
	})
	limit := m.MemorySize() + size/2
	r := recovering(func() { m.SetMemoryLimit(limit) })
	hostmem.SetFault(nil)
	if r == nil {
		t.Fatal("SetMemoryLimit did not panic when the OS refused the block kept")
	}
	if m.memoryLimit != 0 || m.keep.Size() != size {
		t.Fatalf("after the refusal: limit %d, %d bytes kept, want no limit and %d", m.memoryLimit, m.keep.Size(), size)
	}
	m.SetMemoryLimit(limit)
	if m.memoryLimit != limit || m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("SetMemoryLimit again: limit %d, %d bytes kept, %d blocks held", m.memoryLimit, m.keep.Size(), hostmem.Blocks()-held)
	}
}

// When the OS refuses the block kept to an allocation that needs its room,
// the allocation fails as out of memory and the VM keeps the block, within
// its limit. The next allocation that needs the room gets it once the OS
// takes the block.
func TestPressureKeepRefused(t *testing.T) {
	const size = 4 << 20
	held := hostmem.Blocks()
	m, at := keptVM(t, size)
	defer m.Close()
	m.SetMemoryLimit(m.MemorySize() + 3*size)
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" && addr == at {
			return errRefused
		}
		return nil
	})
	_, err := m.Eval("globalThis.s = 'y'.repeat(9 << 20); s.length", EvalGlobal)
	hostmem.SetFault(nil)
	if err == nil {
		t.Fatal("an allocation that needed the room of a block the OS refused passed")
	}
	if m.keep.Size() != size || m.MemorySize()+size > m.memoryLimit {
		t.Fatalf("after the refusal: %d bytes kept, %d used, limit %d", m.keep.Size(), m.MemorySize(), m.memoryLimit)
	}
	if _, err := m.Eval("globalThis.s = 'y'.repeat(9 << 20); s.length", EvalGlobal); err != nil {
		t.Fatalf("the allocation again: %v", err)
	}
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after the allocation again: %d bytes kept, %d blocks held", m.keep.Size(), hostmem.Blocks()-held)
	}
}

// When the OS refuses the block kept and the engine's memory at Close, and
// the block again at the next Close, that Close still gives back the
// engine's memory, and panics. A third Close gives back the block.
func TestCloseAgainKeepRefused(t *testing.T) {
	const size = 4 << 20
	held, before := hostmem.Blocks(), len(gohost.Leftovers())
	m, at := keptVM(t, size)
	rt := m.rt
	engineRefused := false
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call != "unmap" {
			return nil
		}
		if addr == at {
			return errRefused
		}
		if addr <= rt && rt-addr < size && !engineRefused {
			engineRefused = true
			return errRefused
		}
		return nil
	})
	defer hostmem.SetFault(nil)
	if r := recovering(func() { m.Close() }); r == nil || !engineRefused {
		t.Fatalf("Close with the block and the engine refused: panic %v, engine refused %v", r, engineRefused)
	}
	if got := len(gohost.Leftovers()); got != before+1 {
		t.Fatalf("after the first Close %d VMs hold memory, want %d", got, before+1)
	}
	if r := recovering(func() { m.Close() }); r == nil {
		t.Fatal("Close again did not panic when the OS refused the block kept")
	}
	if got := len(gohost.Leftovers()); got != before {
		t.Fatalf("after Close again %d VMs hold memory, want %d: the refused block kept the engine from being given back", got, before)
	}
	hostmem.SetFault(nil)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.keep.Size() != 0 || hostmem.Blocks() != held {
		t.Fatalf("after Close a third time: %d bytes kept, %d blocks held", m.keep.Size(), hostmem.Blocks()-held)
	}
}
