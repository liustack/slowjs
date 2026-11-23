// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import "testing"

// Nothing runs the reactions of promises but ExecutePendingJobs, and it runs
// them all: chains, awaits, and the siblings of a reaction that throws.
func TestExecutePendingJobsDrains(t *testing.T) {
	t.Run("chain", func(t *testing.T) {
		m := newTestVM(t)
		if _, err := m.Eval(`globalThis.x = 0; Promise.resolve().then(() => { x = 1 }).then(() => { x = 2 })`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		wantEval(t, m, `x`, 0)
		n, err := m.ExecutePendingJobs()
		if err != nil || n < 2 {
			t.Fatalf("ran %d jobs, %v", n, err)
		}
		wantEval(t, m, `x`, 2)
	})
	t.Run("await", func(t *testing.T) {
		m := newTestVM(t)
		if _, err := m.Eval(`globalThis.y = 0; (async () => { y = (await 41) + 1 })()`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		wantEval(t, m, `y`, 42)
	})
	t.Run("throw", func(t *testing.T) {
		m := newTestVM(t)
		// The throw rejects the promise then returned: it is no error of
		// the drain, and the other reaction runs.
		if _, err := m.Eval(`globalThis.z = 0;
			Promise.resolve().then(() => { throw new Error('in a reaction') });
			Promise.resolve().then(() => { z = 9 })`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		wantEval(t, m, `z`, 9)
	})
	t.Run("empty", func(t *testing.T) {
		m := newTestVM(t)
		if n, err := m.ExecutePendingJobs(); n != 0 || err != nil {
			t.Fatalf("got %d, %v, want 0, nil", n, err)
		}
	})
}

// A queue that feeds itself is cut, with an error.
func TestExecutePendingJobsCap(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a million jobs")
	}
	m := newTestVM(t)
	if _, err := m.Eval(`(function again() { Promise.resolve().then(again) })()`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	n, err := m.ExecutePendingJobs()
	if n != maxPendingJobs || err == nil || err.Error() != "beastjs: aborted draining after 1048576 pending jobs" {
		t.Fatalf("got %d, %v", n, err)
	}
}
