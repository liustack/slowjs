// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intlhook

import (
	"testing"

	"github.com/liustack/quickjs"
)

func TestStateLifetime(t *testing.T) {
	for range 3 {
		vm, err := quickjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		keys, err := vm.Eval("JSON.stringify(Reflect.ownKeys(globalThis).map(String))", quickjs.EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		state, err := State(vm)
		if err != nil {
			t.Fatal(err)
		}
		state.Free()
		after, err := vm.Eval("JSON.stringify(Reflect.ownKeys(globalThis).map(String))", quickjs.EvalGlobal)
		if err != nil || after != keys {
			t.Fatalf("private state exposed: %v %v", after, err)
		}
		if _, err := vm.Eval("globalThis.globalThis = null", quickjs.EvalGlobal); err != nil {
			t.Fatal(err)
		}
		vm.RunGC()
		state, err = State(vm)
		if err != nil {
			t.Fatal(err)
		}
		get, err := vm.EvalValue("s => s.temporal === undefined && s.locale === undefined", quickjs.EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		got, err := get.Call(quickjs.UndefinedValue, state)
		get.Free()
		state.Free()
		if err != nil || got != true {
			t.Fatalf("state after GC: %v %v", got, err)
		}
		if err := vm.Close(); err != nil {
			t.Fatal(err)
		}
		if _, ok := states.Load(vm); ok {
			t.Fatal("closed VM retained")
		}
	}
}
