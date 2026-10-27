// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"
)

// What a call passes the engine that does not fit the shadow stack, such as
// a source of more than 64 KiB, goes to the VM's heap, under its memory
// limit. When the heap has no room for it, the call ends as running out of
// memory does in the engine: an uncatchable error, which no try catches in
// javascript that called the Go function making the call.
func TestTmpOutOfMemory(t *testing.T) {
	m := newTestVM(t)
	src := "/*" + strings.Repeat("x", 300<<10) + "*/ 1"
	defineBuiltin(t, m, "bigEval", NewBuiltin("bigEval", 0, func(a Args) (Value, error) {
		_, err := a.VM().Eval(src, EvalGlobal)
		return Value{}, err
	}))
	// A Go function that drops the error still ends the javascript that
	// called it: the error stays pending.
	defineBuiltin(t, m, "bigEvalIgnored", NewBuiltin("bigEvalIgnored", 0, func(a Args) (Value, error) {
		a.VM().Eval(src, EvalGlobal)
		return a.VM().NewInt(1), nil
	}))
	wantOOM := func(what string, err error) {
		t.Helper()
		if e, ok := err.(*Error); !ok || !e.OutOfMemory || !e.Uncatchable {
			t.Fatalf("%s failed with %T %v, want the uncatchable out of memory error", what, err, err)
		}
	}
	m.SetMemoryLimit(uintptr(m.MemoryUsage().MallocSize) + 128<<10)
	_, err := m.Eval(src, EvalGlobal)
	wantOOM("Eval of a large source", err)
	_, err = m.Eval(`try { bigEval() } catch (e) { 'caught ' + e }`, EvalGlobal)
	wantOOM("Eval of a large source in a Go function", err)
	_, err = m.Eval(`try { bigEvalIgnored() } catch (e) { 'caught ' + e }`, EvalGlobal)
	wantOOM("Eval of a large source in a Go function that drops the error", err)
	m.SetMemoryLimit(1 << 40)
	if got, err := m.Eval(src, EvalGlobal); err != nil || got != 1 {
		t.Fatalf("with memory again: %v, %v", got, err)
	}
}
