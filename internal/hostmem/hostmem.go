// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package hostmem is the memory the Go host gives the C engine. It is mapped
// from the OS outside the Go heap, so the garbage collector neither scans
// nor moves it, and every VM has its own:
//
//   - the chunks of its heap, which the engine's allocator asks for and
//     manages. The engine counts the blocks it uses itself. Memory counts
//     what the VM holds from the OS, its footprint.
//   - its shadow stack, a fixed region for the C locals whose address is
//     taken, with a guard page at the end it grows to.
//
// The callbacks of the engine map onto it:
//
//	host_memory_grow(vm, size)       m.Grow(size), 0 when it fails
//	host_call, release a chunk       m.Free(base, size)
//	host_call, grow a chunk          m.Remap(base, size, newSize), on linux (CanRemap)
//	VM.ReleaseMemory                 JS_ReleaseStart, then m.Release with
//	                                 JS_ReleaseNextChunk and JS_ReleaseNextRange
//	VM.Close                         JS_FreeRuntime, then m.Close
//
// None of these runs javascript or calls into the engine. A Memory is used by
// one goroutine at a time, as its VM is, and needs no lock. Nothing is shared
// between Memories but the page size.
//
// The errors are what the OS refused, with what was asked, and three more:
//
//   - New refuses a shadow stack too large for the address space before it
//     asks the OS.
//   - Remap matches errors.ErrUnsupported where the OS cannot resize a
//     mapping (CanRemap is false).
//   - On windows, New fails when other threads took the aligned address of
//     the stack 16 times in a row. The error wraps the last
//     ERROR_INVALID_ADDRESS.
//
// What only a bug of the engine or the host does panics: a size the engine
// does not ask for, a chunk or range the VM does not hold, a call after Close
// or after a Release that did not return. New panics too when the OS refuses
// to give back what the setup of a stack mapped itself.
package hostmem // import "github.com/liustack/slowjs/internal/hostmem"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

const (
	// Unit is what chunks are a multiple of: the engine asks for 64 KiB at a
	// time, the allocation granularity of windows.
	Unit = 64 << 10

	// DefaultStackSize is the size of a shadow stack unless the VM asks for
	// another.
	DefaultStackSize = 32 << 20

	// StackKeep is how much of the shadow stack, from its base, Release
	// leaves in memory: about what a render uses.
	StackKeep = 256 << 10
)

// pageSize is the size of an OS page: what pages handed back are rounded to,
// and the size of the guard page.
var pageSize = uintptr(os.Getpagesize())

// fault lets a test make a call to the OS fail. When it is set, each call
// asks it first, with the name of what it does and its range, and an error it
// returns stands for the error of the call. Only tests set it.
var fault func(call string, addr, size uintptr) error

func faulted(call string, addr, size uintptr) error {
	if fault == nil {
		return nil
	}
	return fault(call, addr, size)
}

// Memory is the memory of one VM: the chunks of its heap and its shadow
// stack.
type Memory struct {
	chunks    map[uintptr]uintptr // the base of each chunk to its size
	footprint uintptr             // the sum of the sizes
	stack     uintptr             // the base of the shadow stack
	stackSize uintptr             // 0 once Close unmapped it
	released  Released            // totals of Release
	onlyClose bool                // Close ran, or a Release has not returned
}

// Usage is what a VM holds from the OS.
type Usage struct {
	Chunks    int      // chunks of the heap
	Footprint uintptr  // their bytes, which the engine counts as its footprint too
	Stack     uintptr  // bytes of the shadow stack, the guard page left out
	Released  Released // what Release handed back, in total
}

// Released is what Release handed back to the OS.
type Released struct {
	Chunks uintptr // bytes of whole chunks unmapped: the footprint dropped by as much
	Pages  uintptr // bytes of free pages inside chunks, each range rounded inward to pages
	Stack  uintptr // bytes of pages of the shadow stack
}

// New reserves a shadow stack of stackSize bytes for a VM and starts with no
// chunk. stackSize is a power of two of at least Unit, and the stack is
// aligned to it, so that the engine finds the base and the space left from a
// pointer into the stack alone. The stack grows up, and the page right above
// it is a guard page, which faults when touched: the engine checks the space
// left before it uses it, so a fault there is a bug in the engine.
//
// Only the pages of the stack the engine touches take memory. On windows the
// whole stack is committed at once, so it counts against the commit limit
// from the start.
//
// When a step of the setup fails, New gives back what it still holds and
// returns the error. When the OS refuses that too, for a range New mapped
// itself, the address space is not what the package can account for, and New
// panics with both errors.
func New(stackSize uintptr) (*Memory, error) {
	if stackSize < Unit || stackSize&(stackSize-1) != 0 {
		panic(fmt.Sprintf("hostmem: a shadow stack of %d bytes, not a power of two of at least %d", stackSize, Unit))
	}
	// Room for an aligned stack within twice its size, and the guard page.
	if stackSize > (^uintptr(0)-pageSize)/2 {
		return nil, fmt.Errorf("hostmem: a shadow stack of %d bytes is larger than the address space", stackSize)
	}
	base, err := osReserveStack(stackSize)
	if err != nil {
		return nil, fmt.Errorf("hostmem: reserve a shadow stack of %d bytes: %w", stackSize, err)
	}
	return &Memory{chunks: map[uintptr]uintptr{}, stack: base, stackSize: stackSize}, nil
}

