// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && testhooks

package cengine

import (
	"sync"

	"github.com/liustack/slowjs/internal/gohost"
)

// callbackHook, when a test sets it, runs first in each callback with its
// name and VM id. It runs inside the engine, so a panic or a Goexit there
// leaves through engine frames as one from a real callback would. It exists
// only in a build with the tag testhooks (hooks.go has the normal one).
var callbackHook func(name string, vm uintptr)

// callOverrideHook, when a test sets it, is asked first in each host_call with
// the magic, and answers the call itself when it reports true. The engine
// then goes on as if the host had returned that value, which is how a test
// reaches the code after an abort (the host's own abort never returns).
var callOverrideHook func(magic int32) (gohost.Value, bool)

func callOverride(magic int32) (gohost.Value, bool) {
	if callOverrideHook != nil {
		return callOverrideHook(magic)
	}
	return gohost.Value{}, false
}

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
