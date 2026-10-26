// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && testhooks

package cengine

import "sync"

// callbackHook, when a test sets it, runs first in each callback with its
// name and VM id. It runs inside the engine, so a panic or a Goexit there
// leaves through engine frames as one from a real callback would. It exists
// only in a build with the tag testhooks (hooks.go has the normal one).
var callbackHook func(name string, vm uintptr)

func hook(name string, vm uintptr) {
	if callbackHook != nil {
		callbackHook(name, vm)
	}
}

// The counts are by the id of the host's VM, which no other VM of the process
// has again, so that a VM closed and forgotten is not kept alive by its count.
var (
	teardownMu     sync.Mutex
	teardownCounts = map[uint32]int{}
)

// engineFreed counts the times Close called into the engine to free v.
func engineFreed(v *VM) {
	teardownMu.Lock()
	teardownCounts[v.h.ID()]++
	teardownMu.Unlock()
}

// teardowns is how many times Close called into the engine to free v.
func teardowns(v *VM) int {
	teardownMu.Lock()
	defer teardownMu.Unlock()
	return teardownCounts[v.h.ID()]
}
