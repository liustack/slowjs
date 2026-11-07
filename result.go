// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"reflect"
	"sync"
	"unsafe"
)

// errCycle is the error of a result of a Go function that contains itself,
// which no conversion could finish. encoding/json refuses such a value too.
var errCycle = errors.New("quickjs: a result that contains itself")

// The results of a Go function, of RegisterFunc or RegisterHostFunc, hand
// the call the references they hold, whether they convert or not:
//
//   - A reference is a place that holds a Value of the VM: a result, an
//     element of a slice or an array, a key or a value of a map, the target
//     of a pointer, and the any that holds one of them. A Value copied, by
//     an assignment or into an any, is not a new reference: only Dup makes
//     one, and nothing in a Value tells a copy from a Dup. So a place is a
//     reference, and a place the results reach twice, through the same
//     slice, map or pointer, is still one.
//   - resultRefs collects the references of the results, each once, before
//     anything converts, and the call frees them when it is done: when the
//     results convert, when they do not, when a host function returns an
//     error and when the conversion panics. The conversion only Dups the
//     Values it puts in the javascript value, so what it made and what was
//     handed over are freed apart, each once.
//   - The same walk refuses results that contain themselves, so that the
//     conversion, which follows the same slices, maps, pointers and
//     interfaces, cannot recurse without end, and a Value of another VM,
//     which stays its owner's.
//   - A struct converts through encoding/json, and the Values in it, which
//     marshal as JSON, stay the caller's: the walk does not go into one.

// refWalk is the state of resultRefs.
type refWalk struct {
	m   *VM
	err error
	// The places and maps the walk is in (not done), to find a cycle, and
	// the ones it is done with, to go through each once: the first ones in
	// few, on the stack, and all of them in many once few is full.
	n    int
	few  [16]refSeen
	many map[refPlace]bool
}

type refSeen struct {
	at   refPlace
	done bool
}

// enter records that the walk is in at, or done with it when done is set,
// and returns where in few, or -1 when it is in many. When the walk has met
// at already it returns ok false, after it records errCycle if the walk is
// still inside at.
func (w *refWalk) enter(at refPlace, done bool) (i int, ok bool) {
	if w.many == nil {
		for i := range w.n {
			if w.few[i].at == at {
				if !w.few[i].done {
					w.fail(errCycle)
				}
				return i, false
			}
		}
		if w.n < len(w.few) {
			w.few[w.n] = refSeen{at, done}
			w.n++
			return w.n - 1, true
		}
		w.many = make(map[refPlace]bool, 2*len(w.few))
		for _, s := range w.few {
			w.many[s.at] = s.done
		}
	}
	if d, met := w.many[at]; met {
		if !d {
			w.fail(errCycle)
		}
		return -1, false
	}
	w.many[at] = done
	return -1, true
}

// leave records that the walk is done with at, which enter put at i.
func (w *refWalk) leave(at refPlace, i int) {
	if i >= 0 && w.many == nil {
		w.few[i].done = true
		return
	}
	w.many[at] = true
}

// refPlace is a place in memory, an addressable value or a map, with its
// type, so that an array and its first element are two places.
type refPlace struct {
	p unsafe.Pointer
	t reflect.Type
}

// resultRefs collects the references to Values of m that out holds, after
// those in held, and reports the first of errCycle and errForeignValue it
// finds. It walks all of out even then, so the call frees every reference it
// was handed.
func (m *VM) resultRefs(out []reflect.Value, held []jsValue) ([]jsValue, error) {
	// A result that is a Value, or an any of one, is a place of its own, which
	// needs no refWalk to clear, and most others hold none.
	var err error
	for i, v := range out {
		if v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
		}
		switch {
		case v.Type() == rvt:
			held = m.heldRef(v.Interface().(Value), held, &err)
		case refsIn(v):
			return m.walkRefs(out[i:], held, err)
		}
	}
	return held, err
}

// walkRefs is resultRefs of out, through a refWalk.
func (m *VM) walkRefs(out []reflect.Value, held []jsValue, err error) ([]jsValue, error) {
	w := refWalk{m: m, err: err}
	for _, v := range out {
		held = w.walk(v, false, held)
	}
	return held, w.err
}

// heldRef appends to held the reference of x, a Value of m, or sets *err to
// errForeignValue, unless it is set, for one of another VM.
func (m *VM) heldRef(x Value, held []jsValue, err *error) []jsValue {
	switch {
	case x.vm == m:
		if hasRefCount(x.v) {
			held = append(held, x.v)
		}
	case x.vm != nil && *err == nil:
		*err = errForeignValue
	}
	return held
}

func (w *refWalk) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// refsIn reports whether v can hold a Value or itself: it is not nil, and
// its type, or the type in it for an interface, holdsRefs.
func refsIn(v reflect.Value) bool {
	kind := v.Kind()
	switch kind {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return false
		}
	}
	if kind == reflect.Interface {
		return holdsRefs(v.Elem().Type())
	}
	return holdsRefs(v.Type())
}

