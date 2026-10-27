// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// How javascript calls Go on the C engine: functions of the host
// (JS_NewHostFunction), whose calls the engine sends to host_call of
// internal/cengine, which sends them to hostCall. The id of a function is the
// index of its Builtin below hostFuncs, and hostFuncs plus the magic of its
// Go function (RegisterFunc) from there. A function of RegisterFunc lives
// until the VM closes.

import (
	"fmt"

	"github.com/liustack/slowjs/internal/cengine"
)

// hostFuncs is the first id of the Go functions of RegisterFunc: a Builtin
// has the ids below.
const hostFuncs = 1 << 16

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
	if id < hostFuncs {
		r = m.callBuiltin(saved, (*builtins.list.Load())[id], valueAt(this), argc, argv)
	} else {
		r = m.callGo(saved, id-hostFuncs, valueAt(this), argc, argv)
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

// callGo calls the Go function of RegisterFunc registered as magic.
func (m *VM) callGo(saved uintptr, magic int32, this jsValue, argc int32, argv uintptr) (r jsValue) {
	goFuncsMu.Lock()
	info := goFuncs[magic]
	goFuncsMu.Unlock()

	if info == nil || info.vm != m {
		r = m.jsThrowError(errorKindInternal, fmt.Sprintf("callback id=%d not registered", magic))
		m.sp = saved
		return r
	}

	m.host++
	returned := false
	defer func() { m.called(saved, info.name, &r, returned, recover()) }()
	r = m.callGoFunc(info, this, argc, argv)
	returned = true
	return r
}

// newHostFunction is JS_NewHostFunction of name, length and id.
func (m *VM) newHostFunction(name string, length, id int32) (r jsValue, err error) {
	if m.entered == 0 {
		m.outer(func() { r, err = m.newHostFunction(name, length, id) })
		return r, err
	}
	p, sp, heap := m.tmpString(name, false)
	if p == 0 {
		return jsValue{}, m.tmpError()
	}
	r = val(cengine.JS_NewHostFunction(sp, m.ctx, p, uintptr(len(name)), uintptr(length), uintptr(id)))
	m.tmpFree(p, heap)
	return r, nil
}

// newGoFunction returns a new function object named name, of length length,
// that calls the Go function registered as magic (callGo). It returns the
// exception when the engine threw.
func (m *VM) newGoFunction(name string, magic, length int32) (jsValue, error) {
	return m.newHostFunction(name, length, hostFuncs+magic)
}

// newBuiltinFunction returns a new function object of b (callBuiltin), or
// the exception the engine threw.
func (m *VM) newBuiltinFunction(b *Builtin) jsValue {
	f, err := m.newHostFunction(b.name, b.length, b.magic)
	if err != nil {
		return m.jsThrowOutOfMemory()
	}
	return f
}
