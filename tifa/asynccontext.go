// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package tifa

import "github.com/liustack/slowjs"

// The async context of the VM is the object promise jobs carry (see slowjs.GetAsyncContext):
// the AsyncLocalStorage of tifa's host script keeps its stores in it. The host takes the two
// functions from __tifa_async_context__ and removes it before any code of the app runs. Moved
// here from internal/ssr/asynccontext.go of tifa.
const asyncContextHook = `(get, set) => {
  Object.defineProperty(globalThis, '__tifa_async_context__', { value: { get, set }, configurable: true })
}`

func registerAsyncContext(vm *slowjs.VM) error {
	define, err := vm.EvalValue(asyncContextHook, slowjs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	get, err := vm.NewBuiltinFunction(slowjs.GetAsyncContext)
	if err != nil {
		return err
	}
	defer get.Free()
	set, err := vm.NewBuiltinFunction(slowjs.SetAsyncContext)
	if err != nil {
		return err
	}
	defer set.Free()
	r, err := define.CallValue(slowjs.UndefinedValue, get, set)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}
