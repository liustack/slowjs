// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

// Go functions that javascript calls through reflection: RegisterFunc,
// RegisterHostFunc and the handlers of Then.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// RegisterFunc makes the Go function f a function of the global object named
// name, which javascript calls.
//
// f is any Go function: a function, a closure, a method expression or a
// method value, with any number of parameters, variadic or not. With
// wantThis, its first parameter gets the this of the call, which may be null
// or undefined: it is not replaced by the global object.
//
// Each argument is converted to the type of its parameter, as [VM.Eval]
// converts a value: a number that is int32 is an int, other numbers float64,
// an object an *Object, null a nil of a parameter that takes one. A parameter
// of type [Value] gets the argument itself, valid only during the call: to
// keep it, such as a resolve to call later, keep a Dup. An argument that
// does not convert to its parameter, or a call with too few or too many
// arguments, throws a TypeError.
//
// The result becomes a javascript value; several results become an array of
// them. A nil error is null, and a non-nil one the string of its Error:
// RegisterFunc throws nothing for an error. A result that fails to convert
// throws a TypeError, as do a Value of another VM in a result and a result
// that contains itself, such as a slice that is its own element. How the
// results convert:
//
//   - nil, a nil pointer, interface or map is null, a nil slice [].
//     UndefinedValue and the zero Value are undefined, and so is [Undefined].
//   - A bool, an integer, a float and a string become their javascript
//     value, an integer outside int32 a float. *big.Int is a BigInt.
//     *Object is its JSON parsed.
//   - A slice is an array. A map with string or integer keys is an object of
//     own data properties, in the order of the keys as strings.
//   - A struct converts through encoding/json, and a value whose type has an
//     Error method is the string of Error. A String method changes nothing.
//   - A Value of the VM in a result is handed to the call: the result, an
//     element of a slice or an array, a key or a value of a map and the
//     target of a pointer each hand over one reference, which the call frees
//     when it is done, whether the conversion succeeds, fails or panics. A
//     copy of a Value is no new reference, only a Dup is, and a slice, map or
//     pointer the results reach twice is still one. So a Go function that
//     returns one of its Value arguments returns its Dup. The Values inside a
//     struct or a value with an Error method stay the caller's.
//
// The results are looked at, for the Values they hold, a cycle and a Value of
// another VM, before the conversion runs any code of the program, such as an
// Error or a MarshalJSON method, which then cannot change which references
// are taken and freed. Results whose types can hold no Value are not looked
// at first: they are read as they convert.
//
// A long string argument or result, and a result of many elements or
// properties, stops converting at Interrupt, the eval timeout or an
// uncatchable error of the call, which then ends with that error.
//
// The function becomes the property name of the global object, writable and
// configurable, not enumerable. It replaces a configurable property, one a
// script set or a built-in such as Array. A property that is not
// configurable, such as undefined, NaN, Infinity or a var or function of a
// script, is kept, and RegisterFunc returns an error, as it does when the
// global object is not extensible. The length of the function is the number
// of parameters of f less one, and at least 0.
//
// name is the name javascript sees: invalid UTF-8 in it is U+FFFD a byte, so
// two Go strings can name one function. A NUL byte is an error. name should
// be an identifier: it is not checked yet, and may be later. A name is
// registered once per VM, even after a script deleted or replaced its
// function. A VM holds about a billion Go functions at a time at most.
//
// For a host object prefer [VM.RegisterHostFunc]. A function called often,
// or one that is to look like a built-in, is better as a [Builtin], which
// converts nothing and uses no reflection, and costs far less.
func (m *VM) RegisterFunc(name string, f any, wantThis bool) error {
	if err := m.usable(); err != nil {
		return err
	}
	return m.register(name, f, wantThis, false)
}

