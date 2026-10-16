// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

// Out of memory.
//
// Running out of memory is uncatchable, like an interrupt: no catch or
// finally block of a script runs for it, and the embedder gets it. A script
// that caught it could not do much with the memory left, and a framework
// catching every error to render a fallback would report it as a page error.
// This file is the Go form of the change in internal/patch/quickjs.diff:
//
//   - JS_ThrowOutOfMemory marks the error it throws uncatchable.
//   - JS_ThrowError2 builds the message before the error object, and when
//     either one or setting the message fails, it throws null marked
//     uncatchable. The embedder reports that null as running out of memory.
//     The engine used to throw a catchable null there, and to give an error
//     whose message string failed the exception marker as its message.
//   - The regexp engine running out of memory throws the out of memory
//     error, not an InternalError with a message of its own or a
//     SyntaxError.
//
// internal/fixlib wires these into every ccgo_*.go file.

import (
	"unsafe"

	"modernc.org/libc"
)

// _js_throw_error2 is the end of JS_ThrowError2, after it formats the
// message into buf: it throws an error of class error_num with that message.
func _js_throw_error2(tls *libc.TLS, ctx uintptr, error_num TJSErrorEnum, buf uintptr, add_backtrace TBOOL) TJSValue {
	msg := XJS_NewStringLen(tls, ctx, buf, libc.Xstrlen(tls, buf))
	if jsvTag(msg) == EJS_TAG_EXCEPTION {
		return _js_throw_out_of_memory_null(tls, ctx)
	}

	proto := (*TJSContext)(unsafe.Pointer(ctx)).Fnative_error_proto[error_num]
	obj := XJS_NewObjectProtoClass(tls, ctx, proto, uint32(EJS_CLASS_ERROR))
	if jsvTag(obj) == EJS_TAG_EXCEPTION {
		XFreeValue(tls, ctx, msg)
		return _js_throw_out_of_memory_null(tls, ctx)
	}

	if XJS_DefinePropertyValue(tls, ctx, obj, uint32(EJS_ATOM_message), msg, MJS_PROP_WRITABLE|MJS_PROP_CONFIGURABLE) < 0 {
		XFreeValue(tls, ctx, obj)
		return _js_throw_out_of_memory_null(tls, ctx)
	}

	if add_backtrace != 0 {
		_build_backtrace(tls, ctx, obj, 0, 0, 0, 0)
	}
	return XJS_Throw(tls, ctx, obj)
}

// _js_throw_out_of_memory_null throws null, uncatchable, for an error the
// engine ran out of memory building. Throwing null does not allocate.
func _js_throw_out_of_memory_null(tls *libc.TLS, ctx uintptr) TJSValue {
	XJS_Throw(tls, ctx, jsvMkPtr(EJS_TAG_NULL, 0))
	XJS_SetUncatchableException(tls, ctx, MTRUE)
	return jsvException()
}

// _js_regexp_out_of_memory reports whether the error message lre_compile
// wrote to msg is the one for running out of memory.
func _js_regexp_out_of_memory(msg uintptr) bool {
	return libc.GoString(msg) == "out of memory"
}
