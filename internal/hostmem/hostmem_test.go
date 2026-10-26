// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"testing"
)

// fill writes a pattern that depends on the offset, so that memory that
// moved or was cut short does not read back right.
func fill(addr, size uintptr, seed byte) {
	b := bytesAt(addr, size)
	for i := range b {
		b[i] = byte(i) ^ byte(i>>9) ^ seed
	}
}

// same reports the first offset where the memory differs from fill's
// pattern, or -1.
func same(addr, size uintptr, seed byte) int {
	for i, c := range bytesAt(addr, size) {
		if c != byte(i)^byte(i>>9)^seed {
			return i
		}
	}
	return -1
}

func zero(addr, size uintptr) bool {
	for _, c := range bytesAt(addr, size) {
		if c != 0 {
			return false
		}
	}
	return true
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", what)
		}
	}()
	f()
}

// each returns the ranges one by one, then a size of 0, as the engine's
// JS_ReleaseNextChunk and JS_ReleaseNextRange do.
func each(rs ...[2]uintptr) func() (uintptr, uintptr) {
	return func() (uintptr, uintptr) {
		if len(rs) == 0 {
			return 0, 0
		}
		r := rs[0]
		rs = rs[1:]
		return r[0], r[1]
	}
}

// call is a call to the OS that fault saw.
// optIn skips the test unless the environment variable env is 1.
func optIn(t *testing.T, env, why string) {
	t.Helper()
	if os.Getenv(env) != "1" {
		t.Skipf("%s: set %s=1 to run it", why, env)
	}
}

// retryTaken runs try until it returns true, 4 times at most, and skips the
// test with why when it never does. try returns false only when the layout it
// needs could not be set up or seen, as when another mapping of the process
// takes a place the test needs free, and fails the test through t for
// anything else.
func retryTaken(t *testing.T, why string, try func() bool) {
	t.Helper()
	for range 4 {
		if try() {
			return
		}
	}
	t.Skipf("4 tries, and each time %s", why)
}

// unmapped checks that the size bytes at addr, which held fill's pattern with
// seed, are no longer mapped. Another mapping of the process may be there
// already, with other bytes: then unmapped returns false, for retryTaken. A
// page that still holds the pattern fails the test.
func unmapped(t *testing.T, addr, size uintptr, seed byte) bool {
	t.Helper()
	if !anyMapped(t, addr, size) {
		return true
	}
	for off := uintptr(0); off < size; off += pageSize {
		if holds(addr, off, pageSize, seed) {
			t.Fatalf("the page at %#x is still mapped with what it held", addr+off)
		}
	}
	return false
}

// holds reports whether the n bytes at addr+off can be read and hold fill's
// pattern of a fill at addr.
func holds(addr, off, n uintptr, seed byte) (ok bool) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	for i, c := range bytesAt(addr+off, n) {
		o := off + uintptr(i)
		if c != byte(o)^byte(o>>9)^seed {
			return false
		}
	}
	return true
}

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

