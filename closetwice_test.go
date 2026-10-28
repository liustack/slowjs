// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// Close of a VM that is closed does nothing and returns nil, as Close of
// SharedBytecode does, so that a deferred Close after an explicit one is
// safe. The functions of OnClose run once.
func TestCloseTwice(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("twice", func() int { return 1 }, false); err != nil {
		t.Fatal(err)
	}
	oldID := m.id
	ran := 0
	m.OnClose(func() { ran++ })
	if _, err := m.Eval(`twice()`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if err := m.Close(); err != nil {
			t.Fatalf("Close number %d: %v", i, err)
		}
	}
	if ran != 1 {
		t.Fatalf("the function of OnClose ran %d times, want 1", ran)
	}
	// The id of the closed VM may be another VM's by now: a second Close
	// must leave that one alone.
	other, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if other.id != oldID {
		t.Fatalf("the new VM has id %d, not the %d of the closed one", other.id, oldID)
	}
	// A Builtin finds its VM by id in vms: it fails if Close again took the
	// entry of the new VM out.
	defineBuiltin(t, other, "answer", NewBuiltin("answer", 0, func(a Args) (Value, error) {
		return a.VM().NewInt(42), nil
	}))
	if err := m.Close(); err != nil {
		t.Fatalf("Close after another VM was made: %v", err)
	}
	v, err := other.Eval(`answer()`, EvalGlobal)
	if err != nil || v != int64(42) && v != 42 {
		t.Fatalf("the other VM gives %v, %v", v, err)
	}
}
