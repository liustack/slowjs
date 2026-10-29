// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !testhooks

package gohost

// The normal build has no hooks for tests: hookState takes no room in a VM,
// and pollHook does nothing, so its call in Poll compiles away. The tag
// testhooks swaps in hooks_testhooks.go.

type hookState struct{}

// pollHook runs at the start of each Poll.
func pollHook(v *VM) {}
