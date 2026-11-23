// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faultstate

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// queried reports whether Check asks the system.
const queried = true

// contextFlags are the flags of the CONTEXT windows saved at a fault in the
// measure (CONTEXT_ARM64 with control, integer, floating point, debug
// registers and XSTATE, 0x0040002F).
const contextFlags = 0x400000 | 0xf | 0x20

// Check asks windows, once a process, for the extended state it turns on and
// the size of the CONTEXT with it, and returns what Decide says of them.
func Check() error { return check() }

var check = sync.OnceValue(func() error {
	mask, size, err := Query()
	return Decide(mask, size, err)
})

// Query returns the extended CPU state windows turns on
// (GetEnabledXStateFeatures) and the size of a CONTEXT that holds it
// (InitializeContext2 with no buffer, which fails with
// ERROR_INSUFFICIENT_BUFFER and gives the size; any other answer leaves it
// undefined and is an error).
func Query() (mask uint64, size uint32, err error) {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	features := k32.NewProc("GetEnabledXStateFeatures")
	initialize := k32.NewProc("InitializeContext2")
	if err := features.Find(); err != nil {
		return 0, 0, err
	}
	if err := initialize.Find(); err != nil {
		return 0, 0, err
	}
	m, _, _ := features.Call()
	var ctx uintptr
	r, _, e := initialize.Call(0, contextFlags, uintptr(unsafe.Pointer(&ctx)), uintptr(unsafe.Pointer(&size)), m)
	if r != 0 {
		return 0, 0, errors.New("InitializeContext2 with no buffer succeeded and gave no size")
	}
	if !errors.Is(e, windows.ERROR_INSUFFICIENT_BUFFER) {
		return 0, 0, fmt.Errorf("InitializeContext2 with the extended CPU state %#x: %w", m, e)
	}
	return uint64(m), size, nil
}
