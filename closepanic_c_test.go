// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && ((linux && (amd64 || arm64)) || (darwin && arm64)) && testhooks

package slowjs // import "github.com/liustack/slowjs"

// Close of a VM on the C engine when the OS refuses to unmap a SharedBytecode:
// hostmem.SetFault makes the call fail.

import (
	"errors"
	"slices"
	"testing"

	"github.com/liustack/slowjs/internal/hostmem"
)

var errRefused = errors.New("refused by the test")

// A panic in the release of a SharedBytecode, which the OS refusing to unmap
// it makes, does not keep the functions of OnClose from running or leave the
// VM in vms. Close panics with it.
func TestCloseSharedDropPanics(t *testing.T) {
	b, err := NewSharedBytecode(compileShared(t))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EvalSharedBytecode(b); err != nil {
		t.Fatal(err)
	}
	b.Close() // m holds the last hold
	id := m.id
	var ran []int
	m.OnClose(func() { ran = append(ran, 1) })
	m.OnClose(func() { ran = append(ran, 2) })

	hostmem.SetFault(func(call string, addr, size uintptr) error {
		if call == "unmap" && addr == b.p {
			return errRefused
		}
		return nil
	})
	r := closeRecovering(m)
	hostmem.SetFault(nil)
	// Give the mapping back, which the refused drop left.
	if err := hostmem.UnmapReadOnly(b.p, b.n); err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("Close did not panic")
	}
	if !slices.Equal(ran, []int{1, 2}) {
		t.Fatalf("OnClose ran %v, want [1 2]", ran)
	}
	if vms.Get(id) != nil {
		t.Error("the closed VM is still in vms")
	}
}
