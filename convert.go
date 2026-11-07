// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"unicode/utf8"
	"unsafe"
)

// NewInt returns a new Value from 'n'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewInt(n int) Value {
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		return Value{vm: m, v: newInt32(int32(n))}
	}

	return m.NewFloat64(float64(n))
}

// NewFloat64 returns a new Value from 'n'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewFloat64(n float64) Value {
	return Value{vm: m, v: newFloat(n)}
}

// NewString returns a new Value from 's'.
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) NewString(s string) (r Value, err error) {
	v, err := m.newString(s)
	if err != nil {
		return r, err
	}

	return Value{vm: m, v: v}, nil
}

// NewObjectValue returns a new Value representing '{}'.
func (m *VM) NewObjectValue() (r Value, err error) {
	if err := m.usable(); err != nil {
		return r, err
	}
	v := m.jsNewObject()
	if isException(v) {
		return r, m.errFromException()
	}
	return Value{vm: m, v: v}, nil
}

// newString makes the string of NewString, and of the Go strings of the
// other methods. It refuses a VM a kill left good only for Close here, not in
// NewString, which stays small enough to inline (Builtins return strings).
// When the engine has no memory for the string, the error is its exception.
func (m *VM) newString(s string) (r jsValue, err error) {
	if err := m.usable(); err != nil {
		return r, err
	}
	r, ok := m.newStringFast(s)
	if !ok {
		if r, err = m.newStringEngine(s); err != nil {
			return r, err
		}
	}
	if isException(r) {
		return r, m.errFromException()
	}
	return r, nil
}

// newStringEngine is newString through JS_NewStringLen, for a string longer
// than newStringFast makes.
func (m *VM) newStringEngine(s string) (r jsValue, err error) {
	return m.jsNewStringLen(validUTF8(s))
}

