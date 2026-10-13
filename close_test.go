// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	goruntime "runtime"
	"testing"
	"weak"
)

// loaderHost stands in for whatever a module loader closes over. In a browser
// engine that is the page: its fonts, canvas and document.
type loaderHost struct{ name string }

// loadThrough creates a VM whose module normalizer and loader both close over
// a fresh host, imports a module through them, releases the VM the way
// 'release' asks and returns a weak reference to the host. When it returns,
// the loader table is the only place the host can still be reachable from.
func loadThrough(t *testing.T, release func(*VM) error) weak.Pointer[loaderHost] {
	t.Helper()
	h := &loaderHost{name: "host"}
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	m.SetModuleLoader(
		func(_ *VM, name string) (string, error) {
			return `export const from = "` + h.name + "|" + name + `";`, nil
		},
		func(_ *VM, _, name string) (string, error) {
			return h.name + "/" + name, nil
		},
	)
	if _, err := m.Eval(`import { from } from "mod"; globalThis.from = from;`, EvalModule); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval("from", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "host|host/mod"; g != e {
		t.Fatalf("got %v, expected %v", g, e)
	}

	if err := release(m); err != nil {
		t.Fatal(err)
	}

	return weak.Make(h)
}

// TestCloseReleasesModuleLoader pins that Close releases the module loader and
// normalizer set with SetModuleLoader. SetModuleLoader files them in a
// process-global table keyed by the C runtime, and before Close deleted from
// it too, only SetModuleLoader(nil, nil) did: a closed VM kept its loader, and
// whatever host the loader closed over, reachable for the life of the process.
// modernc.org/webview's loader closes over the page, and one closed page per
// test ran its 32-bit builders out of address space.
//
// It also runs under the memgrind allocator (TestMemgrind2), so it must
// create and Close its own VMs.
func TestCloseReleasesModuleLoader(t *testing.T) {
	for _, tc := range []struct {
		name    string
		release func(*VM) error
	}{
		// The defect: Close alone must be enough.
		{"close", (*VM).Close},
		// What hosts did meanwhile keeps working.
		{"removeThenClose", func(m *VM) error {
			m.SetModuleLoader(nil, nil)
			return m.Close()
		}},
		// A replacement loader takes the previous one's place; a host that
		// swaps loaders without closing the VM must not accumulate them.
		{"replaceThenClose", func(m *VM) error {
			m.SetModuleLoader(func(*VM, string) (string, error) { return "", nil }, nil)
			return m.Close()
		}},
	} {
		wp := loadThrough(t, tc.release)
		goruntime.GC()
		goruntime.GC()
		if wp.Value() != nil {
			t.Fatalf("%s: the host the module loader closed over is still reachable", tc.name)
		}
	}
}
