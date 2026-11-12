// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import "github.com/liustack/quickjs/internal/vmhook"

// The functions of internal/vmhook, with which the Go functions of packages
// intl, temporal and fmt keep the limits of the VM that called them.
func init() {
	vmhook.Stop = func(vm any) bool { return hookVM(vm).goStop() }
	vmhook.Room = func(vm any) int64 { return hookVM(vm).goRoom() }
	vmhook.OutOfMemory = func(vm any) { hookVM(vm).goOutOfMemory() }
	vmhook.StringLen = func(args any, i int) (int, bool) { return hookArgs(args).stringLen(i) }
	vmhook.Chars = func(args any, i int) ([]byte, []uint16, func(), bool) { return hookArgs(args).chars(i) }
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

// goOutOfMemory is vmhook.OutOfMemory.
func (m *VM) goOutOfMemory() {
	if !m.uncatchablePending() {
		m.jsThrowOutOfMemory()
	}
}

// hookArgs is the Args a function of vmhook was given, of a Go function that
// javascript called and that is running.
func hookArgs(args any) Args {
	a := args.(Args)
	hookVM(a.vm)
	return a
}
