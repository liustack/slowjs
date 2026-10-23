// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// The front cache of a runtime sits in front of the engine's small block
// allocator (__js_malloc and __js_free, which cut blocks of 16 to 512 bytes
// out of arenas of 4 KiB, one list of arenas for each block size). A freed
// small block goes on the list of its size in the cache instead of back to
// its arena, and the next allocation of that size takes it from there, so
// neither touches the arena's free list, its count of used blocks or the
// lists of arenas. internal/fixlib wires it in (see fixFrontCache):
//
//   - The cache is in the state of the runtime (see runtime.go), so each
//     runtime has its own, used only by the goroutine that runs the
//     runtime, as the allocator itself is.
//   - js_malloc, js_malloc_rt and __js_malloc call frontTake first,
//     __js_free calls frontGive first.
//   - JS_RunGC calls frontDrain last, which gives every cached block back
//     to its arena, so that after a GC an arena with no block in use is
//     freed and the memory in use is what it was without the cache.
//   - js_malloc_rt and js_realloc_rt call frontRetry when they run out of
//     memory: past the memory limit, it gives the cache back and they try
//     once more if the cache held a block, so that blocks in the cache do
//     not make an allocation of another size fail.
//   - JS_FreeRuntime calls frontOff first, which gives the cache back and
//     turns it off, so that what JS_FreeRuntime frees goes to the arenas.
//
// A cached block keeps its header as the allocator wrote it, so to
// js_malloc_usable_size and to the arena it is a block in use. The pointer
// to the next block of the list is in the first bytes after the header.
// The memory limit counts arenas, which a cached block keeps, so the count
// does not change but that an arena with a cached block in it is not
// freed until the next GC.

// frontMax is the number of blocks of one size the cache keeps at most, so
// the blocks in the cache take 1.6 MiB at most.
const frontMax = 256

// frontHeader is the size of the header of a block, JSMallocBlockHeader.
const frontHeader = 8

// frontLargest is the largest size of an allocation that gets a small
// block: with its header, aligned, it takes MJS_MALLOC_MAX_SMALL_SIZE.
const frontLargest = MJS_MALLOC_MAX_SMALL_SIZE - frontHeader

// frontLists is the number of lists of a cache, a power of two at least
// MJS_MALLOC_BLOCK_SIZE_COUNT, so that an index masked with frontLists-1
// needs no bounds check.
const frontLists = 32

// TfrontCache is the front cache of a runtime.
type TfrontCache struct {
	head  [frontLists]uintptr // the header of the first block of each list
	count [frontLists]int32   // the length of each list, frontMax while the cache is off
	off   bool                // the cache is turned off for good
}

// frontIndex maps (size+7)>>3 of an allocation of 1 to frontLargest bytes
// to the index of its block size, as __js_malloc sizes the block.
var frontIndex = func() (t [64]uint8) {
	for i := 1; i < len(t); i++ {
		t[i] = uint8(_get_block_size_index(nil, Tsize_t(i)*MJS_MALLOC_ALIGN+frontHeader))
	}
	return t
}()

// frontCacheOf returns the front cache of the runtime whose malloc_ctx is
// at s.
func frontCacheOf(s uintptr) *TfrontCache {
	return &forkOf(s).front
}

// frontTake returns a cached block for an allocation of size, or 0. It sizes
// the block as __js_malloc does. It is small enough for Go to inline it
// into js_malloc, js_malloc_rt and __js_malloc.
func frontTake(s uintptr, size Tsize_t) uintptr {
	if MJS_MALLOC_LARGE_BLOCKS_ONLY != 0 || size-1 >= frontLargest {
		return 0
	}
	fc := &(*TJSRuntime)(unsafe.Pointer(s)).Ffork.front
	idx := frontIndex[(size+MJS_MALLOC_ALIGN-1)/MJS_MALLOC_ALIGN%64] % frontLists
	b := fc.head[idx]
	if b == 0 {
		return 0
	}
	fc.head[idx] = *(*uintptr)(unsafe.Pointer(b + frontHeader))
	fc.count[idx]--
	return b + frontHeader
}

// frontGive puts the block at ptr into the cache and reports whether it
// did. A large block, the block of size 0, and a block of a full list are
// left to __js_free, and so is every block while the cache is off. It is
// small enough for Go to inline it into __js_free.
func frontGive(s, ptr uintptr) bool {
	b := ptr - frontHeader
	if ptr == 0 || *(*Tuint16_t)(unsafe.Pointer(b)) == MFREE_NIL {
		return false
	}
	fc := &(*TJSRuntime)(unsafe.Pointer(s)).Ffork.front
	idx := (*TJSMallocBlockHeader)(unsafe.Pointer(b)).Fblock_size_idx % frontLists
	if fc.count[idx] >= frontMax {
		return false
	}
	*(*uintptr)(unsafe.Pointer(ptr)) = fc.head[idx]
	fc.head[idx] = b
	fc.count[idx]++
	return true
}

// frontDrain gives every block in the cache of the runtime at s back to its
// arena and reports whether there was any. While it does, every list counts
// as full, so that __js_free does not put the blocks back into the cache.
func frontDrain(tls *libc.TLS, s uintptr) bool {
	fc := frontCacheOf(s)
	for i := range fc.count {
		fc.count[i] = frontMax
	}
	any := false
	for i := range fc.head {
		for b := fc.head[i]; b != 0; {
			next := *(*uintptr)(unsafe.Pointer(b + frontHeader))
			___js_free(tls, s, b+frontHeader)
			b = next
			any = true
		}
		fc.head[i] = 0
	}
	if !fc.off {
		fc.count = [frontLists]int32{}
	}
	return any
}

// frontRetry is called when an allocation of the runtime at s failed. When
// the runtime has a memory limit, past which js_def_malloc fails, it gives
// the cache back and reports whether there was any block in it, so that
// the allocation is tried once more. Without a limit, a failure is not
// about memory the cache holds, and it does nothing.
func frontRetry(tls *libc.TLS, s uintptr) bool {
	if (*TJSRuntime)(unsafe.Pointer(s)).Fmalloc_ctx.Fmalloc_state.Fmalloc_limit == ^Tsize_t(0) {
		return false
	}
	return frontDrain(tls, s)
}

// frontOff gives the cache of the runtime at s back and turns it off.
func frontOff(tls *libc.TLS, s uintptr) {
	frontCacheOf(s).off = true
	frontDrain(tls, s)
}