// panicked runs f and returns what it panicked with, or nil.
func panicked(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

func newMemory(t testing.TB, stackSize uintptr) *Memory {
	t.Helper()
	m, err := New(stackSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m
}

func grow(t testing.TB, m *Memory, size uintptr) uintptr {
	t.Helper()
	p, err := m.Grow(size)
	if err != nil {
		t.Fatal(err)
	}
	if p == 0 || p%pageSize != 0 {
		t.Fatalf("Grow(%d) = %#x, want a page aligned address", size, p)
	}
	return p
}

func TestMapWriteUnmap(t *testing.T) {
	const size = 4 * Unit
	retryTaken(t, "another mapping was where the memory was unmapped", func() bool {
		p, err := osMap(size)
		if err != nil {
			t.Fatal(err)
		}
		if p%pageSize != 0 {
			t.Fatalf("osMap gave %#x, not aligned to the page size %d", p, pageSize)
		}
		if !zero(p, size) {
			t.Fatal("new memory is not zero")
		}
		fill(p, size, 1)
		if i := same(p, size, 1); i >= 0 {
			t.Fatalf("byte %d does not read back", i)
		}
		if err := osUnmap(p, size); err != nil {
			t.Fatal(err)
		}
		return unmapped(t, p, size, 1)
	})
}

func TestMapRefusesAbsurdSizes(t *testing.T) {
	for _, size := range []uintptr{1 << 50, ^uintptr(0) &^ (Unit - 1)} {
		p, err := osMap(size)
		if err == nil {
			osUnmap(p, size)
			t.Fatalf("osMap(%#x) = %#x, want an error", size, p)
		}
		t.Logf("osMap(%#x): %v", size, err)
	}

	m := newMemory(t, Unit)
	grow(t, m, Unit)
	p, err := m.Grow(1 << 50)
	if err == nil || p != 0 {
		t.Fatalf("Grow(1<<50) = %#x, %v, want 0 and an error", p, err)
	}
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != Unit {
		t.Fatalf("after a refused Grow: %+v, want the one chunk of before", u)
	}

	for _, size := range []uintptr{1 << 62, 1 << 63} {
		m, err := New(size)
		if err == nil {
			m.Close()
			t.Fatalf("New(%#x) succeeded, want an error", size)
		}
		t.Logf("New(%#x): %v", size, err)
	}
}

func TestBadSizesPanic(t *testing.T) {
	mustPanic(t, "New(3 * Unit)", func() { New(3 * Unit) })
	mustPanic(t, "New(Unit / 2)", func() { New(Unit / 2) })
	m := newMemory(t, Unit)
	mustPanic(t, "Grow(0)", func() { m.Grow(0) })
	mustPanic(t, "Grow(Unit + 1)", func() { m.Grow(Unit + 1) })
}

// TestFootprint follows a model of the chunks through every way they come
// and go.
func TestFootprint(t *testing.T) {
	m := newMemory(t, DefaultStackSize)
	model := map[uintptr]uintptr{}
	check := func(when string) {
		t.Helper()
		var sum uintptr
		for _, n := range model {
			sum += n
		}
		u := m.Usage()
		if u.Chunks != len(model) || u.Footprint != sum || u.Stack != DefaultStackSize {
			t.Fatalf("%s: %+v, want %d chunks, footprint %d, stack %d", when, u, len(model), sum, DefaultStackSize)
		}
	}
	check("new")

	var ps []uintptr
	for _, n := range []uintptr{Unit, 3 * Unit, 16 * Unit, 2 * Unit, 128 * Unit} {
		p := grow(t, m, n)
		model[p] = n
		ps = append(ps, p)
		check(fmt.Sprintf("Grow(%d)", n))
	}

	if err := m.Free(ps[1], 3*Unit); err != nil {
		t.Fatal(err)
	}
	delete(model, ps[1])
	check("Free")
	mustPanic(t, "Free of a chunk already freed", func() { m.Free(ps[1], 3*Unit) })
	mustPanic(t, "Free with the wrong size", func() { m.Free(ps[2], 8*Unit) })
	mustPanic(t, "Free inside a chunk", func() { m.Free(ps[2]+Unit, Unit) })
	check("refused Frees")

	if CanRemap {
		p, err := m.Remap(ps[2], 16*Unit, 40*Unit)
		if err != nil {
			t.Fatal(err)
		}
		delete(model, ps[2])
		model[p] = 40 * Unit
		ps[2] = p
		check("Remap up")
		p, err = m.Remap(ps[2], 40*Unit, 8*Unit)
		if err != nil {
			t.Fatal(err)
		}
		delete(model, ps[2])
		model[p] = 8 * Unit
		ps[2] = p
		check("Remap down")
	} else {
		_, err := m.Remap(ps[2], 16*Unit, 40*Unit)
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("Remap on %s: %v, want errors.ErrUnsupported", runtime.GOOS, err)
		}
		check("refused Remap")
	}

	// Release: two whole chunks, and the free pages inside a third.
	r, err := m.Release(each([2]uintptr{ps[0], Unit}, [2]uintptr{ps[3], 2 * Unit}),
		each([2]uintptr{ps[4] + 100, 64 * Unit}), 0)
	if err != nil {
		t.Fatal(err)
	}
	delete(model, ps[0])
	delete(model, ps[3])
	check("Release")
	if r.Chunks != 3*Unit {
		t.Fatalf("Release handed back %d bytes of chunks, want %d", r.Chunks, 3*Unit)
	}
	if want := 64*Unit - pageSize; r.Pages != want {
		t.Fatalf("Release handed back %d bytes of pages, want %d", r.Pages, want)
	}
	if want := uintptr(DefaultStackSize - StackKeep); r.Stack != want {
		t.Fatalf("Release handed back %d bytes of stack, want %d", r.Stack, want)
	}
	if u := m.Usage(); u.Released != r {
		t.Fatalf("totals %+v after one Release of %+v", u.Released, r)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if u := m.Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Fatalf("after Close: %+v", u)
	}
}

// TestReleasePages hands back the pages inside a range, rounded inward, and
// leaves the bytes around them alone.
func TestReleasePages(t *testing.T) {
	m := newMemory(t, Unit)
	const size = 8 * Unit
	p := grow(t, m, size)
	fill(p, size, 7)
	lo, hi := p+100, p+6*Unit+5 // rounded inward: p+page to p+6*Unit
	r, err := m.Release(nil, each([2]uintptr{lo, hi - lo}, [2]uintptr{p + 7*Unit + 1, pageSize}), 0)
	if err != nil {
		t.Fatal(err)
	}
	// The second range holds no whole page.
	if want := 6*Unit - pageSize; r.Pages != want {
		t.Fatalf("handed back %d bytes, want %d", r.Pages, want)
	}
	if i := same(p, pageSize, 7); i >= 0 {
		t.Fatalf("byte %d before the range changed", i)
	}
	// fill's pattern depends on the offset in the whole chunk.
	for o := uintptr(6 * Unit); o < size; o++ {
		if got, want := bytesAt(p+o, 1)[0], byte(o)^byte(o>>9)^7; got != want {
			t.Fatalf("byte %d after the range is %d, want %d", o, got, want)
		}
	}
	if runtime.GOOS == "linux" {
		if !zero(p+pageSize, 6*Unit-pageSize) {
			t.Fatal("pages handed back do not read as zero")
		}
	}
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != size {
		t.Fatalf("pages handed back changed the chunks: %+v", u)
	}

	mustPanic(t, "a range outside every chunk", func() { m.Release(nil, each([2]uintptr{p + size, pageSize}), 0) })
	mustPanic(t, "a range past the end of its chunk", func() { m.Release(nil, each([2]uintptr{p + Unit, size}), 0) })
	mustPanic(t, "a chunk that is not one", func() { m.Release(each([2]uintptr{p + Unit, Unit}), nil, 0) })
}

// TestRemapPrimitive grows a mapping in place and by moving, at the level
// of the system call, where the test controls what lies behind it.
func TestRemapPrimitive(t *testing.T) {
	if !CanRemap {
		_, err := osRemap(0, Unit, 2*Unit)
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("osRemap on %s: %v, want errors.ErrUnsupported", runtime.GOOS, err)
		}
		t.Skipf("no remap on %s", runtime.GOOS)
	}
	const size = 16 * Unit

	t.Run("in place", func(t *testing.T) {
		// Map twice the size and unmap the top half, so that the room behind
		// the chunk is free. Another mapping of the process may take it
		// before the remap, and then the chunk has to move. What is behind
		// after the move does not tell what was there during the remap, so a
		// move only means that the layout was not set up.
		retryTaken(t, "the chunk moved instead of growing in place, as it must when another mapping took the room behind it", func() bool {
			p, err := osMap(2 * size)
			if err != nil {
				t.Fatal(err)
			}
			if err := osUnmap(p+size, size); err != nil {
				t.Fatal(err)
			}
			fill(p, size, 3)
			q, err := osRemap(p, size, 2*size)
			if err != nil {
				osUnmap(p, size)
				t.Fatal(err)
			}
			defer osUnmap(q, 2*size)
			if i := same(q, size, 3); i >= 0 {
				t.Fatalf("byte %d changed", i)
			}
			if !zero(q+size, size) {
				t.Fatal("the new part is not zero")
			}
			return q == p
		})
	})

	t.Run("moving", func(t *testing.T) {
		retryTaken(t, "another mapping was where the chunk moved from", func() bool {
			// The second half stays mapped and blocks growth in place.
			p, err := osMap(2 * size)
			if err != nil {
				t.Fatal(err)
			}
			defer osUnmap(p+size, size)
			fill(p, 2*size, 5)
			q, err := osRemap(p, size, 3*size)
			if err != nil {
				osUnmap(p, size)
				t.Fatal(err)
			}
			defer osUnmap(q, 3*size)
			if q == p {
				t.Fatal("grew in place over a mapping")
			}
			if i := same(q, size, 5); i >= 0 {
				t.Fatalf("byte %d changed in the move", i)
			}
			if !holds(p, size, size, 5) {
				t.Fatal("the mapping behind was changed")
			}
			return unmapped(t, p, size, 5)
		})
	})
}

