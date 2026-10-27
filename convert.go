// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

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

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
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
	return Value{vm: m, v: lib.XJS_NewObject(m.runtime.tls, m.cContext)}, nil
}

func (m *VM) newString(s string) (r lib.TJSValue, err error) {
	if v, ok := newStringFast(m.runtime.tls, m.cContext, s); ok {
		return v, nil
	}

	return m.newStringEngine(s)
}

// newStringEngine is newString through JS_NewStringLen, for a string longer
// than newStringFast makes.
func (m *VM) newStringEngine(s string) (r lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	s = validUTF8(s)
	p, err := libc.CString(s)
	if err != nil {
		return r, err
	}

	defer libc.Xfree(tls, p)

	return lib.XJS_NewStringLen(tls, ctx, p, lib.Tsize_t(len(s))), nil
}

func jsInt(n int64) lib.TJSValue {
	switch {
	case n >= math.MinInt32 && n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

func jsUint(n uint64) lib.TJSValue {
	switch {
	case n <= math.MaxInt32:
		return newInt32(int32(n))
	default:
		return newFloat(float64(n))
	}
}

var (
	jsonFNC = [...]byte{'J', 'S', 'O', 'N', 0}
	jsonFN  = uintptr(unsafe.Pointer(&jsonFNC[0]))
)

func (m *VM) parseJSON(s string, flags int32) (r lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext

	cs, err := libc.CString(s)
	if err != nil {
		return r, err
	}

	defer libc.Xfree(tls, cs)

	r = lib.XJS_ParseJSON2(tls, ctx, cs, lib.Tsize_t(len(s)), jsonFN, flags)
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
func (m *VM) goString(v lib.TJSValue) (string, error) {
	s, ok := cString(m.runtime.tls, m.cContext, v)
	if !ok {
		return "", m.errFromException()
	}

	return s, nil
}

// errString converts the JS value 'v' to a Go string for ErrorFromValue, which
// has no error result, and which errFromException calls, so it cannot turn a
// failure into an error the way goString does without recursing. When the
// conversion throws, errString drops that new exception and returns "": the
// exception being read is the one to report, and a second one left pending
// would be found by the next JS_GetException of an unrelated call.
func errString(tls *libc.TLS, ctx uintptr, v lib.TJSValue) string {
	s, ok := cString(tls, ctx, v)
	if !ok {
		dropException(tls, ctx)
	}

	return s
}

// dropException frees the exception pending in 'ctx'.
func dropException(tls *libc.TLS, ctx uintptr) {
	lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
}

func (m *VM) newObject(v lib.TJSValue) (r *Object, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	json := lib.XJS_JSONStringify(tls, ctx, v, undefined, undefined)

	defer lib.XFreeValue(tls, ctx, json)

	if isException(json) {
		return nil, m.errFromException()
	}

	s, err := m.goString(json)
	if err != nil {
		return nil, err
	}

	return &Object{json: s}, nil
}

var (
	toStringC = [...]byte{'t', 'o', 'S', 't', 'r', 'i', 'n', 'g', 0}
	toString  = uintptr(unsafe.Pointer(&toStringC[0]))

	// C string property names, read by ErrorFromValue.
	messageC  = [...]byte{'m', 'e', 's', 's', 'a', 'g', 'e', 0}
	nameC     = [...]byte{'n', 'a', 'm', 'e', 0}
	stackC    = [...]byte{'s', 't', 'a', 'c', 'k', 0}
	lineNumC  = [...]byte{'l', 'i', 'n', 'e', 'N', 'u', 'm', 'b', 'e', 'r', 0}
	colNumC   = [...]byte{'c', 'o', 'l', 'u', 'm', 'n', 'N', 'u', 'm', 'b', 'e', 'r', 0}
	fileNameC = [...]byte{'f', 'i', 'l', 'e', 'N', 'a', 'm', 'e', 0}
)

// value "unpacks" 'v'. FreeValue(v) is called before returning, 'v' must not
// be used afterwards.
func (m *VM) value(v lib.TJSValue, free bool) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if free && tag(v) < 0 {
		// all tags with a reference count are negative
		defer lib.XFreeValue(tls, ctx, v)
	}

	switch tag(v) {
	case
		lib.EJS_TAG_BIG_INT,       // -9,
		lib.EJS_TAG_SHORT_BIG_INT: // 7,

		ps := lib.XJS_GetPropertyStr(tls, ctx, v, toString)
		if isException(ps) {
			return nil, m.errFromException()
		}

		defer lib.XFreeValue(tls, ctx, ps)

		if tag(ps) != lib.EJS_TAG_OBJECT {
			return nil, fmt.Errorf("failed to get BigInt.toString()")
		}

		*(*lib.TJSValue)(unsafe.Pointer(m.toStringArgv)) = m.int32_16
		jsString := lib.XJS_Call(tls, ctx, ps, v, 1, m.toStringArgv)
		if isException(jsString) {
			return nil, m.errFromException()
		}

		defer lib.XFreeValue(tls, ctx, jsString)

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
		lib.EJS_TAG_STRING_ROPE, // -6,
		lib.EJS_TAG_STRING:      // -7,

		return m.goString(v)
	case lib.EJS_TAG_OBJECT: // -1
		return m.newObject(v)
	case lib.EJS_TAG_INT: //  0,
		return int(jsvToInt32(v)), nil
	case lib.EJS_TAG_BOOL: //  1,
		return jsvToInt32(v) != 0, nil
	case lib.EJS_TAG_NULL: //  2,
		return nil, nil
	case lib.EJS_TAG_UNDEFINED: //  3,
		return Undefined{}, nil
	case lib.EJS_TAG_EXCEPTION: // 6,
		return nil, m.errFromException()
		panic(todo(""))
	case lib.EJS_TAG_FLOAT64: // 8,
		return jsvToFloat64(v), nil
	}
	return Unsupported{}, nil
}

func (m *VM) jsValue(in reflect.Value) (out lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	defer func() {
		if isException(out) && err == nil {
			err = m.errFromException()
		}
	}()

	switch x := in.Interface().(type) {
	case nil:
		return null, nil
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

		p, err := libc.CString(string(b))
		if err != nil {
			return out, err
		}

		defer libc.Xfree(tls, p)

		v := lib.XJS_ParseJSON(tls, ctx, p, libc.Tsize_t(len(b)), uintptr(unsafe.Pointer(&evalFN)))
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

func (m *VM) jsInterfaceValue(in reflect.Value) (out lib.TJSValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	return m.jsValue(in.Elem())
}

func (m *VM) jsPtrValue(in reflect.Value) (out lib.TJSValue, err error) {
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
func (m *VM) jsMap(in reflect.Value) (out lib.TJSValue, err error) {
	if in.IsNil() {
		return null, nil
	}

	// Javascript property names are strings. Integer keys stringify the same way
	// they do in JSON and the same way obj[1] and obj["1"] are the same property
	// in Javascript. Collect and validate them before creating the object so a
	// bad key needs no cleanup.
	type kv struct {
		key string
		val reflect.Value
	}
	var a []kv
	for _, k := range in.MapKeys() {
		var key string
		switch k.Kind() {
		case reflect.String:
			key = k.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			key = strconv.FormatInt(k.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			key = strconv.FormatUint(k.Uint(), 10)
		default:
			return out, fmt.Errorf("unsupported map key type: %s", k.Type())
		}

		a = append(a, kv{key, in.MapIndex(k)})
	}

	// Javascript property order is observable, via Object.keys, JSON.stringify or
	// for-in, while Go map iteration order is randomized. Sort so the same map
	// always produces the same object, which is what encoding/json does too.
	sort.Slice(a, func(i, j int) bool { return a[i].key < a[j].key })

	tls := m.runtime.tls
	ctx := m.cContext
	obj := lib.XJS_NewObject(tls, ctx)
	if isException(obj) {
		return out, m.errFromException()
	}

	for _, kv := range a {
		// Like jsArray, a Value is passed on as is rather than converted, and the
		// reference it holds is the one handed over below.
		var jv lib.TJSValue
		switch x := kv.val.Interface().(type) {
		case Value:
			jv = x.v
		default:
			if jv, err = m.jsValue(kv.val); err != nil {
				lib.XFreeValue(tls, ctx, obj)
				return out, err
			}
		}

		p, err2 := libc.CString(validUTF8(kv.key))
		if err2 != nil {
			lib.XFreeValue(tls, ctx, jv)
			lib.XFreeValue(tls, ctx, obj)
			return out, err2
		}

		// JS_SetPropertyStr takes over the reference to jv, it does not take over
		// the property name.
		rc := lib.XJS_SetPropertyStr(tls, ctx, obj, p, jv)
		libc.Xfree(tls, p)
		if rc < 0 {
			lib.XFreeValue(tls, ctx, obj)
			return out, m.errFromException()
		}
	}

	return obj, nil
}

func (m *VM) jsArray(a []reflect.Value) (out lib.TJSValue, err error) {
	var s []lib.TJSValue
	for _, v := range a {
		switch x := v.Interface().(type) {
		case Value:
			s = append(s, x.v)
		default:
			jv, err := m.jsValue(v)
			if err != nil {
				return out, err
			}

			s = append(s, jv)
		}
	}
	return m.jsArrayOf(s)
}

func (m *VM) jsArrayOf(a []lib.TJSValue) (out lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	argv := libc.Xmalloc(tls, lib.Tsize_t(len(a))*lib.Tsize_t(sizeofJsValue))

	defer libc.Xfree(tls, argv)

	p := argv
	for _, v := range a {
		*(*lib.TJSValue)(unsafe.Pointer(p)) = v
		p += sizeofJsValue
	}
	out = lib.Xjs_array_of(tls, ctx, undefined, int32(len(a)), argv)
	if isException(out) {
		err = m.errFromException()
	}
	for _, v := range a {
		lib.XFreeValue(tls, ctx, v)
	}
	return out, err
}
