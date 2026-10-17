// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

// Uncatchable exceptions.
//
// The engine marks an exception uncatchable (an interrupt) so that no catch
// or finally block of a script runs for it. Engine code that handles an
// exception itself used to take it out of the runtime and go on: a promise
// job or an async function turned it into a rejection, which a handler of
// the script then received, a built-in running an iterator's 'return'
// method threw it again as an ordinary exception, and any later error
// replaced it. This file is the Go form of the change in
// internal/patch/quickjs.diff that keeps an uncatchable exception until the
// embedder takes it with JS_GetException:
//
//   - JS_Throw does not replace a pending uncatchable exception.
//   - The engine's own handlers of an exception (promise jobs, async
//     functions and generators, Promise.all and the other combinators,
//     modules) get it from _js_get_exception, which leaves an uncatchable
//     exception pending and returns a copy.
//   - JS_CallInternal fails at once while an uncatchable exception is
//     pending, so no function runs: not the rejection handler the engine
//     was about to call, nor any other.
//   - JS_IteratorClose does not run an iterator's 'return' method for one.
//
// internal/fixlib wires these into every ccgo_*.go file.

import (
	"unsafe"

	"modernc.org/libc"
)

// _js_uncatchable_pending reports whether an uncatchable exception is
// pending in the runtime of ctx. The flag alone is not enough: it stays set
// after the exception is taken, until the next one is thrown.
func _js_uncatchable_pending(tls *libc.TLS, ctx uintptr) bool {
	if _js_backtrace_uncatchable(tls, ctx) == 0 {
		return false
	}
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	return jsvTag((*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_exception) != EJS_TAG_UNINITIALIZED
}

// _js_get_exception is JS_GetException for the engine's own handlers of an
// exception. An uncatchable exception stays pending, and the caller gets a
// copy to free.
func _js_get_exception(tls *libc.TLS, ctx uintptr) TJSValue {
	if _js_uncatchable_pending(tls, ctx) {
		rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
		return XDupValue(tls, ctx, (*TJSRuntime)(unsafe.Pointer(rt)).Fcurrent_exception)
	}
	return XJS_GetException(tls, ctx)
}

// XJS_IsUncatchableException reports whether the exception pending in ctx is
// uncatchable. Every call into javascript fails while it is.
func XJS_IsUncatchableException(tls *libc.TLS, ctx uintptr) TBOOL {
	if _js_uncatchable_pending(tls, ctx) {
		return 1
	}
	return 0
}
