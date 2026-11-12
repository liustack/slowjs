// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A loop of the register loop that does nothing but call charCodeAt stops at
// the eval timeout and at Interrupt within a few polls. The loop answers
// those calls itself (rv_fast_call in engine/regvm-ops-basic.h): no call is
// made, so no call reads the interrupt flag or counts toward the next poll,
// and what brings the stop is the loop's own jump back. The stack of the
// error names a place of the loop that is not a line of the calls: the stop
// came at the jump back, where it used to come at one of the calls.
func TestRegvmCharCodeAtLoopStops(t *testing.T) {
	const loop = `function go() {
  var s = str, n = 0;
  for (;;) {
    n += s.charCodeAt(0) + s.charCodeAt(1) + s.charCodeAt(2) + s.charCodeAt(3);
    n += s.charCodeAt(4) + s.charCodeAt(5) + s.charCodeAt(6) + s.charCodeAt(7);
    n &= 0xffff;
  }
}`
	for _, c := range []struct{ name, s string }{
		{"8-bit", `'abcdefgh'`},
		{"16-bit", `'\u0100bcdefg\u0101'`},
		// a rope, made flat by the first call that reads it
		{"rope", `(function () { var a = 'a'.repeat(600), b = 'b'.repeat(600); return a + b; })()`},
	} {
		for _, how := range []string{"timeout", "Interrupt"} {
			t.Run(c.name+" "+how, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(loop+`; var str = `+c.s+`; str.charCodeAt(0); 0`, EvalGlobal); err != nil {
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
				var e *Error
				if errors.As(err, &e) {
					if !strings.Contains(e.Stack, "at go (<eval>:") || strings.Contains(e.Stack, "<eval>:4:") || strings.Contains(e.Stack, "<eval>:5:") {
						t.Errorf("stack %q: want the loop of go, not a line of its calls (4 and 5)", e.Stack)
					}
				}
				if n := regvmStat(m, 0); n < 1 {
					t.Errorf("%d functions converted: go did not run in the register loop", n)
				}
				if v, err := m.Eval(`str.charCodeAt(1)`, EvalGlobal); err != nil || v == nil {
					t.Errorf("the VM after the stop: %v, %v", v, err)
				}
			})
		}
	}
}
