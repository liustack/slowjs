// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

// Calling javascript functions from Go, and the arguments of such a call.

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
)

// errNotFunction is the error of a call of a value that is not a function.
var errNotFunction = errors.New("cannot call a non-function")

// Call evaluates function, source that gives a function, such as "parseInt"
// or "obj.method", calls it with args and returns its value as [VM.Eval]
// does. this is the global object, also for "obj.method". function is run as
// a script: "var a = 42; a" runs, and the call fails as 42 is not a function.
// What function throws is the error, such as a ReferenceError for a name that
// is not defined.
//
// Each argument becomes a javascript value by its dynamic Go type:
//
//	Go                                         javascript
//	--                                         ----------
//	nil                                        null
//	Value                                      the value, borrowed
//	Undefined                                  undefined
//	int8, uint8, int16, uint16, int32          a number
//	int, int64, uint, uint32, uint64           a number, a float when
//	                                           outside int32 (past 2^53
//	                                           it loses precision)
//	float64                                    a number
//	bool                                       a boolean
//	string                                     a string, U+FFFD for each
//	                                           byte of invalid UTF-8
//	*big.Int                                   a BigInt, null for nil
//	anything else                              the JSON of encoding/json
//
// A type of its own, such as time.Duration, and float32, []byte (base64),
// structs, maps and *Object go through JSON. So does an error, which most
// often has no exported field and becomes {}: pass err.Error() instead. A
// Value of another VM is an error.
func (m *VM) Call(function string, args ...any) (r any, err error) {
	if err = m.enter(); err != nil {
		return nil, err
	}
	defer m.leave(&err)
	f, g, err := m.evalFunction(function)
	if err != nil {
		return nil, err
	}
	defer m.jsFreeValue(f)
	defer m.jsFreeValue(g)
	return m.call(f, g, args)
}

// CallValue is [VM.Call] that returns the value as a [Value], which the
// caller frees.
func (m *VM) CallValue(function string, args ...any) (r Value, err error) {
	if err = m.enter(); err != nil {
		return r, err
	}
	defer m.leave(&err)
	f, g, err := m.evalFunction(function)
	if err != nil {
		return r, err
	}
	defer m.jsFreeValue(f)
	defer m.jsFreeValue(g)
	return m.callValue(f, g, args)
}

// evalFunction evaluates the source of a function for Call, and returns the
// value and the global object, for the caller to free.
func (m *VM) evalFunction(function string) (f, g jsValue, err error) {
	f, err = m.jsEval(evalName, function, EvalGlobal)
	if err != nil {
		return f, g, err
	}
	if f = m.settled(f); isException(f) {
		return f, g, m.errFromException()
	}
	return f, m.jsGetGlobalObject(), nil
}

// call calls f with this and args, as Call does, and returns its value as a
// Go value. It runs inside a call (enter).
func (m *VM) call(f, this jsValue, args []any) (any, error) {
	v, err := m.invoke(f, this, args)
	if err != nil {
		return nil, err
	}
	return m.value(v, true)
}

// callValue is call that returns a Value.
func (m *VM) callValue(f, this jsValue, args []any) (Value, error) {
	v, err := m.invoke(f, this, args)
	if err != nil {
		return Value{}, err
	}
	if isException(v) {
		return Value{}, m.errFromException()
	}
	return Value{vm: m, v: v}, nil
}

// invoke calls f, which it borrows, with this and args converted, and
// returns its settled result, the exception marker when it threw.
func (m *VM) invoke(f, this jsValue, args []any) (jsValue, error) {
	if !m.jsIsFunction(f) {
		return exception, errNotFunction
	}
	vals, err := m.callArgs(args)
	if err != nil {
		return exception, err
	}
	defer m.freeAll(vals)
	r, err := m.jsCall(f, this, vals)
	if err != nil {
		return exception, err
	}
	return m.settled(r), nil
}

// freeAll frees each of vals.
func (m *VM) freeAll(vals []jsValue) {
	for _, v := range vals {
		m.jsFreeValue(v)
	}
}

// callArgs converts the arguments of a call (see VM.Call). The values are new
// or Dups, for the caller to free once the call is done. When one fails to
// convert, or panics, as a MarshalJSON may, those before it are freed.
func (m *VM) callArgs(args []any) ([]jsValue, error) {
	if len(args) == 0 {
		return nil, nil
	}
	vals := make([]jsValue, 0, len(args))
	done := false
	defer func() {
		if !done {
			m.freeAll(vals)
		}
	}()
	for _, a := range args {
		v, err := m.callArg(a)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	done = true
	return vals, nil
}

// callArg converts one argument of a call, or the value of SetProperty.
func (m *VM) callArg(a any) (jsValue, error) {
	switch x := a.(type) {
	case nil:
		return null, nil
	case Value:
		if m.foreign(x) {
			return exception, errForeignValue
		}
		return dupValue(x.jsValue()), nil
	case Undefined:
		return undefined, nil
	case string:
		return m.newString(x)
	case int:
		return intValue(int64(x)), nil
	case bool:
		return newBool(x), nil
	case float64:
		return newFloat(x), nil
	case int8:
		return newInt32(int32(x)), nil
	case uint8:
		return newInt32(int32(x)), nil
	case int16:
		return newInt32(int32(x)), nil
	case uint16:
		return newInt32(int32(x)), nil
	case int32:
		return newInt32(x), nil
	case int64:
		return intValue(x), nil
	case uint32:
		return uintValue(uint64(x)), nil
	case uint:
		return uintValue(uint64(x)), nil
	case uint64:
		return uintValue(x), nil
	case *big.Int:
		if x == nil {
			return null, nil
		}
		return m.bigIntValue(x)
	}
	b, err := json.Marshal(a)
	if err != nil {
		return exception, err
	}
	return m.parseJSON(string(b))
}

// intValue is the number n: an int32 when it fits, else a float.
func intValue(n int64) jsValue {
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		return newInt32(int32(n))
	}
	return newFloat(float64(n))
}

// uintValue is the number n: an int32 when it fits, else a float.
func uintValue(n uint64) jsValue {
	if n <= math.MaxInt32 {
		return newInt32(int32(n))
	}
	return newFloat(float64(n))
}

// bigIntValue is the BigInt of x, which the engine makes of its literal.
func (m *VM) bigIntValue(x *big.Int) (jsValue, error) {
	v, err := m.eval(x.String()+"n", EvalGlobal)
	if err == nil && isException(v) {
		err = m.errFromException()
	}
	return v, err
}

// parseJSON parses the JSON text s, with the file name <eval> for its errors.
func (m *VM) parseJSON(s string) (jsValue, error) {
	v, err := m.jsParseJSON2(s, false, 0)
	if err == nil && isException(v) {
		err = m.errFromException()
	}
	return v, err
}