// RegisterHostFunc makes fn a function of the global object named name, which
// javascript calls. Each argument of the call is converted to a Go value, as
// [VM.Eval] converts a value, into the slice fn gets. The value fn returns is
// converted back, by the rules of the results of [VM.RegisterFunc], Values
// and their references too. A non-nil error is thrown as a TypeError with the
// text of the error, and the value is not converted: the references it holds
// are freed before the Error method of the error runs. Unlike RegisterFunc,
// which makes the array [value, error] of the two, fn returns one value or
// throws. name is as for RegisterFunc.
//
// Each call converts every argument by reflection: a function called often
// is better as a [Builtin].
func (m *VM) RegisterHostFunc(name string, fn HostFunc) error {
	if err := m.usable(); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("host func %q cannot be nil", name)
	}
	return m.register(name, func(args ...any) (any, error) { return fn(args) }, false, true)
}

// goFunc is a Go function that javascript calls, with what its calls need to
// know of its type.
type goFunc struct {
	// name is the name the function was registered under in Go, for the
	// errors of its calls and its panics.
	name string
	fn   reflect.Value
	// in are the types of the parameters, without a variadic one, whose
	// elements are of type rest.
	in   []reflect.Type
	rest reflect.Type
	// min and max are the fewest and the most arguments of a call, max -1
	// for no limit.
	min, max int
	// wantThis gives the first parameter the this of the call. host is set
	// for RegisterHostFunc. refs is set when a result can hold a Value, so
	// that a call takes the snapshot of its results.
	wantThis, host, refs bool
	nout                 int
}

// register makes the Go function f a global function of m named name, a
// host function when host is set.
func (m *VM) register(name string, f any, wantThis, host bool) error {
	if name == "" {
		return errors.New("func name cannot be empty")
	}
	jsName := validUTF8(name)
	if strings.IndexByte(jsName, 0) >= 0 {
		return fmt.Errorf("func name cannot contain a NUL byte: %q", name)
	}
	if _, ok := m.registered[jsName]; ok {
		return fmt.Errorf("func already registered: %s", name)
	}
	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func || fv.IsNil() {
		return fmt.Errorf("%s is not a function", name)
	}
	ft := fv.Type()
	if wantThis && ft.NumIn() == 0 {
		return errors.New("'wantThis' is true but 'f' has zero parameters")
	}

	info := &goFunc{name: name, fn: fv, wantThis: wantThis, host: host, nout: ft.NumOut()}
	n := ft.NumIn()
	info.in = make([]reflect.Type, n)
	for i := range n {
		info.in[i] = ft.In(i)
	}
	info.min, info.max = n, n
	if wantThis {
		info.min--
		info.max--
	}
	if ft.IsVariadic() {
		info.rest = info.in[n-1].Elem()
		info.in = info.in[:n-1]
		info.min--
		info.max = -1
	}
	for i := range info.nout {
		if host && i == 1 {
			continue
		}
		if holdsRefs(ft.Out(i)) {
			info.refs = true
		}
	}

	id, err := m.keepFunc(info)
	if err != nil {
		return err
	}
	if err := m.defineGoFunc(jsName, id, int32(max(n-1, 0))); err != nil {
		return fmt.Errorf("cannot register func %s: %w", name, err)
	}
	if m.registered == nil {
		m.registered = map[string]int32{}
	}
	m.registered[jsName] = id
	return nil
}

// defineGoFunc defines the global function name, of length length, that
// calls the Go function m keeps under handle.
func (m *VM) defineGoFunc(name string, handle, length int32) error {
	f, err := m.newGoFunction(name, handle, length)
	if err != nil {
		return err
	}
	if err := m.defineGlobalFunc(name, f); err != nil {
		m.goFuncFreed(handle)
		return err
	}
	return nil
}

// defineGlobalFunc defines the property name of the global object as f, as
// Object.defineProperty does: writable, configurable and not enumerable. It
// takes f, whether it succeeds or not. name is valid UTF-8 with no NUL byte.
func (m *VM) defineGlobalFunc(name string, f jsValue) error {
	atom, err := m.jsNewAtom(name)
	if err != nil {
		m.jsFreeValue(f)
		return err
	}
	if atom == atomNull {
		m.jsFreeValue(f)
		return m.errFromException()
	}
	defer m.jsFreeAtom(atom)
	g := m.jsGetGlobalObject()
	defer m.jsFreeValue(g)
	// It takes f, also when it fails.
	if m.jsDefinePropertyValue(g, atom, f, propWritable|propConfigurable|propThrow) < 0 {
		return m.errFromException()
	}
	return nil
}

