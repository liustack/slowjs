// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package gohost

// hookState is what the hooks of a test keep in a VM. It exists only in a
// build with the tag testhooks (hooks.go has the normal one).
type hookState struct {
	interruptAt int64 // Polls at which Poll calls Interrupt, or 0
}

// pollHook runs at the start of each Poll, on the goroutine that runs the
// engine.
func pollHook(v *VM) {
	if v.hooks.interruptAt != 0 && v.polls.Load() >= v.hooks.interruptAt {
		v.hooks.interruptAt = 0
		v.Interrupt()
	}
}

// InterruptAtPoll makes the poll that brings Polls to n call Interrupt, from
// the goroutine that runs the engine, before it looks for a request to stop:
// that poll sets the flag. The place in a script is then the same on any
// machine, where an Interrupt from another goroutine lands wherever the
// engine has got to by then. 0 turns it off. Call it from the goroutine that
// uses the VM, before the call into the engine.
func (v *VM) InterruptAtPoll(n int64) { v.hooks.interruptAt = n }
