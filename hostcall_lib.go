// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

// How javascript calls Go on the engine of lib: the C functions that the
// function objects of RegisterFunc, Then and the Builtins call, and the
// objects themselves.

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callGo(tls *libc.TLS, ctx uintptr, this jsValue, argc int32, argv uintptr, magic int32) (r jsValue) {
	goFuncsMu.Lock()
	info := goFuncs[magic]
	goFuncsMu.Unlock()

	if info == nil {
		return throwError(tls, ctx, errorKindInternal, fmt.Sprintf("callback id=%d not registered", magic))
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

// newGoFunction returns a new function object named name, of length length,
// that calls the Go function registered as magic (callGo). name must not hold
// a NUL byte. It returns the exception when the engine threw.
func (m *VM) newGoFunction(name string, magic, length int32) (jsValue, error) {
	tls := m.runtime.tls
	cName, err := libc.CString(name)
	if err != nil {
		return jsValue{}, err
	}

	// The function's name copies the string.
	defer libc.Xfree(tls, cName)

	return lib.XJS_NewCFunction2(tls, m.cContext, fp(callGo), cName, length, lib.EJS_CFUNC_generic_magic, magic), nil
}

var (
	// callbackClassID is a global JS class ID for callback objects whose
	// lifetime is tied to a JS object's refcount. Each VM registers this
	// class ID once with its runtime. The class carries both an Fcall
	// handler (so the object is callable from JS) and an Ffinalizer (so the
	// goFuncs entry is released when the last JS reference drops).
	callbackClassID     lib.TJSClassID
	initCallbackClassID sync.Once
)

// newCallback creates a JS function object backed by fn and stores fn in the
// global Go-function table keyed by a fresh magic number. The returned
// TJSValue is a custom-class object whose finalizer calls releaseMagic when
// the last JS reference is dropped, so the goFuncs entry is freed even when
// the callback never fires (e.g. the sibling handler in a .then(f, g)).
//
// Callers must free the returned TJSValue when it is no longer needed.
func (m *VM) newCallback(name string, fn func(Value)) (jsValue, int32, error) {
	var zero jsValue
	if err := m.ensureCallbackClass(); err != nil {
		return zero, -1, err
	}

	magic, err := registerCallback(&goFunc{
		f:       reflect.ValueOf(fn),
		maxArgs: 1,
		minArgs: 1,
		name:    name,
		vm:      m,
		in:      []reflect.Type{rvt},
	})
	if err != nil {
		return zero, -1, err
	}

	tls := m.runtime.tls
	ctx := m.cContext

	obj := lib.XJS_NewObjectClass(tls, ctx, int32(callbackClassID))
	if isException(obj) {
		lib.XFreeValue(tls, ctx, obj)
		releaseMagic(magic)
		return zero, -1, m.errFromException()
	}
	lib.XJS_SetOpaque(tls, obj, uintptr(magic))
	return obj, magic, nil
}

// ensureCallbackClass registers the custom callback class on m's runtime if
// not already done. The class is callable from JS (via Fcall) and has a
// finalizer (via Ffinalizer) that releases the magic when the JS object's
// refcount reaches zero — this is what makes Then leak-free.
func (m *VM) ensureCallbackClass() error {
	if m.runtime.callbackClassInit {
		return nil
	}

	var initErr error
	initCallbackClassID.Do(func() {
		if lib.XJS_NewClassID(m.runtime.tls, uintptr(unsafe.Pointer(&callbackClassID))) == 0 {
			initErr = fmt.Errorf("failed to allocate callback class ID")
		}
	})
	if initErr != nil {
		return initErr
	}

	tls := m.runtime.tls
	cName, err := libc.CString("GoCallback")
	if err != nil {
		return err
	}
	defer libc.Xfree(tls, cName)

	// Allocate the class def on the C heap so the uintptr passed to
	// XJS_NewClass stays valid even if Go's stack grows.
	defSize := int(unsafe.Sizeof(lib.TJSClassDef{}))
	defPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(defSize))
	if defPtr == 0 {
		return fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, defPtr)

	*(*lib.TJSClassDef)(unsafe.Pointer(defPtr)) = lib.TJSClassDef{
		Fclass_name: cName,
		Ffinalizer:  fp(callbackFinalizer),
		Fcall:       fp(callbackCall),
	}

	if lib.XJS_NewClass(tls, m.runtime.cRuntime, callbackClassID, defPtr) < 0 {
		return fmt.Errorf("failed to register callback class")
	}

	m.runtime.callbackClassInit = true
	return nil
}

// callbackCall is the JSClassCall handler for callback-class objects. It reads
// the magic from the object's opaque storage and dispatches through the
// existing callGo path.
func callbackCall(tls *libc.TLS, ctx uintptr, funcObj jsValue, this jsValue, argc int32, argv uintptr, flags int32) jsValue {
	magic := int32(lib.XJS_GetOpaque(tls, funcObj, callbackClassID))
	return callGo(tls, ctx, this, argc, argv, magic)
}

// callbackFinalizer is the JSClassFinalizer for callback-class objects. It
// reads the magic from the object's opaque storage and releases the global
// goFuncs entry so that the magic can be reused.
func callbackFinalizer(tls *libc.TLS, rt uintptr, val jsValue) {
	magic := int32(lib.XJS_GetOpaque(tls, val, callbackClassID))
	releaseMagic(magic)
}

// newBuiltinFunction returns a new function object of b (callBuiltin), or
// the exception the engine threw.
func (m *VM) newBuiltinFunction(b *Builtin) jsValue {
	tls := m.runtime.tls
	// The engine makes the name an atom, a copy. Freed before it returns, not
	// in a defer (see tlsMark).
	n := len(b.name) + 1
	p := tls.Alloc(n)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), n), b.name+"\x00")
	f := lib.XJS_NewCFunction2(tls, m.cContext, fp(callBuiltin), p, b.length, lib.EJS_CFUNC_generic_magic, b.magic)
	tls.Free(n)
	return f
}

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callBuiltin(tls *libc.TLS, ctx uintptr, this jsValue, argc int32, argv uintptr, magic int32) (r jsValue) {
	b := (*builtins.list.Load())[magic]
	m := vms.Get(uint32(lib.XJS_GetContextOpaque(tls, ctx)))
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
	if err != nil || v.vm != m && v.vm != nil {
		r = m.builtinFailed(b, v, err)
	} else {
		r = v.jsValue()
	}
	returned = true
	return r
}
