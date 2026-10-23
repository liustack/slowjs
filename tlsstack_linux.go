// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux && (amd64 || arm64)

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"

	"modernc.org/libc"
)

// warmTLS does nothing on linux, where libc keeps one block per slot of the
// TLS stack and does not free it when the stack shrinks.
func warmTLS(tls *libc.TLS) {}

// tlsMark is where the TLS stack of a runtime is. The engine frees its TLS
// frames before it returns, not in a defer (see fixNoDefer in
// internal/fixlib), so a Go panic that goes up through engine functions
// leaves their frames on the stack. The VM marks the stack where it calls
// into javascript, and where javascript calls a Go function, and puts it
// back there when the call ends, panic or not.
//
// On linux, libc keeps one block per slot of the stack and TLS.Free only
// steps back a slot, so the number of slots is where the stack is.
type tlsMark struct {
	slots int
}

func markTLS(tls *libc.TLS) tlsMark {
	return tlsMark{tls.StackSlots()}
}

// restore puts the stack back at mk, dropping the frames a panic left
// above it, and reports whether there were any.
func (mk tlsMark) restore(tls *libc.TLS) bool {
	n := tls.StackSlots()
	if n == mk.slots {
		return false
	}
	if n < mk.slots {
		panic(fmt.Sprintf("quickjs: %d TLS stack slots, fewer than the %d where the call started", n, mk.slots))
	}
	for ; n > mk.slots; n-- {
		tls.Free(0)
	}
	return true
}
