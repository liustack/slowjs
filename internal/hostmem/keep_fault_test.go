// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package hostmem

import (
	"testing"
	"unsafe"
)

// When the OS refuses the smaller block a Keep gives up for a larger one, the
// Keep keeps the smaller one, give panics and a give called again keeps the
// larger one, so that every block goes back to the OS at Close.
func TestKeepSwapRefused(t *testing.T) {
	held, before := Blocks(), Kept()
	k := newKeep()
	small, giveSmall, _, _ := TakeKept[byte](k, BlockMin, never)
	_, giveLarge, _, _ := TakeKept[byte](k, 2*BlockMin, never)
	giveSmall()
	at := uintptr(unsafe.Pointer(unsafe.SliceData(small)))
	refused := 0
	failing(t, func(c call, before []call) error {
		if c.name == "unmap" && c.addr == at && refused == 0 {
			refused++
			return errInjected
		}
		return nil
	})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("give did not panic when the OS refused the block it gives up")
			}
		}()
		giveLarge()
	}()
	if k.Size() != BlockMin || Kept() != before+BlockMin || Blocks() != held+2 {
		t.Fatalf("after the refusal: %d bytes kept, %d in the process, %d blocks held, want %d, %d and 2", k.Size(), Kept()-before, Blocks()-held, BlockMin, BlockMin)
	}
	giveLarge()
	if k.Size() != 2*BlockMin || Kept() != before+2*BlockMin || Blocks() != held+1 {
		t.Fatalf("give again: %d bytes kept, %d in the process, %d blocks held, want %d, %d and 1", k.Size(), Kept()-before, Blocks()-held, 2*BlockMin, 2*BlockMin)
	}
	k.Close()
	if Kept() != before || Blocks() != held {
		t.Fatalf("after Close: %d bytes kept, %d blocks held, want 0", Kept()-before, Blocks()-held)
	}
}
