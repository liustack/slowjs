// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"unsafe"

	"github.com/liustack/quickjs/internal/hostmem"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The functions of internal/vmhook, with which the Go functions of packages
// intl, temporal, fmt and url keep the limits of the VM that called them.
func init() {
	vmhook.Stop = func(vm any) bool { return hookVM(vm).goStop() }
	vmhook.Room = func(vm any) int64 { return hookVM(vm).goRoom() }
	vmhook.OutOfMemory = func(vm any) { hookVM(vm).goOutOfMemory() }
	vmhook.Chars = func(args any, i int) ([]byte, []uint16, func(), bool) { return hookArgs(args).chars(i) }
	vmhook.LongWTF8 = func(args any, i int) (string, func(), bool) { return hookArgs(args).longWTF8(i) }
	vmhook.Take = func(vm any, size int) (unsafe.Pointer, func(), bool) { return hookVM(vm).goTake(size) }
}

// hookVM is the VM a function of vmhook was given, which has to be running a
// Go function that javascript called.
func hookVM(vm any) *VM {
	m := vm.(*VM)
	if m.host == 0 {
		panic("quickjs: a function of internal/vmhook outside a Go function called from javascript")
	}
	return m
}

// goStop is vmhook.Stop.
func (m *VM) goStop() bool {
	if !m.h.Stopping() {
		return m.uncatchablePending()
	}
	if !m.uncatchablePending() {
		// What the engine throws where it reads the interrupt flag.
		m.jsThrowError(errorKindInternal, "interrupted")
		m.jsSetUncatchableException(true)
	}
	return true
}

// goRoom is vmhook.Room.
func (m *VM) goRoom() int64 {
	if m.memoryLimit == 0 {
		return -1
	}
	if n := m.jsGetMemorySize(); n < m.memoryLimit {
		return int64(m.memoryLimit - n)
	}
	return 0
}

// A Go function that javascript called takes the memory for a long input or
// output of its call from goTake: from hostmem.BlockMin bytes, a block mapped
// from the OS outside Go's heap, which a stop can end the call in while it is
// filled (docs/changes/uncatchable.md). The VM keeps the last block given
// back, of at most keepMax bytes, for the next call that takes one, which
// finds its pages in memory: the OS hands over the pages of a new block one
// at a time as they are written, 25 to 40 ms for 16 MiB on the virtual M1
// of GitHub's macos-15 runner, at every call. Only a VM with no memory limit
// keeps a block: the engine counts its memory against the limit where it
// allocates, in C, which knows nothing of the block, so a VM with a limit
// and a block kept could pass the limit by the block. m gives the block
// back to the OS after keepIdle calls that took none, at SetMemoryLimit
// with a limit, and at Close.
const (
	keepMax  = 32 << 20
	keepIdle = 16
)

// goTake is vmhook.Take: size bytes, cleared, and the function that gives
// them back to m.keep. It is false, with the error that ends the call
// pending, when the call is to stop or the OS refuses the memory.
func (m *VM) goTake(size int) (unsafe.Pointer, func(), bool) {
	b, give, stopped, err := hostmem.TakeKept[byte](&m.keep, size, m.goStop)
	switch {
	case err != nil:
		m.goOutOfMemory()
		return nil, nil, false
	case stopped:
		return nil, nil, false
	}
	return unsafe.Pointer(unsafe.SliceData(b)), give, true
}

// mayKeep reports whether m.keep may keep a block: when m has no memory
// limit and is not broken.
func (m *VM) mayKeep(uintptr) bool {
	return m.memoryLimit == 0 && !m.h.Broken()
}

// goOutOfMemory is vmhook.OutOfMemory.
func (m *VM) goOutOfMemory() {
	if !m.uncatchablePending() {
		m.jsThrowOutOfMemory()
	}
}

// longWTF8 is vmhook.LongWTF8.
func (a Args) longWTF8(i int) (string, func(), bool) {
	switch v := a.arg(i); tag(v) {
	case tagString, tagStringRope:
		return a.vm.argBlock(v, true)
	}
	return "", func() {}, false
}

// hookArgs is the Args a function of vmhook was given, of a Go function that
// javascript called and that is running.
func hookArgs(args any) Args {
	a := args.(Args)
	hookVM(a.vm)
	return a
}
