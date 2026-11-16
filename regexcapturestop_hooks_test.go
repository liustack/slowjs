// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

import (
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
