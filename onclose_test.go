// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	goruntime "runtime"
	"slices"
	"sync"
	"testing"
	"time"
	"weak"
)

// The functions given to OnClose run when Close runs, once each and in the
// order given, and at no other time.
func TestOnClose(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	var ran []int
	m.OnClose(func() { ran = append(ran, 1) })
	m.OnClose(func() { ran = append(ran, 2) })
	if _, err := m.Eval(`1 + 1`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	if len(ran) != 0 {
		t.Fatalf("ran %v before Close", ran)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []int{1, 2}) {
		t.Fatalf("ran %v, want [1 2]", ran)
	}
}

// A table outside the VM whose entry holds a closure over the VM, dropped by
// OnClose, lets the VM go once it is closed.
func TestOnCloseFreesTheVM(t *testing.T) {
	var table sync.Map
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	table.Store(m, func() *VM { return m })
	m.OnClose(func() { table.Delete(m) })
	w := weak.Make(m)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m = nil
	for range 10 {
		goruntime.GC()
		if w.Value() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the closed VM is still reachable")
}
