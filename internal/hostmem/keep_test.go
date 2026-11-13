// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"testing"
	"unsafe"
)

func newKeep() *Keep {
	return &Keep{Max: 8 * BlockMin, Allow: func(uintptr) bool { return true }}
}

func never() bool { return false }

// A block given back to a Keep serves the next take of its size or less,
// cleared over what is asked, and goes back to the OS at Close.
func TestKeepReuses(t *testing.T) {
	held := Blocks()
	k := newKeep()
	const n = 2 * BlockMin
	b, give, stopped, err := TakeKept[byte](k, n, never)
	if err != nil || stopped {
		t.Fatal(err, stopped)
	}
	for i := range b {
		b[i] = 0xff
	}
	at := unsafe.SliceData(b)
	give()
	give()
	if k.Size() != n || Blocks() != held+1 {
		t.Fatalf("after give: %d bytes kept, %d blocks held, want %d and 1", k.Size(), Blocks()-held, n)
	}
	c, give, _, _ := TakeKept[uint16](k, n/2-1, never)
	if unsafe.Pointer(unsafe.SliceData(c)) != unsafe.Pointer(at) || k.Size() != 0 || Blocks() != held+1 {
		t.Fatalf("a take of the kept size did not take the kept block: %d bytes kept, %d blocks held", k.Size(), Blocks()-held)
	}
	for i, x := range c {
		if x != 0 {
			t.Fatalf("element %d of a kept block is %#x, want 0", i, x)
		}
	}
	give()
	k.Close()
	if k.Size() != 0 || Blocks() != held {
		t.Fatalf("after Close: %d bytes kept, %d blocks held, want 0", k.Size(), Blocks()-held)
	}
	_, give, _, _ = TakeKept[byte](k, n, never)
	give()
	if k.Size() != 0 || Blocks() != held {
		t.Fatalf("a block given after Close: %d bytes kept, %d blocks held, want 0", k.Size(), Blocks()-held)
	}
}

// TakeKept with a short n is TakeKept[T] with no block, kept or mapped.
func TestKeepShort(t *testing.T) {
	held := Blocks()
	k := newKeep()
	s, give, _, _ := TakeKept[uint64](k, BlockMin/8-1, never)
	give()
	if len(s) != BlockMin/8-1 || k.Size() != 0 || Blocks() != held {
		t.Fatalf("a short take: len %d, %d bytes kept, %d blocks held", len(s), k.Size(), Blocks()-held)
	}
}

// A Keep keeps the larger of two blocks, at most Max bytes and only those
// Allow lets it, and gives the others back to the OS.
func TestKeepChooses(t *testing.T) {
	held := Blocks()
	k := newKeep()
	_, small, _, _ := TakeKept[byte](k, BlockMin, never)
	_, large, _, _ := TakeKept[byte](k, 2*BlockMin, never)
	_, huge, _, _ := TakeKept[byte](k, 2*int(k.Max), never)
	small()
	large()
	huge()
	if k.Size() != 2*BlockMin || Blocks() != held+1 {
		t.Fatalf("%d bytes kept, %d blocks held, want %d and 1", k.Size(), Blocks()-held, 2*BlockMin)
	}
	k.Release()
	k.Allow = func(size uintptr) bool { return size < BlockMin*2 }
	_, large, _, _ = TakeKept[byte](k, 2*BlockMin, never)
	large()
	if k.Size() != 0 || Blocks() != held {
		t.Fatalf("a block Allow refused: %d bytes kept, %d blocks held, want 0", k.Size(), Blocks()-held)
	}
}

// The kept block goes back to the OS after idle calls that took none, and a
// take or a keep starts the count again.
func TestKeepIdle(t *testing.T) {
	held := Blocks()
	k := newKeep()
	_, give, _, _ := TakeKept[byte](k, BlockMin, never)
	give()
	k.Idle(3)
	k.Idle(3)
	_, give, _, _ = TakeKept[byte](k, BlockMin, never)
	give()
	k.Idle(3)
	k.Idle(3)
	if k.Size() == 0 {
		t.Fatal("the block was given back after 2 idle calls since it was kept again, want 3")
	}
	k.Idle(3)
	if k.Size() != 0 || Blocks() != held {
		t.Fatalf("after 3 idle calls: %d bytes kept, %d blocks held, want 0", k.Size(), Blocks()-held)
	}
}

// Clearing a kept block reads stop between pieces of clearPiece bytes, and
// a stop leaves the block kept, and counted as kept all along, so that no
// other Keep takes its share of KeptMax meanwhile.
func TestKeepStops(t *testing.T) {
	held, before := Blocks(), Kept()
	k := newKeep()
	const n = 4 * clearPiece
	_, give, _, _ := TakeKept[byte](k, n, never)
	give()
	reads := 0
	_, _, stopped, err := TakeKept[byte](k, n, func() bool {
		if reads++; Kept() != before+n {
			t.Errorf("while a kept block is cleared, the Keeps keep %d bytes, want %d", Kept()-before, n)
		}
		return reads == 2
	})
	if err != nil || !stopped || reads != 2 {
		t.Fatalf("a stop at the second read: stopped %v, %d reads, %v", stopped, reads, err)
	}
	if k.Size() != n || Kept() != before+n || Blocks() != held+1 {
		t.Fatalf("after a stop: %d bytes kept, %d in the process, %d blocks held, want %d, %d and 1", k.Size(), Kept()-before, Blocks()-held, n, n)
	}
	reads = 0
	_, give, stopped, _ = TakeKept[byte](k, n, func() bool { reads++; return false })
	if stopped || reads != n/clearPiece-1 {
		t.Fatalf("a take of %d kept bytes read stop %d times, want %d", n, reads, n/clearPiece-1)
	}
	give()
	k.Close()
}

// The Keeps of the process keep at most KeptMax bytes together: a block
// that would pass it goes back to the OS.
func TestKeepProcessCap(t *testing.T) {
	held, before := Blocks(), Kept()
	if before != 0 {
		t.Fatalf("the Keeps of the test keep %d bytes before it", before)
	}
	const size = KeptMax / 2
	var keeps [3]*Keep
	var gives [3]func()
	for i := range keeps {
		keeps[i] = &Keep{Max: size, Allow: func(uintptr) bool { return true }}
		_, give, _, err := TakeKept[byte](keeps[i], size, never)
		if err != nil {
			t.Fatal(err)
		}
		gives[i] = give
	}
	for _, give := range gives {
		give()
	}
	if Kept() != KeptMax || keeps[2].Size() != 0 || Blocks() != held+2 {
		t.Fatalf("three blocks of KeptMax/2 given: %d bytes kept, the third keeps %d, %d blocks held, want %d, 0 and 2", Kept(), keeps[2].Size(), Blocks()-held, KeptMax)
	}
	keeps[0].Close()
	_, give, _, _ := TakeKept[byte](keeps[2], size, never)
	give()
	if Kept() != KeptMax || keeps[2].Size() != size {
		t.Fatalf("a block given once there was room: %d bytes kept, the third keeps %d", Kept(), keeps[2].Size())
	}
	for _, k := range keeps {
		k.Close()
	}
	if Kept() != 0 || Blocks() != held {
		t.Fatalf("after Close: %d bytes kept, %d blocks held, want 0", Kept(), Blocks()-held)
	}
}
