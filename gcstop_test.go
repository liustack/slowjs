// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"os"
	"os/exec"
	goruntime "runtime"
	"testing"
	"time"
)

const gcStopChild = "QUICKJS_TEST_GCSTOP_CHILD"

// gcLoopPolls is how many polls the loop must make before a GC, and again
// after it, to count as running.
const gcLoopPolls = 3

// gcLoopWait is how long the test waits for those polls. A loop polls many
// times a millisecond, so only a loop that stopped polling runs into it.
const gcLoopWait = 30 * time.Second

// TestGCStopsLoopWithoutCalls shows that a GC is not held up by a script that
// loops without calling anything. Go's asynchronous preemption cannot stop the
// generated engine (docs/architecture.md): the goroutine in it stops at the
// entry of a Go function, and a loop like these enters one only at its polls
// (gohost.Poll). The loops keep their counter a small integer, so no slow
// path of an overflow gives the GC another way in.
//
// The test waits until the loop polls, runs a GC, and checks that the loop
// polls on after the GC returns: the GC came through while the loop ran. It
// does not time the GC: a GC that is only slow, on a loaded machine under the
// race detector, is no fault. The polls between the request and the stop
// cannot be counted from here either, since the loop runs on during the GC's
// concurrent mark: the polls a GC sees measure the mark, not the stop.
//
// When the loop stops polling, the GC never returns, and nothing of the
// process runs Go code again: the GC waits for every goroutine to stop, so no
// timer fires and -test.timeout cannot end it. So the test runs in a child
// process, and the parent ends it after 2 minutes, which a loop that polls
// never comes near.
func TestGCStopsLoopWithoutCalls(t *testing.T) {
	if os.Getenv(gcStopChild) == "" {
		var out lockedBuffer
		cmd := exec.Command(os.Args[0], "-test.run=^TestGCStopsLoopWithoutCalls$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), gcStopChild+"=1")
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			t.Logf("child:\n%s", out.String())
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Minute):
			cmd.Process.Kill()
			t.Fatalf("a GC did not return in 2 minutes: a loop stopped polling\n%s", out.String())
		}
		return
	}

	for _, c := range []struct{ name, js string }{
		{"goto", `let i = 0; for (;;) { i = (i + 1) & 0xffff }`},
		{"if_true", `let i = 0; do { i = (i + 1) & 0xffff } while (i > -1)`},
		{"if_false", `let i = 0; while (i > -1) { i = (i + 1) & 0xffff }`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			done := make(chan error, 1)
			start := time.Now()
			go func() {
				_, err := m.Eval(c.js, EvalGlobal)
				done <- err
			}()
			// polling waits until the loop has polled gcLoopPolls times
			// after from. A loop that stopped polling cannot see an
			// Interrupt either, so it is left running.
			polling := func(from int64, what string) {
				t.Helper()
				deadline := time.Now().Add(gcLoopWait)
				for m.h.Polls() < from+gcLoopPolls {
					select {
					case err := <-done:
						t.Fatalf("the loop ended %s: %v", what, err)
					default:
					}
					if time.Now().After(deadline) {
						t.Fatalf("the loop did not poll %s within %v", what, gcLoopWait)
					}
					goruntime.Gosched()
				}
			}
			polling(0, "before the first GC")
			for range 5 {
				goruntime.GC()
				polling(m.h.Polls(), "after the GC")
			}
			m.Interrupt()
			checkStopped(t, m, <-done, time.Since(start), stopPolls, "")
		})
	}
}

// TestLoopWithoutCallsPolls is the companion of TestGCStopsLoopWithoutCalls,
// which fails only when a loop stops polling altogether. A loop that polls
// too rarely holds a GC, and a stop, as long as it runs between two polls,
// and that test would pass. Here each loop runs a million times and must poll
// at least once every 10,000 iterations, that is spend at least a tick of the
// poll's count on each: about 32 now, so the bound has room for a faster loop
// but not for one that runs without counting. It counts polls, not time, so
// a loaded machine does not change it.
func TestLoopWithoutCallsPolls(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"goto", `let i = 0, k = 0; for (;;) { i = (i + 1) & 0xffff; if (++k === 1e6) break }`},
		{"if_true", `let i = 0, k = 0; do { i = (i + 1) & 0xffff } while (++k < 1e6)`},
		{"if_false", `let i = 0, k = 0; while (k < 1e6) { i = (i + 1) & 0xffff; k++ }`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			before := m.h.Polls()
			if _, err := m.Eval(c.js, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if n := m.h.Polls() - before; n < 100 {
				t.Errorf("a million iterations polled %d times, want at least 100", n)
			}
		})
	}
}
