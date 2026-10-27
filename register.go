// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	staticMagic atomic.Int32

	goFuncsMu sync.Mutex
	goFuncs   = map[int32]*goFunc{}
	freeMagic []int32
)

type goFunc struct {
	f            reflect.Value
	in           []reflect.Type
	name         string
	out          []reflect.Type
	variadicType reflect.Type
	vm           *VM

	minArgs int
	maxArgs int

	wantThis bool
	hostFunc bool
}

// RegisterFunc registers a Go function 'f' and makes it callable from
// Javascript.
//
// The 'f' argument can be a regular Go function, a closure, a method
// expression or a method value. All of them are called 'Go function' below.
//
// The Go function can have zero or more parameters. If 'wantThis' is true then
// the first parameter of the Go function will get the Javascript value of
// 'this'.  Depending on context, 'this' can be Javascript null or undefined.
//
// Go functions with multiple results return them as an Javascript array.
//
// Go nil errors are converted to Javascript null.
//
// Go non-nil errors are converted to Javascript strings using the Error()
// method.
//
// Any Go <-> Javascript failing type conversion between arguments/return
// values throws a Javascript type error exception.
//
// There is a limit on the total number of currently registered Go
// functions.
//
// The function becomes the property 'name' of the global object: writable,
// configurable and not enumerable. A configurable property already there,
// such as one a script assigned or a built-in like Array, is replaced. A
// property that is not configurable, such as undefined, NaN, Infinity or one
// a script declared with var or function, is kept and RegisterFunc returns an
// error, as it does when the global object is not extensible. Registering a
// name a Go function of this VM already has returns an error too, even when
// a script has since deleted or replaced that function. The name is the one
// javascript sees: invalid UTF-8 is replaced as Go decodes it (see Eval), so
// two Go strings can be the same name, and a NUL byte, which would end the
// name, is an error.
//
// Note: The 'name' argument should be a valid Javascript identifier. It is not
// currently enforced but this may change later.
//
// For binding host objects (a DOM, console, fetch, …) prefer RegisterHostFunc,
// which gives the more convenient single-return / error-throws-exception shape.
//
// A function javascript calls often, say once per item of a list, or one that
// has to look like a built-in, is better a [Builtin]: it converts nothing and
// uses no reflection, so a call costs far less.
func (m *VM) RegisterFunc(name string, f any, wantThis bool) error {
	return m.registerFunc(name, f, wantThis, false)
}

// HostFunc is a Go function exposed to JavaScript by RegisterHostFunc. It
// receives the call arguments already converted to Go values and returns a
// single value to convert back to JS, or a non-nil error which is thrown as a
// JavaScript exception.
type HostFunc func(args []any) (any, error)

// RegisterHostFunc exposes fn to JavaScript as a global function named name,
// using the calling convention an embedding host wants: every JS call argument
// arrives in the single []any slice (already converted to Go values), fn returns
// exactly one value to convert back to JS, and a non-nil error is thrown as a
// JavaScript exception.
//
// This differs from RegisterFunc, whose reflection mapping turns a Go
// multiple-return into a JS array and converts a returned error like any other
// value (so a (value, error) pair becomes the array [value, error] rather than a
// value-or-throw). RegisterHostFunc is the better fit for binding host objects;
// RegisterFunc remains for direct, typed Go calls.
//
// Each call converts every argument to a Go value through reflection. A
// function javascript calls often, or one that has to look like a built-in,
// is better a [Builtin], which reads only the arguments it wants, as they
// are.
//
// The name is handled as by RegisterFunc, including when the global object
// already has it.
func (m *VM) RegisterHostFunc(name string, fn HostFunc) error {
	if fn == nil {
		return fmt.Errorf("host func %q cannot be nil", name)
	}

	// Register a variadic adapter so every JS argument is collected into the one
	// []any; the host flag makes callGo return fn's value directly and throw on a
	// non-nil error, instead of array-wrapping the (value, error) pair.
	return m.registerFunc(name, func(args ...any) (any, error) { return fn(args) }, false, true)
}

