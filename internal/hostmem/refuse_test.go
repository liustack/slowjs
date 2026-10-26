// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"
	"testing"
)

// These tests make the OS refuse calls, through fault, and check what a
// Memory counts and holds after.

// TestCloseFailsPartly makes the OS refuse to unmap one chunk and the shadow
// stack: Close keeps them counted, refuses every use after it, and gives them
// back when it runs again.
func TestCloseFailsPartly(t *testing.T) {
	m, err := New(Unit)
	if err != nil {
		t.Fatal(err)
	}
	a := grow(t, m, Unit)
	b := grow(t, m, 4*Unit)
	c := grow(t, m, 2*Unit)
	fill(b, 4*Unit, 4)
	base, size := m.Stack()
	refuse := true
	calls := failing(t, func(c call, before []call) error {
		if refuse && c.name == "unmap" && (c.addr == b || c.addr == base) {
			return errInjected
		}
		return nil
	})

	if err := m.Close(); !errors.Is(err, errInjected) {
		t.Fatalf("Close: %v, want the injected error", err)
	}
	unmapped := map[uintptr]bool{}
	for _, c := range *calls {
		unmapped[c.addr] = c.name == "unmap"
	}
	if len(*calls) != 4 || !unmapped[a] || !unmapped[b] || !unmapped[c] || !unmapped[base] {
		t.Fatalf("Close made %+v, want an unmap of each chunk and of the stack", *calls)
	}
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != 4*Unit || u.Stack != size {
		t.Fatalf("after a Close that failed: %+v, want the chunk and the stack it could not unmap", u)
	}
	if p, n := m.Stack(); p != base || n != size {
		t.Fatalf("Stack() = %#x, %d after a Close that failed, want %#x, %d", p, n, base, size)
	}
	if i := same(b, 4*Unit, 4); i >= 0 {
		t.Fatalf("byte %d of the chunk left changed", i)
	}
	bytesAt(base, 1)[0] = 1

	mustPanic(t, "Grow while closing", func() { m.Grow(Unit) })
	mustPanic(t, "Free while closing", func() { m.Free(b, 4*Unit) })
	mustPanic(t, "Remap while closing", func() { m.Remap(b, 4*Unit, 8*Unit) })
	mustPanic(t, "Release while closing", func() { m.Release(nil, nil, 0) })

	refuse = false
	*calls = nil
	if err := m.Close(); err != nil {
		t.Fatalf("Close again: %v", err)
	}
	if len(*calls) != 2 || (*calls)[0].name != "unmap" || (*calls)[1].name != "unmap" ||
		(*calls)[0].addr != b && (*calls)[0].addr != base || (*calls)[0].addr == (*calls)[1].addr {
		t.Fatalf("Close again made %+v, want an unmap of the chunk and of the stack left", *calls)
	}
	if u := m.Usage(); u != (Usage{}) {
		t.Fatalf("after Close again: %+v", u)
	}
	*calls = nil
	if err := m.Close(); err != nil || len(*calls) != 0 {
		t.Fatalf("a third Close: %v and %+v, want nothing to do", err, *calls)
	}
}
