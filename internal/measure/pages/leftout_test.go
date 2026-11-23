// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package pages

import (
	"os"
	"testing"
	"time"

	"github.com/liustack/tifajs"
)

// countCalls puts counters in front of __tifa_random_bytes__ and of get and set of
// __tifa_async_context__, before the bundle takes them, and gives the counts to
// globalThis.__pagesHookCalls().
const countCalls = `(() => {
  const n = { random: 0, get: 0, set: 0 }
  const random = globalThis.__tifa_random_bytes__, context = globalThis.__tifa_async_context__
  Object.defineProperty(globalThis, '__tifa_random_bytes__', { value: function (k) { n.random++; return random(k) }, writable: true, configurable: true })
  Object.defineProperty(globalThis, '__tifa_async_context__', {
    value: { get() { n.get++; return context.get() }, set(v) { n.set++; return context.set(v) } }, configurable: true })
  Object.defineProperty(globalThis, '__pagesHookCalls', { value: () => JSON.stringify(n) })
})()`

// The hooks the stand-in mode leaves out, __tifa_random_bytes__ and __tifa_async_context__, are
// called 0 times while a bundle of engine/pgo/pages loads and while each of its pages renders,
// in the tifa mode, which has them: so the stand-in mode measures the same work without them.
func TestLeftOutNotCalled(t *testing.T) {
	if testing.Short() {
		t.Skip("renders the 23 pages of engine/pgo/pages once each, a few seconds")
	}
	repo, err := Repo("")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"wb", "store", "mui"} {
		k, err := FindKind(repo, name, nil)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Open(Options{Setup: Setup{Mode: Tifa}, Limit: defaultMemory, Timeout: time.Minute, Bundle: k.Bundle,
			BeforeBundle: func(vm *tifajs.VM) error { _, err := vm.Eval(countCalls, tifajs.EvalGlobal); return err }})
		if err != nil {
			t.Fatal(err)
		}
		calls := func() any {
			v, err := r.VM().Eval(`__pagesHookCalls()`, tifajs.EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		const none = `{"random":0,"get":0,"set":0}`
		if got := calls(); got != none {
			t.Errorf("%s: loading the bundle called %v", name, got)
		}
		for _, in := range k.Inputs {
			js := readFile(t, in)
			if _, _, err := r.Render(js); err != nil {
				t.Fatalf("%s: %v", PageName(in), err)
			}
			if err := r.After(); err != nil {
				t.Fatal(err)
			}
			if got := calls(); got != none {
				t.Errorf("%s: rendering %s called %v", name, PageName(in), got)
			}
		}
		// The counters are on the path the bundle takes: crypto and AsyncLocalStorage, which its
		// host script made from the hooks, count.
		if _, err := r.VM().Eval(`crypto.randomUUID(); new AsyncLocalStorage().run({}, () => 1)`, tifajs.EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if got := calls(); got != `{"random":1,"get":2,"set":2}` {
			t.Errorf("%s: crypto.randomUUID and AsyncLocalStorage.run counted %v", name, got)
		}
		r.Close()
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The stand-in mode loads the bundles of engine/pgo/pages and renders a page of each kind with
// its preludes, which stand in for what the native URL and Intl of the tifa mode give.
func TestStandInRendersThePages(t *testing.T) {
	repo, err := Repo("")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"wb", "store", "mui"} {
		k, err := FindKind(repo, name, nil)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Open(Options{Setup: Setup{Mode: StandIn}, Limit: defaultMemory, Bundle: k.Bundle, Preludes: StandInPreludes(repo)})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, _, err := r.Render(readFile(t, k.Inputs[0])); err != nil {
			t.Errorf("%s: %s: %v", name, PageName(k.Inputs[0]), err)
		}
		r.Close()
	}
}
