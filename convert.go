// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Converting javascript values to Go values, the leaves of the results of Go
// functions to javascript values, and Go strings on their way to the engine.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// value returns the Go value of v (see [VM.Eval]), and frees v when free is
// set, whether it converts or not. An object and a BigInt run javascript to
// convert: a caller is inside a call (enter).
func (m *VM) value(v jsValue, free bool) (any, error) {
	switch tag(v) {
	case tagException:
		return nil, m.errFromException()
	case tagNull:
		return nil, nil
	case tagUndefined:
		return Undefined{}, nil
	case tagInt:
		return int(jsvToInt32(v)), nil
	case tagBool:
		return jsvToInt32(v) != 0, nil
	case tagFloat64:
		return jsvToFloat64(v), nil
	}
	if free {
		// Deferred: converting an object or a BigInt runs javascript, and a
		// Go function it calls may panic through here.
		defer m.jsFreeValue(v)
	}
	var r any
	var err error
	switch tag(v) {
	case tagString, tagStringRope:
		var s string
		if s, err = m.goString(v); err == nil {
			r = s
		}
	case tagBigInt, tagShortBigInt:
		var n *big.Int
		if n, err = m.bigInt(v); err == nil {
			r = n
		}
	case tagObject:
		var o *Object
		if o, err = m.objectJSON(v); err == nil {
			r = o
		}
	default:
		r = Unsupported{}
	}
	return r, err
}

// bigInt returns the Go value of the BigInt v: its toString(16), which a page
// may have replaced, read in base 16.
func (m *VM) bigInt(v jsValue) (*big.Int, error) {
	f := m.settled(m.jsGetProperty(v, atomToString))
	if isException(f) {
		return nil, m.errFromException()
	}
	defer m.jsFreeValue(f)
	if tag(f) != tagObject {
		return nil, errors.New("failed to get BigInt.toString()")
	}
	s, err := m.jsCall(f, v, []jsValue{newInt32(16)})
	if err != nil {
		return nil, err
	}
	if s = m.settled(s); isException(s) {
		return nil, m.errFromException()
	}
	// A replaced toString may return an object, whose string runs
	// javascript.
	defer m.jsFreeValue(s)
	text, err := m.goString(s)
	if err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(text, 16)
	if !ok {
		return nil, errors.New("big.Int.SetString failed")
	}
	return n, nil
}

// objectJSON returns the Object of v, its JSON.
func (m *VM) objectJSON(v jsValue) (*Object, error) {
	j := m.settled(m.jsJSONStringify(v))
	if isException(j) {
		return nil, m.errFromException()
	}
	defer m.jsFreeValue(j)
	s, err := m.goString(j)
	if err != nil {
		return nil, err
	}
	return &Object{json: s}, nil
}

// The types the conversion of a leaf looks for.
var (
	rvt           = reflect.TypeFor[Value]()
	undefinedType = reflect.TypeFor[Undefined]()
	objectPtrType = reflect.TypeFor[*Object]()
	bigIntPtrType = reflect.TypeFor[*big.Int]()
)

// jsValue converts in, a leaf of the results of a Go function (result.go),
// one that holds no Value, to a new javascript value:
//
//   - nil, a nil pointer, interface or map: null. A nil slice: [].
//   - Undefined: undefined. A value whose type has an Error method, a nil
//     one too: the string of Error.
//   - *Object: its JSON parsed. *big.Int: the BigInt. Another pointer: the
//     value it points to, unless that is a pointer too.
//   - A bool, an integer and a float: a boolean and a number, an integer
//     outside int32 a float. A string: the string.
//   - A struct: its JSON of encoding/json parsed.
//   - A slice: an array of its elements. A map with string or integer keys:
//     an object of own data properties in the order of the keys as strings.
//   - Anything else, an array, a channel, a function, a complex or an
//     uintptr, is an error.
func (m *VM) jsValue(in reflect.Value) (jsValue, error) {
	r, err := m.leafValue(in)
	if err == nil && isException(r) {
		err = m.errFromException()
	}
	return r, err
}

