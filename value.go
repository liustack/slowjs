// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

// Value, a javascript value held from Go, its properties, and the
// constructors of values.

import "errors"

// Value is a javascript value that Go holds a reference to. The value lives as
// long as a reference to it does, and the javascript GC frees it once none
// does, so Go code that holds a Value keeps to these rules:
//
//   - Each Value is freed with Free once, and not used after. Dropping one
//     unfreed leaks it, until the VM closes.
//   - A copy of a Value, by an assignment, is not a new reference. Dup makes
//     one, which is freed on its own. Hand a Value on as a Dup, which the
//     receiver frees, typically with defer.
//   - A Value does not outlive its VM.
//
// Dup and Free pair up, and the pairs nest as calls do. That is easy to get
// wrong across goroutines or channels: say g1 hands a Value to g2 by
// assignment, not by Dup, and both free it when they are done. If g1 frees
// first, g2 then uses a value that may be gone, and frees it a second time.
// The fix is for g1 to wait until g2 is done before it frees, or to hand g2 a
// Dup.
//
// Where a Go value does, as with Eval and Call, use it: it needs none of
// this.
//
// UndefinedValue and the zero Value belong to no VM, hold no reference and
// are undefined to every VM: freeing one does nothing, and Dup of one panics,
// as that is most often a Value used after its Free.
type Value struct {
	vm *VM
	v  jsValue
}

// VM returns the VM of v, nil for UndefinedValue and the zero Value.
func (v *Value) VM() *VM { return v.vm }

// SetProperty sets the property prop of v to val (see [VM.SetProperty]).
func (v Value) SetProperty(prop Atom, val any) (err error) {
	return v.vm.SetProperty(v, prop, val)
}

// SetPropertyValue sets the property prop of v to val (see
// [VM.SetPropertyValue]).
func (v Value) SetPropertyValue(prop Atom, val Value) (err error) {
	return v.vm.SetPropertyValue(v, prop, val)
}

// GetPropertyValue returns the property prop of v (see
// [VM.GetPropertyValue]).
func (v Value) GetPropertyValue(prop Atom) (r Value, err error) {
	return v.vm.GetPropertyValue(v, prop)
}

// GetProperty returns the property prop of v as a Go value.
//
// Deprecated: this is not used: the property is read on v. Use
// [Value.Property].
func (v Value) GetProperty(this Value, prop Atom) (r any, err error) {
	return v.Property(prop)
}

// Property returns the property prop of v as a Go value (see
// [VM.GetProperty]).
func (v Value) Property(prop Atom) (r any, err error) {
	return v.vm.GetProperty(v, prop)
}

// MarshalJSON returns the JSON of v, as JSON.stringify makes it, so that
// encoding/json writes a Value as the javascript value. For a value
// JSON.stringify gives undefined for, such as a function, it is the text
// undefined, which is not JSON. It runs javascript, such as toJSON, inside
// the eval timeout.
func (v Value) MarshalJSON() (r []byte, err error) {
	m := v.vm
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	j := m.settled(m.jsJSONStringify(v.v))
	if isException(j) {
		return nil, m.errFromException()
	}
	defer m.jsFreeValue(j)
	s, err := m.goString(j)
	if err != nil {
		return nil, err
	}
	// A copy of its own: the string of the engine is freed as this returns.
	return []byte(s), nil
}

// Dup returns a new reference to v, which is freed on its own. It panics for
// a Value of no VM, which is most often one used after its Free.
func (v Value) Dup() Value {
	_ = v.vm.id // the panic of a Value of no VM
	return Value{vm: v.vm, v: dupValue(v.v)}
}

// Free gives up the reference v holds, and makes v the zero Value. It does
// nothing for UndefinedValue and the zero Value.
func (v *Value) Free() {
	v.vm.jsFreeValue(v.v)
	*v = Value{}
}

// Any returns v as a Go value, as [VM.Eval] converts it. v stays the
// caller's. Converting an object or a BigInt runs javascript, inside the eval
// timeout. UndefinedValue and the zero Value are Undefined{}.
func (v Value) Any() (r any, err error) {
	m := v.vm
	if m == nil {
		return Undefined{}, nil
	}
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	return m.value(v.v, false)
}

// IsUndefined reports whether v is undefined. UndefinedValue and the zero
// Value are.
func (v Value) IsUndefined() bool {
	return tag(v.jsValue()) == tagUndefined
}

// errCallNoVM is the error of calling a Value of no VM.
var errCallNoVM = errors.New("cannot call a Value not associated with a VM")

// Call calls v with this and args and returns its value as a Go value, as
// [VM.Call] does with the same arguments. this is used as it is, not replaced
// by the global object: UndefinedValue is undefined.
//
// A Go error is not among the types an argument converts by, so it goes as
// its JSON, which for an error of the standard library is {}: pass
// err.Error(). A Value that a function of [VM.RegisterFunc] gets, such as the
// resolve of a promise, is valid only during the call it got it in: to call
// it later, keep a Dup and free it when done.
func (v Value) Call(this Value, args ...any) (r any, err error) {
	m := v.vm
	if m == nil {
		return nil, errCallNoVM
	}
	if m.foreign(this) {
		return nil, errForeignValue
	}
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	return m.call(v.v, this.jsValue(), args)
}

