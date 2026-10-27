// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "errors"

// MemoryUsage is what a VM holds, as its engine counts it (see
// [VM.MemoryUsage]). Sizes are in bytes.
type MemoryUsage struct {
	// MallocSize is what the VM's allocations take, which [VM.MemorySize]
	// returns too, and MallocCount how many there are. MallocLimit is the
	// limit of [VM.SetMemoryLimit], -1 for none.
	MallocSize, MallocLimit, MallocCount int64
	// MemoryUsedSize and MemoryUsedCount are what the objects below take,
	// counted one by one.
	MemoryUsedSize, MemoryUsedCount int64

	AtomCount, AtomSize int64
	StrCount, StrSize   int64
	ObjCount, ObjSize   int64
	PropCount, PropSize int64
	ShapeCount          int64
	ShapeSize           int64
	// The functions of javascript, their bytecode and their line tables.
	JSFuncCount, JSFuncSize, JSFuncCodeSize int64
	JSFuncPC2LineCount, JSFuncPC2LineSize   int64
	// Functions of the engine and of Go.
	CFuncCount int64
	// ArrayCount counts the arrays, FastArrayCount those that keep their
	// elements in a plain list, and FastArrayElements those elements.
	ArrayCount, FastArrayCount, FastArrayElements int64
	// ArrayBuffers and typed arrays.
	BinaryObjectCount, BinaryObjectSize int64

	// What the VM took from the system, its limit (0 for none), in how many
	// chunks, and the bytes [VM.ReleaseMemory] offered back so far. The
	// engine of lib takes its memory from libc and leaves all four 0.
	Footprint, FootprintLimit, Chunks, ReleaseCandidate int64
}

// MemoryUsage returns what m holds, in more detail than [VM.MemorySize]. It
// walks every object, so it is for measurements, not for a hot path. Run
// [VM.RunGC] first to leave out what javascript can no longer reach.
//
// As with SetGCThreshold, no other goroutine may use m meanwhile (see [VM]),
// and a call after Close panics.
func (m *VM) MemoryUsage() MemoryUsage {
	return m.memoryUsage()
}

// ReleaseMemory gives memory that m keeps free back to the system, all but
// keep bytes of it, and returns how many bytes it gave back. The objects of m
// stay as they are. Call it when m has done a large piece of work, such as
// after loading a bundle or after [VM.RunGC]: the memory comes back when m
// needs it again. On the engine of lib, which takes its memory from libc, it
// gives nothing back and returns 0.
//
// It returns an error while m runs javascript, as from a Go function that
// javascript called. As with SetGCThreshold, no other goroutine may use m
// meanwhile (see [VM]).
func (m *VM) ReleaseMemory(keep uintptr) (uintptr, error) {
	if m.depth != 0 {
		return 0, errors.New("slowjs: ReleaseMemory while the VM runs javascript")
	}

	return m.releaseMemory(keep), nil
}