func (m *VM) leafValue(in reflect.Value) (jsValue, error) {
	if !in.IsValid() {
		return null, nil
	}
	if in.Kind() == reflect.Interface {
		if in.IsNil() {
			return null, nil
		}
		in = in.Elem()
	}
	t := in.Type()
	switch {
	case t == rvt:
		return exception, errors.New("internal error: a Value outside the snapshot of a result")
	case t == undefinedType:
		return undefined, nil
	case t.Implements(errorType):
		return m.newString(in.Interface().(error).Error())
	}
	switch k := in.Kind(); k {
	case reflect.Pointer:
		switch {
		case in.IsNil():
			return null, nil
		case t == objectPtrType:
			return m.jsParseJSON2(in.Interface().(*Object).json, true, 0)
		case t == bigIntPtrType:
			return m.bigIntValue(in.Interface().(*big.Int))
		}
		elem := in.Elem()
		if elem.Kind() == reflect.Pointer {
			return exception, fmt.Errorf("type not supported: %T", in.Interface())
		}
		return m.leafValue(elem)
	case reflect.Bool:
		return newBool(in.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intValue(in.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uintValue(in.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return newFloat(in.Float()), nil
	case reflect.String:
		return m.newString(in.String())
	case reflect.Struct:
		b, err := json.Marshal(in.Interface())
		if err != nil {
			return exception, err
		}
		return m.jsParseJSON2(string(b), false, 0)
	case reflect.Map:
		return m.jsMap(in)
	case reflect.Slice:
		return m.jsArray(in.Len(), in.Index)
	default:
		return exception, fmt.Errorf("internal error: %v", k)
	}
}

// jsArray returns an array of the n values that at gives, converted. Each
// element counts toward the stop of the call (elemStop).
func (m *VM) jsArray(n int, at func(int) reflect.Value) (jsValue, error) {
	if n == 0 {
		return m.jsNewArray(), nil
	}
	vals := make([]jsValue, 0, n)
	defer func() { m.freeAll(vals) }()
	for i := range n {
		if m.elemStop() {
			return exception, m.errFromException()
		}
		v, err := m.jsValue(at(i))
		if err != nil {
			return exception, err
		}
		vals = append(vals, v)
	}
	return m.jsArrayOfValues(vals)
}

// mapEntry is a property of the object of a map, before it is defined.
type mapEntry struct {
	key string
	val reflect.Value
}

// jsMap returns an object of the entries of the map in, each an own data
// property, in the order of the keys as strings: the order of the properties
// of an object can be seen, and the order of a Go map changes from one range
// to the next. A key is defined, not assigned, with all its bytes: no setter
// runs, and __proto__ is a key like another.
func (m *VM) jsMap(in reflect.Value) (jsValue, error) {
	if in.IsNil() {
		return null, nil
	}
	key := mapKey(in.Type().Key())
	if key == nil {
		if in.Len() == 0 {
			// No key needs a name.
			return m.jsNewObject(), nil
		}
		return exception, fmt.Errorf("unsupported map key type: %s", in.Type().Key())
	}
	entries := make([]mapEntry, 0, in.Len())
	for it := in.MapRange(); it.Next(); {
		entries = append(entries, mapEntry{key(it.Key()), it.Value()})
	}
	slices.SortFunc(entries, func(a, b mapEntry) int { return strings.Compare(a.key, b.key) })

	obj := m.jsNewObject()
	if isException(obj) {
		return exception, nil
	}
	done := false
	defer func() {
		if !done {
			m.jsFreeValue(obj)
		}
	}()
	for _, e := range entries {
		if m.elemStop() {
			return exception, m.errFromException()
		}
		v, err := m.jsValue(e.val)
		if err != nil {
			return exception, err
		}
		atom, err := m.atomOf(e.key)
		if err != nil {
			m.jsFreeValue(v)
			return exception, err
		}
		// It takes v, also when it fails.
		rc := m.jsDefinePropertyValue(obj, atom, v, propCWE|propThrow)
		m.jsFreeAtom(atom)
		if rc < 0 {
			return exception, m.errFromException()
		}
	}
	done = true
	return obj, nil
}

// validUTF8 returns s when it is valid UTF-8, and else s with each byte that
// starts no valid sequence, as a range over s reads it, made U+FFFD.
func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	b := make([]byte, 0, len(s)+len(s)/2)
	for _, c := range s {
		b = utf8.AppendRune(b, c)
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// wellFormed returns the string of b, UTF-8 the engine wrote, with each lone
// surrogate made U+FFFD. The engine writes a lone surrogate as the three
// bytes of its code point, 0xED then 0xA0 to 0xBF then one more, which is
// not UTF-8. U+FFFD takes three bytes too, so the string keeps its length.
func wellFormed(b []byte) string {
	var out []byte
	for i := 0; i < len(b); {
		j := bytes.IndexByte(b[i:], 0xED)
		if j < 0 {
			break
		}
		i += j
		if i+2 >= len(b) || b[i+1] < 0xA0 {
			i++
			continue
		}
		if out == nil {
			out = make([]byte, len(b))
			copy(out, b)
		}
		out[i], out[i+1], out[i+2] = 0xEF, 0xBF, 0xBD
		i += 3
	}
	if out == nil {
		return string(b)
	}
	return unsafe.String(unsafe.SliceData(out), len(out))
}
