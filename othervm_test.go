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

	for _, v := range []Value{str, obj} {
		if n := *refCount(v.v); n != 1 {
			t.Errorf("a Value of b has a count of %d, want 1", n)
		}
	}
	if got, err := str.Any(); err != nil || got != strings.Repeat("b", 100) {
		t.Errorf("the string of b is now %q, %v", got, err)
	}
}
