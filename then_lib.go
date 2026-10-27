// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !slowjs_cengine

package slowjs

import (
	"fmt"
)

// Then attaches fulfillment and rejection handlers to the Promise v,
// returning the new promise for chaining. Either onFulfilled or onRejected
// may be nil.
func (v Value) Then(onFulfilled, onRejected func(Value)) (_ Value, err error) {
	if v.vm == nil {
		return Value{}, fmt.Errorf("cannot call Then on a Value not associated with a VM")
	}
	m := v.vm

	fns := [2]func(Value){onFulfilled, onRejected}
	names := [2]string{"onFulfilled", "onRejected"}

	jsArgs := []jsValue{undefined, undefined}

	// magics[i] >= 0 marks a slot for which we created a callback and so own a
	// reference to jsArgs[i]; -1 means a nil handler or a slot whose callback
	// was never created (e.g. newCallback failed on the other slot).
	magics := [2]int32{-1, -1}

	defer func() {
		for i := range jsArgs {
			if magics[i] < 0 {
				continue
			}
			m.jsFreeValue(jsArgs[i])
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
