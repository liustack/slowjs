// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Promises from Go: a promise with its resolve and reject, and handlers
// attached to a promise.

import "errors"

// PromiseCapability is a promise and the two functions that settle it. Each
// of the three is a reference, which Free gives up.
type PromiseCapability struct {
	Promise Value
	Resolve Value
	Reject  Value
}

// Free frees the three values of pc.
func (pc *PromiseCapability) Free() {
	pc.Resolve.Free()
	pc.Reject.Free()
	pc.Promise.Free()
}

// Dup returns a new PromiseCapability with a Dup of each value of pc, which is
// freed on its own.
func (pc *PromiseCapability) Dup() *PromiseCapability {
	return &PromiseCapability{Promise: pc.Promise.Dup(), Resolve: pc.Resolve.Dup(), Reject: pc.Reject.Dup()}
}

// NewPromiseCapability returns a new pending promise with its resolve and
// reject, without evaluating any source, for Go to settle later. It runs no
// javascript.
//
// A Go function of [VM.RegisterFunc] that is to give javascript a promise
// returns pc.Promise, which hands its reference to the call, and keeps
// pc.Resolve and pc.Reject, which it frees once it has called one:
//
//	var waiting *slowjs.PromiseCapability
//	m.RegisterFunc("later", func() slowjs.Value {
//		pc, err := m.NewPromiseCapability()
//		if err != nil {
//			return slowjs.UndefinedValue
//		}
//		waiting = pc
//		return pc.Promise // the call takes this reference
//	}, false)
//	m.Eval(`later().then(v => globalThis.got = v)`, slowjs.EvalGlobal)
//	// Later, on the goroutine that uses m:
//	waiting.Resolve.Call(slowjs.UndefinedValue, "done")
//	waiting.Resolve.Free()
//	waiting.Reject.Free()
//	m.ExecutePendingJobs()
//
// Only a function that returns pc.Promise.Dup(), and so keeps the promise,
// frees pc.Promise too.
func (m *VM) NewPromiseCapability() (*PromiseCapability, error) {
	if err := m.usable(); err != nil {
		return nil, err
	}
	promise, resolve, reject, err := m.jsNewPromiseCapability()
	if err != nil {
		return nil, err
	}
	if isException(promise) {
		return nil, m.errFromException()
	}
	return &PromiseCapability{
		Promise: Value{vm: m, v: promise},
		Resolve: Value{vm: m, v: resolve},
		Reject:  Value{vm: m, v: reject},
	}, nil
}

// errThenNoVM is the error of Then on a Value of no VM.
var errThenNoVM = errors.New("cannot call Then on a Value not associated with a VM")

// Then attaches onFulfilled and onRejected to the promise v, as v.then does,
// and returns the promise then returns, which the caller frees. Either may be
// nil. A handler gets the value or the reason, valid only during its call.
// Each is held until the engine frees the function that calls it, even when
// it never ran, or the VM closes.
func (v Value) Then(onFulfilled, onRejected func(Value)) (_ Value, err error) {
	m := v.vm
	if m == nil {
		return Value{}, errThenNoVM
	}
	if err := m.usable(); err != nil {
		return Value{}, err
	}
	// The handlers' functions: the call of then holds what it keeps of them.
	funcs := [2]jsValue{undefined, undefined}
	defer func() {
		m.jsFreeValue(funcs[0])
		m.jsFreeValue(funcs[1])
	}()
	if onFulfilled != nil {
		if funcs[0], err = m.newCallback("onFulfilled", onFulfilled); err != nil {
			funcs[0] = undefined
			return Value{}, err
		}
	}
	if onRejected != nil {
		if funcs[1], err = m.newCallback("onRejected", onRejected); err != nil {
			funcs[1] = undefined
			return Value{}, err
		}
	}

	if err = m.enter(); err != nil {
		return Value{}, err
	}
	defer m.leave(&err)
	then := m.settled(m.jsGetProperty(v.v, atomThen))
	if isException(then) {
		return Value{}, m.errFromException()
	}
	r, err := m.jsCall(then, v.v, funcs[:])
	m.jsFreeValue(then)
	if err != nil {
		return Value{}, err
	}
	if r = m.settled(r); isException(r) {
		return Value{}, m.errFromException()
	}
	return Value{vm: m, v: r}, nil
}
