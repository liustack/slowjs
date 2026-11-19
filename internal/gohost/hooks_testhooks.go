// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package gohost

// hookState is what the hooks of a test keep in a VM. It exists only in a
// build with the tag testhooks (hooks.go has the normal one).
type hookState struct {
	interruptAt int64  // Polls at which Poll calls Interrupt, or 0
	abortAt     int64  // Polls at which Poll aborts as the engine does, or 0
	stopTrace   func() // runs at each poll that saw the request to stop, or nil
}

// pollHook runs at the start of each Poll, on the goroutine that runs the
// engine.
func pollHook(v *VM) {
	if v.hooks.interruptAt != 0 && v.polls.Load() >= v.hooks.interruptAt {
		v.hooks.interruptAt = 0
		v.Interrupt()
	}
	if v.hooks.abortAt != 0 && v.polls.Load() >= v.hooks.abortAt {
		v.hooks.abortAt = 0
		// what Call does for the engine's abort
		v.MarkBroken()
		panic(ErrAbort)
	}
}

// AbortAtPoll makes the poll that brings Polls to n end the call as the
// engine's abort does (CallAbort in Call): the VM is broken and the poll
// panics with ErrAbort, through the engine's frames. 0 turns it off. Call it
// from the goroutine that uses the VM, between calls into the engine.
func (v *VM) AbortAtPoll(n int64) { v.hooks.abortAt = n }

// InterruptAtPoll makes the poll that brings Polls to n call Interrupt, from
// the goroutine that runs the engine, before it looks for a request to stop:
// that poll sets the flag. The place in a script is then the same on any
// machine, where an Interrupt from another goroutine lands wherever the
// engine has got to by then. 0 turns it off. Call it from the goroutine that
// uses the VM, before the call into the engine.
func (v *VM) InterruptAtPoll(n int64) { v.hooks.interruptAt = n }

// stopHook runs at each Poll that saw the request to stop, after it counted
// the poll in StopPolls, on the goroutine that runs the engine.
func stopHook(v *VM) {
	if v.hooks.stopTrace != nil {
		v.hooks.stopTrace()
	}
}

// SetStopTrace makes f run at each poll that saw the request to stop, after
// the poll is counted in StopPolls and before it may kill the script. f runs
// on the goroutine that runs the engine, with the engine's frames on its
// stack, so it can tell where the engine polled. It must not call into the
// engine. nil turns it off.
func (v *VM) SetStopTrace(f func()) { v.hooks.stopTrace = f }

// FreeingCode is the word after the engine's interrupt flag, which the engine
// sets from the stop of a compile to its return (SetInterruptFlag), or 0 when
// v has no flag. Call it between calls into the engine.
func (v *VM) FreeingCode() int32 {
	if v.flag == 0 {
		return 0
	}
	return *(*int32)(addr(v.flag + 4))
}
