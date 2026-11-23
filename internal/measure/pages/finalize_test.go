// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liustack/beastjs"
)

// finalizerBundle registers a cycle in a FinalizationRegistry at its top level and in each render,
// so that only a collection frees it. The callback notes the held value in globalThis.finalized
// and queues a job that notes it again. For the held value "throw" it throws. A render writes
// what was noted so far.
const finalizerBundle = `(function () {
  'use strict'
  globalThis.finalized = []
  globalThis.registry = new FinalizationRegistry((held) => {
    if (held === 'throw') throw new Error('the finalizer of ' + held + ' failed')
    finalized.push(held)
    Promise.resolve().then(() => finalized.push(held + '-job'))
  })
  const cycle = (held) => { const o = {}; o.self = o; registry.register(o, held) }
  cycle('load')
  /*TOP*/
  globalThis.startRender = (input) => {
    cycle(input)
    globalThis.renderResult = '<p>' + finalized.join() + '</p>'
  }
})()
`

func openFinalizer(t *testing.T, top string) (*Runner, error) {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle.js")
	if err := os.WriteFile(bundle, []byte(strings.Replace(finalizerBundle, "/*TOP*/", top, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return Open(Options{Setup: Setup{Mode: Tifa}, Limit: defaultMemory, Timeout: time.Minute, Bundle: bundle})
}

func finalized(t *testing.T, r *Runner) string {
	t.Helper()
	v, err := r.VM().Eval(`finalized.join()`, beastjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	return v.(string)
}

// tifa's Load runs the jobs the collection after the bundle queued (runFinalizers of
// internal/ssr/renderer.go), so the callbacks of the bundle's top level run before the first render,
// and the jobs they queue too.
func TestFinalizersRunAfterLoad(t *testing.T) {
	r, err := openFinalizer(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if got := finalized(t, r); got != "load,load-job" {
		t.Fatalf("after Open, finalized %q, want %q", got, "load,load-job")
	}
	html, _, err := r.Render("first")
	if err != nil {
		t.Fatal(err)
	}
	if html != "<p>load,load-job</p>" {
		t.Fatalf("the first render wrote %s", html)
	}
}

// tifa's pool runs the jobs the collection after a render queued before the VM renders again
// (collect and finalize of internal/ssr/pool.go), so a callback runs in After, outside the render,
// and not in the next render.
func TestFinalizersRunAfterARender(t *testing.T) {
	r, err := openFinalizer(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, in := range []string{"a", "b"} {
		if _, _, err := r.Render(in); err != nil {
			t.Fatal(err)
		}
		if err := r.After(); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := finalized(t, r), "load,load-job,a,a-job,b,b-job"; got != want {
		t.Fatalf("after two renders and their collections, finalized %q, want %q", got, want)
	}
}

// A callback that throws after a render fails After, as it retires the VM in tifa.
func TestFinalizerThatThrowsFailsAfter(t *testing.T) {
	r, err := openFinalizer(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, _, err := r.Render("throw"); err != nil {
		t.Fatal(err)
	}
	if err := r.After(); err == nil || !strings.Contains(err.Error(), "the finalizer of throw failed") {
		t.Fatalf("After returned %v, want the error of the callback", err)
	}
}

// tifa loads the bundle as a module, which settles at once without a top-level await, so a job
// the top level queued is still queued when Load collects, and runs with the callbacks after the
// collection. A cycle such a job makes is then garbage only after the collection, and is not freed
// by it.
func TestTopLevelJobsRunAfterTheLoadCollection(t *testing.T) {
	r, err := openFinalizer(t, `Promise.resolve().then(() => cycle('top-job'))`)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if got := finalized(t, r); got != "load,load-job" {
		t.Fatalf("after Open, finalized %q, want %q: the job of the top level ran before the collection", got, "load,load-job")
	}
}

// What the order above rests on: a module without a top-level await, loaded as tifa loads its
// bundle (Compile as a module, shared bytecode), leaves the jobs its top level queued in the queue.
func TestModuleLeavesTopLevelJobsQueued(t *testing.T) {
	vm, err := beastjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	code, err := vm.Compile(`Promise.resolve().then(() => { globalThis.ran = true }); export {}`, beastjs.EvalModule)
	if err != nil {
		t.Fatal(err)
	}
	b, err := beastjs.NewSharedBytecode(code)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := vm.EvalSharedBytecode(b); err != nil {
		t.Fatal(err)
	}
	if ran, err := vm.Eval(`globalThis.ran === true`, beastjs.EvalGlobal); err != nil || ran != false {
		t.Fatalf("the job of the module ran during its load (%v, %v)", ran, err)
	}
	if n, err := vm.ExecutePendingJobs(); err != nil || n != 1 {
		t.Fatalf("ExecutePendingJobs ran %d jobs (%v), want 1", n, err)
	}
}
