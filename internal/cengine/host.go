// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine

package cengine

import (
	"fmt"

	"github.com/liustack/slowjs/internal/gohost"
)

// The callbacks the generated assembly calls by name. Their Go signatures
// follow what the generator lays out: argument 0 is the shadow pointer, the
// first free byte of the shadow stack, then one word for each argument of
// the C function and two for a JSValue, and the result likewise. A C int
// comes in a whole word whose upper half is garbage, so each is cut to its
// C type here. engine_decl.go lists the signatures the engine was generated
// with at its end.

// host_memory_grow is uintptr_t host_memory_grow(uint32_t vm, uintptr_t size).
func host_memory_grow(sp, vm, size uintptr) uintptr {
	hook("host_memory_grow", vm)
	return gohost.MemoryGrow(uint32(vm), size)
}

// host_now is int64_t host_now(uint32_t vm, int kind).
func host_now(sp, vm, kind uintptr) int64 {
	hook("host_now", vm)
	return gohost.Now(uint32(vm), int32(kind))
}

// host_call is JSValue host_call(uint32_t vm, const JSValue *this_val, int
// argc, JSValueConst *argv, int magic). The this value is passed by address,
// so that the call has 6 words with the shadow pointer, as many as the body
// calling mode has registers for.
func host_call(sp, vm, this, argc, argv, magic uintptr) (r0, r1 uintptr) {
	hook("host_call", vm)
	if r, ok := callOverride(int32(magic)); ok {
		return r.U, r.Tag
	}
	r := gohost.Call(sp, uint32(vm), this, int32(argc), argv, int32(magic))
	return r.U, r.Tag
}

// host_trap takes the shadow pointer and returns nothing. The C never calls
// it: the shadow-stack pass puts a call to it where a trap instruction would
// have been, which in Go would end the process. It panics and the VM is
// broken (gohost.Trap).
func host_trap(sp uintptr) {
	gohost.Trap(sp)
}

// host_poll is void host_poll(void), which the generator calls when the
// count of cycles runs out (internal/goasm/gen/pass/poll.cpp): the C has no
// call of it. Its stack check is where the Go runtime stops the goroutine in
// the engine, which the post-link check holds it to.
func host_poll(sp uintptr) {
	gohost.Poll(sp)
}

// host_release is not called yet: no Go function is registered. It takes the
// form the plan gives it: host_release(vm, handle).
func host_release(sp, vm, handle uintptr) {
	gohost.Release(uint32(vm), uint32(handle))
}

// goasmMissing is what a function the generator could not take calls in its
// place: i indexes missingNames of engine_decl.go.
func goasmMissing(i uintptr) {
	panic(fmt.Sprintf("cengine: %s was not generated", missingNames[i]))
}
