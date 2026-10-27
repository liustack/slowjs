// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

var (
	// UndefinedValue is a Value representing Javascript value 'undefined'. It is
	// not associated with any particular VM.
	UndefinedValue = Value{v: undefined}
)

// Atom is an unique identifier of, for example, a string value. Atom values
// are VM-specific.
type Atom = lib.TJSAtom

// NewAtom returns an unique indentifier of 's' or an error, if any.
func (m *VM) NewAtom(s string) (r Atom, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	p, err := libc.CString(validUTF8(s))
	if err != nil {
		return lib.MJS_ATOM_NULL, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, p)

	if r = lib.XJS_NewAtom(tls, ctx, p); r == lib.MJS_ATOM_NULL {
		err = fmt.Errorf("OOM")
	}

	return r, err
}

// SetPropertyValue sets this.prop = val.
func (m *VM) SetPropertyValue(this Value, prop Atom, val Value) (err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	dup := val.Dup()
	if lib.XSetProperty(tls, ctx, this.v, prop, dup.v) < 0 {
		return fmt.Errorf("failed to set property")
	}

	return nil
}

// SetProperty sets this.prop = val.
func (m *VM) SetProperty(this Value, prop Atom, val any) (err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	jsArgs, free, err := m.convertArgs(val)
	if err != nil {
		return err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}

	return m.SetPropertyValue(this, prop, Value{vm: m, v: jsArgs[0]})
}

// GetPropertyValue returns this.prop.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) GetPropertyValue(this Value, prop Atom) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	v := lib.XGetProperty(tls, ctx, this.v, prop)
	if isException(v) {
		err = m.errFromException()
	}
	return Value{vm: m, v: v}, err
}

// GetProperty returns this.prop.
func (m *VM) GetProperty(this Value, prop Atom) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	return m.value(lib.XGetProperty(tls, ctx, this.v, prop), true)
}

// Value represents a native Javascript value. Values are reference counted and
// their lifetime is managed by an independent Javascript garbage collector.
// To avoid memory corruption/leaks caused by tripping the Javascript GC, a
// Value must not
//
//   - be copied. Use the Dup method instead.
//   - become unreachable without calling its Free method.
//   - be used after its Free() method was called.
//   - outlive its VM.
//
// It is recommended to use native Go values instead of Value where possible.
//
// When passing a Value down the call stack use Dup. For example in main
//
//	v, _ := EvalValue(someScript)
//	defer v.Free()
//	foo(v.Dup()) // Instead of foo(v)
//
// In 'foo' Free must be used. For example
//
//	func foo(v Value) {
//	        defer v.Free()
//	        ...
//	}
//
// This ensures the/only topmost Free marks 'v' eligible for garbage collection.
//
// Beware that the correct setup/handling becomes more complicated when using
// closures, Values are sent through a channel etc. In particular, if a
// goroutine 1 passes a Dup of 'v' to goroutine 2 and goroutine 1 completes and
// thus frees 'v' before goroutine 2 completes, the reference counting
// mechanism will fail. In other words, every Free must be strictly paired with
// the Dup that preceded obtaining the Value and the Dup/Free calls must
// respect the original nesting. This is correct.
//
//	Dup             // in main
//	        Dup     // in foo
//	        Free    // in foo
//	Free            // in main
//
// This will fail, for example in the above discussed goroutines scenario.
//
//	Dup            // in g1
//	        Dup    // in g2
//	Free           // in g1
//	        Free   // in g2
//
// The fix might be in this case to arrange goroutine 1 to wait for goroutine 2
// to complete before executing Free in goroutine 1.
//
// See TestMemgrind for an example of how to detect memory leaks caused not
// only by improper Value use.
type Value struct {
	vm *VM
	v  lib.TJSValue
}

// VM returns the VM associated with 'v'.
func (v *Value) VM() *VM {
	return v.vm
}

// SetProperty sets v.prop = val.
func (v Value) SetProperty(prop Atom, val any) (err error) {
	return v.vm.SetProperty(v, prop, val)
}

// SetPropertyValue sets v.prop = val.
func (v Value) SetPropertyValue(prop Atom, val Value) (err error) {
	return v.vm.SetPropertyValue(v, prop, val)
}