func TestRemap(t *testing.T) {
	m := newMemory(t, Unit)
	const size = 32 * Unit
	p := grow(t, m, size)
	fill(p, size, 9)
	if !CanRemap {
		_, err := m.Remap(p, size, 2*size)
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("Remap on %s: %v, want errors.ErrUnsupported", runtime.GOOS, err)
		}
		if i := same(p, size, 9); i >= 0 {
			t.Fatalf("a refused Remap changed byte %d", i)
		}
		return
	}
	mustPanic(t, "Remap of a chunk with the wrong size", func() { m.Remap(p, 2*size, 4*size) })
	q, err := m.Remap(p, size, 4*size)
	if err != nil {
		t.Fatal(err)
	}
	if i := same(q, size, 9); i >= 0 {
		t.Fatalf("byte %d changed", i)
	}
	if q != p {
		mustPanic(t, "Free of the old place", func() { m.Free(p, size) })
	}
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != 4*size {
		t.Fatalf("after Remap: %+v", u)
	}
	if err := m.Free(q, 4*size); err != nil {
		t.Fatal(err)
	}
}

// TestStackTrim hands back the pages of the shadow stack above what is in use
// and keeps the bytes below.
func TestStackTrim(t *testing.T) {
	const size = 1 << 20
	m := newMemory(t, size)
	base, _ := m.Stack()
	calls := failing(t, func(c call, before []call) error { return nil })
	var total uintptr
	for _, tc := range []struct{ used, from uintptr }{
		{0, StackKeep},
		{StackKeep - 1, StackKeep},
		{600<<10 + 100, alignUp(600<<10+100, pageSize)},
		{size - 1, size},
		{size, size},
	} {
		fill(base, size, 8)
		*calls = nil
		r, err := m.Release(nil, nil, tc.used)
		if err != nil {
			t.Fatal(err)
		}
		if r.Stack != size-tc.from {
			t.Fatalf("a trim at %d handed back %d bytes, want %d", tc.used, r.Stack, size-tc.from)
		}
		total += r.Stack
		want := []call{{"release", base + tc.from, size - tc.from}}
		if tc.from == size {
			want = nil
		}
		if !slices.Equal(*calls, want) {
			t.Fatalf("a trim at %d made %+v, want %+v", tc.used, *calls, want)
		}
		if i := same(base, tc.from, 8); i >= 0 {
			t.Fatalf("a trim at %d changed byte %d, below the pages it hands back", tc.used, i)
		}
		if runtime.GOOS == "linux" && !zero(base+tc.from, size-tc.from) {
			t.Fatalf("pages trimmed at %d do not read as zero", tc.used)
		}
	}
	if u := m.Usage(); u.Released.Stack != total {
		t.Fatalf("the totals count %d bytes of stack, want %d", u.Released.Stack, total)
	}
	mustPanic(t, "a trim above the stack", func() { m.Release(nil, nil, size+1) })
}

