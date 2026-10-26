// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !testhooks

package hostmem

// faulted is where a test can make a call to the OS fail. This is the normal
// build, where it does nothing and the calls to it compile away. The tag
// testhooks swaps in fault_hook.go.
func faulted(call string, addr, size uintptr) error { return nil }
