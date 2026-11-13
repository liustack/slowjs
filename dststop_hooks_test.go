// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package quickjs // import "github.com/liustack/quickjs"

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/gohost"
)

// dstCleanup are the functions of the engine that collect or free objects.
// A poll under one of them cannot end the script: the collector and the
// freeing of an object poll as long as they take, before the loop throws or
// after it, and the kill judges them (docs/changes/go-api.md), not the
// stop of the loop.
var dstCleanup = map[string]bool{
	"JS_RunGCInternal":   true,
	"__JS_FreeValueRT":   true,
	"free_object":        true,
	"free_zero_refcount": true,
	"gc_free_cycles":     true,
}

// inCleanup reports whether the engine polled under a function of
// dstCleanup. It reads the frames of the goroutine that runs the engine,
// from the hook of a poll.
func inCleanup() bool {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if i := strings.LastIndex(f.Function, "/cengine."); i >= 0 && dstCleanup[f.Function[i+len("/cengine."):]] {
			return true
		}
		if !more {
			return false
		}
	}
}

// TestArrayDestructuringRestStop: the rest of an array of 100,000 elements
// in a loop that calls nothing else stops at the eval timeout and at
// Interrupt within stopPolls polls of the loop, those that saw the request
// outside a collection and the freeing of an object. Each turn makes and
// frees an array that long, and those polls are not counted: they made 25
// to 50 of all the polls that saw the request, on the engine before the
// destructuring took no iterator as after it. The test checks that it saw
// some of them, so that a name of dstCleanup that the engine no longer has
// does not pass unseen.
func TestArrayDestructuringRestStop(t *testing.T) {
	const setup = `var big = Array.from({length: 1e5}, (_, i) => i & 7);
function go() { var n = 0; for (;;) { const [x, ...r] = big; n = (n + r.length) & 0xffff; } }; 0`
	cleanup := 0
	for _, how := range []string{"timeout", "Interrupt"} {
		for k := 0; k < 8; k++ {
			m := newTestVM(t)
			if _, err := m.Eval(setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			var loop int64
			m.h.SetStopTrace(func() {
				if inCleanup() {
					cleanup++
				} else {
					loop++
				}
			})
			d := time.Duration(5+3*k) * time.Millisecond
			stop := func() {}
			if how == "timeout" {
				m.SetEvalTimeout(d)
			} else {
				stop = interruptAfter(m, d)
			}
			start := time.Now()
			_, err := m.Call("go")
			took := time.Since(start)
			stop()
			m.h.SetStopTrace(nil)
			m.SetEvalTimeout(0)
			checkStopped(t, m, err, took, gohost.KillPolls, how+": ")
			t.Logf("%s after %v: %d polls of the loop, %d in all", how, d, loop, m.h.StopPolls())
			if loop > stopPolls {
				t.Errorf("%s after %v: %d polls of the loop saw the request to stop, want at most %d (%d in all)",
					how, d, loop, stopPolls, m.h.StopPolls())
			}
			if n := regvmStat(m, 0); n < 1 {
				t.Errorf("%d functions converted: go did not run in the register loop", n)
			}
			if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
				t.Errorf("the VM after the stop: 1+1 got %v, %v", v, err)
			}
		}
	}
	if cleanup == 0 {
		t.Errorf("no poll that saw the request was under %v: the names of dstCleanup are gone from the engine", dstCleanup)
	}
}
