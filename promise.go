// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// PromiseCapability holds a Promise along with its resolve and reject
// functions, obtained without evaluating JS source.
type PromiseCapability struct {
	Promise Value
	Resolve Value
	Reject  Value
}

// Free releases all values held by the capability.
func (pc *PromiseCapability) Free() {
	pc.Resolve.Free()
	pc.Reject.Free()
	pc.Promise.Free()
}

// Dup duplicates the capability — its promise and both resolving functions —
// returning a new PromiseCapability that must itself be released via Free.
func (pc *PromiseCapability) Dup() *PromiseCapability {
	return &PromiseCapability{
		Promise: pc.Promise.Dup(),
		Resolve: pc.Resolve.Dup(),
		Reject:  pc.Reject.Dup(),
	}
}

// NewPromiseCapability creates a new JavaScript Promise along with its
// resolve and reject functions.
//
// The returned PromiseCapability must be freed via Free when no longer
// needed.
//
// To expose a promise to JS from a Go function registered with
// RegisterFunc, return pc.Promise from the Go function.  The
// resolve/reject values captured by the closure need no Dup because
// the C API already holds a reference for each:
//
//	m.RegisterFunc("delayed", func(delay int) Value {
//	    pc, err := m.NewPromiseCapability()
//	    if err != nil { panic(err) }
//	    youreventloop.Schedule(func() {
//	        time.Sleep(time.Duration(delay) * time.Millisecond)
//	        pc.Resolve.Call(UndefinedValue, "done")
//	        pc.Resolve.Free()
//	        pc.Reject.Free()
//	        // pc.Promise.Free() // <- only if returned pc.Promise.Dup()!
//	    })
//	    return pc.Promise
//	}, false)
func (m *VM) NewPromiseCapability() (*PromiseCapability, error) {
	tls := m.runtime.tls
	ctx := m.cContext

	// Allocate on the C heap so the pointer stays valid if Go's stack grows.
	resolvingFuncsSize := 2 * int(sizeofJsValue)
	resolvingFuncsPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(resolvingFuncsSize))
	if resolvingFuncsPtr == 0 {
		return nil, fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, resolvingFuncsPtr)

	promise := lib.XJS_NewPromiseCapability(tls, ctx, resolvingFuncsPtr)
	if isException(promise) {
		lib.XFreeValue(tls, ctx, promise)
		return nil, m.errFromException()
	}

	resolvingFuncs := unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(resolvingFuncsPtr)), 2)

	return &PromiseCapability{
		Promise: Value{vm: m, v: promise},
		Resolve: Value{vm: m, v: resolvingFuncs[0]},
		Reject:  Value{vm: m, v: resolvingFuncs[1]},
	}, nil
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
func (m *VM) newCallback(name string, fn func(Value)) (lib.TJSValue, int32, error) {
	var zero lib.TJSValue
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

// Then attaches fulfillment and rejection handlers to the Promise v,
// returning the new promise for chaining. Either onFulfilled or onRejected
// may be nil.
func (v Value) Then(onFulfilled, onRejected func(Value)) (_ Value, err error) {
	if v.vm == nil {
		return Value{}, fmt.Errorf("cannot call Then on a Value not associated with a VM")
	}
	m := v.vm
	tls := m.runtime.tls
	ctx := m.cContext

	fns := [2]func(Value){onFulfilled, onRejected}
	names := [2]string{"onFulfilled", "onRejected"}

	// Allocate on the C heap so the pointer stays valid if Go's stack grows.
	jsArgsSize := 2 * int(sizeofJsValue)
	jsArgsPtr := libc.Xcalloc(tls, 1, libc.Tsize_t(jsArgsSize))
	if jsArgsPtr == 0 {
		return Value{}, fmt.Errorf("OOM")
	}
	defer libc.Xfree(tls, jsArgsPtr)
	jsArgs := unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(jsArgsPtr)), 2)

	// magics[i] >= 0 marks a slot for which we created a callback and so own a
	// reference to jsArgs[i]; -1 means a nil handler or a slot whose callback
	// was never created (e.g. newCallback failed on the other slot).
	magics := [2]int32{-1, -1}

	defer func() {
		for i := range jsArgs {
			if magics[i] < 0 {
				continue
			}
			lib.XFreeValue(tls, ctx, jsArgs[i])
		}
	}()

	for i, fn := range fns {
		if fn == nil {
			jsArgs[i] = undefined
			magics[i] = -1
			continue
		}
		jsFn, magic, err := m.newCallback(names[i], fn)
		if err != nil {
			return Value{}, err
		}
		jsArgs[i] = jsFn
		magics[i] = magic
	}

	// Re-arm the eval-timeout deadline, as the Eval*/Call entry points do:
	// fetching and invoking .then runs Javascript (a then getter or a
	// user-overridden then), so without this the deadline from the last Eval
	// keeps ticking through idle time and could interrupt this call.
	m.enter()
	defer m.leave()
	thenFn := lib.XGetProperty(tls, ctx, v.v, lib.EJS_ATOM_then)
	result := m.settled(lib.XJS_Call(tls, ctx, thenFn, v.v, 2, jsArgsPtr))
	lib.XFreeValue(tls, ctx, thenFn)
	if isException(result) {
		lib.XFreeValue(tls, ctx, result)
		err = m.errFromException()
		return
	}

	return Value{vm: m, v: result}, nil
}
