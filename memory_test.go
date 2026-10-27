// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

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