// callbackFunc is the goFunc of fn, a handler of Then named name: one
// argument, a Value valid during the call, and no result.
func callbackFunc(m *VM, name string, fn func(Value)) *goFunc {
	return &goFunc{name: name, fn: reflect.ValueOf(fn), in: []reflect.Type{rvt}, min: 1, max: 1}
}

// throwType throws a TypeError of msg and returns the exception.
func (m *VM) throwType(msg string) jsValue {
	return m.jsThrowError(errorKindType, msg)
}

// paramType is the type of the parameter that argument i of a call, counting
// this, goes to.
func (info *goFunc) paramType(i int) reflect.Type {
	if i < len(info.in) {
		return info.in[i]
	}
	return info.rest
}

// nilable reports whether a value of type t can be nil.
func nilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return true
	}
	return false
}

// callGoFunc calls the Go function of info with the argc arguments at argv,
// in the memory of the engine, and this, and returns its result or the
// exception it threw.
func (m *VM) callGoFunc(info *goFunc, this jsValue, argc int32, argv uintptr) jsValue {
	n := int(argc)
	if n < info.min {
		return m.throwType("not enough arguments in call to " + info.name)
	}
	if info.max >= 0 && n > info.max {
		return m.throwType("too many arguments in call to " + info.name)
	}
	k := n
	if info.wantThis {
		k++
	}
	args := make([]reflect.Value, k)
	// The Dups that the parameters of type Value get, freed when the call is
	// done.
	var buf [4]jsValue
	held := buf[:0]
	defer func() { m.freeAll(held) }()
	for i := range k {
		x := this
		if info.wantThis {
			if i > 0 {
				x = valueAt(argv + uintptr(i-1)*sizeofJsValue)
			}
		} else {
			x = valueAt(argv + uintptr(i)*sizeofJsValue)
		}
		t := info.paramType(i)
		if t == rvt {
			d := dupValue(x)
			held = append(held, d)
			args[i] = reflect.ValueOf(Value{vm: m, v: d})
			continue
		}
		g, err := m.value(x, false)
		if err != nil {
			return m.throwType(fmt.Sprintf("calling %s: argument conversion error: %v", info.name, err))
		}
		if g == nil {
			if !nilable(t) {
				return m.throwType(fmt.Sprintf("calling %s: cannot assign null to %v", info.name, t))
			}
			args[i] = reflect.Zero(t)
			continue
		}
		a := reflect.ValueOf(g)
		if !a.Type().AssignableTo(t) {
			return m.throwType(fmt.Sprintf("calling %s: cannot assign %v to %v", info.name, a.Type(), t))
		}
		args[i] = a
	}

	out := info.fn.Call(args)

	var r jsValue
	var err error
	switch {
	case info.host:
		if e := out[1]; !e.IsNil() {
			if info.refs {
				// Before Error, which may panic: the references go first.
				m.dropResult(out[0])
			}
			return m.throwType(e.Interface().(error).Error())
		}
		if r, err = m.oneResult(info, out[:1]); err != nil {
			return m.throwType(fmt.Sprintf("callback %s: %v", info.name, err))
		}
	case info.nout == 0:
		return undefined
	case info.nout == 1:
		if r, err = m.oneResult(info, out[:1]); err != nil {
			return m.throwType(fmt.Sprintf("cannot convert value: %v", err))
		}
	default:
		if info.refs {
			r, err = m.convertResults(out, true)
		} else {
			r, err = m.jsArray(len(out), func(i int) reflect.Value { return out[i] })
		}
		if err != nil {
			return m.throwType(fmt.Sprintf("callback %s: %v", info.name, err))
		}
	}
	return r
}

// oneResult converts out, the one result of a call of info, or the value of
// a host function.
func (m *VM) oneResult(info *goFunc, out []reflect.Value) (jsValue, error) {
	if info.refs {
		return m.convertResults(out, false)
	}
	return m.jsValue(out[0])
}
