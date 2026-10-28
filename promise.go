// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs

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
	if err := m.usable(); err != nil {
		return nil, err
	}
	promise, resolve, reject, err := m.jsNewPromiseCapability()
	if err != nil {
		return nil, err
	}

	if isException(promise) {
		m.jsFreeValue(promise)
		return nil, m.errFromException()
	}

	return &PromiseCapability{
		Promise: Value{vm: m, v: promise},
		Resolve: Value{vm: m, v: resolve},
		Reject:  Value{vm: m, v: reject},
	}, nil
}
