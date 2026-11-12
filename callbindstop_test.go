// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// A loop of the register loop that calls a function through f.call, or
// through a chain of 100,000 bound functions, stops at the eval timeout and
// at Interrupt within a few polls. The loop takes such a call apart into the
// call it stands for (rv_unwrap_call in engine/regvm-ops-basic.h): the
// wrappers make no call of their own, so they count no poll, and a chain of
// bound functions reads the interrupt flag at each one it takes apart, where
// a stop asked in the middle of it leaves the rest to the call out of the
// loop, which throws it.
func TestInterruptCallBind(t *testing.T) {
	for _, c := range []struct{ name, setup, loop string }{
		{"f.call", `function f() { return 1; }`,
			`function go() { var n = 0; for (;;) n = (n + f.call(null)) & 0xffff; }`},
		// each bound function gets a name that is not a string, so that the
		// next bind names it "bound " and the chain is made in linear time
		{"bound chain", `var g = function () { return 1; };
for (var i = 0; i < 100000; i++) {
  g = g.bind(null);
  Object.defineProperty(g, 'name', {value: 0});
}`,
			`function go() { var n = 0; for (;;) n = (n + g()) & 0xffff; }`},
	} {
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
