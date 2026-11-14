// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"testing"

	"github.com/liustack/quickjs/internal/gohost"
)

// TestAlignSP: AlignSP rounds a pointer up to the next multiple of 16 and
// leaves one on 16 as it is. The first shadow pointer of a VM, past the
// scratch at the base of its region, is one, and so is the pointer a host
// function gets from the engine (TestHostFunction calls back from there).
func TestAlignSP(t *testing.T) {
	if SPAlign != 16 {
		t.Fatalf("SPAlign is %d, the generated code lays out its frames on 16", SPAlign)
	}
	for _, c := range []struct{ in, want uintptr }{
		{0, 0}, {1, 16}, {8, 16}, {15, 16}, {16, 16}, {17, 32}, {31, 32}, {32, 32},
		{1<<40 + 4096, 1<<40 + 4096}, {1<<40 + 4097, 1<<40 + 4112},
	} {
		if got := AlignSP(c.in); got != c.want {
			t.Errorf("AlignSP(%#x) = %#x, want %#x", c.in, got, c.want)
		}
	}
	v := newVM(t, Options{})
	defer closeVM(t, v)
	if v.SP()%SPAlign != 0 {
		t.Errorf("the first shadow pointer %#x is not on %d bytes", v.SP(), SPAlign)
	}
	seen := uintptr(1)
	testHostFuncs[9001] = func(_ *VM, sp uintptr, _ []gohost.Value) (u, tag uintptr) {
		seen = sp
		return 0, tagInt
	}
	defer delete(testHostFuncs, 9001)
	testVMs[v.Host().ID()] = v
	defer delete(testVMs, v.Host().ID())
	if thrown := defineHostFunc(v, "spOf", 0, 9001); thrown != "" {
		t.Fatal(thrown)
	}
	if _, err := v.Eval("spOf()", "sp.js"); err != nil {
		t.Fatal(err)
	}
	if seen == 1 || seen%SPAlign != 0 {
		t.Errorf("a host function got the shadow pointer %#x, not on %d bytes", seen, SPAlign)
	}
}
