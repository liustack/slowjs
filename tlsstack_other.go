// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(linux && (amd64 || arm64))

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"unsafe"

	"modernc.org/libc"
)

// Outside linux, libc keeps the TLS stack in segments of about 4 KiB,
// each malloc'd under the one lock of libc's allocator, and frees a
// segment once the stack is two segments below it. A javascript call
// takes about 1.3 KiB of TLS frames, so a render that goes a few calls up
// and down past the end of a segment mallocs and frees segments again and
// again, and VMs on other goroutines wait for the lock.
//
// warmTLS allocates and frees one block of tlsWarm bytes right after
// NewTLS. libc keeps the block as the segment after the first, and moves
// to it whenever the first is full, so every frame up to about tlsWarm
// bytes deep goes on it and no call mallocs a segment. The block is 255
// segments, which with libc's header of a stack segment takes 256: libc's
// allocator maps it as a region of 1 MiB. Only the pages the stack reaches
// are ever written, so they are all the memory it takes, and Close frees
// it with the rest of the stack.
const tlsWarm = 255 * (1<<12 - 4*int(unsafe.Sizeof(int(0))))

func warmTLS(tls *libc.TLS) {
	tls.Alloc(tlsWarm)
	tls.Free(tlsWarm)
}

// tlsMark is where the TLS stack of a runtime is. The engine frees its TLS
// frames before it returns, not in a defer (see fixNoDefer in
// internal/fixlib), so a Go panic that goes up through engine functions
// leaves their frames on the stack. The VM marks the stack where it calls
// into javascript, and where javascript calls a Go function, and puts it
// back there when the call ends, panic or not. A frame left on the stack
// would otherwise be freed by the frames below it as their own, and libc
// would then hand out memory before the start of a segment.
//
// Outside linux, libc keeps the stack as bytes in segments: TLS.Alloc and
// TLS.Free move the top by the size rounded up to 16 bytes, and a Free that
// empties a segment goes back to the one before. The mark is the number of
// slots and the address of the top.
type tlsMark struct {
	slots int
	top   uintptr
}

// tlsRewindMax bounds the steps of restore, 1 GiB of stack: more means that
// libc no longer works as restore expects.
const tlsRewindMax = 1 << 26

// tlsTop returns the address the next TLS.Alloc returns. TLS.Alloc(0)
// moves nothing, and TLS.Free(0) neither: the top segment is never empty
// while the runtime lives, its first holds errno.
func tlsTop(tls *libc.TLS) uintptr {
	p := tls.Alloc(0)
	tls.Free(0)
	return p
}

func markTLS(tls *libc.TLS) tlsMark {
	return tlsMark{tls.StackSlots(), tlsTop(tls)}
}

// restore puts the stack back at mk, dropping the frames a panic left
// above it, and reports whether there were any. It steps back 16 bytes at a time, so that every segment it
// empties is freed as when the frames on it are.
func (mk tlsMark) restore(tls *libc.TLS) bool {
	n := tls.StackSlots()
	if n == mk.slots {
		return false
	}
	if n < mk.slots {
		panic(fmt.Sprintf("quickjs: %d TLS stack slots, fewer than the %d where the call started", n, mk.slots))
	}
	for i := 0; tlsTop(tls) != mk.top; i++ {
		if i == tlsRewindMax {
			panic("quickjs: cannot find where the TLS stack was when the call started")
		}
		tls.Free(16)
	}
	for tls.StackSlots() < mk.slots {
		tls.Alloc(0)
	}
	for tls.StackSlots() > mk.slots {
		tls.Free(0)
	}
	return true
}
