// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"strings"
	"testing"
)

// benchVM returns a new VM, after setup, that the end of the benchmark
// closes. It uses only the exported API.
func benchVM(b *testing.B, setup string) *VM {
	b.Helper()
	m, err := NewVM()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { m.Close() })
	if _, err := m.Eval(setup, EvalGlobal); err != nil {
		b.Fatal(err)
	}
	return m
}

// benchFunc returns the global function name of m.
func benchFunc(b *testing.B, m *VM, name string) Value {
	b.Helper()
	f, err := m.EvalValue(name, EvalGlobal)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(f.Free)
	return f
}

// BenchmarkPublicAPI times the calls a program makes into the engine on the
// way tifa renders a page: a call with long string arguments and a long
// string result, a call by name, an eval, the conversion of an object, Go
// functions that javascript calls, and the drain of the job queue.
func BenchmarkPublicAPI(b *testing.B) {
	page := strings.Repeat("<div class=\"row\">text &amp; more</div>", 2000)
	const setup = `
		function id(a) { return a }
		function render(input, text) { return input.length + text.length }
		function html() { return globalThis.page }
		function obj() { return {a: 1, b: 'two', c: [3, 4, 5], d: {e: true}} }
		function nothing() {}`

	b.Run("ValueCall/args", func(b *testing.B) {
		m := benchVM(b, setup)
		f := benchFunc(b, m, "render")
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := f.Call(UndefinedValue, page, "short text"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ValueCall/result", func(b *testing.B) {
		m := benchVM(b, setup)
		if _, err := m.Call("(p => { globalThis.page = p })", page); err != nil {
			b.Fatal(err)
		}
		f := benchFunc(b, m, "html")
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := f.Call(UndefinedValue); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ValueCall/none", func(b *testing.B) {
		m := benchVM(b, setup)
		f := benchFunc(b, m, "nothing")
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := f.Call(UndefinedValue); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Call/int", func(b *testing.B) {
		m := benchVM(b, setup)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := m.Call("id", 42); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Call/object", func(b *testing.B) {
		m := benchVM(b, setup)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := m.Call("obj"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Eval", func(b *testing.B) {
		m := benchVM(b, setup)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := m.Eval(`1 + 2`, EvalGlobal); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("NewString", func(b *testing.B) {
		m := benchVM(b, setup)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			s, err := m.NewString("a short string")
			if err != nil {
				b.Fatal(err)
			}
			s.Free()
		}
	})
	// The Go functions: ns/op and allocs/op are per call from javascript.
	b.Run("RegisterFunc", func(b *testing.B) {
		m := benchVM(b, setup)
		if err := m.RegisterFunc("gofn", func(s string, n int) string { return s }, false); err != nil {
			b.Fatal(err)
		}
		if _, err := m.Eval(`globalThis.loop = n => { for (let i = 0; i < n; i++) gofn('text', i) }`, EvalGlobal); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		if _, err := m.Call("loop", b.N); err != nil {
			b.Fatal(err)
		}
	})
	b.Run("RegisterHostFunc", func(b *testing.B) {
		m := benchVM(b, setup)
		if err := m.RegisterHostFunc("hostfn", func(args []any) (any, error) { return args[0], nil }); err != nil {
			b.Fatal(err)
		}
		if _, err := m.Eval(`globalThis.loop = n => { for (let i = 0; i < n; i++) hostfn('text', i) }`, EvalGlobal); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		if _, err := m.Call("loop", b.N); err != nil {
			b.Fatal(err)
		}
	})
	// One op is a drain of a chain of 100 jobs, which a call queues first.
	b.Run("ExecutePendingJobs", func(b *testing.B) {
		m := benchVM(b, setup)
		queue := benchFunc(b, m, `(() => { let p = Promise.resolve(); for (let i = 0; i < 100; i++) p = p.then(id) })`)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := queue.Call(UndefinedValue); err != nil {
				b.Fatal(err)
			}
			if n, err := m.ExecutePendingJobs(); err != nil || n < 100 {
				b.Fatalf("ran %d jobs, %v", n, err)
			}
		}
	})
}
