// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"strings"
	"testing"
)

// TestValueOfAnotherVM gives a string and an object of VM b to every entry of
// VM a that takes a Value: as the value and the receiver of the property
// methods, as the this of EvalThis, as an argument and the this of a call, to
// NewArray, and as the result of a Go function, alone, in a slice, in a map
// and among several results. Each refuses it before it reaches the engine:
// a method with an error, a Go function by throwing a TypeError. b's values
// stay b's, with their counts as they were, so b frees them and closes with
// nothing of a in it, and a closes with nothing of b.
func TestValueOfAnotherVM(t *testing.T) {
	a := newTestVM(t)
	b := newTestVM(t)
	str, err := b.NewString(strings.Repeat("b", 100))
	if err != nil {
		t.Fatal(err)
	}
	defer str.Free()
	obj, err := b.EvalValue(`({ p: 1 })`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Free()
	mine, err := a.EvalValue(`({})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer mine.Free()
	p := atomOrFatal(t, a, "p")

	methods := []struct {
		name string
		run  func() error
	}{
		{"SetPropertyValue of its value", func() error { return a.SetPropertyValue(mine, p, str) }},
		{"SetPropertyValue of its receiver", func() error { return a.SetPropertyValue(obj, p, a.NewInt(1)) }},
		{"SetProperty of its receiver", func() error { return a.SetProperty(obj, p, 1) }},
		{"SetProperty of its value", func() error { return a.SetProperty(mine, p, str) }},
		{"GetPropertyValue", func() error { _, err := a.GetPropertyValue(obj, p); return err }},
		{"GetProperty", func() error { _, err := a.GetProperty(obj, p); return err }},
		{"EvalThis", func() error { _, err := a.EvalThis(obj, `this.p`, EvalGlobal); return err }},
		{"EvalThisValue", func() error { _, err := a.EvalThisValue(obj, `this.p`, EvalGlobal); return err }},
		{"Call", func() error { _, err := a.Call(`(x => x)`, str); return err }},
		{"Value.Call of its this", func() error {
			f := evalValueOrFatal(t, a, `(function () { return this })`)
			defer f.Free()
			_, err := f.Call(obj)
			return err
		}},
		{"NewArray", func() error { _, err := a.NewArray(str); return err }},
	}
	for _, c := range methods {
		if err := c.run(); err == nil {
			t.Errorf("%s took a Value of another VM", c.name)
		}
	}

	funcs := []struct {
		name string
		f    any
	}{
		{"theirs", func() Value { return str }},
		{"theirsInSlice", func() []any { return []any{"x", str} }},
		{"theirsInMap", func() map[string]any { return map[string]any{"k": str} }},
		{"theirsAmongResults", func() (string, Value) { return "x", obj }},
		{"theirsAsAny", func() any { return str }},
		{"theirsAsAnyInSlice", func() []any { return []any{[]any{obj}} }},
	}
	for _, f := range funcs {
		if err := a.RegisterFunc(f.name, f.f, false); err != nil {
			t.Fatal(err)
		}
		got, err := a.Eval(`try { `+f.name+`(); 'returned' } catch (e) { e.constructor.name }`, EvalGlobal)
		if err != nil || got != "TypeError" {
			t.Errorf("%s: got %v, %v, want a TypeError", f.name, got, err)
		}
	}

	if err := a.RegisterHostFunc("theirsFromHost", func([]any) (any, error) { return obj, nil }); err != nil {
		t.Fatal(err)
	}
	if got, err := a.Eval(`try { theirsFromHost(); 'returned' } catch (e) { e.constructor.name }`, EvalGlobal); err != nil || got != "TypeError" {
		t.Errorf("theirsFromHost: got %v, %v, want a TypeError", got, err)
	}

	for _, v := range []Value{str, obj} {
		if n := *refCount(v.v); n != 1 {
			t.Errorf("a Value of b has a count of %d, want 1", n)
		}
	}
	if got, err := str.Any(); err != nil || got != strings.Repeat("b", 100) {
		t.Errorf("the string of b is now %q, %v", got, err)
	}
}

// TestValueOfNoVM gives UndefinedValue and the zero Value, which belong to
// no VM, to the entries that take a Value. Each is javascript's undefined
// there, as for a Builtin: an argument, a property value, the this of a call
// and the result of a Go function, alone or in a slice.
func TestValueOfNoVM(t *testing.T) {
	m := newTestVM(t)
	p := atomOrFatal(t, m, "p")
	var v Value
	if err := m.RegisterFunc("one", func() Value { return v }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("slice", func() []Value { return []Value{v} }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("asAny", func() any { return v }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHostFunc("host", func([]any) (any, error) { return v, nil }); err != nil {
		t.Fatal(err)
	}
	for name, val := range map[string]Value{"UndefinedValue": UndefinedValue, "the zero Value": {}} {
		t.Run(name, func(t *testing.T) {
			v = val
			typeOf := func(what string, r any, err error) {
				t.Helper()
				if err != nil || r != "undefined" {
					t.Errorf("%s: got %v, %v, want undefined", what, r, err)
				}
			}

			r, err := m.Call(`(x => typeof x)`, v)
			typeOf("Call", r, err)

			obj := evalValueOrFatal(t, m, `({ p: 1 })`)
			defer obj.Free()
			if err := m.SetPropertyValue(obj, p, v); err != nil {
				t.Fatal(err)
			}
			r, err = m.EvalThis(obj.Dup(), `typeof this.p`, EvalGlobal)
			typeOf("SetPropertyValue", r, err)

			f := evalValueOrFatal(t, m, `(function () { 'use strict'; return typeof this })`)
			defer f.Free()
			r, err = f.Call(v)
			typeOf("Value.Call of its this", r, err)

			if _, err := m.GetProperty(v, p); err == nil {
				t.Error("GetProperty of undefined gave no error")
			}

			r, err = m.Eval(`typeof one()`, EvalGlobal)
			typeOf("the result of a Go function", r, err)
			r, err = m.Eval(`typeof slice()[0]`, EvalGlobal)
			typeOf("a slice a Go function returns", r, err)
			r, err = m.Eval(`typeof asAny()`, EvalGlobal)
			typeOf("an any a Go function returns", r, err)
			r, err = m.Eval(`typeof host()`, EvalGlobal)
			typeOf("the value of a host function", r, err)
		})
	}
}
