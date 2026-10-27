// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callGo(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) (r lib.TJSValue) {
	goFuncsMu.Lock()
	info := goFuncs[magic]
	goFuncsMu.Unlock()

	if info == nil {
		return throwInternalError(tls, ctx, fmt.Sprintf("callback id=%d not registered", magic))
	}

	m := info.vm
	m.host++
	mark := markTLS(tls)
	returned := false
	defer func() {
		m.host--
		if m.hostEnded(mark, returned) {
			return
		}
		switch p := recover(); {
		case p != nil:
			m.hostPanicked(info.name, p)
		case !returned:
			m.broken = true // runtime.Goexit, which goes on up through the engine
		}
		// An interrupt or running out of memory in converting the
		// arguments, in the Go function or in converting its result stays
		// pending (see takeException). It is the result whatever the Go
		// function returned, and no error callGo threw replaced it.
		if m.uncatchablePending() {
			if !isException(r) {
				lib.XFreeValue(tls, ctx, r)
			}
			r = exception
		}
	}()
	r = m.callGoFunc(info, this, argc, argv)
	returned = true
	return r
}

// callbackCall is the JSClassCall handler for callback-class objects. It reads
// the magic from the object's opaque storage and dispatches through the
// existing callGo path.
func callbackCall(tls *libc.TLS, ctx uintptr, funcObj lib.TJSValue, this lib.TJSValue, argc int32, argv uintptr, flags int32) lib.TJSValue {
	magic := int32(lib.XJS_GetOpaque(tls, funcObj, callbackClassID))
	return callGo(tls, ctx, this, argc, argv, magic)
}

// callbackFinalizer is the JSClassFinalizer for callback-class objects. It
// reads the magic from the object's opaque storage and releases the global
// goFuncs entry so that the magic can be reused.
func callbackFinalizer(tls *libc.TLS, rt uintptr, val lib.TJSValue) {
	magic := int32(lib.XJS_GetOpaque(tls, val, callbackClassID))
	releaseMagic(magic)
}

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callBuiltin(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) (r lib.TJSValue) {
	b := (*builtins.list.Load())[magic]
	m := vms.vm(uint32(lib.XJS_GetContextOpaque(tls, ctx)))
	m.host++
	mark := markTLS(tls)
	returned := false
	defer func() {
		m.host--
		if m.hostEnded(mark, returned) {
			return
		}
		switch p := recover(); {
		case p != nil:
			m.hostPanicked(b.name, p)
		case !returned:
			m.broken = true // runtime.Goexit, which goes on up through the engine
		}
		// As in callGo: an interrupt or running out of memory in the Go
		// function stays pending and is the result.
		if m.uncatchablePending() {
			if !isException(r) {
				lib.XFreeValue(tls, ctx, r)
			}
			r = exception
		}
	}()
	v, err := b.fn(Args{vm: m, this: this, argv: argv, argc: int(argc)})
	switch {
	case err != nil:
		if v.vm == m {
			v.Free()
		}
		r = m.throw(err)
	case v.vm != m && v.vm != nil:
		// Freeing it here would use the other VM, maybe on another goroutine.
		r = throwTypeError(tls, ctx, fmt.Sprintf("Go function %s returned a Value of another VM", b.name))
	default:
		r = v.jsValue()
	}
	returned = true
	return r
}
