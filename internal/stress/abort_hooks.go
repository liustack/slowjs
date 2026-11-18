// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package main

import "github.com/liustack/slowjs"

// canAbort says whether this build can make the engine abort.
const canAbort = true

// polls is how many polls the engine of vm made so far.
func polls(vm *slowjs.VM) int64 { return vm.TestPolls() }

// abortAfterPolls makes the n-th poll of vm from now abort as the engine
// does, or turns that off with 0.
func abortAfterPolls(vm *slowjs.VM, n int64) { vm.TestAbortAfterPolls(n) }
