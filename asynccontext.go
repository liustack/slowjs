// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import lib "github.com/liustack/slowjs/lib"

// The async context of a VM is an object javascript sets with
// SetAsyncContext and reads with GetAsyncContext, which promise jobs keep: a
// reaction, what then, catch, finally and await register, runs with the
// context the VM had when it was registered, and the VM gets back the
// context it had when the reaction ends. That is what an AsyncLocalStorage
// needs to follow an async function across await, and what V8 keeps for
// Node as the continuation preserved embedder data. A VM starts with no
// context, and promises then cost what they did before it.
//
// The context is an object or none. An AsyncLocalStorage keeps the stores of
// each of its instances in the object, as a Map from the instance to its
// store, and makes a new object for each run, so the object of a context is
// not changed once it is set.
var (
	// GetAsyncContext returns the async context of the VM, undefined when it
	// has none.
	GetAsyncContext = NewBuiltin("getAsyncContext", 0, func(a Args) (Value, error) {
		return Value{vm: a.vm, v: lib.XJS_GetAsyncContext(a.vm.runtime.tls, a.vm.cContext)}, nil
	})
	// SetAsyncContext sets the async context of the VM to its first argument
	// when it is an object, and to none for any other value.
	SetAsyncContext = NewBuiltin("setAsyncContext", 1, func(a Args) (Value, error) {
		lib.XJS_SetAsyncContext(a.vm.runtime.tls, a.vm.cContext, a.arg(0))
		return UndefinedValue, nil
	})
)
