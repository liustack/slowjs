// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
	"modernc.org/libc"
)

// A Builtin is a Go function that javascript calls as a built-in function:
// [VM.NewBuiltinFunction] makes it a function object shaped like the
// engine's own built-ins, such as Math.max. It has a name and a length and
// no other property of its own, no prototype property, it cannot be called
// with new, and its toString shows [native code] rather than source.
//
// Unlike [VM.RegisterFunc] and [VM.RegisterHostFunc], nothing is converted
// on the way in and no reflection is involved: the Go function reads the
// arguments it wants from [Args], which give a number only for a number and
// a string only for a string, and returns a [Value]. That makes a call about
// as cheap as a call of a javascript function, which suits functions called
// once per row of a table, as the formatters of package intl are.
//
// A Builtin belongs to no VM. It is made once, typically into a package
// level variable, and every VM makes its own function objects of it: Args
// says which VM called. So a process can have at most 32768 Builtins, and
// they are never released. VMs on several goroutines call the same Builtin
// at once, so its function has to be safe for concurrent use, and state it
// keeps for each VM is found by [Args.VM].
type Builtin struct {
	name   string
	length int32
	fn     func(Args) (Value, error)
	magic  int32
}

// builtins are the Builtins of the process, indexed by their magic. The
// slice is replaced, never changed, so callBuiltin reads it without a lock.
var builtins struct {
	mu   sync.Mutex
	list atomic.Pointer[[]*Builtin]
}

// NewBuiltin returns a Builtin calling fn, for function objects named name
// whose length property is length.
//
// fn owns the Value it returns, which goes to javascript: a Value fn did not
// make itself, such as an argument, is returned as its Dup. UndefinedValue
// and the zero Value return undefined. A Value of another VM is neither
// returned nor freed: the call throws a TypeError.
//
// A non-nil error is thrown instead, and a Value of the VM returned with it
// is freed. An [*Error], found with [errors.As], is thrown as a new error of
// the constructor its Name says when that is one of Error, TypeError,
// RangeError, SyntaxError, ReferenceError and InternalError, and as an Error
// for any other name, with its Message. Any other error is thrown as a
// TypeError with the error's text, as RegisterHostFunc throws it. The engine
// cuts the message of a TypeError, RangeError, SyntaxError, ReferenceError
// or InternalError to 255 bytes. The stack of each starts at the Builtin, as
// that of an error a built-in of the engine throws does.
//
// A panic in fn goes on in the Go code that called javascript, as one in a
// function of RegisterHostFunc does.
//
// NewBuiltin panics when name has a NUL byte, when length is not in 0 to
// 255, when fn is nil, and when the process already has 32768 Builtins:
// all mistakes of the code making them, which runs once.
func NewBuiltin(name string, length int, fn func(Args) (Value, error)) *Builtin {
	switch {
	case strings.IndexByte(name, 0) >= 0:
		panic(fmt.Sprintf("quickjs: the name of a Builtin cannot contain a NUL byte: %q", name))
	case length < 0 || length > math.MaxUint8:
		panic(fmt.Sprintf("quickjs: the length of Builtin %s is %d, not in 0 to 255", name, length))
	case fn == nil:
		panic(fmt.Sprintf("quickjs: Builtin %s has no function", name))
	}

	builtins.mu.Lock()
	defer builtins.mu.Unlock()

	var list []*Builtin
	if p := builtins.list.Load(); p != nil {
		list = *p
	}
	if len(list) > math.MaxInt16 {
		panic("quickjs: too many Builtins")
	}

	b := &Builtin{name: validUTF8(name), length: int32(length), fn: fn, magic: int32(len(list))}
	list = append(list[:len(list):len(list)], b)
	builtins.list.Store(&list)
	return b
}

// NewBuiltinFunction returns a new function object of b. The caller owns it:
// it passes it to javascript, for example as an argument of [Value.Call] or
// with [VM.SetPropertyValue], and then frees it.
func (m *VM) NewBuiltinFunction(b *Builtin) (Value, error) {
	tls := m.runtime.tls
	ctx := m.cContext
	// The engine makes the name an atom, a copy. Freed before it returns, not
	// in a defer (see tlsMark).
	n := len(b.name) + 1
	p := tls.Alloc(n)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), n), b.name+"\x00")
	f := lib.XJS_NewCFunction2(tls, ctx, fp(callBuiltin), p, b.length, lib.EJS_CFUNC_generic_magic, b.magic)
	tls.Free(n)
	if isException(f) {
		return Value{}, m.errFromException()
	}

	return Value{vm: m, v: f}, nil
}

// Args are the arguments of a call of a [Builtin], and the VM that called it.
// What they return is borrowed: a Value is valid for the duration of the
// call (see [Value.Dup] to keep one longer).
type Args struct {
	vm   *VM
	this lib.TJSValue
	argv uintptr
	argc int
}

// VM returns the VM that called the Builtin. The Builtin may evaluate more
// javascript on it, which runs inside the time left to the call that got
// there (see [VM.SetEvalTimeout]).
func (a Args) VM() *VM { return a.vm }

