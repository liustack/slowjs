// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

// A module lets go of its top level once it has run, which only the engine of
// lib does so far.

import (
	"strconv"
	"testing"
)

// TestModuleFreesItsTopLevel loads a module whose top level holds a large
// array in a variable no function captures, as a script's top-level function
// holds its locals. Once the module has run, nothing can call its top-level
// function again: the function goes, and the array with it, while the
// functions the module made keep the variables they use.
func TestModuleFreesItsTopLevel(t *testing.T) {
	for _, src := range []string{
		"const big = new Uint8Array(" + strconv.Itoa(moduleGarbage) + ").fill(1); let kept = big[0] + 1;\nglobalThis.get = () => kept++\n",
		"const big = new Uint8Array(" + strconv.Itoa(moduleGarbage) + ").fill(1); let kept = big[0] + 1;\nawait null;\nglobalThis.get = () => kept++\n",
	} {
		code := moduleBytecode(t, src)
		m := newTestVM(t)
		if _, err := m.EvalBytecode(code); err != nil {
			t.Fatal(err)
		}
		m.RunGC()
		if size := m.MemorySize(); size > moduleGarbage/2 {
			t.Errorf("%q: the VM holds %d bytes after the module ran, want the %d of the array freed", src, size, moduleGarbage)
		}
		wantEval(t, m, "get() + get()", 5)
	}
}
