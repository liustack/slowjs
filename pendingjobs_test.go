// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

func TestExecutePendingJobsDrains(t *testing.T) {
	vm, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()

	// Chained Promise reactions run only once the jobs are drained.
	if _, err := vm.Eval(`globalThis.x = 0; Promise.resolve().then(function(){ globalThis.x = 1; }).then(function(){ globalThis.x = 2; });`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if r, _ := vm.Eval(`globalThis.x`, EvalGlobal); r != 0 {
		t.Fatalf("before drain x=%v, want 0 (reactions must not run on their own)", r)
	}
	n, err := vm.ExecutePendingJobs()
	if err != nil {
		t.Fatalf("ExecutePendingJobs: %v", err)
	}
	if n < 2 {
		t.Errorf("ran %d jobs, want >= 2 (two chained .then)", n)
	}
	if r, _ := vm.Eval(`globalThis.x`, EvalGlobal); r != 2 {
		t.Errorf("after drain x=%v, want 2", r)
	}

	// async/await resumes via the job queue.
	if _, err := vm.Eval(`globalThis.y = 0; (async function(){ globalThis.y = (await 41) + 1; })();`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	vm.ExecutePendingJobs()
	if r, _ := vm.Eval(`globalThis.y`, EvalGlobal); r != 42 {
		t.Errorf("after drain y=%v, want 42 (await resumed)", r)
	}

	// A .then() that throws is an unhandled rejection, not a drain error; the other
	// queued jobs still run.
	if _, err := vm.Eval(`globalThis.z = 0; Promise.resolve().then(function(){ throw new Error("boom"); }); Promise.resolve().then(function(){ globalThis.z = 9; });`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	vm.ExecutePendingJobs()
	if r, _ := vm.Eval(`globalThis.z`, EvalGlobal); r != 9 {
		t.Errorf("after a throwing .then, z=%v, want 9 (sibling jobs still run)", r)
	}

	// Draining an empty queue is a no-op.
	if got, err := vm.ExecutePendingJobs(); err != nil || got != 0 {
		t.Errorf("empty drain = (%d, %v), want (0, nil)", got, err)
	}
}
