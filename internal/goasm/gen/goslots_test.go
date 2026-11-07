// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"reflect"
	"testing"
)

// TestGoSlots: the words of the Go argument area have the names of the Go
// declaration, a0 and on across the words of the arguments and ret, or r0
// and r1 for a JSValue, after them, at the offsets of layout.
func TestGoSlots(t *testing.T) {
	for _, c := range []struct {
		sig             Sig
		decl            string
		params, results []GoSlot
	}{
		{Sig{Params: []Type{Ptr, Ptr}}, "func f(a0 uintptr, a1 uintptr)\n",
			[]GoSlot{{"a0", 0, false}, {"a1", 8, false}}, nil},
		{Sig{Params: []Type{Ptr, I64, F64, Ptr}, Ret: I64}, "func f(a0 uintptr, a1 uintptr, a2 float64, a3 uintptr) uintptr\n",
			[]GoSlot{{"a0", 0, false}, {"a1", 8, false}, {"a2", 16, true}, {"a3", 24, false}}, []GoSlot{{"ret", 32, false}}},
		{Sig{Params: []Type{Ptr, JSV, Ptr}, Ret: JSV}, "func f(a0 uintptr, a1, a2 uintptr, a3 uintptr) (r0, r1 uintptr)\n",
			[]GoSlot{{"a0", 0, false}, {"a1", 8, false}, {"a2", 16, false}, {"a3", 24, false}}, []GoSlot{{"r0", 32, false}, {"r1", 40, false}}},
		{Sig{Params: []Type{Ptr, F64}, Ret: F64}, "func f(a0 uintptr, a1 float64) float64\n",
			[]GoSlot{{"a0", 0, false}, {"a1", 8, true}}, []GoSlot{{"ret", 16, true}}},
	} {
		if got := GoDecl("f", c.sig); got != c.decl {
			t.Errorf("GoDecl(%v) = %q, want %q", c.sig, got, c.decl)
		}
		params, results := GoSlots(c.sig)
		if !reflect.DeepEqual(params, c.params) || !reflect.DeepEqual(results, c.results) {
			t.Errorf("GoSlots(%v) = %v %v, want %v %v", c.sig, params, results, c.params, c.results)
		}
	}
}