func jsInt(n int64) jsValue {
	switch {
	case n >= math.MinInt32 && n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

func jsUint(n uint64) jsValue {
	switch {
	case n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

func (m *VM) parseJSON(s string, flags int32) (r jsValue, err error) {
	r, err = m.jsParseJSON2(s, true, flags)
	if err != nil {
		return r, err
	}

	if isException(r) {
		err = m.errFromException()
	}
	return r, err
}

// wellFormed returns the UTF-8 b as a Go string with U+FFFD for each lone
// surrogate, as Go's utf16.Decode, TextEncoder and Node's Buffer make a string
// of UTF-16 valid. JS_ToCStringLen2 joins the halves of a surrogate pair into
// one character, but writes a lone surrogate as the three bytes of its code
// point, 0xED then 0xA0 to 0xBF, which is not UTF-8. They are the only bytes
// it writes in that form, and U+FFFD takes three bytes too.
func wellFormed(b []byte) string {
	var fixed []byte
	for i := 0; i < len(b); {
		j := bytes.IndexByte(b[i:], 0xED)
		if j < 0 {
			break
		}
		i += j
		if i+2 < len(b) && b[i+1] >= 0xA0 {
			if fixed == nil {
				fixed = bytes.Clone(b)
			}
			copy(fixed[i:], "\uFFFD")
		}
		i++
	}
	if fixed == nil {
		return string(b)
	}
	return unsafe.String(unsafe.SliceData(fixed), len(fixed))
}

// validUTF8 returns s with U+FFFD for each byte that does not start a valid
// UTF-8 sequence, the Go -> JS counterpart of wellFormed. That is how Go
// itself decodes a string (for range, []rune, utf16.Encode) and how
// encoding/json writes one, so a Go string reaches javascript the same way
// whether it is passed as it is or inside a struct, which goes as JSON. The
// engine's JS_NewStringLen skipped the bytes after an invalid one, ASCII
// included ("\x80a" became a lone U+FFFD), and took the three bytes of a
// surrogate (0xED 0xA0 to 0xBF) as that surrogate. A sequence cut short
// ("\xe4\xb8") gives one U+FFFD per byte here, where a browser's TextDecoder
// gives one for the sequence.
func validUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	b := make([]byte, 0, len(s)+2*utf8.UTFMax)
	for _, r := range s {
		b = utf8.AppendRune(b, r) // an invalid byte comes as utf8.RuneError
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// goString converts the JS value 'v' to a Go string. When the conversion
// throws, it takes the exception out of the context and returns it as the
// error, for example the engine's out of memory error.
func (m *VM) goString(v jsValue) (string, error) {
	s, ok := m.argString(v, false)
	if !ok {
		return "", m.errFromException()
	}

	return s, nil
}

// errString converts the JS value 'v' to a Go string for readError, which
// errFromException calls, so it cannot turn a failure into an error the way
// goString does without recursing. When the conversion throws what the
// script throws, errString drops that new exception and returns "": the
// exception being read is the one to report, and a second one left pending
// would be found by the next JS_GetException of an unrelated call. ok is
// false when the conversion threw an uncatchable exception, which stays
// pending (dropCatchable).
func (m *VM) errString(v jsValue) (s string, ok bool) {
	s, ok = m.argString(v, false)
	if !ok {
		return "", m.dropCatchable()
	}

	return s, true
}

// dropCatchable drops the exception that a read of an error threw, and
// reports true, when the script threw it. An uncatchable one (an interrupt,
// the eval timeout, running out of memory, a panic in a Go function) ends
// the call whatever reads what: it stays pending, and dropCatchable reports
// false.
func (m *VM) dropCatchable() bool {
	if m.uncatchablePending() {
		return false
	}
	m.dropException()
	return true
}

// dropException frees the exception pending in m's context.
func (m *VM) dropException() {
	m.jsFreeValue(m.jsGetException())
}

func (m *VM) newObject(v jsValue) (r *Object, err error) {
	json := m.jsJSONStringify(v)

	defer m.jsFreeValue(json)

	if isException(json) {
		return nil, m.errFromException()
	}

	s, err := m.goString(json)
	if err != nil {
		return nil, err
	}

	return &Object{json: s}, nil
}

// value "unpacks" 'v'. FreeValue(v) is called before returning, 'v' must not
// be used afterwards.
func (m *VM) value(v jsValue, free bool) (r any, err error) {
	if free && tag(v) < 0 {
		// all tags with a reference count are negative
		defer m.jsFreeValue(v)
	}

	switch tag(v) {
	case
		tagBigInt,      // -9,
		tagShortBigInt: // 7,

		ps := m.jsGetProperty(v, atomToString)
		if isException(ps) {
			return nil, m.errFromException()
		}

		defer m.jsFreeValue(ps)

		if tag(ps) != tagObject {
			return nil, fmt.Errorf("failed to get BigInt.toString()")
		}

		jsString, err := m.jsCall(ps, v, []jsValue{newInt32(16)})
		if err != nil {
			return nil, err
		}
		if isException(jsString) {
			return nil, m.errFromException()
		}

		defer m.jsFreeValue(jsString)

		s, err := m.goString(jsString)
		if err != nil {
			return nil, err
		}

		n := big.NewInt(0)
		if _, ok := n.SetString(s, 16); !ok {
			return nil, fmt.Errorf("big.Int.SetString failed")
		}

		return n, nil
	case
		tagStringRope, // -6,
		tagString:     // -7,

		return m.goString(v)
	case tagObject: // -1
		return m.newObject(v)
	case tagInt: //  0,
		return int(jsvToInt32(v)), nil
	case tagBool: //  1,
		return jsvToInt32(v) != 0, nil
	case tagNull: //  2,
		return nil, nil
	case tagUndefined: //  3,
		return Undefined{}, nil
	case tagException: // 6,
		return nil, m.errFromException()
	case tagFloat64: // 8,
		return jsvToFloat64(v), nil
	}
	return Unsupported{}, nil
}

func (m *VM) jsValue(in reflect.Value) (out jsValue, err error) {
	defer func() {
		if isException(out) && err == nil {
			err = m.errFromException()
		}
	}()

	// A Value of m is taken as it is, without a copy: the conversion of a
	// result takes it. One of another VM stays its owner's.
	switch x := in.Interface().(type) {
	case nil:
		return null, nil
	case Value:
		if m.foreign(x) {
			return out, errForeignValue
		}
		return x.jsValue(), nil
	case Undefined:
		return undefined, nil
	case error:
		return m.newString(x.Error())
	}

	switch in.Kind() {
	case reflect.Pointer:
		return m.jsPtrValue(in)
	case reflect.Bool:
		return newBool(in.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return jsInt(in.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return jsUint(in.Uint()), nil
	case reflect.Float64, reflect.Float32:
		return newFloat(in.Float()), nil
	case reflect.String:
		return m.newString(in.String())
	case reflect.Struct:
		b, err := json.Marshal(in.Interface())
		if err != nil {
			return out, err
		}

		v, err := m.jsParseJSON2(string(b), false, 0)
		if err != nil {
			return out, err
		}

		if isException(v) {
			return out, m.errFromException()
		}

		return v, nil
	case reflect.Interface:
		return m.jsInterfaceValue(in)
	case reflect.Map:
		return m.jsMap(in)
	case reflect.Slice:
		var a []reflect.Value
		for i := 0; i < in.Len(); i++ {
			a = append(a, in.Index(i))
		}
		return m.jsArray(a)
	default:
		return out, fmt.Errorf("internal error: %v (%v:)", in.Kind(), origin(1))
	}
}

func (m *VM) jsInterfaceValue(in reflect.Value) (out jsValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	return m.jsValue(in.Elem())
}

func (m *VM) jsPtrValue(in reflect.Value) (out jsValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	switch x := in.Interface().(type) {
	case *Object:
		return m.parseJSON(x.json, 0)
	case *big.Int:
		s := x.String() + "n"
		return m.eval(s, EvalGlobal)
	default:
		ev := in.Elem()
		if ev.Kind() == reflect.Pointer { // avoid unbound recursion
			return out, fmt.Errorf("type not supported: %T", x)
		}

		return m.jsValue(ev)
	}
}

// jsMap converts a Go map to a Javascript object. Like jsArray, and unlike the
// JSON round trip used for structs, it converts every value with jsValue, so a
// Value stays a live reference and a *big.Int stays a BigInt.
func (m *VM) jsMap(in reflect.Value) (out jsValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	// Javascript property names are strings. Integer keys stringify the same way
	// they do in JSON and the same way obj[1] and obj["1"] are the same property
	// in Javascript. Check the type of the keys before creating the object so a
	// bad key needs no cleanup but the Values of the map.
	var key func(k reflect.Value) string
	switch in.Type().Key().Kind() {
	case reflect.String:
		key = reflect.Value.String
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		key = func(k reflect.Value) string { return strconv.FormatInt(k.Int(), 10) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		key = func(k reflect.Value) string { return strconv.FormatUint(k.Uint(), 10) }
	default:
		err = fmt.Errorf("unsupported map key type: %s", in.Type().Key())
	}
	type kv struct {
		key string
		val reflect.Value
	}
	a := make([]kv, 0, in.Len())
	for it := in.MapRange(); it.Next(); {
		if err != nil {
			m.freeResult(it.Value())
			continue
		}
		a = append(a, kv{key(it.Key()), it.Value()})
	}
	if err != nil {
		return out, err
	}

	// Javascript property order is observable, via Object.keys, JSON.stringify or
	// for-in, while Go map iteration order is randomized. Sort so the same map
	// always produces the same object, which is what encoding/json does too.
	sort.Slice(a, func(i, j int) bool { return a[i].key < a[j].key })

	// The object takes the Values of the map, also when it cannot be made:
	// what is not in it yet is freed then.
	fail := func(i int, err error) (jsValue, error) {
		for _, kv := range a[i:] {
			m.freeResult(kv.val)
		}
		return out, err
	}
	obj := m.jsNewObject()
	if isException(obj) {
		return fail(0, m.errFromException())
	}

	for i, kv := range a {
		jv, err := m.jsValue(kv.val)
		if err != nil {
			m.jsFreeValue(obj)
			return fail(i+1, err)
		}

		// The key is an atom of all its bytes, NULs too, and an own data
		// property as in an object literal: no setter runs, and __proto__ is
		// a key. JS_DefinePropertyValue takes jv, also when it fails.
		atom, err := m.jsNewAtom(validUTF8(kv.key))
		if err == nil && atom == atomNull {
			err = m.errFromException()
		}
		if err != nil {
			m.jsFreeValue(jv)
			m.jsFreeValue(obj)
			return fail(i+1, err)
		}
		rc := m.jsDefinePropertyValue(obj, atom, jv, propCWE|propThrow)
		m.jsFreeAtom(atom)
		if rc < 0 {
			m.jsFreeValue(obj)
			return fail(i+1, m.errFromException())
		}
	}

	return obj, nil
}

// freeResult frees the Values of m in v, a result or a part of one that is
// not converted after another failed: what jsValue would have taken, in
// the slices, maps, interfaces and pointers it goes through. A Value of
// another VM is its owner's.
func (m *VM) freeResult(v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			m.freeResult(v.Elem())
		}
	case reflect.Pointer:
		// jsPtrValue converts no pointer to a pointer.
		if !v.IsNil() && v.Elem().Kind() != reflect.Pointer {
			m.freeResult(v.Elem())
		}
	case reflect.Slice:
		if mayHoldValue(v.Type().Elem()) {
			for i := range v.Len() {
				m.freeResult(v.Index(i))
			}
		}
	case reflect.Map:
		if mayHoldValue(v.Type().Elem()) {
			for it := v.MapRange(); it.Next(); {
				m.freeResult(it.Value())
			}
		}
	case reflect.Struct:
		if v.Type() == rvt {
			if x := v.Interface().(Value); !m.foreign(x) {
				m.jsFreeValue(x.v)
			}
		}
	}
}

// mayHoldValue reports whether an element of type t can hold a Value that
// jsValue takes, so that freeResult skips the elements of a []byte.
func mayHoldValue(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Slice, reflect.Map:
		return true
	case reflect.Struct:
		return t == rvt
	}
	return false
}

// jsArray converts the elements a to a javascript array. The array takes the
// Values among them, also when it cannot be made: then it frees what it
// converted and the Values it was handed.
func (m *VM) jsArray(a []reflect.Value) (out jsValue, err error) {
	s := make([]jsValue, 0, len(a))
	for i, v := range a {
		jv, err := m.jsValue(v)
		if err != nil {
			for _, v := range s {
				m.jsFreeValue(v)
			}
			for _, v := range a[i+1:] {
				m.freeResult(v)
			}
			return out, err
		}
		s = append(s, jv)
	}
	return m.jsArrayOf(s)
}

func (m *VM) jsArrayOf(a []jsValue) (out jsValue, err error) {
	out, err = m.jsArrayOfValues(a)
	if err == nil && isException(out) {
		err = m.errFromException()
	}
	for _, v := range a {
		m.jsFreeValue(v)
	}
	return out, err
}