func (m *VM) registerFunc(name string, f any, wantThis, host bool) (err error) {
	if name == "" {
		return fmt.Errorf("func name cannot be empty")
	}

	jsName := validUTF8(name)
	if strings.IndexByte(jsName, 0) >= 0 {
		return fmt.Errorf("func name cannot contain a NUL byte: %q", name)
	}

	if _, ok := m.goFuncs[jsName]; ok {
		return fmt.Errorf("func already registered: %s", name)
	}

	val := reflect.ValueOf(f)
	typ := reflect.TypeOf(f)
	if typ.Kind() != reflect.Func {
		return fmt.Errorf("%s is not a function", name)
	}

	if wantThis && typ.NumIn() == 0 {
		return fmt.Errorf("'wantThis' is true but 'f' has zero parameters")
	}

	minArgs := typ.NumIn()
	if wantThis {
		minArgs--
	}
	maxArgs := minArgs
	var vt reflect.Type
	if typ.IsVariadic() {
		maxArgs = -1
		sliceT := typ.In(typ.NumIn() - 1)
		vt = sliceT.Elem()
		minArgs--
	}
	info := &goFunc{
		f:            val,
		maxArgs:      maxArgs,
		minArgs:      minArgs,
		name:         name,
		variadicType: vt,
		vm:           m,
		wantThis:     wantThis,
		hostFunc:     host,
	}
	for i := 0; i < typ.NumIn(); i++ {
		info.in = append(info.in, typ.In(i))
	}
	if maxArgs < 0 {
		info.in = info.in[:len(info.in)-1]
	}
	for i := 0; i < typ.NumOut(); i++ {
		info.out = append(info.out, typ.Out(i))
	}

	magic, err := registerCallback(info)
	if err != nil {
		return err
	}

	argc := int32(typ.NumIn()) - 1
	if argc < 0 {
		argc = 0
	}
	if err := m.defineGoFunc(jsName, magic, argc); err != nil {
		releaseMagic(magic)
		return fmt.Errorf("cannot register func %s: %w", name, err)
	}

	if m.goFuncs == nil {
		m.goFuncs = map[string]int32{}
	}
	m.goFuncs[jsName] = magic
	return nil
}

// defineGoFunc defines the global property name, a valid UTF-8 string without
// NUL bytes, as a javascript function of length 'length' that calls the Go
// function registered as magic. The property is writable, configurable and
// not enumerable, and it replaces a configurable property of that name. It
// does not use JS_SetPropertyFunctionList, which is meant for new objects:
// it aborts the process when the property already exists.
func (m *VM) defineGoFunc(name string, magic, length int32) error {
	// Both the function's name and the atom copy the string.
	f, err := m.newGoFunction(name, magic, length)
	if err != nil {
		return err
	}

	if isException(f) {
		return m.errFromException()
	}

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

	g := m.globalObject()

	defer m.jsFreeValue(g)

	// JS_DefinePropertyValue takes f, also when it fails.
	if m.jsDefinePropertyValue(g, atom, f, propWritable|propConfigurable|propThrow) < 0 {
		return m.errFromException()
	}

	return nil
}

// registerCallback allocates a magic number for the given goFunc and stores
// it in the global goFuncs map. It returns the allocated magic number or an error.
func registerCallback(info *goFunc) (int32, error) {
	goFuncsMu.Lock()
	defer goFuncsMu.Unlock()
	if n := len(freeMagic); n > 0 {
		k := freeMagic[n-1]
		freeMagic = freeMagic[:n-1]
		goFuncs[k] = info
		return k, nil
	}
	magic := staticMagic.Add(1)
	if magic > math.MaxInt16 {
		staticMagic.Add(-1)
		return -1, fmt.Errorf("too many registered functions")
	}
	goFuncs[magic] = info
	return magic, nil
}

// releaseMagic removes the Go function associated with magic from the global
// tables, returning it to the free pool.
func releaseMagic(magic int32) {
	goFuncsMu.Lock()
	// Append only if the magic was actually registered. freeMagic is a slice,
	// so unlike the old map it does not dedup: a double release would queue the
	// same magic twice and later hand it to two live callbacks at once.
	if _, ok := goFuncs[magic]; ok {
		delete(goFuncs, magic)
		freeMagic = append(freeMagic, magic)
	}
	goFuncsMu.Unlock()
}

var rvt = reflect.TypeOf(Value{})

func isNative(t reflect.Type) bool {
	return t == rvt
}

