// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"

	"modernc.org/libc"
)

// failCAlloc makes every allocation of C memory for a call fail until the
// test ends.
func failCAlloc(t *testing.T) {
	old := cMalloc
	cMalloc = func(*libc.TLS, libc.Tsize_t) uintptr { return 0 }
	t.Cleanup(func() { cMalloc = old })
}

// When there is no C memory for the arguments of a call, the call returns an
// error, it does not panic, and the VM goes on.
func TestCallArgumentsNoMemory(t *testing.T) {
	m := newTestVM(t)
	pc := newPC(t, m)
	if _, err := m.Eval("globalThis.add = (a, b) => a + b; globalThis.arr = (...a) => a.length", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	f, err := m.EvalValue("add", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Free()

	failCAlloc(t)
	if _, err := pc.Promise.Then(nil, nil); err == nil || !strings.Contains(err.Error(), "OOM") {
		t.Errorf("Then: %v, want the out of memory error", err)
	}
	if _, err := m.Call("add", 1, 2); err == nil || !strings.Contains(err.Error(), "OOM") {
		t.Errorf("Call: %v, want the out of memory error", err)
	}
	if _, err := f.Call(UndefinedValue, 1, 2); err == nil || !strings.Contains(err.Error(), "OOM") {
		t.Errorf("Value.Call: %v, want the out of memory error", err)
	}
	if _, err := m.Call("arr", []int{1, 2, 3}); err == nil || !strings.Contains(err.Error(), "OOM") {
		t.Errorf("Call with an array: %v, want the out of memory error", err)
	}

	cMalloc = libc.Xmalloc
	r, err := m.Call("add", 1, 2)
	if err != nil || r != 3 {
		t.Fatalf("after: %v, %v", r, err)
	}
	p, err := pc.Promise.Then(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Free()
}
