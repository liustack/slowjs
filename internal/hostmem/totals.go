// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import "sync/atomic"

// Totals is what every Memory of the process and every read-only mapping
// hold from the OS together. Once every VM is closed and every shared copy
// freed, all of it is zero, unless the OS refused to unmap something, which
// then stays counted until a later Close unmaps it. A long run reads it to
// see that what the VMs hold stays bounded, and that closing them all gives
// everything back.
type Totals struct {
	Memories int64 // Memories whose shadow stack is mapped
	Stacks   int64 // the bytes of those shadow stacks, the guard pages left out
	Chunks   int64 // chunks of the heaps of all Memories
	Heap     int64 // their bytes: the sum of the footprints
	ReadOnly int64 // bytes of the mappings of MapReadOnly not unmapped
}

var totalMemories, totalStacks, totalChunks, totalHeap, totalReadOnly atomic.Int64

// ProcessTotals returns the Totals of the process now. Each count is read on
// its own, so while other goroutines map or unmap, they may not all be of the
// same instant.
func ProcessTotals() Totals {
	return Totals{
		Memories: totalMemories.Load(),
		Stacks:   totalStacks.Load(),
		Chunks:   totalChunks.Load(),
		Heap:     totalHeap.Load(),
		ReadOnly: totalReadOnly.Load(),
	}
}

// countChunk adds n chunks of size bytes each to the totals (n is 1 or -1).
func countChunk(size uintptr, n int64) {
	totalChunks.Add(n)
	totalHeap.Add(n * int64(size))
}

// countStack adds n shadow stacks of size bytes each to the totals.
func countStack(size uintptr, n int64) {
	totalMemories.Add(n)
	totalStacks.Add(n * int64(size))
}
