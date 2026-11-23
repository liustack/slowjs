// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package beastjs // import "github.com/liustack/beastjs"

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRegExpCaptureLoopsStopPolls: the loops of captureLoops stop within
// stepStopPolls polls that saw the request outside a collection and the
// freeing of an object (dstCleanup). 1024 captures of a step are one property
// read and one define each, or a substring and a store for exec. The polls of
// freeing what the loop made, the result of 32768 strings of the last exec or
// the array a custom exec result filled, are as many as it is long: they are
// not counted.
func TestRegExpCaptureLoopsStopPolls(t *testing.T) {
	for _, how := range []string{"timeout", "Interrupt"} {
		for _, w := range captureLoops {
			t.Run(how+"/"+w.name, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(w.setup, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				var loop, cleanup int64
				m.h.SetStopTrace(func() {
					if inCleanup() {
						cleanup++
					} else {
						loop++
					}
				})
				stop := func() {}
				if how == "timeout" {
					m.SetEvalTimeout(10 * time.Millisecond)
				} else {
					stop = interruptAfter(m, 10*time.Millisecond)
				}
				start := time.Now()
				_, err := m.Eval(w.loop, EvalGlobal)
				took := time.Since(start)
				stop()
				m.h.SetStopTrace(nil)
				m.SetEvalTimeout(0)
				t.Logf("%d polls of the loop, %d of freeing, %d in all", loop, cleanup, m.h.StopPolls())
				checkStopped(t, m, err, took, m.h.StopPolls(), "")
				if loop > stepStopPolls {
					t.Errorf("%d polls of the loop saw the request to stop, want at most %d (%d in all)", loop, stepStopPolls, m.h.StopPolls())
				}
			})
		}
	}
}

// captureStopPolls bounds the polls of TestRegExpCaptureLoopsStopAtEachPoll
// that see a request in a loop over the captures of a match. The loops read
// the flag every 1024 captures, fewer turns than the generated engine runs
// between two polls (gohost.DefaultPollInterval ticks), so only the poll that
// asked sees it before a read. Without the reads, a request in the loop over
// the 32767 captures of a match was seen by 11 polls (replace with a
// function) and 4 (split), on darwin/arm64.
const captureStopPolls = 1

// replaceCopyPolls are the polls of the loop of replace with a function that
// copies where each capture of a match starts and ends, before its loop over
// the captures: it reads no flag, and takes 3 polls for 32767 captures.
const replaceCopyPolls = 3

// regexpMatcher are the functions of the matcher, which reads the flag on its
// own (TestRegExpManyCapturesStop).
var regexpMatcher = map[string]bool{
	"lre_exec":           true,
	"lre_exec_inside":    true,
	"lre_exec_backtrack": true,
}

// inMatcher reports whether the engine polled under a function of
// regexpMatcher, as inCleanup does for dstCleanup.
func inMatcher() bool {
	pcs := make([]uintptr, 128)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if i := strings.LastIndex(f.Function, "/cengine."); i >= 0 && regexpMatcher[f.Function[i+len("/cengine."):]] {
			return true
		}
		if !more {
			return false
		}
	}
}

// TestRegExpCaptureLoopsStopAtEachPoll: a stop asked at any poll of one
// replace with a function and one split on their paths without exec, with a
// regexp of 32767 groups, is seen by at most captureStopPolls polls outside
// the matcher and the freeing of objects (dstCleanup). InterruptAtPoll puts
// the request at each poll of the call in turn, the same polls on any
// machine, so the polls of the loops over the captures are among them. A
// timer rarely lands there, as the matcher takes most of the time.
func TestRegExpCaptureLoopsStopAtEachPoll(t *testing.T) {
	for _, w := range []struct {
		name, call string
		limit      int64
	}{
		{"replace with a function", `"b".replace(re, () => "")`, captureStopPolls + replaceCopyPolls},
		{"split without names", `"bb".split(re)`, captureStopPolls},
	} {
		t.Run(w.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(captureManyPlain, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Eval(w.call, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			before := m.h.Polls()
			if _, err := m.Eval(w.call, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			n := m.h.Polls() - before
			var worst, at, matcher int64
			for i := int64(1); i <= n; i++ {
				var loop int64
				m.h.SetStopTrace(func() {
					if inMatcher() {
						matcher++
					} else if !inCleanup() {
						loop++
					}
				})
				m.h.InterruptAtPoll(m.h.Polls() + i)
				_, err := m.Eval(w.call, EvalGlobal)
				m.h.SetStopTrace(nil)
				m.h.InterruptAtPoll(0)
				// asked at one of the last polls, the call ends before a read
				var e *Error
				if err != nil && (!errors.As(err, &e) || !e.Interrupted) {
					t.Fatalf("stop at poll %d of %d: got %v, want interrupted", i, n, err)
				}
				if loop > worst {
					worst, at = loop, i
				}
			}
			t.Logf("%d polls in a call, at most %d saw the request outside the matcher (asked at poll %d)", n, worst, at)
			if matcher == 0 || worst == 0 {
				t.Fatalf("%d polls under the matcher, %d outside: a function of regexpMatcher was renamed or inlined", matcher, worst)
			}
			if worst > w.limit {
				t.Errorf("a stop asked at poll %d of %d was seen by %d polls outside the matcher, want at most %d", at, n, worst, w.limit)
			}
		})
	}
}