// TestStackResidency counts the pages of the shadow stack in memory, which
// the other tests do not: a count is a snapshot of the system, which may page
// out or, with huge pages, in.
func TestStackResidency(t *testing.T) {
	optIn(t, "HOSTMEM_RESIDENCY", "exact counts of pages in memory")
	m := newMemory(t, DefaultStackSize)
	base, size := m.Stack()
	pages := size / pageSize
	if n := resident(t, base, size); n != 0 {
		t.Fatalf("%d of %d pages resident before any touch", n, pages)
	}

	// Touch the first MiB, one page in the middle and the last page.
	touch := func(off, n uintptr) {
		b := bytesAt(base+off, n)
		for i := uintptr(0); i < n; i += pageSize {
			b[i] = byte(i>>12) | 1
		}
	}
	touch(0, 1<<20)
	touch(size/2, pageSize)
	touch(size-pageSize, pageSize)
	touched := int((1<<20)/pageSize + 2)
	if n := resident(t, base, size); n != touched {
		t.Fatalf("%d pages resident, want the %d touched", n, touched)
	}
	t.Logf("%s: page %d bytes, %d of %d pages resident after the touch", runtime.GOOS, pageSize, touched, pages)

	if _, err := m.Release(nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	keep := int(StackKeep / pageSize)
	n := resident(t, base, size)
	t.Logf("%s: %d pages resident after an idle trim that keeps %d", runtime.GOOS, n, keep)
	if runtime.GOOS == "linux" && n != keep {
		t.Fatalf("%d pages resident after an idle trim, want the %d kept", n, keep)
	}

	// A trim inside a callback keeps what the engine still uses.
	touch(0, 1<<20)
	used := uintptr(600 << 10)
	if _, err := m.Release(nil, nil, used); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		if n, want := resident(t, base, size), int(alignUp(used, pageSize)/pageSize); n != want {
			t.Fatalf("%d pages resident after a trim at %d, want %d", n, used, want)
		}
	}
}