// Len returns the number of arguments of the call.
func (a Args) Len() int { return a.argc }

// This returns the this value of the call.
func (a Args) This() Value { return Value{vm: a.vm, v: a.this} }

// Value returns argument i, undefined when the call has no argument i.
func (a Args) Value(i int) Value { return Value{vm: a.vm, v: a.arg(i)} }

func (a Args) arg(i int) lib.TJSValue {
	if i < 0 || i >= a.argc {
		return undefined
	}

	return *(*lib.TJSValue)(unsafe.Pointer(a.argv + uintptr(i)*sizeofJsValue))
}

// Number returns argument i when it is a number. It converts nothing:
// a string or a Number object is not a number.
func (a Args) Number(i int) (float64, bool) {
	switch v := a.arg(i); tag(v) {
	case lib.EJS_TAG_INT:
		return float64(jsvToInt32(v)), true
	case lib.EJS_TAG_FLOAT64:
		return jsvToFloat64(v), true
	}
	return 0, false
}

// Bool returns argument i when it is a boolean.
func (a Args) Bool(i int) (bool, bool) {
	if v := a.arg(i); tag(v) == lib.EJS_TAG_BOOL {
		return jsvToInt32(v) != 0, true
	}

	return false, false
}

// String returns argument i when it is a string, as UTF-8 with U+FFFD for
// each lone surrogate. It converts nothing else. ok is false too when there
// is no memory for the conversion, and the call then fails with the out of
// memory error whatever the Builtin returns.
func (a Args) String(i int) (s string, ok bool) {
	switch v := a.arg(i); tag(v) {
	case lib.EJS_TAG_STRING, lib.EJS_TAG_STRING_ROPE:
		return argString(a.vm.runtime.tls, a.vm.cContext, v, false)
	}
	return "", false
}

// WTF8 is String, but with each lone surrogate as the three bytes of its
// code point (0xED, then 0xA0 to 0xBF, then a continuation byte), the form
// the engine writes it in, which is not UTF-8. A surrogate pair is its one
// character in UTF-8, as in String. A Builtin that has to tell lone
// surrogates apart, as a collator orders them, reads its string this way.
func (a Args) WTF8(i int) (s string, ok bool) {
	switch v := a.arg(i); tag(v) {
	case lib.EJS_TAG_STRING, lib.EJS_TAG_STRING_ROPE:
		return argString(a.vm.runtime.tls, a.vm.cContext, v, true)
	}
	return "", false
}

// typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
func callBuiltin(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) (r lib.TJSValue) {
	b := (*builtins.list.Load())[magic]
	m := vms.vm(uint32(lib.XJS_GetContextOpaque(tls, ctx)))
	m.host++
	mark := markTLS(tls)
	returned := false
	defer func() {
		m.host--
		if m.hostEnded(mark, returned) {
			return
		}
		switch p := recover(); {
		case p != nil:
			m.hostPanicked(b.name, p)
		case !returned:
			m.broken = true // runtime.Goexit, which goes on up through the engine
		}
		// As in callGo: an interrupt or running out of memory in the Go
		// function stays pending and is the result.
		if m.uncatchablePending() {
			if !isException(r) {
				lib.XFreeValue(tls, ctx, r)
			}
			r = exception
		}
	}()
	v, err := b.fn(Args{vm: m, this: this, argv: argv, argc: int(argc)})
	switch {
	case err != nil:
		if v.vm == m {
			v.Free()
		}
		r = m.throw(err)
	case v.vm != m && v.vm != nil:
		// Freeing it here would use the other VM, maybe on another goroutine.
		r = throwTypeError(tls, ctx, fmt.Sprintf("Go function %s returned a Value of another VM", b.name))
	default:
		r = v.jsValue()
	}
	returned = true
	return r
}

// throw throws err in m's context, as NewBuiltin describes, and returns the
// exception.
func (m *VM) throw(err error) lib.TJSValue {
	tls := m.runtime.tls
	ctx := m.cContext
	var e *Error
	if !errors.As(err, &e) {
		return throwTypeError(tls, ctx, err.Error())
	}

	if e.Uncatchable && m.uncatchablePending() {
		// The error the Go function got from javascript it evaluated, which
		// is still pending.
		return exception
	}

	switch e.Name {
	case "TypeError":
		return throwf(tls, ctx, lib.XJS_ThrowTypeError, e.Message)
	case "RangeError":
		return throwf(tls, ctx, lib.XJS_ThrowRangeError, e.Message)
	case "SyntaxError":
		return throwf(tls, ctx, lib.XJS_ThrowSyntaxError, e.Message)
	case "ReferenceError":
		return throwf(tls, ctx, lib.XJS_ThrowReferenceError, e.Message)
	case "InternalError":
		return throwf(tls, ctx, lib.XJS_ThrowInternalError, e.Message)
	}

	return m.throwError(e.Message)
}

