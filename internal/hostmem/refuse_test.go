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

// TestFreeRefused: a chunk the OS does not unmap stays mapped and counted,
// and can be freed again.
func TestFreeRefused(t *testing.T) {
	m := newMemory(t, Unit)
	p := grow(t, m, 2*Unit)
	fill(p, 2*Unit, 2)
	refuse := true
	failing(t, func(c call, before []call) error {
		if refuse && c.name == "unmap" && c.addr == p {
			return errInjected
		}
		return nil
	})
	if err := m.Free(p, 2*Unit); !errors.Is(err, errInjected) {
		t.Fatalf("Free: %v, want the injected error", err)
	}
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != 2*Unit {
		t.Fatalf("after a refused Free: %+v, want the chunk still counted", u)
	}
	if i := same(p, 2*Unit, 2); i >= 0 {
		t.Fatalf("a refused Free changed byte %d", i)
	}
	refuse = false
	if err := m.Free(p, 2*Unit); err != nil {
		t.Fatal(err)
	}
	if u := m.Usage(); u.Chunks != 0 || u.Footprint != 0 {
		t.Fatalf("after Free: %+v", u)
	}
}

// TestRemapRefused: a chunk the OS does not resize stays where it was, with
// its contents and its record.
func TestRemapRefused(t *testing.T) {
	if !CanRemap {
		t.Skip("no remap here: TestRemap checks the refusal")
	}
	for _, how := range []string{"injected", "too large"} {
		t.Run(how, func(t *testing.T) {
			m := newMemory(t, Unit)
			const size = 4 * Unit
			p := grow(t, m, size)
			fill(p, size, 6)
			newSize := uintptr(8 * Unit)
			if how == "injected" {
				failing(t, func(c call, before []call) error {
					if c.name == "remap" {
						return errInjected
					}
					return nil
				})
			} else {
				newSize = 1 << 50
			}
			q, err := m.Remap(p, size, newSize)
			if err == nil || q != 0 || how == "injected" && !errors.Is(err, errInjected) {
				t.Fatalf("Remap to %d = %#x, %v, want 0 and an error", newSize, q, err)
			}
			t.Log(err)
			if i := same(p, size, 6); i >= 0 {
				t.Fatalf("a refused Remap changed byte %d", i)
			}
			if u := m.Usage(); u.Chunks != 1 || u.Footprint != size {
				t.Fatalf("after a refused Remap: %+v", u)
			}
			if err := m.Free(p, size); err != nil {
				t.Fatalf("Free of the chunk a Remap left: %v", err)
			}
		})
	}
}

// TestReleasePartly makes the OS refuse one chunk, one range and the shadow
// stack: Release goes on with the rest, returns every error, and counts only
// what it handed back.
func TestReleasePartly(t *testing.T) {
	const stackSize = 1 << 20
	m := newMemory(t, stackSize)
	a := grow(t, m, Unit)
	b := grow(t, m, 2*Unit)
	c := grow(t, m, 8*Unit)
	d := grow(t, m, 8*Unit)
	base, _ := m.Stack()
	refuse := true
	calls := failing(t, func(cl call, before []call) error {
		if !refuse {
			return nil
		}
		switch {
		case cl.name == "unmap" && cl.addr == b,
			cl.name == "release" && cl.addr == c+4*Unit,
			cl.name == "release" && cl.addr == base+StackKeep:
			return errInjected
		}
		return nil
	})
	chunks := func() func() (uintptr, uintptr) { return each([2]uintptr{a, Unit}, [2]uintptr{b, 2 * Unit}) }
	ranges := func() func() (uintptr, uintptr) {
		return each([2]uintptr{c, 2 * Unit}, [2]uintptr{c + 4*Unit, 2 * Unit}, [2]uintptr{d, 8 * Unit})
	}

	r, err := m.Release(chunks(), ranges(), 0)
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) || len(joined.Unwrap()) != 3 || !errors.Is(err, errInjected) {
		t.Fatalf("Release: %v, want the three injected errors", err)
	}
	want := []call{
		{"unmap", a, Unit}, {"unmap", b, 2 * Unit},
		{"release", c, 2 * Unit}, {"release", c + 4*Unit, 2 * Unit}, {"release", d, 8 * Unit},
		{"release", base + StackKeep, stackSize - StackKeep},
	}
	if len(*calls) != len(want) {
		t.Fatalf("Release made %+v, want %+v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("call %d of Release: %+v, want %+v", i, (*calls)[i], want[i])
		}
	}
	if want := (Released{Chunks: Unit, Pages: 10 * Unit}); r != want {
		t.Fatalf("Release handed back %+v, want %+v", r, want)
	}
	if u := m.Usage(); u.Chunks != 3 || u.Footprint != 18*Unit || u.Released != r {
		t.Fatalf("after Release: %+v, want b, c and d, and the totals of one Release", u)
	}

	// The next Release hands back what the OS refused, and the totals grow.
	refuse = false
	r2, err := m.Release(each([2]uintptr{b, 2 * Unit}), ranges(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Released{Chunks: 2 * Unit, Pages: 12 * Unit, Stack: stackSize - StackKeep}); r2 != want {
		t.Fatalf("the second Release handed back %+v, want %+v", r2, want)
	}
	if u, want := m.Usage().Released, (Released{r.Chunks + r2.Chunks, r.Pages + r2.Pages, r.Stack + r2.Stack}); u != want {
		t.Fatalf("totals %+v after two Releases, want %+v", u, want)
	}
}
