// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"testing"
	"time"
)

// TestAsyncCodeStopFails runs into the eval timeout in async code that a
// getter, a setter or toJSON starts: an async function before its first
// await, an async generator, a for await over a sync iterator, and the close
// of an iterator by Set.prototype.isDisjointFrom. Each method returns the
// timeout, not the value the code turned the stop into (a pending promise, a
// success), and leaves no exception behind.
func TestAsyncCodeStopFails(t *testing.T) {
	const setup = `
globalThis.o = {};
Object.defineProperty(o, "get", { get: async function () { for (;;) {} } });
Object.defineProperty(o, "set", { set: async function (v) { for (;;) {} } });
Object.defineProperty(o, "gen", { get: function () {
  return (async function* () { for (;;) {} })().next(); } });
Object.defineProperty(o, "syncIter", { get: function () {
  return (async function () {
    for await (const x of { [Symbol.iterator]() { return { next() { for (;;) {} } }; } }) {}
  })(); } });
Object.defineProperty(o, "setClose", { get: function () {
  return new Set([1, 2]).isDisjointFrom({ size: 1, has() { return true; }, keys() {
    return { next() { return { value: 1, done: false }; }, return() { for (;;) {} } }; } });
} });
globalThis.j = {};
Object.defineProperty(j, "toJSON", { get: async function () { for (;;) {} } });
0;
`
	type stopCase struct {
		name string
		run  func(t *testing.T, m *VM) error
	}
	var cases []stopCase
	for _, prop := range []string{"get", "gen", "syncIter", "setClose"} {
		cases = append(cases, stopCase{"GetPropertyValue " + prop, func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "o")
			defer obj.Free()
			v, err := m.GetPropertyValue(obj, atomOrFatal(t, m, prop))
			v.Free()
			return err
		}})
	}
	cases = append(cases, stopCase{"SetPropertyValue", func(t *testing.T, m *VM) error {
		obj := evalValueOrFatal(t, m, "o")
		defer obj.Free()
		return m.SetPropertyValue(obj, atomOrFatal(t, m, "set"), m.NewInt(1))
	}}, stopCase{"MarshalJSON", func(t *testing.T, m *VM) error {
		obj := evalValueOrFatal(t, m, "j")
		defer obj.Free()
		_, err := obj.MarshalJSON()
		return err
	}})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
			wantKind(t, c.run(t, m), interrupted)
			wantNoPendingException(t, m)
			if err := m.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			wantEval(t, m, "1 + 1", 2)
		})
	}
}
