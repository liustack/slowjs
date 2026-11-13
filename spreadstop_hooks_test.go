// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/gohost"
)

// TestSpreadCopyStops: the spread of a long string gives each of its
// characters a property, and the copy of a source with many keys, a string
// or an object, defines one key after another in C. The copy reads the
// interrupt flag every 4,096 keys (docs/changes/uncatchable.md), so the eval
// timeout stops it within copyStopPolls polls of the copy wherever in it the
// stop comes. The polls under the freeing of what the copy made, an object of
// as many properties as it copied and the list of the keys of its source,
// are not counted (dstCleanup and JS_FreePropertyEnum): they are as many as
// the object and the list are long, and the kill judges them. The other
// steps that are not a key of the copy, the list of the keys of a string,
// the scan of the shape of an object and the growth of the property table of
// the target, are each one pass over the source or the target, which polls
// as many times as it is long: 65,536 keys keep each within a few polls,
// 131,072 took up to 11 and 524,288 20 to 25. The call ends with the interrupt, not the kill,
// and the VM goes on.
func TestSpreadCopyStops(t *testing.T) {
	const setup = `var s = "x".repeat(1 << 16), big = {}; ` +
		`for (var i = 0; i < 1 << 16; i++) big["k" + i] = i`
	cleanup := 0
	for _, c := range []struct{ name, call, want string }{
		{"string", `({...s})`, "65536"},
		{"string after a key", `({a: 1, ...s})`, "65537"},
		{"Object.assign of a string", `Object.assign({}, s)`, "65536"},
		{"object after a key", `({a: 1, ...big})`, "65537"},
		{"rest of an object", `(({k0, ...r}) => r)(big)`, "65535"},
	} {
		t.Run(c.name, func(t *testing.T) {
			vm := func() *VM {
				m := newTestVM(t)
				if _, err := m.Eval(setup+`; 0`, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				return m
			}
			// one call, then a stop at each fourth of it
			m := vm()
			start := time.Now()
			v, err := m.Eval(`Object.keys(`+c.call+`).length + ""`, EvalGlobal)
			call := time.Since(start)
			if err != nil || v != c.want {
				t.Fatalf("one call: got %v, %v, want %s", v, err, c.want)
			}
			for k := 1; k <= 3; k++ {
				m := vm()
				var loop int64
				where := map[string]int{}
				m.h.SetStopTrace(func() {
					if inCleanup() || underFunc("JS_FreePropertyEnum") {
						cleanup++
						return
					}
					loop++
					where[innermostEngineFunc()]++
				})
				if err := m.SetEvalTimeout(max(call*time.Duration(k)/4, time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				_, err := m.Eval(`for (;;) `+c.call, EvalGlobal)
				took := time.Since(start)
				m.h.SetStopTrace(nil)
				if errors.Is(err, ErrKilled) {
					t.Fatalf("killed after %v of a call of %v, want the interrupt", took, call)
				}
				checkStopped(t, m, err, took, gohost.KillPolls, "")
				t.Logf("stop at %d/4 of %v: %d polls of the copy, %d in all, %v", k, call, loop, m.h.StopPolls(), where)
				if loop > copyStopPolls {
					t.Errorf("stop at %d/4 of %v: %d polls of the copy saw the request to stop, want at most %d (%d in all, %v)",
						k, call, loop, copyStopPolls, m.h.StopPolls(), where)
				}
				if err := m.SetEvalTimeout(0); err != nil {
					t.Fatal(err)
				}
				if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
					t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
				}
			}
		})
	}
	if cleanup == 0 {
		t.Errorf("no poll that saw the request was under %v: the names of dstCleanup are gone from the engine", dstCleanup)
	}
}

// underFunc reports whether the engine polled under its function name: the
// list of the keys of a source is freed by JS_FreePropertyEnum, one key
// after another.
func underFunc(name string) bool {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.Function, "/cengine."+name) {
			return true
		}
		if !more {
			return false
		}
	}
}

// innermostEngineFunc names the innermost function of the generated engine
// on the stack of a poll.
func innermostEngineFunc() string {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if i := strings.LastIndex(f.Function, "/cengine."); i >= 0 && !strings.HasPrefix(f.Function[i+len("/cengine."):], "host") {
			return f.Function[i+len("/cengine."):]
		}
		if !more {
			return "?"
		}
	}
}
