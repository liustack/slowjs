// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import (
	"strings"
	"testing"
)

// MemoryUsage counts what the VM holds: its total is what MemorySize gives,
// and the objects, arrays, strings and functions a script keeps show in their
// counts.
func TestMemoryUsage(t *testing.T) {
	m := newTestVM(t)
	before := m.MemoryUsage()
	if before.MallocSize <= 0 || before.MallocCount <= 0 || before.MemoryUsedSize <= 0 ||
		before.AtomCount <= 0 || before.ObjCount <= 0 || before.ShapeCount <= 0 || before.CFuncCount <= 0 {
		t.Fatalf("a new VM: %+v", before)
	}
	if got := uintptr(before.MallocSize); got != m.MemorySize() {
		t.Errorf("MallocSize %d, MemorySize %d", got, m.MemorySize())
	}
	if before.MallocLimit != -1 {
		t.Errorf("MallocLimit %d with no limit, want -1", before.MallocLimit)
	}

	if _, err := m.Eval(`globalThis.kept = []
for (let i = 0; i < 1000; i++) kept.push({ i, s: 'str' + i + 'x'.repeat(i % 7), f() { return i } }, [i, i + 1])`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	after := m.MemoryUsage()
	for _, c := range []struct {
		name          string
		before, after int64
		grew          int64
	}{
		{"ObjCount", before.ObjCount, after.ObjCount, 3000},
		{"ArrayCount", before.ArrayCount, after.ArrayCount, 1000},
		{"FastArrayCount", before.FastArrayCount, after.FastArrayCount, 1000},
		{"FastArrayElements", before.FastArrayElements, after.FastArrayElements, 3000},
		{"StrCount", before.StrCount, after.StrCount, 1000},
		// The 1000 closures of f share its bytecode, and the script's own
		// is gone once it has run.
		{"JSFuncCount", before.JSFuncCount, after.JSFuncCount, 1},
		{"MallocSize", before.MallocSize, after.MallocSize, 100_000},
	} {
		if c.after-c.before < c.grew {
			t.Errorf("%s went from %d to %d, want %d more", c.name, c.before, c.after, c.grew)
		}
	}
	if got := uintptr(after.MallocSize); got != m.MemorySize() {
		t.Errorf("MallocSize %d, MemorySize %d", got, m.MemorySize())
	}

	m.SetMemoryLimit(64 << 20)
	if got := m.MemoryUsage().MallocLimit; got != 64<<20 {
		t.Errorf("MallocLimit %d, want %d", got, 64<<20)
	}
}

// ReleaseMemory works only while the VM runs no javascript, and says so from
// a Go function that javascript called.
func TestReleaseMemory(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`globalThis.junk = Array.from({ length: 10000 }, (_, i) => ({ i })); junk = null`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	before := m.MemoryUsage()
	released, err := m.ReleaseMemory(0)
	if err != nil {
		t.Fatal(err)
	}
	after := m.MemoryUsage()
	if after.ObjCount != before.ObjCount || after.MallocSize != before.MallocSize {
		t.Errorf("objects %d to %d, size %d to %d: ReleaseMemory freed something", before.ObjCount, after.ObjCount, before.MallocSize, after.MallocSize)
	}
	if after.Footprint > before.Footprint || before.Footprint-after.Footprint > int64(released) {
		t.Errorf("released %d, footprint %d to %d", released, before.Footprint, after.Footprint)
	}
	if got := evalString(t, m, `String(globalThis.junk)`); got != "null" {
		t.Errorf("after ReleaseMemory: %s", got)
	}

	var inside error
	defineBuiltin(t, m, "release", NewBuiltin("release", 0, func(a Args) (Value, error) {
		_, inside = a.VM().ReleaseMemory(0)
		return Value{}, nil
	}))
	if _, err := m.Eval(`release()`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if inside == nil || !strings.Contains(inside.Error(), "runs javascript") {
		t.Errorf("ReleaseMemory from a Go function javascript called: %v", inside)
	}
}

// The constructors of a Value return the error of running out of memory,
// not a Value of the exception the engine returns then.
func TestConstructorsOutOfMemory(t *testing.T) {
	big := strings.Repeat("x", 2<<20)
	for name, f := range map[string]func(m *VM) (Value, error){
		"NewString":        func(m *VM) (Value, error) { return m.NewString(big) },
		"NewString latin1": func(m *VM) (Value, error) { return m.NewString(strings.Repeat("é", 1<<20)) },
		"NewString wide":   func(m *VM) (Value, error) { return m.NewString(strings.Repeat("中", 1<<20)) },
		"NewStringLatin1":  func(m *VM) (Value, error) { return m.NewStringLatin1([]byte(big)) },
		"NewStringUTF16":   func(m *VM) (Value, error) { return m.NewStringUTF16(make([]uint16, 2<<20)) },
		"Call with a string": func(m *VM) (Value, error) {
			_, err := m.Call("String", big)
			return Value{}, err
		},
		"NewObjectValue": func(m *VM) (Value, error) {
			// Fill the memory, so that an object fails.
			var kept []Value
			defer func() {
				for _, v := range kept {
					v.Free()
				}
			}()
			for range 1 << 20 {
				v, err := m.NewObjectValue()
				if err != nil || isException(v.v) {
					return v, err
				}
				kept = append(kept, v)
			}
			return Value{}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestVM(t)
			m.SetMemoryLimit(1 << 20)
			v, err := f(m)
			if err == nil {
				t.Fatalf("got no error, a Value of tag %d", tag(v.v))
			}
			wantOutOfMemory(t, name, err)
			if e := err.(*Error); !e.Uncatchable || !e.OutOfMemory {
				t.Fatalf("got %#v, want an uncatchable out of memory error", e)
			}
			wantNoPendingException(t, m)
			m.SetMemoryLimit(^uintptr(0))
			if r, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || r != 2 {
				t.Fatalf("the next call got %v, %v", r, err)
			}
		})
	}
}

// MemoryFree is the part of MemorySize no block in use holds: never more than
// MemorySize, larger once a collection leaves arenas with holes, and the rest
// of MemorySize, what is in use, comes back when the same objects are made
// again. It can be read at any time, from a Go function too.
func TestMemoryFree(t *testing.T) {
	m := newTestVM(t)
	check := func(what string) (size, free uintptr) {
		t.Helper()
		size, free = m.MemorySize(), m.MemoryFree()
		if free > size {
			t.Fatalf("%s: MemoryFree %d over MemorySize %d", what, free, size)
		}
		if u := m.MemoryUsage(); uintptr(u.FreeSize) != free || uintptr(u.MallocSize) != size {
			t.Fatalf("%s: MemoryUsage gives %d free of %d, MemoryFree %d of %d", what, u.FreeSize, u.MallocSize, free, size)
		}
		return size, free
	}
	m.RunGC()
	size0, free0 := check("a new VM")
	// Make 64000 objects at once and keep one of every 16: the arenas of the
	// others stay, with holes, as long as one object of each lives. The kept
	// objects hold each other in a chain, not in an array: an array grows by
	// realloc, and its block is as big as the room it was given, which
	// depends on what was around it.
	if _, err := m.Eval(`{
	const all = []
	for (let i = 0; i < 64000; i++) all.push({ i, s: 'v' + i, next: null })
	globalThis.kept = null
	for (let i = 0; i < all.length; i += 16) { all[i].next = kept; kept = all[i] }
	undefined
}`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	size1, free1 := check("one object of 16 kept")
	if free1-free0 < 256<<10 {
		t.Errorf("MemoryFree %d to %d with 60000 objects freed among 4000 kept, want 256 KiB more", free0, free1)
	}
	used1 := size1 - free1
	if used1 <= size0-free0 {
		t.Errorf("in use %d, before %d: the kept objects do not show", used1, size0-free0)
	}
	// The same 4000 objects made again, with no garbage between: the same
	// bytes in use, in fewer arenas.
	if _, err := m.Eval(`kept = null`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	if _, err := m.Eval(`for (let i = 0; i < 64000; i += 16) kept = { i, s: 'v' + i, next: kept }; undefined`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	size2, free2 := check("the kept objects made alone")
	if used2 := size2 - free2; used2 != used1 {
		t.Errorf("in use %d with the objects made among garbage, %d made alone", used1, used2)
	}
	if size2 >= size1 {
		t.Errorf("MemorySize %d among garbage, %d alone: the test makes no holes", size1, size2)
	}

	var inside, insideSize uintptr
	defineBuiltin(t, m, "free", NewBuiltin("free", 0, func(a Args) (Value, error) {
		inside, insideSize = a.VM().MemoryFree(), a.VM().MemorySize()
		return Value{}, nil
	}))
	if _, err := m.Eval(`{ const junk = []; for (let i = 0; i < 1000; i++) junk.push({ i }); free() }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if inside > insideSize || insideSize == 0 {
		t.Errorf("from a Go function: MemoryFree %d, MemorySize %d", inside, insideSize)
	}
}

// MemoryFree walks the lists of arenas, not the objects: on an Apple M4, a VM
// of 6400 blocks with holes takes it about 50 microseconds here, and the first
// call after a full collection of the store pages (about 6000 blocks) 100 to
// 400, as the arenas are no longer in the cache.
func BenchmarkMemoryFree(b *testing.B) {
	m, err := NewVM()
	if err != nil {
		b.Fatal(err)
	}
	defer m.Close()
	if _, err := m.Eval(`{
	const all = []
	for (let i = 0; i < 200000; i++) all.push({ i, s: 'v' + i, next: null })
	globalThis.kept = null
	for (let i = 0; i < all.length; i += 16) { all[i].next = kept; kept = all[i] }
	undefined
}`, EvalGlobal); err != nil {
		b.Fatal(err)
	}
	m.RunGC()
	for b.Loop() {
		m.MemoryFree()
	}
	b.ReportMetric(float64(m.MemoryUsage().MallocCount), "blocks")
}

// MemoryUsage counts the register code of the functions that ran, apart from
// their bytecode, and in MemoryUsedSize.
func TestMemoryUsageRegisterCode(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`globalThis.fs = []
for (let i = 0; i < 50; i++) fs.push(new Function('a', 'b', 'let s = 0; for (let j = a; j < b; j++) s += j * ' + i + '; return s'))`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	before := m.MemoryUsage()
	if _, err := m.Eval(`for (const f of fs) f(0, 10)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	after := m.MemoryUsage()
	if after.RegisterCodeCount-before.RegisterCodeCount < 50 || after.RegisterCodeSize-before.RegisterCodeSize < 50*64 {
		t.Errorf("register code %d functions, %d bytes, then %d, %d: want 50 functions more, of 64 bytes or more", before.RegisterCodeCount, before.RegisterCodeSize, after.RegisterCodeCount, after.RegisterCodeSize)
	}
	if after.JSFuncCodeSize != before.JSFuncCodeSize {
		t.Errorf("JSFuncCodeSize %d to %d: the bytecode did not change", before.JSFuncCodeSize, after.JSFuncCodeSize)
	}
	if grew := after.MemoryUsedSize - before.MemoryUsedSize; grew < after.RegisterCodeSize-before.RegisterCodeSize {
		t.Errorf("MemoryUsedSize grew %d, the register code %d", grew, after.RegisterCodeSize-before.RegisterCodeSize)
	}
}
