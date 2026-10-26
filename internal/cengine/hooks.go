// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && !testhooks

package cengine

import "github.com/liustack/slowjs/internal/gohost"

// The normal build has no hooks for tests: these do nothing, and the calls to
// them compile away. The tag testhooks swaps in hooks_testhooks.go.

// hook is where a test can stop a callback of the engine. host_memory_grow,
// host_now and host_call call it first.
func hook(name string, vm uintptr) {}

// engineFreed is where a test counts the times Close frees the engine.
func engineFreed(v *VM) {}

// callOverride is where a test can answer a host_call itself.
func callOverride(magic int32) (gohost.Value, bool) { return gohost.Value{}, false }
