// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// The async context of a runtime is an object javascript sets, which promise
// reactions keep: a handler that perform_promise_then registers while the
// context is an object runs with that context, whatever the context is when
// its job runs, and the context is given back after it. That is what an
// AsyncLocalStorage needs to follow await and promise reactions, and what
// V8 keeps for Node as the continuation preserved embedder data.
//
// internal/fixlib wires it in (see fixAsyncContext): perform_promise_then
// passes each handler through asyncContextWrap, and so does
// js_promise_resolve_function_call with the then of a thenable before it
// queues the job that calls it, which then runs with the context of the
// resolve, as in Node. Without a context, which is all a runtime has until
// javascript sets one, asyncContextWrap returns the function as it is, so
// promises do what they did before at the cost of one test. With one, the
// function becomes a function of asyncContextCall that holds the context and
// the function.

// asyncContextOf returns the async context of the runtime of ctx: an object
// the runtime holds a reference to, or a value whose tag is not
// JS_TAG_OBJECT for none. JS_NewRuntime2 clears the state of the fork, so a
// runtime starts with none.
func asyncContextOf(ctx uintptr) *TJSValue {
	return &forkOf((*TJSContext)(unsafe.Pointer(ctx)).Frt).async
}

// XJS_GetAsyncContext returns the async context of the runtime of ctx, which
// the caller owns, or undefined when it has none.
func XJS_GetAsyncContext(tls *libc.TLS, ctx uintptr) TJSValue {
	v := asyncContextOf(ctx)
	if v.Ftag != EJS_TAG_OBJECT {
		return TJSValue{Ftag: EJS_TAG_UNDEFINED}
	}
	return XDupValue(tls, ctx, *v)
}

// XJS_SetAsyncContext makes v, which the caller keeps, the async context of
// the runtime of ctx when it is an object, and leaves the runtime without
// one for any other value. The runtime frees the context it had. A host
// sets undefined before it frees the context, as a context held by the
// runtime would outlive the objects of the context.
func XJS_SetAsyncContext(tls *libc.TLS, ctx uintptr, v TJSValue) {
	slot := asyncContextOf(ctx)
	old := *slot
	*slot = TJSValue{Ftag: EJS_TAG_UNDEFINED}
	if v.Ftag == EJS_TAG_OBJECT {
		*slot = XDupValue(tls, ctx, v)
	}
	if old.Ftag == EJS_TAG_OBJECT {
		XFreeValue(tls, ctx, old)
	}
}

// asyncContextWrap returns handler, a value its caller holds a reference to,
// for the reaction or the job it queues, and takes that reference.
// With an async context and a function handler, it returns a new function of
// asyncContextCall that holds both, else handler itself. When the engine
// cannot make the function, as when it runs out of memory, the handler runs
// without the context: the exception is left for the caller of the engine,
// as out of memory is uncatchable.
func asyncContextWrap(tls *libc.TLS, ctx uintptr, handler TJSValue) TJSValue {
	context := asyncContextOf(ctx)
	if context.Ftag != EJS_TAG_OBJECT || handler.Ftag != EJS_TAG_OBJECT {
		return handler
	}
	bp := tls.Alloc(32)
	*(*TJSValue)(unsafe.Pointer(bp)) = *context
	*(*TJSValue)(unsafe.Pointer(bp + 16)) = handler
	wrapped := XJS_NewCFunctionData(tls, ctx, __ccgo_fp(asyncContextCall), 1, 0, 2, bp)
	tls.Free(32)
	if wrapped.Ftag == EJS_TAG_EXCEPTION {
		return handler
	}
	XFreeValue(tls, ctx, handler)
	return wrapped
}

// asyncContextCall is the function asyncContextWrap makes, a
// JSCFunctionData: func_data holds the async context and the handler. It
// calls the handler with the context as the context of the runtime, and
// then gives the runtime the context it had, also when the handler threw or
// set another context.
func asyncContextCall(tls *libc.TLS, ctx uintptr, this_val TJSValue, argc int32, argv uintptr, magic int32, func_data uintptr) TJSValue {
	slot := asyncContextOf(ctx)
	outer := *slot
	*slot = XDupValue(tls, ctx, *(*TJSValue)(unsafe.Pointer(func_data)))
	r := XJS_Call(tls, ctx, *(*TJSValue)(unsafe.Pointer(func_data + 16)), this_val, argc, argv)
	if slot.Ftag == EJS_TAG_OBJECT {
		XFreeValue(tls, ctx, *slot)
	}
	*slot = outer
	return r
}
