// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// Call evaluates 'function(args...)' and returns the resulting (value, error).
//
// Argument types must be one of:
//
//	Go argument type                        Javascript argument type
//	----------------------------------------------------------------
//	nil                                     null
//	Undefined                               undefined
//	string                                  string
//	int*/uint* (value in int32 range)       int
//	int*/uint* (value out of int32 range)   float
//	bool                                    bool
//	float64                                 float64
//	*math/big.Int                           BigInt
//	*Object                                 object
//	Value                                   native Javascript Value
//	any other type                          object from JSON produced by encoding.json/Marshall(arg)
func (m *VM) Call(function string, args ...any) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(function)
	if err != nil {
		return nil, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	// Re-arm the eval-timeout deadline, as the Eval* functions do: Call runs JS (it
	// evaluates the function reference and invokes it), so without this the
	// deadline set by the last Eval keeps ticking through idle time and interrupts a
	// later Call that has not itself run long.
	m.enter()
	defer m.leave()

	f := m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(function)), uintptr(unsafe.Pointer(&evalFN)), int32(EvalGlobal)))

	if isException(f) {
		// Evaluating the function expression threw, eg. out of memory or an
		// unresolvable name. Report that exception, not a non-function.
		return nil, m.errFromException()
	}

	defer lib.XFreeValue(tls, ctx, f)

	g := m.globalObject()

	defer lib.XFreeValue(tls, ctx, g)

	return m.call(f, g, args...)
}

// CallValue is like Call but returns (Value, error) like EvalValue
//
// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) CallValue(function string, args ...any) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	ps, err := libc.CString(function)
	if err != nil {
		return r, fmt.Errorf("OOM")
	}

	defer libc.Xfree(tls, ps)

	// Re-arm the eval-timeout deadline, as the Eval* functions do (see Call).
	m.enter()
	defer m.leave()

	f := m.settled(lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(function)), uintptr(unsafe.Pointer(&evalFN)), int32(EvalGlobal)))

	if isException(f) {
		return r, m.errFromException()
	}

	defer lib.XFreeValue(tls, ctx, f)

	g := m.globalObject()

	defer lib.XFreeValue(tls, ctx, g)

	return m.callValue(f, g, args...)
}

func (m *VM) convertArgs(goArgs ...any) (jsArgs, free []lib.TJSValue, err error) {
	tls := m.runtime.tls
	ctx := m.cContext

	defer func() {
		if err != nil && len(free) != 0 {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
			free = nil
		}
	}()

	for _, v := range goArgs {
		switch x := v.(type) {
		case nil:
			jsArgs = append(jsArgs, null)
		case Value:
			if x.vm != m && tag(x.v) < 0 {
				return nil, free, fmt.Errorf("cannot use a Value from a different VM")
			}

			dup := x.Dup().v
			free = append(free, dup)
			jsArgs = append(jsArgs, dup)
		case Undefined:
			jsArgs = append(jsArgs, undefined)
		case int8:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint8:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case int16:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint16:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case int32:
			jsArgs = append(jsArgs, newInt32(int32(x)))
		case uint32:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case int:
			switch {
			case x >= math.MinInt32 && x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case uint:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case int64:
			switch {
			case x >= math.MinInt32 && x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case uint64:
			switch {
			case x <= math.MaxInt32:
				jsArgs = append(jsArgs, newInt32(int32(x)))
			default:
				jsArgs = append(jsArgs, newFloat(float64(x)))
			}
		case bool:
			jsArgs = append(jsArgs, newBool(x))
		case string:
			jv, err := m.newString(x)
			if err != nil {
				return nil, free, err
			}

			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		case *big.Int:
			s := x.String() + "n"
			jv, err := m.eval(s, EvalGlobal)
			if err != nil {
				return nil, free, err
			}

			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		default:
			b, err := json.Marshal(x)
			if err != nil {
				return nil, free, err
			}

			p, err := libc.CString(string(b))
			if err != nil {
				return nil, free, err
			}

			defer libc.Xfree(tls, p)

			jv := lib.XJS_ParseJSON(tls, ctx, p, libc.Tsize_t(len(b)), uintptr(unsafe.Pointer(&evalFN)))
			free = append(free, jv)
			jsArgs = append(jsArgs, jv)
		}
	}
	return jsArgs, free, nil
}

// Note: See the [Value] documentation for details about manual memory
// management of Value objects.
func (m *VM) callValue(f, this lib.TJSValue, args ...any) (r Value, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if lib.XJS_IsFunction(tls, ctx, f) == 0 {
		return r, fmt.Errorf("cannot call a non-function")
	}

	jsArgs, free, err := m.convertArgs(args...)
	if err != nil {
		return r, err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}
	var argv uintptr
	if len(jsArgs) != 0 {
		sz := libc.Tsize_t(sizeofJsValue * uintptr(len(jsArgs)))
		argv = libc.Xmalloc(tls, sz)

		defer libc.Xfree(tls, argv)

		for i, v := range jsArgs {
			unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(argv)), len(jsArgs))[i] = v
		}

	}
	v := m.settled(lib.XJS_Call(tls, ctx, f, this, int32(len(jsArgs)), argv))
	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return r, m.errFromException()
	}

	return Value{vm: m, v: v}, nil
}

func (m *VM) call(f, this lib.TJSValue, args ...any) (r any, err error) {
	tls := m.runtime.tls
	ctx := m.cContext
	if lib.XJS_IsFunction(tls, ctx, f) == 0 {
		return nil, fmt.Errorf("cannot call a non-function")
	}

	jsArgs, free, err := m.convertArgs(args...)
	if err != nil {
		return nil, err
	}

	if len(free) != 0 {
		defer func() {
			for _, v := range free {
				lib.XFreeValue(tls, ctx, v)
			}
		}()
	}
	var argv uintptr
	if len(jsArgs) != 0 {
		sz := libc.Tsize_t(sizeofJsValue * uintptr(len(jsArgs)))
		argv = libc.Xmalloc(tls, sz)

		defer libc.Xfree(tls, argv)

		for i, v := range jsArgs {
			unsafe.Slice((*lib.TJSValue)(unsafe.Pointer(argv)), len(jsArgs))[i] = v
		}

	}
	return m.value(m.settled(lib.XJS_Call(tls, ctx, f, this, int32(len(jsArgs)), argv)), true)
}