// faultAt writes a byte at addr and returns the address of the fault it
// makes, or 0 when there is none.
func faultAt(addr uintptr) (at uintptr, other any) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		r := recover()
		if e, ok := r.(interface{ Addr() uintptr }); ok {
			at = e.Addr()
		} else {
			other = r
		}
	}()
	bytesAt(addr, 1)[0] = 1
	return 0, nil
}

// readable reports whether every page of [addr, addr+size) can be read.
func readable(addr, size uintptr) (ok bool) {
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	b := bytesAt(addr, size)
	for i := uintptr(0); i < size; i += pageSize {
		sink += b[i]
	}
	return true
}

// sink keeps the reads of readable from being optimized away.
var sink byte

// TestStackLayout: a shadow stack of each size is aligned to it, can be
// written from its base to its last byte, and faults on the guard page above.
func TestStackLayout(t *testing.T) {
	for _, size := range []uintptr{Unit, 1 << 20, DefaultStackSize} {
		m := newMemory(t, size)
		base, got := m.Stack()
		if got != size || base%size != 0 {
			t.Fatalf("New(%d).Stack() = %#x, %d: want %d bytes aligned to their size", size, base, got, size)
		}
		for _, addr := range []uintptr{base, base + size - 1} {
			if at, other := faultAt(addr); at != 0 || other != nil {
				t.Fatalf("writing the stack of %d bytes at %#x: fault at %#x, %v", size, addr, at, other)
			}
		}
		for _, off := range []uintptr{0, pageSize - 1} {
			at, other := faultAt(base + size + off)
			if at != base+size+off {
				t.Fatalf("writing the guard page at %#x: fault at %#x, %v", base+size+off, at, other)
			}
		}
	}
}

// TestManyStacks reserves 300 stacks of the default size: only the pages
// touched take memory, and none overlaps another.
func TestManyStacks(t *testing.T) {
	if runtime.GOOS == "windows" {
		optIn(t, "HOSTMEM_BIGCOMMIT", "300 stacks of 32 MiB commit 9.4 GiB on windows")
	}
	const n = 300
	type region struct{ base, size uintptr }
	var rs []region
	for i := 0; i < n; i++ {
		m := newMemory(t, DefaultStackSize)
		base, size := m.Stack()
		bytesAt(base, 1)[0] = 1
		rs = append(rs, region{base, size})
	}
	slices.SortFunc(rs, func(a, b region) int { return cmp.Compare(a.base, b.base) })
	total := 0
	for i, r := range rs {
		if i > 0 && rs[i-1].base+rs[i-1].size+pageSize > r.base {
			t.Fatalf("stacks at %#x and %#x overlap", rs[i-1].base, r.base)
		}
		total += resident(t, r.base, r.size)
	}
	t.Logf("%d stacks of %d MiB: %d pages of %d bytes resident", n, DefaultStackSize>>20, total, pageSize)
	if total > n {
		t.Fatalf("%d pages resident, want at most the %d touched", total, n)
	}
}

