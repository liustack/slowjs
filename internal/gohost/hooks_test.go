// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package gohost

import (
	"testing"
	"unsafe"
)

// TestInterruptAtPoll: the poll that brings Polls to the count InterruptAtPoll
// set asks to stop, and sets the flag, and no poll before it does. It asks
// once: the next outermost entry has no request.
func TestInterruptAtPoll(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	testFlag = 0
	v.SetInterruptFlag(uintptr(unsafe.Pointer(&testFlag)))
	v.SetKillGrace(-1)
	v.EnterScript(0)
	v.InterruptAtPoll(v.Polls() + 5)
	for range 4 {
		Poll(base + 64)
	}
	if v.Interrupted() || testFlag != 0 {
		t.Fatalf("before the poll: interrupted %v, flag %d", v.Interrupted(), testFlag)
	}
	Poll(base + 64)
	if !v.Interrupted() || testFlag != 1 || v.StopPolls() != 1 {
		t.Fatalf("at the poll: interrupted %v, flag %d, %d polls saw the request", v.Interrupted(), testFlag, v.StopPolls())
	}
	v.Leave()
	v.EnterScript(0)
	for range 10 {
		Poll(base + 64)
	}
	if v.Interrupted() {
		t.Fatal("the next entry was interrupted too")
	}
	v.Leave()
}
