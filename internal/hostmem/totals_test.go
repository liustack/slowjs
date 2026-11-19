// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package hostmem

import "testing"

// The totals of the process follow every map and unmap of chunks, stacks and
// read-only copies, and come back to where they were once all is closed.
func TestProcessTotals(t *testing.T) {
	before := ProcessTotals()
	want := before
	check := func(when string) {
		t.Helper()
		if got := ProcessTotals(); got != want {
			t.Fatalf("%s: totals %+v, want %+v", when, got, want)
		}
	}
	m, err := New(DefaultStackSize)
	if err != nil {
		t.Fatal(err)
	}
	want.Memories++
	want.Stacks += DefaultStackSize
	check("New")

	a := grow(t, m, Unit)
	b := grow(t, m, 4*Unit)
	c := grow(t, m, 2*Unit)
	want.Chunks += 3
	want.Heap += 7 * Unit
	check("Grow")

	if err := m.Free(a, Unit); err != nil {
		t.Fatal(err)
	}
	want.Chunks--
	want.Heap -= Unit
	check("Free")

	if CanRemap {
		if b, err = m.Remap(b, 4*Unit, 9*Unit); err != nil {
			t.Fatal(err)
		}
		want.Heap += 5 * Unit
		check("Remap")
		if err := m.Free(b, 9*Unit); err != nil {
			t.Fatal(err)
		}
		want.Heap -= 9 * Unit
	} else {
		if err := m.Free(b, 4*Unit); err != nil {
			t.Fatal(err)
		}
		want.Heap -= 4 * Unit
	}
	want.Chunks--
	check("Free after Remap")

	d := grow(t, m, Unit)
	if _, err := m.Release(each([2]uintptr{d, Unit}), nil, 0); err != nil {
		t.Fatal(err)
	}
	check("Grow and Release of a whole chunk")

	p, err := MapReadOnly(make([]byte, 3*int(pageSize)+1))
	if err != nil {
		t.Fatal(err)
	}
	want.ReadOnly += int64(4 * pageSize)
	check("MapReadOnly")
	if err := UnmapReadOnly(p, 3*int(pageSize)+1); err != nil {
		t.Fatal(err)
	}
	want.ReadOnly -= int64(4 * pageSize)
	check("UnmapReadOnly")

	_ = c // still held: Close unmaps it with the stack
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	want = before
	check("Close")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	check("Close again")
}
