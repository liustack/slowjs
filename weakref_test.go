// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// The WeakRef constructor and deref keep their target to the end of the job,
// as Node does: a target nothing else holds is there for deref in the call
// that made the WeakRef or read it, and in a Go function that call runs, and
// gone once the call has returned, or in the next promise job.
func TestWeakRefKeptDuringJob(t *testing.T) {
	m := newTestVM(t)
	wantEval(t, m, `globalThis.wr = new WeakRef({ a: 1 }); String(wr.deref()?.a)`, "1")
	wantEval(t, m, `String(wr.deref())`, "undefined")

	wantEval(t, m, `globalThis.o = { b: 2 }; globalThis.wr2 = new WeakRef(o); 'ok'`, "ok")
	wantEval(t, m, `wr2.deref(); o = null; String(wr2.deref()?.b)`, "2")
	wantEval(t, m, `String(wr2.deref())`, "undefined")

	var inner any
	if err := m.RegisterFunc("look", func() {
		inner, _ = m.Eval(`String(w3.deref()?.c)`, EvalGlobal)
	}, false); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `globalThis.w3 = new WeakRef({ c: 3 }); look(); String(w3.deref()?.c)`, "3")
	if inner != "3" {
		t.Fatalf("in a Go function the call ran: got %v, want 3", inner)
	}
	wantEval(t, m, `String(w3.deref())`, "undefined")

	// A Go function that runs the jobs inside javascript does not end the
	// job of the call it is in, with jobs to run and with none.
	if err := m.RegisterFunc("drain", func() {
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Error(err)
		}
	}, false); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `globalThis.w4 = new WeakRef({ d: 4 }); Promise.resolve().then(() => {}); drain(); drain();
		String(w4.deref()?.d)`, "4")
	wantEval(t, m, `String(w4.deref())`, "undefined")

	// So does Go code that runs the jobs inside a call while no javascript
	// runs: here Call converts an argument after it evaluated the function.
	got, err := m.Call(`(globalThis.w5 = new WeakRef({}), () => String(w5.deref() !== undefined))`,
		mutJSON{func() {
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Error(err)
			}
		}})
	if err != nil || got != "true" {
		t.Fatalf("running the jobs while Call converted an argument: got %v, %v, want true", got, err)
	}
	wantEval(t, m, `String(w5.deref())`, "undefined")

	wantEval(t, m, `globalThis.seen = []; Promise.resolve()
		.then(() => { globalThis.wj = new WeakRef({}); seen.push(wj.deref() !== undefined) })
		.then(() => seen.push(wj.deref() !== undefined)); 'ok'`, "ok")
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `seen.join()`, "true,false")
}

// A VM used for many calls, as a pool of VMs uses one for many renders, keeps
// the targets of a call to the end of that call only: the memory each call
// made comes back after it.
func TestWeakRefKeptReleasedAfterCall(t *testing.T) {
	m := newTestVM(t)
	run := func() {
		wantEval(t, m, `globalThis.big = new WeakRef(new Array(1 << 16).fill(1)); String(big.deref().length)`, "65536")
	}
	run()
	m.RunGC()
	base := vmMemory(m).Fmalloc_size
	for i := 0; i < 200; i++ {
		run()
	}
	m.RunGC()
	if got := vmMemory(m).Fmalloc_size; got > base+64<<10 {
		t.Fatalf("after 200 calls the VM holds %d bytes, %d after the first", got, base)
	}
	wantEval(t, m, `String(big.deref())`, "undefined")
}

// A loop that waits within one call for deref to give undefined does not end:
// each deref keeps the target to the end of the call, whatever collections run
// in it. Here the target is a cycle, which only a collection frees, and the
// loop allocates enough for collections to run. The target goes when the call
// returns, and a collection asked for between two calls takes it.
func TestWeakRefKeptThroughLoop(t *testing.T) {
	m := newTestVM(t)
	wantEval(t, m, `globalThis.gone = (() => { const cycle = {}; cycle.self = cycle; return new WeakRef(cycle) })();
		(() => {
			const kept = []
			let n = 0
			while (gone.deref() !== undefined && n < 300000) { kept.push({}); n++ }
			return String(n)
		})()`, "300000")
	m.RunGC()
	wantEval(t, m, `String(gone.deref())`, "undefined")
}

// A call that ends with an error lets its targets go too: here the eval
// timeout stops it.
func TestWeakRefKeptReleasedAfterTimeout(t *testing.T) {
	m := newTestVM(t)
	if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`globalThis.w = new WeakRef({ e: 5 }); for (;;) {}`, EvalGlobal); err == nil {
		t.Fatal("the endless loop returned")
	}
	if err := m.SetEvalTimeout(0); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `String(w.deref())`, "undefined")
}
