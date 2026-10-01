// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs

import (
	"fmt"
	"unsafe"

	"modernc.org/libc"
	lib "modernc.org/libquickjs"
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