// TestSmallStacks reserves 64 stacks of 1 MiB, which every system can commit:
// each is aligned to its size, none overlaps another or its guard page, and
// each is trimmed and closed.
func TestSmallStacks(t *testing.T) {
	const n, size = 64, 1 << 20
	type region struct {
		m    *Memory
		base uintptr
		seed byte
	}
	var rs []region
	for i := range n {
		m := newMemory(t, size)
		base, got := m.Stack()
		if got != size || base%size != 0 {
			t.Fatalf("Stack() = %#x, %d: want %d bytes aligned to their size", base, got, size)
		}
		fill(base, size, byte(i))
		rs = append(rs, region{m, base, byte(i)})
	}
	slices.SortFunc(rs, func(a, b region) int { return cmp.Compare(a.base, b.base) })
	for i, r := range rs {
		if i > 0 && rs[i-1].base+size+pageSize > r.base {
			t.Fatalf("stacks at %#x and %#x overlap", rs[i-1].base, r.base)
		}
		rel, err := r.m.Release(nil, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if rel.Stack != size-StackKeep {
			t.Fatalf("the trim of the stack at %#x handed back %d bytes, want %d", r.base, rel.Stack, size-StackKeep)
		}
		if j := same(r.base, StackKeep, r.seed); j >= 0 {
			t.Fatalf("the trim of the stack at %#x changed byte %d of the part kept", r.base, j)
		}
		if err := r.m.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClose(t *testing.T) {
	retryTaken(t, "another mapping was where Close unmapped", func() bool {
		const stackSize = 1 << 20
		m, err := New(stackSize)
		if err != nil {
			t.Fatal(err)
		}
		a := grow(t, m, Unit)
		b := grow(t, m, 64*Unit)
		fill(a, Unit, 1)
		fill(b, 64*Unit, 2)
		base, size := m.Stack()
		fill(base, size, 3)
		calls := failing(t, func(c call, before []call) error { return nil })
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		fault = nil
		for _, c := range *calls {
			// The guard page goes with the stack. windows frees the whole
			// reservation, which takes no size.
			if c.addr == base && runtime.GOOS != "windows" && c.size != size+pageSize {
				t.Fatalf("Close unmapped %d bytes of the stack, want %d with the guard page", c.size, size+pageSize)
			}
		}
		if u := m.Usage(); u != (Usage{}) {
			t.Fatalf("Usage after Close: %+v", u)
		}
		if err := m.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		mustPanic(t, "Grow after Close", func() { m.Grow(Unit) })
		mustPanic(t, "Release after Close", func() { m.Release(nil, nil, 0) })
		return unmapped(t, a, Unit, 1) && unmapped(t, b, 64*Unit, 2) && unmapped(t, base, size, 3)
	})
}

// TestConcurrentVMs runs VMs on many goroutines at once, and hands one VM
// from goroutine to goroutine, for the race detector: nothing is shared
// between Memories, and one needs no lock when goroutines take turns.
func TestConcurrentVMs(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Go(func() {
			for i := 0; i < 20; i++ {
				m, err := New(1 << 20)
				if err != nil {
					t.Error(err)
					return
				}
				a, err1 := m.Grow(Unit)
				b, err2 := m.Grow(8 * Unit)
				if err := errors.Join(err1, err2); err != nil {
					t.Error(err)
					return
				}
				fill(b, 8*Unit, byte(g))
				if CanRemap {
					if b, err = m.Remap(b, 8*Unit, 16*Unit); err != nil {
						t.Error(err)
						return
					}
				}
				bytesAt(m.Stack())[0] = 1
				if _, err := m.Release(nil, each([2]uintptr{b, 4 * Unit}), 0); err != nil {
					t.Error(err)
				}
				if err := m.Free(a, Unit); err != nil {
					t.Error(err)
				}
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			}
		})
	}

	// The channel holds the VM's last chunk, and whoever takes it has the VM.
	m := newMemory(t, 1<<20)
	turn := make(chan uintptr, 1)
	turn <- grow(t, m, Unit)
	for g := 0; g < 4; g++ {
		wg.Go(func() {
			for i := 0; i < 10; i++ {
				p := <-turn
				if _, err := m.Release(each([2]uintptr{p, Unit}), nil, 0); err != nil {
					t.Error(err)
				}
				p, err := m.Grow(Unit)
				if err != nil {
					t.Error(err)
				}
				turn <- p
			}
		})
	}
	wg.Wait()
	if u := m.Usage(); u.Chunks != 1 || u.Footprint != Unit {
		t.Fatalf("after 40 turns: %+v, want the last chunk", u)
	}
}

func BenchmarkGrowFree(b *testing.B) {
	m := newMemory(b, Unit)
	for b.Loop() {
		p := grow(b, m, 4<<20)
		bytesAt(p, 1)[0] = 1
		if err := m.Free(p, 4<<20); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIdleTrim is an idle trim of a 32 MiB stack after a render that
// touched its first n bytes, the touching included.
func BenchmarkIdleTrim(b *testing.B) {
	for _, n := range []uintptr{0, StackKeep, 1 << 20} {
		b.Run(fmt.Sprintf("touched=%dKiB", n>>10), func(b *testing.B) {
			m := newMemory(b, DefaultStackSize)
			base, _ := m.Stack()
			for b.Loop() {
				s := bytesAt(base, n)
				for i := uintptr(0); i < n; i += pageSize {
					s[i] = 1
				}
				if _, err := m.Release(nil, nil, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkNewClose(b *testing.B) {
	for b.Loop() {
		m, err := New(DefaultStackSize)
		if err != nil {
			b.Fatal(err)
		}
		m.Close()
	}
}
