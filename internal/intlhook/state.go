// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intlhook

import (
	"sync"

	"github.com/liustack/slowjs"
)

// states contains borrowed handles. A private field on the VM's global
// object roots each state until Close. It cannot be enumerated, read or
// deleted by a page, even when the page replaces globalThis. No Go-owned
// reference survives Close, and OnClose only removes the borrowed handle.
var states sync.Map // *slowjs.VM -> slowjs.Value

// State returns an owned reference to the private object shared by the two
// shims. Both Register functions call it before page code runs. Keeping the
// callbacks in JS lets the engine trace their closures and release them
// with the context, without Go roots that would keep the context alive.
func State(vm *slowjs.VM) (slowjs.Value, error) {
	if v, ok := states.Load(vm); ok {
		return v.(slowjs.Value).Dup(), nil
	}
	v, err := vm.EvalValue(`(() => {
        const state = {__proto__: null, temporal: undefined, locale: undefined};
        class Base { constructor(o) { return o; } }
        class Root extends Base { #state = state; }
        new Root(globalThis);
        return state;
    })()`, slowjs.EvalGlobal)
	if err != nil {
		return slowjs.Value{}, err
	}
	states.Store(vm, v)
	vm.OnClose(func() { states.Delete(vm) })
	return v, nil
}