// CallValue is [Value.Call] that returns the value as a [Value], which the
// caller frees.
func (v Value) CallValue(this Value, args ...any) (r Value, err error) {
	m := v.vm
	if m == nil {
		return r, errCallNoVM
	}
	if m.foreign(this) {
		return r, errForeignValue
	}
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	return m.callValue(v.v, this.jsValue(), args)
}

// NewAtom returns the atom of the property name s, a number the VM knows the
// name by, the same for the same name. Invalid UTF-8 in s is U+FFFD a byte.
// An atom is never freed.
func (m *VM) NewAtom(s string) (r Atom, err error) {
	if err = m.usable(); err != nil {
		return atomNull, err
	}
	return m.atomOf(s)
}

// SetPropertyValue sets the property prop of this to val, as an assignment in
// strict code does: a setter runs, and a write the object refuses, as a
// Proxy can, is a TypeError. val stays the caller's. UndefinedValue and the
// zero Value are undefined. A Value of another VM is an error.
func (m *VM) SetPropertyValue(this Value, prop Atom, val Value) (err error) {
	if m.foreign(this) || m.foreign(val) {
		return errForeignValue
	}
	if err = m.enter(); err != nil {
		return err
	}
	defer m.leave(&err)
	return m.setProperty(this.jsValue(), prop, dupValue(val.jsValue()))
}

// SetProperty sets the property prop of this to val, converted as an argument
// of [VM.Call] is, as [VM.SetPropertyValue] does.
func (m *VM) SetProperty(this Value, prop Atom, val any) (err error) {
	if m.foreign(this) {
		return errForeignValue
	}
	if err = m.enter(); err != nil {
		return err
	}
	defer m.leave(&err)
	v, err := m.callArg(val)
	if err != nil {
		return err
	}
	return m.setProperty(this.jsValue(), prop, v)
}

// setProperty sets the property prop of this to v, which it takes.
func (m *VM) setProperty(this jsValue, prop Atom, v jsValue) error {
	if m.jsSetProperty(this, prop, v) < 0 || m.uncatchablePending() {
		return m.errFromException()
	}
	return nil
}

// GetPropertyValue returns the property prop of this, which the caller frees.
// A getter runs, inside the eval timeout. A Value of another VM is an error.
func (m *VM) GetPropertyValue(this Value, prop Atom) (r Value, err error) {
	if m.foreign(this) {
		return r, errForeignValue
	}
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	v := m.settled(m.jsGetProperty(this.jsValue(), prop))
	if isException(v) {
		err = m.errFromException()
	}
	return Value{vm: m, v: v}, err
}

// GetProperty returns the property prop of this as a Go value, as [VM.Eval]
// converts it. Reading it and converting it run inside one eval timeout.
func (m *VM) GetProperty(this Value, prop Atom) (r any, err error) {
	if m.foreign(this) {
		return nil, errForeignValue
	}
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	return m.value(m.settled(m.jsGetProperty(this.jsValue(), prop)), true)
}

// NewInt returns the number n. Outside int32 it is a float, which loses
// precision past 2^53.
func (m *VM) NewInt(n int) Value {
	return Value{vm: m, v: intValue(int64(n))}
}

// NewFloat64 returns the number n.
func (m *VM) NewFloat64(n float64) Value {
	return Value{vm: m, v: newFloat(n)}
}

// NewString returns the string s, with U+FFFD for each byte of invalid UTF-8,
// which the caller frees. In a Go function that javascript called, a long
// string stops at Interrupt, the eval timeout or an uncatchable error of the
// call, and NewString returns that error, as a loop of the engine stops.
func (m *VM) NewString(s string) (r Value, err error) {
	v, err := m.newString(s)
	if err != nil {
		return r, err
	}
	return Value{vm: m, v: v}, nil
}

// newString returns the string s, or the error that stopped it.
func (m *VM) newString(s string) (jsValue, error) {
	if err := m.usable(); err != nil {
		return exception, err
	}
	v := m.newStringFast(s)
	if isException(v) {
		return v, m.errFromException()
	}
	return v, nil
}

// goString returns the Go string of v, which it borrows: a lone surrogate is
// U+FFFD and a NUL stays.
func (m *VM) goString(v jsValue) (string, error) {
	s, ok := m.argString(v, false)
	if !ok {
		return "", m.errFromException()
	}
	return s, nil
}

// NewObjectValue returns a new empty object, which the caller frees.
func (m *VM) NewObjectValue() (r Value, err error) {
	if err = m.usable(); err != nil {
		return r, err
	}
	v := m.jsNewObject()
	if isException(v) {
		return r, m.errFromException()
	}
	return Value{vm: m, v: v}, nil
}