// canBeNil reports whether nil is a valid value of type 't'. Javascript null
// converts to a Go nil, so it can be passed only to a parameter able to hold
// one. Coercing it to the zero value of, say, string or int would silently
// make f(null) indistinguishable from f("") or f(0).
func canBeNil(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return true
	}

	return false
}

// callGoFunc converts the arguments of a call of info from javascript,
// calls it and converts its results.
func (m *VM) callGoFunc(info *goFunc, this jsValue, argc int32, argv uintptr) (r jsValue) {
	haveArgs := int(argc)
	if haveArgs < info.minArgs {
		return m.jsThrowError(errorKindType, fmt.Sprintf("not enough arguments in call to %s", info.name))
	}

	if info.maxArgs >= 0 && haveArgs > info.maxArgs {
		return m.jsThrowError(errorKindType, fmt.Sprintf("too many arguments in call to %s", info.name))
	}

	var in []reflect.Value
	i := 0
	if info.wantThis {
		switch {
		case isNative(info.in[0]):
			dup := dupValue(this)
			defer m.jsFreeValue(dup)
			in = append(in, reflect.ValueOf(Value{vm: m, v: dup}))
		default:
			v, err := m.value(this, false)
			if err != nil {
				return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: argument conversion error: %v", info.name, err))
			}

			rv := reflect.ValueOf(v)
			switch {
			case !rv.IsValid(): // Javascript null, m.value returns a nil any for it.
				if !canBeNil(info.in[0]) {
					return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: cannot assign null to %s", info.name, info.in[0]))
				}

				rv = reflect.Zero(info.in[0])
			case !rv.Type().AssignableTo(info.in[0]):
				return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: cannot assign %s to %s", info.name, rv.Type(), info.in[0]))
			}

			in = append(in, rv)
		}
		i++
	}
	types := info.in[i:]
	for i := 0; i < int(argc); i++ {
		var typ reflect.Type
		switch {
		case i < len(types):
			typ = types[i]
		default:
			typ = info.variadicType
		}
		switch {
		case isNative(typ):
			dup := dupValue(valueAt(argv))
			defer m.jsFreeValue(dup)
			in = append(in, reflect.ValueOf(Value{vm: m, v: dup}))
		default:
			v, err := m.value(valueAt(argv), false)
			if err != nil {
				return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: argument conversion error: %v", info.name, err))
			}

			rv := reflect.ValueOf(v)
			switch {
			case !rv.IsValid(): // Javascript null, m.value returns a nil any for it.
				if !canBeNil(typ) {
					return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: cannot assign null to %s", info.name, typ))
				}

				rv = reflect.Zero(typ)
			case !rv.Type().AssignableTo(typ):
				return m.jsThrowError(errorKindType, fmt.Sprintf("calling %s: cannot assign %s to %s", info.name, rv.Type(), typ))
			}

			in = append(in, rv)
		}
		argv += sizeofJsValue
	}

	out := info.f.Call(in)
	var err error

	// A host function (RegisterHostFunc) has the fixed shape func(...any) (any,
	// error): return its single value, or throw a JS exception for a non-nil
	// error — rather than array-wrapping the (value, error) pair.
	if info.hostFunc {
		if e, _ := out[1].Interface().(error); e != nil {
			return m.jsThrowError(errorKindType, e.Error())
		}
		r, err = m.jsValue(out[0])
		if err != nil {
			return m.jsThrowError(errorKindType, fmt.Sprintf("callback %s: %v", info.name, err))
		}
		return r
	}

	switch len(info.out) {
	case 0:
		return undefined
	case 1:
		switch {
		case isNative(info.out[0]):
			x, ok := out[0].Interface().(Value)
			if !ok {
				return m.jsThrowError(errorKindType, fmt.Sprintf("internal error: native Javascript value does not have type 'Value' (%v:)", origin(1)))
			}

			r = x.v
		default:
			r, err = m.jsValue(out[0])
			if err != nil {
				return m.jsThrowError(errorKindType, fmt.Sprintf("cannot convert value: %v", err))
			}
		}
	default:
		r, err = m.jsArray(out)
	}
	if err != nil {
		return m.jsThrowError(errorKindType, fmt.Sprintf("callback %s: %v", info.name, err))
	}

	return r
}
