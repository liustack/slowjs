// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !testhooks

package gohost

// The normal build has no hooks for tests: hookState takes no room in a VM,
// and pollHook and stopHook do nothing, so their calls in Poll compile away. The tag
// testhooks swaps in hooks_testhooks.go.

type hookState struct{}

// pollHook runs at the start of each Poll.
func pollHook(v *VM) {}

// stopHook runs at each Poll that saw the request to stop, after it counted
// the poll in StopPolls.
func stopHook(v *VM) {}
