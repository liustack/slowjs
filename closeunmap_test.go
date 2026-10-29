// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

// Close of a VM on the C engine when the OS refuses to unmap: hostmem.SetFault
// makes the calls fail.

import (
	"errors"
	"testing"

	"github.com/liustack/slowjs/internal/gohost"
	"github.com/liustack/slowjs/internal/hostmem"
)

// When the OS refuses to unmap the memory of the VM, Close returns the error,
// and Close again unmaps what is left and returns nil. Nothing else runs
// twice: the functions of OnClose ran once, and the id, which another VM may
// have by then, is left alone.
func TestCloseUnmapRefusedThenRetried(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	m.OnClose(func() { ran++ })
	if _, err := m.Eval(`1 + 1`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	before := len(gohost.Leftovers())

	refused := 0
	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" && refused == 0 {
			refused++
			return errRefused
		}
		return nil
	})
	err = m.Close()
	hostmem.SetFault(nil)
	if !errors.Is(err, errRefused) || refused != 1 {
		t.Fatalf("Close with an unmap refused: %v, refused %d times", err, refused)
	}
	if got := len(gohost.Leftovers()); got != before+1 {
		t.Fatalf("after the refused unmap %d VMs hold memory, want %d", got, before+1)
	}

	// The id of m is free for another VM now, and its builtin finds it.
	other, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	defineBuiltin(t, other, "answer", NewBuiltin("answer", 0, func(a Args) (Value, error) {
		return a.VM().NewInt(42), nil
	}))

	if err := m.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}
	if got := len(gohost.Leftovers()); got != before {
		t.Fatalf("after Close again %d VMs hold memory, want %d", got, before)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close a third time: %v", err)
	}
	if ran != 1 {
		t.Fatalf("OnClose ran %d times, want 1", ran)
	}
	if v, err := other.Eval(`answer()`, EvalGlobal); err != nil || v != int64(42) && v != 42 {
		t.Fatalf("the other VM gives %v, %v", v, err)
	}
}
