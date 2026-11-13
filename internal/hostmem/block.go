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

// A block is the memory a Go function that javascript called works in when
// it holds a copy of a long input or a long output for the call, mapped
// from the OS in place of a Go allocation (docs/changes/uncatchable.md).
// Go clears an allocation in one call that reads nothing, and the OS hands
// over the pages of memory Go gave back a page at a time while it does: 30
// ms for 16 MiB on a slow virtual machine, during which the call cannot
// stop. Mapping a block costs the same for any size, its pages are zero and
// come from the OS at their first write, in the loop that fills them, which
// reads whether the call is to stop, and giving it back costs about 0.4 ms
// for 16 MiB.
//
// A block is outside the Go heap: the garbage collector neither scans nor
// moves it, and its elements hold no pointers. Nothing may keep a slice or a
// string over it once it is given back.

// BlockMin is the least size Take maps from the OS. A shorter one is a Go
// allocation, which Go clears in less time than a piece of the work.
const BlockMin = 1 << 20

// blocks is how many blocks are mapped and not given back, for the tests.
var blocks atomic.Int64

// Blocks returns how many blocks Take mapped that are not given back.
func Blocks() int64 { return blocks.Load() }

// Elem are the types a block holds: those with no pointer, which the
// garbage collector does not look for in it.
type Elem interface {
	~bool | ~uint8 | ~uint16 | ~int32 | ~uint32 | ~int | ~uint64
}

// Mapped reports whether Take maps n elements of T from the OS: from
// BlockMin bytes. It panics, as Take does, on a negative n, and on one whose
// size overflows.
func Mapped[T Elem](n int) bool {
	var t T
	if n < 0 || uintptr(n) > ^uintptr(0)/unsafe.Sizeof(t) {
		panic(fmt.Sprintf("hostmem: Take of %d elements of %d bytes", n, unsafe.Sizeof(t)))
	}
	return uintptr(n)*unsafe.Sizeof(t) >= BlockMin
}

// Take returns n zero elements of T, for the length of a call, and the
// function that gives them back, which must be called once nothing uses them
// any more, on every path: deferred, or by the owner of the slice when it
// keeps it past the call. Calling it again does nothing. Fewer than BlockMin
// bytes are a Go allocation, and give back nothing. A larger slice is a block
// mapped from the OS; err is what the OS refused, and the caller stops the
// call with the out of memory error.
func Take[T Elem](n int) (s []T, give func(), err error) {
	if !Mapped[T](n) {
		return make([]T, n), func() {}, nil
	}
	var t T
	size := alignUp(uintptr(n)*unsafe.Sizeof(t), pageSize)
	addr, err := osMapBlock(size)
	if err != nil {
		return nil, nil, fmt.Errorf("hostmem: mapping a block of %d bytes: %w", size, err)
	}
	blocks.Add(1)
	given := false
	give = func() {
		if given {
			return
		}
		unmapBlock(addr, size)
		given = true
	}
	return unsafe.Slice((*T)(pointer(addr)), n), give, nil
}
