// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs

import (
	"fmt"
	"reflect"
)

// Then attaches fulfillment and rejection handlers to the Promise v,
// returning the new promise for chaining. Either onFulfilled or onRejected
// may be nil. A handler is held until the engine frees the function that
// calls it, also when it never runs, or until the VM closes.
func (v Value) Then(onFulfilled, onRejected func(Value)) (_ Value, err error) {
	if v.vm == nil {
		return Value{}, fmt.Errorf("cannot call Then on a Value not associated with a VM")
	}
	m := v.vm

	fns := [2]func(Value){onFulfilled, onRejected}
	names := [2]string{"onFulfilled", "onRejected"}

	// A nil handler stays undefined, which freeing leaves alone.
	jsArgs := []jsValue{undefined, undefined}
	defer func() {
		for _, f := range jsArgs {
			m.jsFreeValue(f)
		}
	}()

	for i, fn := range fns {
		if fn == nil {
			continue
		}
		jsFn, err := m.newCallback(names[i], fn)
		if err != nil {
			return Value{}, err
		}
		jsArgs[i] = jsFn
	}

	// Re-arm the eval-timeout deadline, as the Eval*/Call entry points do:
	// fetching and invoking .then runs Javascript (a then getter or a
	// user-overridden then), so without this the deadline from the last Eval
	// keeps ticking through idle time and could interrupt this call.
	m.enter()
	defer m.leave(&err)
	thenFn := m.jsGetProperty(v.v, atomThen)
	result, err := m.jsCall(thenFn, v.v, jsArgs)
	m.jsFreeValue(thenFn)
	if err != nil {
		return Value{}, err
	}
	result = m.settled(result)
	if isException(result) {
		m.jsFreeValue(result)
		err = m.errFromException()
		return
	}

	return Value{vm: m, v: result}, nil
}

// callbackFunc is the Go function of a handler fn of Then, which takes one
// Value.
func callbackFunc(m *VM, name string, fn func(Value)) *goFunc {
	return &goFunc{
		f:       reflect.ValueOf(fn),
		maxArgs: 1,
		minArgs: 1,
		name:    name,
		vm:      m,
		in:      []reflect.Type{rvt},
	}
}
