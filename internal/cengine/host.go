// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine

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
// calling mode has registers for. A magic of 0 or more is a function of the
// host (JS_NewHostFunction), which the HostFunc registered answers. Below 0
// are the requests of the engine, which gohost answers, and so is a function
// of the host when none is registered.
func host_call(sp, vm, this, argc, argv, magic uintptr) (r0, r1 uintptr) {
	hook("host_call", vm)
	if r, ok := callOverride(int32(magic)); ok {
		return r.U, r.Tag
	}
	if id := int32(magic); id >= 0 && hostFunc != nil {
		return hostFunc(sp, uint32(vm), this, int32(argc), argv, id)
	}
	r := gohost.Call(sp, uint32(vm), this, int32(argc), argv, int32(magic))
	return r.U, r.Tag
}

// A HostFunc answers the calls of the functions of the host: id is what
// JS_NewHostFunction was given, vm the VM's id, this the address of the this
// value and argv that of argc values, all in the engine's memory. It returns
// the result as the two words of a JSValue, which the caller owns, or the
// exception after throwing one. sp is the first free byte of the shadow
// stack: what the function passes the engine by address, and the frames of a
// call back into the engine, go above it, since what is below holds the
// frames of the call. The engine finds the VM again by sp, so a call back
// into another VM from it is not allowed.
type HostFunc func(sp uintptr, vm uint32, this uintptr, argc int32, argv uintptr, id int32) (u, tag uintptr)

// hostFunc is the HostFunc registered, or nil.
var hostFunc HostFunc

// SetHostFunc registers f, which answers the calls of every function of the
// host from then on. It is registered once, before any VM runs a script,
// as from an init function: it panics when f is nil or one is registered.
func SetHostFunc(f HostFunc) {
	if f == nil || hostFunc != nil {
		panic("cengine: SetHostFunc of nil, or a second time")
	}
	hostFunc = f
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
