// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intlhook

import (
	"github.com/liustack/beastjs"
	"github.com/liustack/beastjs/internal/vmhook"
)

// The functions here make every string and array a Builtin of intl or
// temporal answers: the other way of the door (door.go).

// emitEvery is how many elements an array is made of between two checks of
// the call: NewArray checks the call itself as it fills the array, and this
// is for the strings made before it.
const emitEvery = 4096

// Emit is the javascript string of s.
func Emit(vm *beastjs.VM, s string) (beastjs.Value, error) { return vm.NewString(s) }

// EmitStrings is a javascript array of the strings of l.
func EmitStrings(vm *beastjs.VM, l []string) (beastjs.Value, error) {
	vals := make([]beastjs.Value, 0, len(l))
	defer func() { freeAll(vals) }()
	for i, s := range l {
		if i%emitEvery == emitEvery-1 && vmhook.Stop(vm) {
			return beastjs.UndefinedValue, nil
		}
		v, err := vm.NewString(s)
		if err != nil {
			return beastjs.Value{}, err
		}
		vals = append(vals, v)
	}
	return vm.NewArray(vals...)
}

// EmitUTF16 is the javascript string of the UTF-16 code units u, lone
// surrogates as they are.
func EmitUTF16(vm *beastjs.VM, u []uint16) (beastjs.Value, error) { return vm.NewStringUTF16(u) }

// EmitStringsAs is EmitStrings, but element i is the number asInt gives
// for it, where it gives one.
func EmitStringsAs(vm *beastjs.VM, l []string, asInt func(i int, s string) (int, bool)) (beastjs.Value, error) {
	vals := make([]beastjs.Value, 0, len(l))
	defer func() { freeAll(vals) }()
	for i, s := range l {
		if i%emitEvery == emitEvery-1 && vmhook.Stop(vm) {
			return beastjs.UndefinedValue, nil
		}
		if n, ok := asInt(i, s); ok {
			vals = append(vals, vm.NewInt(n))
			continue
		}
		v, err := vm.NewString(s)
		if err != nil {
			return beastjs.Value{}, err
		}
		vals = append(vals, v)
	}
	return vm.NewArray(vals...)
}

// EmitInts is a javascript array of the numbers of l.
func EmitInts(vm *beastjs.VM, l []int) (beastjs.Value, error) {
	vals := make([]beastjs.Value, len(l))
	for i, n := range l {
		vals[i] = vm.NewInt(n)
	}
	return vm.NewArray(vals...)
}

// EmitValues is a javascript array of vals, made by the functions above or
// numbers, which it borrows as VM.NewArray does.
func EmitValues(vm *beastjs.VM, vals []beastjs.Value) (beastjs.Value, error) {
	return vm.NewArray(vals...)
}

// freeAll frees each value of vals. A caller defers it in a closure, so
// that it sees vals as far as it has been filled when the caller returns:
// NewArray borrows the values, and the array holds its own references.
func freeAll(vals []beastjs.Value) {
	for i := range vals {
		vals[i].Free()
	}
}