// Stack returns the base and the size of the shadow stack, 0 and 0 once Close
// unmapped it.
func (m *Memory) Stack() (base, size uintptr) {
	return m.stack, m.stackSize
}

// Usage returns what the VM holds now. After a Close that unmapped all, it is
// zero but for the totals of Release.
func (m *Memory) Usage() Usage {
	return Usage{Chunks: len(m.chunks), Footprint: m.footprint, Stack: m.stackSize, Released: m.released}
}

// Grow maps a chunk of size bytes for the heap, a multiple of Unit: it is
// host_memory_grow. When the OS refuses, it returns 0 and the error, and the
// engine runs out of memory. The engine checks its limit and its footprint
// budget before it asks.
func (m *Memory) Grow(size uintptr) (uintptr, error) {
	m.mustBeOpen()
	mustBeChunkSize(size)
	p, err := osMap(size)
	if err != nil {
		return 0, fmt.Errorf("hostmem: map a chunk of %d bytes: %w", size, err)
	}
	m.add(p, size)
	return p, nil
}

// Free unmaps the chunk of size bytes at base at once: the engine emptied it
// and took it out of its heap, as it does with a chunk that held one large
// block. When the OS refuses, the chunk stays mapped and counted, for a later
// Free or Close.
func (m *Memory) Free(base, size uintptr) error {
	m.mustBeOpen()
	return m.free(base, size)
}

func (m *Memory) free(base, size uintptr) error {
	m.mustHold(base, size)
	if err := osUnmap(base, size); err != nil {
		return fmt.Errorf("hostmem: unmap a chunk of %d bytes at %#x: %w", size, base, err)
	}
	delete(m.chunks, base)
	m.footprint -= size
	return nil
}

// Remap resizes the chunk of size bytes at base to newSize, a multiple of
// Unit, and returns where it is now. The contents up to the smaller size stay,
// and the system moves the pages rather than copy them. The engine asks for it
// when the one large block of a chunk grows. Only linux can (CanRemap): the
// error elsewhere matches errors.ErrUnsupported, and the engine maps a new
// chunk, copies and frees the old one. When the OS refuses, the chunk stays as
// it was.
func (m *Memory) Remap(base, size, newSize uintptr) (uintptr, error) {
	m.mustBeOpen()
	m.mustHold(base, size)
	mustBeChunkSize(newSize)
	p, err := osRemap(base, size, newSize)
	if err != nil {
		return 0, fmt.Errorf("hostmem: remap a chunk of %d bytes at %#x to %d: %w", size, base, newSize, err)
	}
	delete(m.chunks, base)
	m.footprint -= size
	m.add(p, newSize)
	return p, nil
}

// Release hands memory back to the OS, for VM.ReleaseMemory. chunks gives one
// by one the whole chunks the engine took out of its heap, and ranges the free
// memory inside the chunks it keeps, each until it returns a size of 0 (nil
// for none). Release unmaps the chunks, hands back the pages inside each
// range, and then the pages of the shadow stack above stackUsed bytes from its
// base, or above StackKeep when that is more. stackUsed is 0 when the VM is
// idle, or else where the innermost callback left the shadow stack.
//
// Pages handed back read as zero when next touched on linux (MADV_DONTNEED),
// and the memory they took is free at once. On darwin (MADV_FREE) and on
// windows (MEM_RESET) the system takes them when it needs memory, and until
// then they may keep what they held. Pages that were not in memory count as
// handed back too.
//
// An error of the OS on one chunk or range does not stop the others. Release
// returns them all, and a chunk the OS did not unmap stays mapped and counted.
//
// chunks and ranges do not call the Memory. When one of them panics, or
// Release does on a range outside every chunk, the walk of the engine stops
// halfway, and the Memory takes only Close from then on. What was handed back
// until then is in the totals.
func (m *Memory) Release(chunks, ranges func() (addr, size uintptr), stackUsed uintptr) (Released, error) {
	m.mustBeOpen()
	if stackUsed > m.stackSize {
		panic(fmt.Sprintf("hostmem: %d bytes of a shadow stack of %d in use", stackUsed, m.stackSize))
	}
	m.onlyClose = true
	var r Released
	var errs []error
	for chunks != nil {
		base, size := chunks()
		if size == 0 {
			break
		}
		if err := m.free(base, size); err != nil {
			errs = append(errs, err)
			continue
		}
		r.Chunks += size
		m.released.Chunks += size
	}
	for ranges != nil {
		addr, size := ranges()
		if size == 0 {
			break
		}
		if !m.inChunk(addr, size) {
			panic(fmt.Sprintf("hostmem: free range of %d bytes at %#x outside every chunk", size, addr))
		}
		start, end := alignUp(addr, pageSize), alignDown(addr+size, pageSize)
		if start >= end {
			continue
		}
		if err := osRelease(start, end-start); err != nil {
			errs = append(errs, fmt.Errorf("hostmem: hand back %d bytes at %#x: %w", end-start, start, err))
			continue
		}
		r.Pages += end - start
		m.released.Pages += end - start
	}
	if from := alignUp(max(stackUsed, StackKeep), pageSize); from < m.stackSize {
		if err := osRelease(m.stack+from, m.stackSize-from); err != nil {
			errs = append(errs, fmt.Errorf("hostmem: hand back %d bytes of the shadow stack: %w", m.stackSize-from, err))
		} else {
			r.Stack = m.stackSize - from
			m.released.Stack += r.Stack
		}
	}
	m.onlyClose = false
	return r, errors.Join(errs...)
}

