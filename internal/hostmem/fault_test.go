// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package hostmem

import (
	"errors"
	"runtime"
	"slices"
	"testing"
)

// These tests and helpers watch or fail the calls to the OS through fault,
// which exists only with the tag testhooks (fault_hook.go).

type call struct {
	name       string
	addr, size uintptr
}

var errInjected = errors.New("injected")

// failing sets fault for the rest of the test. f sees each call and the ones
// before it, and returns the error the call gives, or nil to let it run.
func failing(t testing.TB, f func(c call, before []call) error) *[]call {
	t.Helper()
	var calls []call
	fault = func(name string, addr, size uintptr) error {
		c := call{name, addr, size}
		before := calls
		calls = append(calls, c)
		return f(c, before)
	}
	t.Cleanup(func() { fault = nil })
	return &calls
}

// count is how many calls of the name there are.
func count(calls []call, name string) int {
	n := 0
	for _, c := range calls {
		if c.name == name {
			n++
		}
	}
	return n
}

// TestStackTrimCalls: a trim hands back its pages in one call, from the byte
// where TestStackTrim says, and makes no call when nothing is above it.
func TestStackTrimCalls(t *testing.T) {
	const size = 1 << 20
	m := newMemory(t, size)
	base, _ := m.Stack()
	calls := failing(t, func(c call, before []call) error { return nil })
	for _, tc := range trimCases {
		*calls = nil
		if _, err := m.Release(nil, nil, tc.used); err != nil {
			t.Fatal(err)
		}
		want := []call{{"release", base + tc.from, size - tc.from}}
		if tc.from == size {
			want = nil
		}
		if !slices.Equal(*calls, want) {
			t.Fatalf("a trim at %d made %+v, want %+v", tc.used, *calls, want)
		}
	}
}

// TestCloseCalls: Close unmaps each chunk and the shadow stack with its guard
// page.
func TestCloseCalls(t *testing.T) {
	const stackSize = 1 << 20
	m, err := New(stackSize)
	if err != nil {
		t.Fatal(err)
	}
	grow(t, m, Unit)
	grow(t, m, 64*Unit)
	base, size := m.Stack()
	calls := failing(t, func(c call, before []call) error { return nil })
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	fault = nil
	if count(*calls, "unmap") != 3 {
		t.Fatalf("Close made %+v, want an unmap of each chunk and of the stack", *calls)
	}
	for _, c := range *calls {
		// The guard page goes with the stack. windows frees the whole
		// reservation, which takes no size.
		if c.addr == base && runtime.GOOS != "windows" && c.size != size+pageSize {
			t.Fatalf("Close unmapped %d bytes of the stack, want %d with the guard page", c.size, size+pageSize)
		}
	}
}
