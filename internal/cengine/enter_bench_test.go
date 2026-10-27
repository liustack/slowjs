// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine

package cengine

import "testing"

// The cost of going into the engine and out again, which gohost.Enter and
// Leave add to, SetPanicOnFault included (see docs/fork-changes/c-engine.md).

func benchVM(b *testing.B) *VM {
	b.Helper()
	v, err := New(Options{Seed: 1})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { v.Close() })
	return v
}

// BenchmarkEnterOnly is an entry that calls nothing.
func BenchmarkEnterOnly(b *testing.B) {
	v := benchVM(b)
	f := func() {}
	for b.Loop() {
		v.enter(f)
	}
}

// BenchmarkEnterCall is an entry with the smallest call of the engine in it,
// one that reads a counter.
func BenchmarkEnterCall(b *testing.B) {
	v := benchVM(b)
	f := func() { JS_GetMemorySize(v.sp, v.rt) }
	for b.Loop() {
		v.enter(f)
	}
}

// BenchmarkEvalTiny is a script that does next to nothing: a few entries
// around a few hundred nanoseconds of the engine.
func BenchmarkEvalTiny(b *testing.B) {
	v := benchVM(b)
	for b.Loop() {
		if _, err := v.Eval("1", "tiny.js"); err != nil {
			b.Fatal(err)
		}
	}
}
