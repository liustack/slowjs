// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package cengine

import "testing"

// TestPlaceHoldsObjectsTheScriptCannotSee: Place(k) leaves k objects in the
// heap, held until Close, which the script does not see as a global, and
// Close frees them all.
func TestPlaceHoldsObjectsTheScriptCannotSee(t *testing.T) {
	const globals = `Object.getOwnPropertyNames(globalThis).join()`
	plain := newVM(t, Options{})
	defer closeVM(t, plain)
	placed := newVM(t, Options{})
	defer closeVM(t, placed)
	before := objects(placed)
	if err := placed.Place(8); err != nil {
		t.Fatal(err)
	}
	if got := objects(placed); got != before+8 {
		t.Errorf("Place(8) left %d objects, %d before", got, before)
	}
	want, err := plain.Eval(globals, "g.js")
	if err != nil {
		t.Fatal(err)
	}
	got, err := placed.Eval(globals, "g.js")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("the globals after Place(8):\n%s\nwithout it:\n%s", got, want)
	}
	// The objects are held across a collection.
	placed.RunGC()
	plain.RunGC()
	if d, d0 := objects(placed), objects(plain); d != d0+8 {
		t.Errorf("after a GC the placed VM has %d objects, the plain one %d", d, d0)
	}
}

// objects is the number of objects the engine counts in v
// (JS_ComputeMemoryUsage).
func objects(v *VM) int64 {
	v.enter(func() { JS_ComputeMemoryUsage(v.sp, v.rt, v.base+scratchUsage) })
	return (*memoryUsage)(addr(v.base + scratchUsage)).ObjCount
}

// TestPlaceZeroDoesNothing: Place(0) leaves the VM as New made it, so that
// k = 0 is the run of today.
func TestPlaceZeroDoesNothing(t *testing.T) {
	a := newVM(t, Options{})
	defer closeVM(t, a)
	b := newVM(t, Options{})
	defer closeVM(t, b)
	if err := b.Place(0); err != nil {
		t.Fatal(err)
	}
	if x, y := a.Usage().MemorySize, b.Usage().MemorySize; x != y {
		t.Errorf("Place(0) moved the memory from %d to %d bytes", x, y)
	}
	if x, y := objects(a), objects(b); x != y {
		t.Errorf("Place(0) made %d objects into %d", x, y)
	}
}

// TestPlaceRefusesANegativeCount: k below 0 is an error, not a quiet 0.
func TestPlaceRefusesANegativeCount(t *testing.T) {
	v := newVM(t, Options{})
	defer closeVM(t, v)
	if err := v.Place(-1); err == nil {
		t.Error("Place(-1) gave no error")
	}
}