// throwError throws an Error with msg, whole, in m's context and returns the
// exception. The engine has no function throwing an Error, and an Error made
// with JS_NewError gets its stack only from the javascript that called the
// Builtin: so it throws a TypeError, whose stack starts at the Builtin as
// those of the engine's built-ins do, and makes it an Error.
func (m *VM) throwError(msg string) lib.TJSValue {
	tls := m.runtime.tls
	ctx := m.cContext
	if r := throwTypeError(tls, ctx, msg); m.uncatchablePending() {
		return r
	}

	exc := lib.XJS_GetException(tls, ctx)
	o := lib.XJS_NewError(tls, ctx)
	if isException(o) {
		lib.XFreeValue(tls, ctx, exc)
		return o
	}

	proto := lib.XJS_GetPrototype(tls, ctx, o) // Error.prototype, a page cannot change it
	lib.XFreeValue(tls, ctx, o)
	ok := lib.XJS_SetPrototype(tls, ctx, exc, proto) >= 0
	lib.XFreeValue(tls, ctx, proto)
	if !ok {
		lib.XFreeValue(tls, ctx, exc)
		return exception
	}

	// The TypeError has the message cut to 255 bytes.
	s, err := m.newString(msg)
	if err != nil {
		lib.XFreeValue(tls, ctx, exc)
		return lib.XJS_ThrowOutOfMemory(tls, ctx)
	}

	if isException(s) {
		lib.XFreeValue(tls, ctx, exc)
		return s
	}

	// JS_DefinePropertyValue takes s, also when it fails.
	if lib.XJS_DefinePropertyValue(tls, ctx, exc, lib.EJS_ATOM_message, s, lib.MJS_PROP_WRITABLE|lib.MJS_PROP_CONFIGURABLE) < 0 {
		lib.XFreeValue(tls, ctx, exc)
		return exception
	}

	return lib.XJS_Throw(tls, ctx, exc)
}

// jsValue returns the javascript value of v, which the caller hands on:
// undefined for the zero Value.
func (v Value) jsValue() lib.TJSValue {
	var zero lib.TJSValue
	if v.vm == nil && v.v == zero {
		return undefined
	}

	return v.v
}

// vms finds the VM of a context, which javascript calling a Builtin passes
// on: a VM's id is its context's opaque pointer. Reading takes no lock, so
// VMs on several goroutines calling Builtins do not wait for each other.
var vms vmTable

const (
	vmChunkBits = 10
	vmChunkSize = 1 << vmChunkBits
	vmChunks    = 1 << 10 // a million VMs at once
)

type vmTable struct {
	mu     sync.Mutex
	free   []uint32
	next   uint32 // the ids handed out so far, 0 is never one
	chunks [vmChunks]atomic.Pointer[[vmChunkSize]atomic.Pointer[VM]]
}

// add gives m an id.
func (t *vmTable) add(m *VM) (uint32, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var id uint32
	switch n := len(t.free); {
	case n > 0:
		id = t.free[n-1]
		t.free = t.free[:n-1]
	case t.next+1 >= vmChunks*vmChunkSize:
		return 0, fmt.Errorf("quickjs: too many VMs")
	default:
		t.next++
		id = t.next
	}
	c := t.chunks[id>>vmChunkBits].Load()
	if c == nil {
		c = new([vmChunkSize]atomic.Pointer[VM])
		t.chunks[id>>vmChunkBits].Store(c)
	}
	c[id&(vmChunkSize-1)].Store(m)
	return id, nil
}

// remove takes id back.
func (t *vmTable) remove(id uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.chunks[id>>vmChunkBits].Load()[id&(vmChunkSize-1)].Store(nil)
	t.free = append(t.free, id)
}

func (t *vmTable) vm(id uint32) *VM {
	return t.chunks[id>>vmChunkBits].Load()[id&(vmChunkSize-1)].Load()
}

// NewArray returns a new array of vals, made as an array literal makes one:
// its elements are own properties, which no setter a page put on
// Array.prototype sees. It borrows vals, as [VM.SetPropertyValue] and
// [Value.Call] do: the caller still owns them. The zero Value is undefined.
// A Value of another VM, a number too, is an error, as it is for a
// [Builtin] to return one.
func (m *VM) NewArray(vals ...Value) (Value, error) {
	for _, v := range vals {
		if v.vm != m && v.vm != nil {
			return Value{}, errors.New("quickjs: NewArray of a Value of another VM")
		}
	}

	tls := m.runtime.tls
	ctx := m.cContext
	a := lib.XJS_NewArray(tls, ctx)
	if isException(a) {
		return Value{}, m.errFromException()
	}

	for i, v := range vals {
		// JS_DefinePropertyValueUint32 takes the copy, also when it fails.
		e := lib.XDupValue(tls, ctx, v.jsValue())
		if lib.XJS_DefinePropertyValueUint32(tls, ctx, a, uint32(i), e, lib.MJS_PROP_C_W_E|lib.MJS_PROP_THROW) < 0 {
			lib.XFreeValue(tls, ctx, a)
			return Value{}, m.errFromException()
		}
	}
	return Value{vm: m, v: a}, nil
}
