// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifa

import "github.com/liustack/tifajs"

// The async context of the VM is the object promise jobs carry (see tifajs.GetAsyncContext):
// the AsyncLocalStorage of tifa's host script keeps its stores in it. The host takes the two
// functions from __tifa_async_context__ and removes it before any code of the app runs. Moved
// here from internal/ssr/asynccontext.go of tifa.
const asyncContextHook = `(get, set) => {
  Object.defineProperty(globalThis, '__tifa_async_context__', { value: { get, set }, configurable: true })
}`

func registerAsyncContext(vm *tifajs.VM) error {
	define, err := vm.EvalValue(asyncContextHook, tifajs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	get, err := vm.NewBuiltinFunction(tifajs.GetAsyncContext)
	if err != nil {
		return err
	}
	defer get.Free()
	set, err := vm.NewBuiltinFunction(tifajs.SetAsyncContext)
	if err != nil {
		return err
	}
	defer set.Free()
	r, err := define.CallValue(tifajs.UndefinedValue, get, set)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}