// Close unmaps every chunk and the shadow stack, after the engine freed its
// runtime. From then on only Usage, Stack and Close may be called. When the OS
// refuses some, Close returns the errors and keeps those counted, so Usage
// tells what is left, and Close again tries them again. Once all is unmapped,
// Close again does nothing.
func (m *Memory) Close() error {
	m.onlyClose = true
	var errs []error
	for base, size := range m.chunks {
		if err := osUnmap(base, size); err != nil {
			errs = append(errs, fmt.Errorf("hostmem: unmap a chunk of %d bytes at %#x: %w", size, base, err))
			continue
		}
		delete(m.chunks, base)
		m.footprint -= size
	}
	if m.stackSize != 0 {
		if err := osFreeStack(m.stack, m.stackSize); err != nil {
			errs = append(errs, fmt.Errorf("hostmem: unmap the shadow stack at %#x: %w", m.stack, err))
		} else {
			m.stack, m.stackSize = 0, 0
		}
	}
	return errors.Join(errs...)
}

// lost panics: the setup of a shadow stack failed with err and the OS refused,
// with cerr, to give back the range of size bytes at addr it still held.
func lost(addr, size uintptr, err, cerr error) {
	panic(fmt.Errorf("hostmem: the setup of a shadow stack cannot give back the %d bytes at %#x it holds: %w", size, addr, errors.Join(err, cerr)))
}

func (m *Memory) add(base, size uintptr) {
	if _, ok := m.chunks[base]; ok {
		panic(fmt.Sprintf("hostmem: the OS mapped a chunk at %#x twice", base))
	}
	m.chunks[base] = size
	m.footprint += size
}

// mustHold panics unless the VM has a chunk of size bytes at base: the engine
// names only chunks it was given, so anything else is a bug in it.
func (m *Memory) mustHold(base, size uintptr) {
	if n, ok := m.chunks[base]; !ok || n != size {
		panic(fmt.Sprintf("hostmem: no chunk of %d bytes at %#x", size, base))
	}
}

// inChunk reports whether [addr, addr+size) lies in one chunk. Release is
// rare and a VM holds a few dozen chunks, so it looks at each.
func (m *Memory) inChunk(addr, size uintptr) bool {
	for base, n := range m.chunks {
		if addr >= base && addr-base < n && size <= n-(addr-base) {
			return true
		}
	}
	return false
}

func (m *Memory) mustBeOpen() {
	if m.onlyClose {
		panic("hostmem: Memory used after Close, or inside or after a Release that did not return")
	}
}

func mustBeChunkSize(size uintptr) {
	if size == 0 || size%Unit != 0 {
		panic(fmt.Sprintf("hostmem: a chunk of %d bytes, not a multiple of %d", size, Unit))
	}
}

// alignUp and alignDown round x to a multiple of a, a power of two.
func alignUp(x, a uintptr) uintptr   { return (x + a - 1) &^ (a - 1) }
func alignDown(x, a uintptr) uintptr { return x &^ (a - 1) }

// pointer turns an address outside the Go heap into a pointer, for the
// system calls of x/sys that take one and for bytesAt. The rules of
// unsafe.Pointer do not cover making a pointer from a uintptr like this: it
// holds because the garbage collector ignores pointers outside the memory it
// manages, as it does for memory from C, which is how Go works today and not
// a promise. So pointer takes only addresses the package mapped and still
// holds, and each caller checks that first: the range lies in a chunk on
// record, in the shadow stack, or in what the same setup mapped. go vet cannot
// tell such an address from a Go pointer kept in a uintptr, which would be
// wrong, so the conversion reads the address through memory.
func pointer(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

// bytesAt is the memory of size bytes at addr, an address as pointer takes.
func bytesAt(addr, size uintptr) []byte {
	return unsafe.Slice((*byte)(pointer(addr)), size)
}
