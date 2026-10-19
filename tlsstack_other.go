// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(linux && (amd64 || arm64 || loong64 || ppc64le || s390x || riscv64 || 386 || arm))

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"

	"modernc.org/libc"
)

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
