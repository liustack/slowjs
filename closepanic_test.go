// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"slices"
	"testing"
)

// closeRecovering runs m.Close and returns what it panicked with, nil when it
// did not.
func closeRecovering(m *VM) (r any) {
	defer func() { r = recover() }()
	m.Close()
	return nil
}

// A function of OnClose that panics does not keep the others from running.
// Close runs them all, then panics with the first panic, and Close again does
// nothing. A Close inside a function returns nil at once.
func TestCloseCallbackPanics(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	id := m.id
	var ran []string
	m.OnClose(func() { ran = append(ran, "a"); panic("first") })
	m.OnClose(func() {
		ran = append(ran, "b")
		if err := m.Close(); err != nil {
			t.Errorf("Close inside OnClose: %v", err)
		}
		panic("second")
	})
	m.OnClose(func() { ran = append(ran, "c") })

	if r := closeRecovering(m); r != "first" {
		t.Fatalf("Close panicked with %v, want the first panic", r)
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(ran, want) {
		t.Fatalf("OnClose ran %v, want %v", ran, want)
	}
	if vms.Get(id) != nil {
		t.Error("the closed VM is still in vms")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}
	if len(ran) != 3 {
		t.Fatalf("Close again ran OnClose again: %v", ran)
	}
}
