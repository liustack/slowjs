// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intlhook

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The Builtins of the Intl and Temporal shims are made with NewBuiltin and
// read their string arguments with String and WTF8, which stop the Builtin
// when the string cannot be read.
//
// Reading a string can fail: a rope, a string kept as the pieces it was
// joined from, has to be joined first, which takes memory under the VM's
// limit and can be interrupted. Args.String then returns ok false, as it does
// for an argument that is not a string, and leaves the uncatchable error
// pending, which ends the call whatever the Builtin returns. A Builtin that
// took the empty string for the argument and went on could do work for
// nothing, or fail an invariant of its own and panic, which would go on in
// the Go code that called javascript. So String and WTF8 panic with
// failedArg instead, and the Builtin's function, made by NewBuiltin, returns
// at once. A Builtin reads its arguments before it makes a Value, so that
// the panic leaves none behind.

// failedArg is what String and WTF8 panic with when an argument that is a
// string could not be read.
type failedArg struct{}

// NewBuiltin is quickjs.NewBuiltin, for a function fn that reads its string
// arguments with String and WTF8: when one fails, the Builtin returns at
// once, and the call ends with the pending error.
func NewBuiltin(name string, length int, fn func(quickjs.Args) (quickjs.Value, error)) *quickjs.Builtin {
	return quickjs.NewBuiltin(name, length, func(a quickjs.Args) (v quickjs.Value, err error) {
		defer func() {
			switch p := recover(); p.(type) {
			case nil:
			case failedArg:
				// The error is pending (see Args.String).
				v, err = quickjs.UndefinedValue, nil
			default:
				panic(p)
			}
		}()
		return fn(a)
	})
}

// String is a.String(i), for a function given to NewBuiltin.
func String(a quickjs.Args, i int) (string, bool) {
	s, ok := a.String(i)
	if !ok && failed(a, i) {
		panic(failedArg{})
	}
	return s, ok
}

// WTF8 is a.WTF8(i), for a function given to NewBuiltin.
func WTF8(a quickjs.Args, i int) (string, bool) {
	s, ok := a.WTF8(i)
	if !ok && failed(a, i) {
		panic(failedArg{})
	}
	return s, ok
}

// failed reports whether argument i of a, which Args.String or Args.WTF8 did
// not read, is a string they failed to read: it is none of the other values
// the shims pass, and the error that ends the call is pending.
func failed(a quickjs.Args, i int) bool {
	if i >= a.Len() || a.Value(i).IsUndefined() {
		return false
	}
	if _, ok := a.Number(i); ok {
		return false
	}
	if _, ok := a.Bool(i); ok {
		return false
	}
	return vmhook.Stop(a.VM())
}
