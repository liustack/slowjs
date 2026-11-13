// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"testing"
	"unsafe"
)

// Below BlockMin bytes Take is a Go allocation and maps nothing. From it,
// Take maps a block of zeros, which the slice writes and reads, and give
// unmaps it once, however often it is called.
func TestTake(t *testing.T) {
	held := Blocks()
	s, give, err := Take[uint16](BlockMin/2 - 1)
	if err != nil {
		t.Fatal(err)
	}
	if Blocks() != held {
		t.Fatalf("Take of %d bytes mapped a block", 2*len(s))
	}
	give()

	const n = 3 * BlockMin / 2
	u, give, err := Take[uint16](n)
	if err != nil {
		t.Fatal(err)
	}
	if Blocks() != held+1 {
		t.Fatalf("Take of %d bytes: %d blocks held, want %d", 2*n, Blocks(), held+1)
	}
	if len(u) != n || cap(u) != n {
		t.Fatalf("Take(%d): len %d cap %d", n, len(u), cap(u))
	}
	for i := range u {
		if u[i] != 0 {
			t.Fatalf("element %d of a new block is %#x", i, u[i])
		}
		u[i] = uint16(i)
	}
	for i := range u {
		if u[i] != uint16(i) {
			t.Fatalf("element %d reads %#x, %#x written", i, u[i], uint16(i))
		}
	}
	give()
	give()
	if Blocks() != held {
		t.Fatalf("after give, %d blocks held, want %d", Blocks(), held)
	}
}

// A block takes no page from the OS until it is written: mapping it costs the
// same for any size, and the pages come in the loop that fills it.
func TestTakeTakesNoPage(t *testing.T) {
	const n = 64 << 20
	b, give, err := Take[byte](n)
	if err != nil {
		t.Fatal(err)
	}
	defer give()
	addr := uintptr(unsafe.Pointer(unsafe.SliceData(b)))
	if r := resident(t, addr, n); r != 0 {
		t.Fatalf("a new block of 64 MiB has %d pages in memory, want 0", r)
	}
	for i := 0; i < BlockMin; i += int(pageSize) {
		b[i] = 1
	}
	if r := resident(t, addr, n); r == 0 || uintptr(r) > (BlockMin/pageSize)*uintptr(touchPages) {
		t.Fatalf("1 MiB written of a block of 64 MiB: %d pages in memory, want 1 to %d", r, (BlockMin/pageSize)*uintptr(touchPages))
	}
}

// Take panics on a negative count, and on one whose size overflows.
func TestTakePanics(t *testing.T) {
	for _, n := range []int{-1, int(^uint(0) >> 1)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Take[int32](%d) did not panic", n)
				}
			}()
			Take[int32](n)
		}()
	}
}