// walk appends to held the references of v, and returns it. inline is set for a copy of a key or a value of a map, and
// for the elements of an array in one: they are not places of their own, but
// parts of the entry of their map.
func (w *refWalk) walk(v reflect.Value, inline bool, held []jsValue) []jsValue {
	if !refsIn(v) {
		return held
	}
	kind := v.Kind()

	var at refPlace
	switch {
	case kind == reflect.Map:
		at = refPlace{v.UnsafePointer(), v.Type()}
	case !inline && v.CanAddr():
		at = refPlace{unsafe.Pointer(v.UnsafeAddr()), v.Type()}
	}
	// A Value holds nothing, so the walk is done with it as it enters it.
	leaf := kind == reflect.Struct
	seenAt := -1
	if at.p != nil {
		i, ok := w.enter(at, leaf)
		if !ok {
			return held
		}
		seenAt = i
	}

	switch kind {
	case reflect.Struct: // a Value, holdsRefs says
		var x Value
		if v.CanAddr() {
			x = *(*Value)(unsafe.Pointer(v.UnsafeAddr()))
		} else {
			x = v.Interface().(Value)
		}
		held = w.m.heldRef(x, held, &w.err)
	case reflect.Interface, reflect.Pointer:
		held = w.walk(v.Elem(), false, held)
	case reflect.Slice:
		if v.Type() == anySliceType {
			return w.walkAnySlice(v, at, seenAt, held)
		}
		for i := range v.Len() {
			held = w.walk(v.Index(i), false, held)
		}
	case reflect.Array:
		for i := range v.Len() {
			held = w.walk(v.Index(i), inline, held)
		}
	case reflect.Map:
		if v.Type() == anyMapType {
			return w.walkAnyMap(v, at, seenAt, held)
		}
		// The key and the value of each entry are copied into one variable
		// each, which does not allocate as MapIter.Key and Value do.
		t := v.Type()
		var key, val reflect.Value
		if holdsRefs(t.Key()) {
			key = reflect.New(t.Key()).Elem()
		}
		if holdsRefs(t.Elem()) {
			val = reflect.New(t.Elem()).Elem()
		}
		for it := v.MapRange(); it.Next(); {
			if key.IsValid() {
				key.SetIterKey(it)
				held = w.walk(key, true, held)
			}
			if val.IsValid() {
				val.SetIterValue(it)
				held = w.walk(val, true, held)
			}
		}
	}

	if at.p != nil && !leaf {
		w.leave(at, seenAt)
	}
	return held
}

// freeRefs frees the references resultRefs collected, and keeps their room
// for the next call, unless it is large.
func (m *VM) freeRefs(held []jsValue) {
	for _, v := range held {
		m.jsFreeValue(v)
	}
	if cap(held) <= 64 {
		m.refs = held[:0]
	}
}

// refTypes caches holdsRefs of the types of containers.
var refTypes sync.Map // reflect.Type -> bool

var errorType = reflect.TypeFor[error]()

// holdsRefs reports whether a value of type t can hold a Value, or itself,
// in what the conversion of a result goes through: interfaces, pointers,
// slices, arrays and maps. An error converts through its Error method, and
// a struct through encoding/json, so neither is walked, but for Value.
func holdsRefs(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		return t == rvt
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
	default:
		return false
	}
	if r, ok := refTypes.Load(t); ok {
		return r.(bool)
	}
	r := holdsRefsOf(t, map[reflect.Type]bool{})
	refTypes.Store(t, r)
	return r
}

// holdsRefsOf is holdsRefs. A type on path holds itself, and so can make a
// cycle.
func holdsRefsOf(t reflect.Type, path map[reflect.Type]bool) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		return t == rvt
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
	default:
		return false
	}
	if t.Implements(errorType) {
		return false
	}
	if path[t] {
		return true
	}
	path[t] = true
	if t.Kind() == reflect.Map && holdsRefsOf(t.Key(), path) {
		return true
	}
	return holdsRefsOf(t.Elem(), path)
}

// anySliceType and anyMapType are the types of the most common results that
// hold values of any type, which walkAnySlice and walkAnyMap go through with
// a range of Go, many times faster than one of reflect.
var (
	anySliceType = reflect.TypeFor[[]any]()
	anyMapType   = reflect.TypeFor[map[string]any]()
)

// walkAnySlice is the walk of v, a []any the walk has entered at at, from
// the case of a slice on: it walks only the elements whose type holdsRefs.
func (w *refWalk) walkAnySlice(v reflect.Value, at refPlace, seenAt int, held []jsValue) []jsValue {
	var s []any
	if v.CanAddr() {
		s = *(*[]any)(unsafe.Pointer(v.UnsafeAddr()))
	} else {
		s = v.Interface().([]any)
	}
	for i, x := range s {
		if x != nil && holdsRefs(reflect.TypeOf(x)) {
			held = w.walk(reflect.ValueOf(&s[i]).Elem(), false, held)
		}
	}
	if at.p != nil {
		w.leave(at, seenAt)
	}
	return held
}

// walkAnyMap is walkAnySlice of v, a map[string]any.
func (w *refWalk) walkAnyMap(v reflect.Value, at refPlace, seenAt int, held []jsValue) []jsValue {
	for _, x := range v.Interface().(map[string]any) {
		if x != nil && holdsRefs(reflect.TypeOf(x)) {
			held = w.walk(reflect.ValueOf(x), true, held)
		}
	}
	w.leave(at, seenAt)
	return held
}
