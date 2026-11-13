// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

// A Keep is the one block a VM keeps for its next call, so that a call that
// takes a block again finds its pages in memory: a block the OS maps anew
// hands over each page at its first write, about 6 µs a page of 4 KiB on a
// slow virtual machine, at every call. A block given back goes to the Keep
// when it is at most Max bytes and Allow lets it, the larger of two when the
// Keep has one already, and the other goes back to the OS. The block kept
// goes back to the OS after Idle has been called idle times with no block
// taken from it or kept in it, and at Close. All the Keeps of the process
// keep at most KeptMax bytes together, so that a pool of many VMs idle
// between calls holds no more.
//
// A block from a Keep is cleared, over what the call asks of it, a piece at a
// time with a read of the call's stop between two. A Keep is used by the
// goroutine running its VM, as the VM is, and needs no lock.
type Keep struct {
	// Max is the largest block the Keep keeps.
	Max uintptr
	// Allow reports whether the Keep may keep a block of size bytes now.
	Allow func(size uintptr) bool
	// Hold, when set, is told the size of the block the Keep keeps each
	// time it changes, for the memory limit of the VM to count it.
	Hold func(size uintptr)

	addr, size uintptr
	// clearing is the size of the block k kept that TakeKept is clearing
	// for a call, which k holds still until the call has it.
	clearing uintptr
	idle     int
	closed   bool
}

// KeptMax is the most bytes the Keeps of the process keep together.
const KeptMax = 64 << 20

// kept is how many bytes the Keeps of the process keep.
var kept atomic.Int64

// Kept returns how many bytes the Keeps of the process keep.
func Kept() int64 { return kept.Load() }

// clearPiece is how many bytes of a kept block are cleared between two
// reads of the call's stop: well under a millisecond, as the pages are in
// memory.
const clearPiece = 1 << 20

// Size is the size of the block k keeps, 0 for none.
func (k *Keep) Size() uintptr { return k.size }

// Held is the size of the block k holds: the one it keeps, or the one it is
// clearing for a call, which TryRelease cannot give back.
func (k *Keep) Held() uintptr { return k.size + k.clearing }

// hold tells Hold the size of the block k holds.
func (k *Keep) hold() {
	if k.Hold != nil {
		k.Hold(k.Held())
	}
}

// TakeKept is Take for a call of the VM of k: the block k keeps when it holds
// n elements of T, else a block mapped from the OS, and give hands the block
// to k. Clearing a kept block reads stop between pieces, and stopped is true,
// with the block kept, when it says the call is to end. Fewer than BlockMin
// bytes are a Go allocation, as for Take.
func TakeKept[T Elem](k *Keep, n int, stop func() bool) (s []T, give func(), stopped bool, err error) {
	if !Mapped[T](n) {
		return make([]T, n), func() {}, false, nil
	}
	var t T
	asked := uintptr(n) * unsafe.Sizeof(t)
	size := alignUp(asked, pageSize)
	var addr uintptr
	if k.size >= size {
		addr, size = k.addr, k.size
		k.addr, k.size, k.idle, k.clearing = 0, 0, 0, size
		// The block stays counted in kept, and held (clearing), while it
		// is cleared, and goes back to k when the clear does not end, by
		// a stop or a panic of stop, so that it is still k's to give back
		// within KeptMax and within what Hold was told.
		cleared := false
		defer func() {
			if !cleared {
				k.addr, k.size, k.clearing = addr, size, 0
			}
		}()
		b := unsafe.Slice((*byte)(pointer(addr)), asked)
		for lo := uintptr(0); lo < asked; lo += clearPiece {
			if lo > 0 && stop() {
				return nil, nil, true, nil
			}
			clear(b[lo:min(asked, lo+clearPiece)])
		}
		cleared = true
		k.clearing = 0
		kept.Add(-int64(size))
		k.hold()
	} else {
		if addr, err = osMapBlock(size); err != nil {
			return nil, nil, false, fmt.Errorf("hostmem: mapping a block of %d bytes: %w", size, err)
		}
		blocks.Add(1)
	}
	given := false
	give = func() {
		if given {
			return
		}
		// The block is given once k keeps it or the OS took it back, so
		// that a give called again after the OS refused a block tries
		// again.
		if !k.put(addr, size) {
			unmapBlock(addr, size)
		}
		given = true
	}
	return unsafe.Slice((*T)(pointer(addr)), n), give, false, nil
}

// put keeps the block at addr, in place of the smaller one k kept, which goes
// back to the OS, when k may and the Keeps of the process stay within KeptMax
// with it. It reports whether k keeps the block. When the OS refuses the
// block k kept, put panics with k as it was, keeping that block, and the
// block at addr is the caller's still.
func (k *Keep) put(addr, size uintptr) bool {
	if k.closed || size > k.Max || size <= k.size || !k.Allow(size) {
		return false
	}
	// kept never counts more than KeptMax, even for a moment: another
	// Keep reads it to know whether there is room.
	more := int64(size - k.size)
	for {
		n := kept.Load()
		if n > KeptMax-more {
			return false
		}
		if kept.CompareAndSwap(n, n+more) {
			break
		}
	}
	if k.size != 0 {
		if err := osUnmap(k.addr, k.size); err != nil {
			kept.Add(-more)
			panic(unmapError(k.addr, k.size, err))
		}
		blocks.Add(-1)
	}
	k.addr, k.size, k.idle = addr, size, 0
	k.hold()
	return true
}

// Idle counts a call of the VM that ended, and gives the block k keeps back
// to the OS when idle calls have ended since it was last kept or taken.
func (k *Keep) Idle(idle int) {
	if k.size == 0 {
		return
	}
	if k.idle++; k.idle >= idle {
		k.Release()
	}
}

// Release gives the block k keeps back to the OS. It panics when the OS
// refuses, with the block still kept, for a Release called again.
func (k *Keep) Release() {
	if err := k.TryRelease(); err != nil {
		panic(err)
	}
}

// TryRelease is Release returning the error of the OS refusing the block,
// for a caller that must not panic.
func (k *Keep) TryRelease() error {
	if k.size == 0 {
		return nil
	}
	if err := osUnmap(k.addr, k.size); err != nil {
		return unmapError(k.addr, k.size, err)
	}
	blocks.Add(-1)
	kept.Add(-int64(k.size))
	k.addr, k.size, k.idle = 0, 0, 0
	k.hold()
	return nil
}

// Close gives the block k keeps back to the OS, and each block given to k
// from now on.
func (k *Keep) Close() {
	k.closed = true
	k.Release()
}

// unmapBlock gives a block back to the OS.
func unmapBlock(addr, size uintptr) {
	if err := osUnmap(addr, size); err != nil {
		panic(unmapError(addr, size, err))
	}
	blocks.Add(-1)
}

// unmapError is the error of the OS refusing to take a block back.
func unmapError(addr, size uintptr, err error) error {
	return fmt.Errorf("hostmem: giving back the block of %d bytes at %#x: %w", size, addr, err)
}