// GetPropertyValue returns v.prop.
func (v Value) GetPropertyValue(prop Atom) (r Value, err error) {
	return v.vm.GetPropertyValue(v, prop)
}

// GetProperty returns v.prop. The 'this' argument is ignored.
//
// Deprecated: The 'this' argument is vestigial, the property is always looked
// up on 'v'. Use [Value.Property] instead.
func (v Value) GetProperty(this Value, prop Atom) (r any, err error) {
	return v.vm.GetProperty(v, prop)
}

// Property returns v.prop. It is [Value.GetProperty] without the vestigial
// 'this' argument.
func (v Value) Property(prop Atom) (r any, err error) {
	return v.vm.GetProperty(v, prop)
}

// MarshalJSON implements encoding/json.Marshaler.
func (v Value) MarshalJSON() (r []byte, err error) {
	tls := v.vm.runtime.tls
	ctx := v.vm.cContext
	json := lib.XJS_JSONStringify(tls, ctx, v.v, undefined, undefined)

	defer lib.XFreeValue(tls, ctx, json)

	if isException(json) {
		return nil, v.vm.errFromException()
	}

	// A copy: the JS buffer is released before this returns.
	s, err := v.vm.goString(json)
	if err != nil {
		return nil, err
	}

	return []byte(s), nil
}

// Dup returns a copy of 'v' while updating its reference count.
func (v Value) Dup() Value {
	_ = v.vm.runtime // a Value of no VM, such as the zero Value, panics
	return Value{
		vm: v.vm,
		v:  lib.DupValue(v.v),
	}
}

// Free marks 'v' as no longer used and updates its reference count. 'v' must
// not be used afterwards.
func (v *Value) Free() {
	lib.XFreeValue(v.vm.runtime.tls, v.vm.cContext, v.v)
	*v = Value{}
}

// Any attemtps to convert 'v' to any using the same rules as there are for
// the return value of VM.Eval.
func (v Value) Any() (r any, err error) {
	return v.vm.value(v.v, false)
}

// IsUndefined reports whether 'v' represents the Javascript value 'undefined'.
func (v Value) IsUndefined() bool {
	return tag(v.v) == lib.EJS_TAG_UNDEFINED
}

// Call evaluates 'v(args...)' with 'this' as the receiver and returns the
// resulting (value, error). 'v' must be a Javascript function. Pass
// [UndefinedValue] as 'this' when the function does not use it.
//
//	resolve.Call(slowjs.UndefinedValue, "done")
//	reject.Call(slowjs.UndefinedValue, err.Error())
//
// Argument types are converted using the same rules as in [VM.Call] and the
// result is unpacked using the same rules as there are for the return value of
// [VM.Eval]. Note that a Go error is not one of the recognized types, so it is
// marshaled to JSON like any other value — and because the standard error
// types have no exported fields that yields '{}'. Pass err.Error() instead.
//
// Note: A Value passed to a Go function registered by [VM.RegisterFunc] is
// valid only for the duration of that call. To call it later, for example to
// settle a promise after the Go function has returned, retain it using
// [Value.Dup] and release it using [Value.Free] when done.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (v Value) Call(this Value, args ...any) (r any, err error) {
	if err = v.checkCall(this); err != nil {
		return nil, err
	}

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see VM.Call).
	v.vm.enter()
	defer v.vm.leave()
	return v.vm.call(v.v, this.v, args...)
}

// CallValue is like [Value.Call] but returns (Value, error) like
// [VM.EvalValue].
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (v Value) CallValue(this Value, args ...any) (r Value, err error) {
	if err = v.checkCall(this); err != nil {
		return r, err
	}

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see VM.Call).
	v.vm.enter()
	defer v.vm.leave()
	return v.vm.callValue(v.v, this.v, args...)
}

// checkCall validates the receiver and 'this' of the Value call methods. It
// reports the problem as an error rather than panicking in v.vm because both
// callers already return one.
func (v Value) checkCall(this Value) error {
	if v.vm == nil {
		return fmt.Errorf("cannot call a Value not associated with a VM")
	}

	// Same test as in convertArgs: values with a reference count, ie. those
	// belonging to a particular runtime heap, must not cross VMs.
	if this.vm != v.vm && tag(this.v) < 0 {
		return fmt.Errorf("cannot use a Value from a different VM")
	}

	return nil
}
