// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// SharedBytecode is bytecode of [VM.Compile] that several VMs load with
// [VM.EvalSharedBytecode] from one copy. The source text of each function,
// which its toString returns, and the table that maps its bytecode to lines,
// which error stacks read, stay in that copy: a VM that loads it does not
// copy them into its own memory, as [VM.EvalBytecode] does. So they count
// once for the process, outside the memory of every VM: [VM.SetMemoryLimit]
// and [VM.MemorySize] do not count them. Everything else a VM reads from the
// bytecode, the code of its functions among it, is still its own.
//
// The copy lives outside Go's heap. It stays while the SharedBytecode is open
// or a VM that loaded it is: Close gives up the hold of the SharedBytecode,
// and each VM gives up its own when it is closed. The last of them frees it.
//
// A SharedBytecode is safe for use by several goroutines at once, and so
// several VMs on several goroutines can load it at the same time.
type SharedBytecode struct {
	mu sync.Mutex
	p  uintptr // the bytes, in C memory, 0 once freed
	n  int
	// refs counts the holds on the bytes: the one of the SharedBytecode until
	// Close, and one for each VM that loaded it until that VM is closed.
	refs   int
	closed bool
}

// errSharedClosed is the error of loading a SharedBytecode after its Close.
var errSharedClosed = errors.New("slowjs: the shared bytecode is closed")

// NewSharedBytecode copies bytecode, which [VM.Compile] returned, once, for
// VMs to load with [VM.EvalSharedBytecode]. As with [VM.EvalBytecode], the
// bytecode is not checked, so it must not come from an untrusted source.
func NewSharedBytecode(bytecode []byte) (*SharedBytecode, error) {
	n := len(bytecode)
	tls := libc.NewTLS()
	defer tls.Close()
	p := libc.Xmalloc(tls, libc.Tsize_t(max(n, 1)))
	if p == 0 {
		return nil, fmt.Errorf("slowjs: out of memory for %d bytes of shared bytecode", n)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), n), bytecode)
	return &SharedBytecode{p: p, n: n, refs: 1}, nil
}

// Len returns the size of the bytecode in bytes, the memory it takes once
// for the process.
func (b *SharedBytecode) Len() int { return b.n }

// Close gives up the hold of b on its bytes: they are freed now if no VM that
// loaded them is open, or else when the last of them is closed. Loading b
// after Close fails. Closing b again does nothing.
func (b *SharedBytecode) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.dropLocked()
	return nil
}

// hold takes a hold on the bytes for a VM that loads them, unless b is
// closed.
func (b *SharedBytecode) hold() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	b.refs++
	return true
}

// drop gives up a hold that hold took, once the runtime of the VM, and every
// function in it that points into the bytes, is freed.
func (b *SharedBytecode) drop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropLocked()
}

func (b *SharedBytecode) dropLocked() {
	if b.refs--; b.refs > 0 {
		return
	}
	tls := libc.NewTLS()
	libc.Xfree(tls, b.p)
	tls.Close()
	b.p = 0
}

// EvalSharedBytecode is like [VM.EvalBytecode], but loads b without copying
// the source text and line tables of its functions into m (see
// [SharedBytecode]). m holds b until it is closed.
func (m *VM) EvalSharedBytecode(b *SharedBytecode) (r any, err error) {
	v, err := m.EvalSharedBytecodeValue(b)
	if err != nil {
		return nil, err
	}

	return m.value(v.v, true)
}

// EvalSharedBytecodeValue is like [VM.EvalBytecodeValue], but loads b as
// [VM.EvalSharedBytecode] does.
func (m *VM) EvalSharedBytecodeValue(b *SharedBytecode) (r Value, err error) {
	if !b.hold() {
		return r, errSharedClosed
	}
	// The functions read from b point into it until the runtime is freed, also
	// when the read fails half way, so m keeps its hold until Close.
	m.shared = append(m.shared, b)
	return m.evalBytecode(b.p, b.n, lib.MJS_READ_OBJ_BYTECODE|lib.ReadObjSharedDebug)
}
