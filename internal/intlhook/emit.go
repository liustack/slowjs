// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intlhook

import (
	"errors"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The functions here make every string and array a Builtin of intl or
// temporal answers: the other way of the door (door.go).

// emitEvery is how many elements an array is made of between two checks of
// the call: NewArray checks the call itself as it fills the array, and this
// is for the strings made before it.
const emitEvery = 4096

// Emit is the javascript string of s.
func Emit(vm *quickjs.VM, s string) (quickjs.Value, error) { return vm.NewString(s) }

// EmitStrings is a javascript array of the strings of l.
func EmitStrings(vm *quickjs.VM, l []string) (quickjs.Value, error) {
	vals := make([]quickjs.Value, 0, len(l))
	defer func() { freeAll(vals) }()
	for i, s := range l {
		if i%emitEvery == emitEvery-1 && vmhook.Stop(vm) {
			return quickjs.UndefinedValue, nil
		}
		v, err := vm.NewString(s)
		if err != nil {
			return quickjs.Value{}, err
		}
		vals = append(vals, v)
	}
	return vm.NewArray(vals...)
}

// EmitMixed is a javascript array of l, strings and ints.
func EmitMixed(vm *quickjs.VM, l []any) (quickjs.Value, error) {
	vals := make([]quickjs.Value, 0, len(l))
	defer func() { freeAll(vals) }()
	for i, x := range l {
		if i%emitEvery == emitEvery-1 && vmhook.Stop(vm) {
			return quickjs.UndefinedValue, nil
		}
		var v quickjs.Value
		switch x := x.(type) {
		case string:
			var err error
			if v, err = vm.NewString(x); err != nil {
				return quickjs.Value{}, err
			}
		case int:
			v = vm.NewInt(x)
		default:
			return quickjs.Value{}, errors.New("intlhook: internal error: a value of another type in an array")
		}
		vals = append(vals, v)
	}
	return vm.NewArray(vals...)
}

// EmitInts is a javascript array of the numbers of l.
func EmitInts(vm *quickjs.VM, l []int) (quickjs.Value, error) {
	vals := make([]quickjs.Value, len(l))
	for i, n := range l {
		vals[i] = vm.NewInt(n)
	}
	return vm.NewArray(vals...)
}

// EmitValues is a javascript array of vals, made by the functions above or
// numbers, which it borrows as VM.NewArray does.
func EmitValues(vm *quickjs.VM, vals []quickjs.Value) (quickjs.Value, error) {
	return vm.NewArray(vals...)
}

// freeAll frees each value of vals. A caller defers it in a closure, so
// that it sees vals as far as it has been filled when the caller returns:
// NewArray borrows the values, and the array holds its own references.
func freeAll(vals []quickjs.Value) {
	for i := range vals {
		vals[i].Free()
	}
}
