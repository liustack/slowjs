// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// dstLoops destructure a plain array in a loop that calls nothing else. The
// register loop takes such an array apart with no iterator
// (engine/regvm-ops-dst.h): no call of values, whose call polled, so an empty
// pattern reads no flag of its own and stops by the poll of the loop's jump
// back, and a single element by the poll of its read. The rest of a long
// array is in dststop_hooks_test.go: it makes and frees an array that long
// each time, and the collection and the freeing poll as long as they take,
// which StopPolls counts too.
var dstLoops = []struct{ name, setup, loop string }{
	{"empty pattern", `var a = [1, 2];`,
		`function go() { for (;;) { const [] = a; } }`},
	{"single element", `var a = [1, 2];`,
		`function go() { var n = 0; for (;;) { const [x] = a; n = (n + x) & 0xffff; } }`},
}

// TestInterruptArrayDestructuring: each loop of dstLoops stops at the eval
// timeout and at Interrupt within a few polls.
func TestInterruptArrayDestructuring(t *testing.T) {
	for _, c := range dstLoops {
		for _, how := range []string{"timeout", "Interrupt"} {
			t.Run(c.name+" "+how, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(c.setup+"\n"+c.loop+"; 0", EvalGlobal); err != nil {
					t.Fatal(err)
				}
				stop := func() {}
				if how == "timeout" {
					m.SetEvalTimeout(10 * time.Millisecond)
				} else {
					stop = interruptAfter(m, 10*time.Millisecond)
				}
				start := time.Now()
				done := make(chan error, 1)
				go func() {
					_, err := m.Call("go")
					done <- err
				}()
				var err error
				select {
				case err = <-done:
				case <-time.After(10 * time.Second):
					m.Interrupt()
					<-done
					t.Fatal("the loop did not stop within 10s")
				}
				took := time.Since(start)
				stop()
				m.SetEvalTimeout(0)
				checkStopped(t, m, err, took, stopPolls, "")
				if n := regvmStat(m, 0); n < 1 {
					t.Errorf("%d functions converted: go did not run in the register loop", n)
				}
				if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
					t.Errorf("the VM after the stop: 1+1 got %v, %v", v, err)
				}
			})
		}
	}
}

// TestArrayDestructuringPolls: a million destructurings of a two-element
// array with an empty pattern, the one form that reads no flag of its own,
// poll at least 100 times, as the loops of TestLoopWithoutCallsPolls do.
func TestArrayDestructuringPolls(t *testing.T) {
	m := newTestVM(t)
	before := m.h.Polls()
	if _, err := m.Eval(`var a = [1, 2]; for (let k = 0; k < 1e6; k++) { const [] = a; }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if n := m.h.Polls() - before; n < 100 {
		t.Errorf("a million destructurings polled %d times, want at least 100", n)
	}
}
