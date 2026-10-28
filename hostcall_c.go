// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && ((linux && amd64) || (darwin && arm64))

package slowjs // import "github.com/liustack/slowjs"

// How javascript calls Go on the C engine: functions of the host
// (JS_NewHostFunction), whose calls the engine sends to host_call of
// internal/cengine, which sends them to hostCall. The id of a function is the
// handle of its Go function, or builtinFuncs plus the index of its Builtin:
// the host of the VM keeps each Go function of RegisterFunc, RegisterHostFunc
// and Then under a handle (gohost.VM.Keep), until the engine frees the
// function object that calls it, which then passes the handle to host_release
// (JS_NewHostFunctionRelease), or the VM closes.

import (
	"fmt"

	"github.com/liustack/slowjs/internal/cengine"
	"github.com/liustack/slowjs/internal/gohost"
)

// builtinFuncs is the id of the first Builtin: the handles of the host, from
// 1 to gohost.MaxHandles, are the ids below.
const builtinFuncs = gohost.MaxHandles + 1

const _ = uint32(1<<31 - builtinFuncs - 1<<15) // fails to compile when the id of a Builtin is past an int32

func init() {
	cengine.SetHostFunc(hostCall)
}

// hostCall answers the call of a function of the host of the VM vmID. sp is
// the first free byte of the shadow stack, above the frames of the call: the
// calls the Go function makes into the engine start there, and the frames
// below are not touched, also when they call back here in turn. m.sp is put
// back when the Go function ends, also by a panic or a Goexit.
func hostCall(sp uintptr, vmID uint32, this uintptr, argc int32, argv uintptr, id int32) (uintptr, uintptr) {
	m := vms.Get(vmID)
	if m == nil {
		panic(fmt.Sprintf("slowjs: a call of a host function of VM %d, which is not open", vmID))
	}
	saved := m.sp
	m.sp = (sp + 15) &^ 15
	var r jsValue
	if id >= builtinFuncs {
		r = m.callBuiltin(saved, (*builtins.list.Load())[id-builtinFuncs], valueAt(this), argc, argv)
	} else {
		r = m.callGo(saved, id, valueAt(this), argc, argv)
	}
	return r.u, r.tag
}

// called ends a call of a Go function name from javascript, deferred with
// the shadow pointer to put back once it is done with the engine, the result r and whether the Go function
// returned. A panic of the Go function becomes an uncatchable error
// (hostPanicked), and goes on where the Go code that called javascript gets
// back. A panic that went through the engine's frames below, a memory fault,
// and a Goexit go on up through the engine, and the VM is broken.
func (m *VM) called(saved uintptr, name string, r *jsValue, returned bool, p any) {
	m.host--
	switch {
	case p == nil && !returned:
		m.sp = saved
		m.h.MarkBroken() // runtime.Goexit, which goes on up through the engine
		return
	case p != nil && (m.h.Broken() || isFault(p)):
		m.sp = saved
		m.h.MarkBroken()
		panic(p)
	case p == nil && m.h.Broken():
		m.sp = saved
		panic(framesLeftPanic)
	case p != nil:
		// Above the frames of the call still: what the engine is passed
		// goes past them.
		m.hostPanicked(name, p)
	}
	defer func() { m.sp = saved }()
	// An interrupt or running out of memory in converting the arguments,
	// in the Go function or in converting its result stays pending (see
	// takeException). It is the result whatever the Go function returned.
	if m.uncatchablePending() {
		if !isException(*r) {
			m.jsFreeValue(*r)
		}
		*r = exception
	}
}

// callBuiltin calls b, whose function javascript called.
func (m *VM) callBuiltin(saved uintptr, b *Builtin, this jsValue, argc int32, argv uintptr) (r jsValue) {
	m.host++
	returned := false
	defer func() { m.called(saved, b.name, &r, returned, recover()) }()
	v, err := b.fn(Args{vm: m, this: this, argv: argv, argc: int(argc)})
	if err != nil || v.vm != m && v.vm != nil {
		r = m.builtinFailed(b, v, err)
	} else {
		r = v.jsValue()
	}
	returned = true
	return r
}

// callGo calls the Go function the host of m keeps under handle.
func (m *VM) callGo(saved uintptr, handle int32, this jsValue, argc int32, argv uintptr) (r jsValue) {
	info, _ := m.h.Handle(uint32(handle)).(*goFunc)
	if info == nil {
		// Only a function of the handle calls it, and the engine releases
		// the handle once the function is gone.
		panic(fmt.Sprintf("slowjs: a call of the Go function of handle %d, which VM %d does not hold", handle, m.id))
	}

	m.host++
	returned := false
	defer func() { m.called(saved, info.name, &r, returned, recover()) }()
	r = m.callGoFunc(info, this, argc, argv)
	returned = true
	return r
}

// newHostFunction is JS_NewHostFunction of name, length and id, or
// JS_NewHostFunctionRelease when release is set.
func (m *VM) newHostFunction(name string, length, id int32, release bool) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.newHostFunction(name, length, id, release) })
		return r, err
	}
	p, sp, heap := m.tmpString(name, false)
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	if release {
		r = val(cengine.JS_NewHostFunctionRelease(sp, m.ctx, p, uintptr(len(name)), uintptr(length), uintptr(id)))
	} else {
		r = val(cengine.JS_NewHostFunction(sp, m.ctx, p, uintptr(len(name)), uintptr(length), uintptr(id)))
	}
	m.tmpFree(p, heap)
	return r, nil
}

// keepFunc keeps info, which m's functions call, and returns its handle.
func (m *VM) keepFunc(info *goFunc) (int32, error) {
	h, err := m.h.Keep(info)
	return int32(h), err
}

// goFuncFreed has nothing to do: the engine released the handle of the
// function it freed.
func (m *VM) goFuncFreed(handle int32) {}

// forgetFuncs has nothing to do: Close of the host drops the handles.
func (m *VM) forgetFuncs() {}

// newGoFunction returns a new function object named name, of length length,
// that calls the Go function kept under handle (callGo), and holds the handle
// until the engine frees it. When it makes no function it gives the handle
// up.
func (m *VM) newGoFunction(name string, handle, length int32) (jsValue, error) {
	f, err := m.newHostFunction(name, length, handle, true)
	if err == nil && isException(f) {
		err = m.errFromException()
	}
	if err != nil {
		gohost.Release(m.id, uint32(handle))
		return jsValue{}, err
	}
	return f, nil
}

// newCallback returns a new function object that calls fn, a handler of
// Then, and holds it until the engine frees the object.
func (m *VM) newCallback(name string, fn func(Value)) (jsValue, error) {
	handle, err := m.keepFunc(callbackFunc(m, name, fn))
	if err != nil {
		return jsValue{}, err
	}
	return m.newGoFunction(name, handle, 1)
}

// newBuiltinFunction returns a new function object of b (callBuiltin), or
// the exception the engine threw.
func (m *VM) newBuiltinFunction(b *Builtin) jsValue {
	f, err := m.newHostFunction(b.name, b.length, builtinFuncs+b.magic, false)
	if err != nil {
		return m.jsThrowOutOfMemory()
	}
	return f
}
