// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package hostmem

// fault lets a test make a call to the OS fail. Every call to the OS asks
// faulted first, and faulted asks fault when a test set it. It exists only
// in a build with the tag testhooks, which only tests use: the normal build
// has no variable and no check (fault.go).
//
// When it is set, each call asks it, with the name of what it does and its
// range, and an error it returns stands for the error of the call.
var fault func(call string, addr, size uintptr) error

// SetFault sets the hook that makes calls to the OS fail, for the tests of a
// host built on Memory, which cannot reach fault. f gets the name of each
// call, such as "map", "unmap", "release" or "remap", and its range. nil
// takes it away. It is not for use while VMs run on other goroutines. Like
// fault, it is there only with the tag testhooks.
func SetFault(f func(call string, addr, size uintptr) error) {
	fault = f
}

func faulted(call string, addr, size uintptr) error {
	if fault == nil {
		return nil
	}
	return fault(call, addr, size)
}
