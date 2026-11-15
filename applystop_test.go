// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A function called by f.apply(this, arguments) from the register loop runs
// in the loop (CALLMA in engine/regvm-ops-args.h), with no C call of apply.
// A loop in it stops at the eval timeout and at Interrupt within a few
// polls, the stack of the error keeps the frame of apply between the
// callee and its caller, as the C call of apply had it, and the VM answers
// after the stop: the frame's apply was freed with it.
func TestRegvmApplyArgumentsStops(t *testing.T) {
	const src = `function spin(a, b) {
  var n = 0;
  for (;;) { n = (n + a + b) & 0xffff; }
}
function fwd() { return spin.apply(this, arguments); }
function sum(a, b) { return a + b; }
function add() { return sum.apply(this, arguments); }
add(1, 2); add(1, 2);`
	for _, how := range []string{"timeout", "Interrupt"} {
		t.Run(how, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(src, EvalGlobal); err != nil {
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
				_, err := m.Call("fwd", 1, 2)
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
			var e *Error
			if errors.As(err, &e) {
				spin := strings.Index(e.Stack, "at spin (<eval>:")
				apply := strings.Index(e.Stack, "at apply (native)")
				fwd := strings.Index(e.Stack, "at fwd (<eval>:")
				if spin < 0 || apply < spin || fwd < apply {
					t.Errorf("stack %q: want spin, then apply, then fwd", e.Stack)
				}
			}
			if v, err := m.Eval(`add(3, 4)`, EvalGlobal); err != nil || fmt.Sprint(v) != "7" {
				t.Errorf("the VM after the stop: %v (%T), %v", v, v, err)
			}
		})
	}
}
