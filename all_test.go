// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

// goString of a Go value, for messages: its type and value.
func show(v any) string { return fmt.Sprintf("%T(%v)", v, v) }

// sameGo reports whether got is want, comparing an *Object by its JSON and a
// *big.Int by its value.
func sameGo(got, want any) bool {
	switch w := want.(type) {
	case *Object:
		g, ok := got.(*Object)
		return ok && g.json == w.json
	case *big.Int:
		g, ok := got.(*big.Int)
		return ok && g.Cmp(w) == 0
	}
	return reflect.DeepEqual(got, want)
}

func TestEval(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		src  string
		want any
	}{
		{`42*314;`, 13188},
		{`'foo'+'bar';`, "foobar"},
		{`42 < 314;`, true},
		{`null;`, nil},
		{`undefined;`, Undefined{}},
		{`0.5 * 3;`, 1.5},
		{`Symbol('s');`, Unsupported{}},
		{`a = {}; a;`, &Object{json: `{}`}},
		{`({foo:'bar',baz:"qux",i:42});`, &Object{json: `{"foo":"bar","baz":"qux","i":42}`}},
		{`[];`, &Object{json: `[]`}},
		{`[1,2,3];`, &Object{json: `[1,2,3]`}},
	} {
		got, err := m.Eval(c.src, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if !sameGo(got, c.want) {
			t.Errorf("%s: got %s, want %s", c.src, show(got), show(c.want))
		}
	}
	_, err := m.Eval(`throw new Error('FAIL')`, EvalGlobal)
	if err == nil || err.Error() != "Error: FAIL" {
		t.Errorf("throw: got %v, want Error: FAIL", err)
	}
}

func TestEvalBigInt(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct{ src, want string }{
		{`BigInt('1234567890123456789')`, "1234567890123456789"},
		{`BigInt('-1234567890123456789')`, "-1234567890123456789"},
		{`1234567890123456789n`, "1234567890123456789"},
		{`-1234567890123456789n`, "-1234567890123456789"},
		{`0n`, "0"},
		{`-1n`, "-1"},
	} {
		got, err := m.Eval(c.src, EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if _, ok := got.(*big.Int); !ok || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %s, want *big.Int %s", c.src, show(got), c.want)
		}
	}
}

func TestCallNotAFunction(t *testing.T) {
	m := newTestVM(t)
	for _, src := range []string{`42;`, `var a = 42; a;`, `var a = { }; a;`} {
		if _, err := m.Call(src); err == nil {
			t.Errorf("Call(%q): no error", src)
		} else if err.Error() != "cannot call a non-function" {
			t.Errorf("Call(%q): %v", src, err)
		}
	}
}

func TestCallResults(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		ret  string
		want any
	}{
		{`null`, nil},
		{`undefined`, Undefined{}},
		{`"foo"`, "foo"},
		{`42`, 42},
		{`true`, true},
		{`3.14`, 3.14},
		{`1n`, big.NewInt(1)},
		{`{1:2,3:4}`, &Object{json: `{"1":2,"3":4}`}},
	} {
		got, err := m.Call(`function f() { return ` + c.ret + `; }; f`)
		if err != nil {
			t.Errorf("%s: %v", c.ret, err)
			continue
		}
		if !sameGo(got, c.want) {
			t.Errorf("%s: got %s, want %s", c.ret, show(got), show(c.want))
		}
	}
}

type pairT struct {
	A int
	B string
}

func TestCallArgs(t *testing.T) {
	m := newTestVM(t)
	const id = `function f(a) { return a }; f`
	for _, c := range []struct {
		fn   string
		args []any
		want any
	}{
		{id, nil, Undefined{}},
		{id, []any{nil}, nil},
		{id, []any{Undefined{}}, Undefined{}},
		{id, []any{123}, 123},
		{id, []any{int64(1234567890123)}, 1.234567890123e12},
		{id, []any{uint64(1 << 31)}, float64(1 << 31)},
		{id, []any{int8(-8), uint8(8)}, -8},
		{id, []any{true}, true},
		{id, []any{"foo"}, "foo"},
		{id, []any{big.NewInt(42)}, big.NewInt(42)},
		{id, []any{pairT{11, "aa"}}, &Object{json: `{"A":11,"B":"aa"}`}},
		{id, []any{&pairT{11, "aa"}}, &Object{json: `{"A":11,"B":"aa"}`}},
		{id, []any{float32(2.5)}, 2.5},
		{id, []any{[]byte("hi")}, "aGk="},
		{`function f(a) { return a.A }; f`, []any{pairT{11, "aa"}}, 11},
		{`function f(a, b, c, d) { return {a:a, b:b, c:c, d:d} }; f`, []any{"aa", 11, "cc", 22}, &Object{json: `{"a":"aa","b":11,"c":"cc","d":22}`}},
		{`var obj = {"a": 42, foo() { return obj.a }}; obj.foo`, nil, 42},
	} {
		got, err := m.Call(c.fn, c.args...)
		if err != nil {
			t.Errorf("%s %v: %v", c.fn, c.args, err)
			continue
		}
		if !sameGo(got, c.want) {
			t.Errorf("%s %v: got %s, want %s", c.fn, c.args, show(got), show(c.want))
		}
	}
}

// A nil *big.Int is null, as it is as a result. It used to be the source
// "<nil>n", whose syntax error went on as the argument.
func TestCallNilBigInt(t *testing.T) {
	m := newTestVM(t)
	got, err := m.Call(`(a => a === null)`, (*big.Int)(nil))
	if err != nil || got != true {
		t.Fatalf("got %v, %v, want true", got, err)
	}
	wantNoPendingException(t, m)
}

// An argument that goes as JSON and fails to parse, here for want of memory,
// is the error of the call, with nothing left pending.
func TestCallJSONArgFails(t *testing.T) {
	m := newTestVM(t)
	big := make([]struct{}, 200000)
	m.SetMemoryLimit(m.MemorySize() + 1<<20)
	_, err := m.Call(`(a => a.length)`, big)
	wantKind(t, err, outOfMemory)
	wantNoPendingException(t, m)
	m.SetMemoryLimit(^uintptr(0))
	wantEval(t, m, `1 + 1`, 2)
}

func TestCallValueThis(t *testing.T) {
	m := newTestVM(t)
	v, err := m.EvalValue(`var obj = {a: 42, g() { return this }}; obj`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Free()
	got, err := m.Call(`function f(o) { return o.a }; f`, v)
	if err != nil || got != 42 {
		t.Fatalf("Call: got %v, %v", got, err)
	}
	// this is the global object, not obj.
	r, err := m.CallValue(`obj.g`, v)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()
	if got, err := r.Any(); err != nil || !sameGo(got, &Object{json: `{"obj":{"a":42}}`}) {
		t.Fatalf("CallValue: got %s, %v", show(got), err)
	}
}

// An exception of the function expression is the error, not "cannot call a
// non-function".
func TestCallExpressionThrows(t *testing.T) {
	m := newTestVM(t)
	_, err := m.Call(`nope`)
	var e *Error
	if !errors.As(err, &e) || e.Name != "ReferenceError" {
		t.Fatalf("got %v, want a ReferenceError", err)
	}
	wantNoPendingException(t, m)
}

func TestRegisterFuncRefused(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		name     string
		f        any
		wantThis bool
		want     string
	}{
		{"myfunc", 42, false, "myfunc is not a function"},
		{"myfunc", func() {}, true, "'wantThis' is true but 'f' has zero parameters"},
		{"myfunc", nil, false, "myfunc is not a function"},
		{"myfunc", (func())(nil), false, "myfunc is not a function"},
		{"", func() {}, false, "func name cannot be empty"},
		{"a\x00b", func() {}, false, `func name cannot contain a NUL byte: "a\x00b"`},
	} {
		err := m.RegisterFunc(c.name, c.f, c.wantThis)
		if err == nil || err.Error() != c.want {
			t.Errorf("RegisterFunc(%q, %T, %v): got %v, want %s", c.name, c.f, c.wantThis, err, c.want)
		}
	}
	if err := m.RegisterFunc("myfunc", func() {}, false); err != nil {
		t.Fatalf("the name after the refusals: %v", err)
	}
	if err := m.RegisterFunc("myfunc", func() {}, false); err == nil || err.Error() != "func already registered: myfunc" {
		t.Fatalf("again: %v", err)
	}
}

// The length of a Go function is its parameters less one.
func TestRegisterFuncLength(t *testing.T) {
	m := newTestVM(t)
	for name, f := range map[string]any{
		"l0": func() {},
		"l1": func(int) {},
		"l2": func(a, b, c int) {},
		"l3": func(s string, args ...any) {},
	} {
		if err := m.RegisterFunc(name, f, false); err != nil {
			t.Fatal(err)
		}
	}
	wantEval(t, m, `[l0.length, l1.length, l2.length, l3.length].join()`, "0,0,2,1")
}

type T struct {
	A int
	B string
}

func TestRegisterFuncResults(t *testing.T) {
	m := newTestVM(t)
	obj, err := m.Eval(`({x: [1, "y"]})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	i := 495
	funcs := map[string]any{
		"r0":  func() {},
		"r1":  func() any { return nil },
		"r2":  func() any { return Undefined{} },
		"r3":  func() any { return fmt.Errorf("abc") },
		"r4":  func() any { return error(nil) },
		"r5":  func() error { return errors.New("abc") },
		"r6":  func() error { return nil },
		"r7":  func() *int { return nil },
		"r8":  func() any { return &i },
		"r9":  func() bool { return true },
		"r10": func() bool { return false },
		"r11": func() float64 { return 0.5 },
		"r12": func() float32 { return 2.5 },
		"r13": func() string { return "2.5" },
		"r14": func() any { return big.NewInt(500) },
		"r15": func() *big.Int { return big.NewInt(501) },
		"r16": func() any { return obj },
		"r17": func() any { return []any{42, "foo"} },
		"r18": func() []any { return []any{42, "foo"} },
		"r19": func() []int { return []int{42, 314} },
		"r20": func() (int, string) { return 42, "foo" },
		"r21": func() T { return T{511, "foo"} },
		"r22": func() int { return 542 },
	}
	for name, f := range funcs {
		if err := m.RegisterFunc(name, f, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		src  string
		want any
	}{
		{`r0()`, Undefined{}},
		{`r1()`, nil},
		{`r2()`, Undefined{}},
		{`r3()`, "abc"},
		{`r4()`, nil},
		{`r5()`, "abc"},
		{`r6()`, nil},
		{`r7()`, nil},
		{`r8()`, 495},
		{`r9()`, true},
		{`r10()`, false},
		{`r11()`, 0.5},
		{`r12()`, 2.5},
		{`r13()`, "2.5"},
		{`r14()`, big.NewInt(500)},
		{`r15()`, big.NewInt(501)},
		{`r16()`, &Object{json: `{"x":[1,"y"]}`}},
		{`r17()`, &Object{json: `[42,"foo"]`}},
		{`r18()`, &Object{json: `[42,"foo"]`}},
		{`r19()`, &Object{json: `[42,314]`}},
		{`r20()`, &Object{json: `[42,"foo"]`}},
		{`r21()`, &Object{json: `{"A":511,"B":"foo"}`}},
		{`r9() === true && r10() === false && r22() === 542`, true},
	} {
		got, err := m.Eval(c.src, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if !sameGo(got, c.want) {
			t.Errorf("%s: got %s, want %s", c.src, show(got), show(c.want))
		}
	}
}

func TestRegisterFuncArgs(t *testing.T) {
	m := newTestVM(t)
	funcs := []struct {
		name     string
		f        any
		wantThis bool
	}{
		{"g1", func(this any) any { return this }, true},
		{"g3", func(i int, s string) (string, int) { return s, i }, false},
		{"g4", func(in ...any) []any { return in }, false},
		{"g7", func(s string, args ...any) string { return fmt.Sprintf(s, args...) }, false},
		{"g8", func(this Value) (any, error) {
			if err := this.SetProperty(atomOrFatal(t, m, "foo"), 522); err != nil {
				return nil, err
			}
			return this.Any()
		}, true},
		{"g9", func(this Value) Value {
			if err := this.SetProperty(atomOrFatal(t, m, "foo"), 530); err != nil {
				t.Error(err)
			}
			return this.Dup()
		}, true},
		{"g10", func(a Value) (int, Value) { return 534, a.Dup() }, false},
	}
	for _, f := range funcs {
		if err := m.RegisterFunc(f.name, f.f, f.wantThis); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		src  string
		want any
	}{
		{`var a = {foo: 512, bar: g1}; a.bar()`, &Object{json: `{"foo":512}`}},
		{`g3(514, 'foo')`, &Object{json: `["foo",514]`}},
		{`g4(515, 'foo')`, &Object{json: `[515,"foo"]`}},
		{`g4()`, &Object{json: `[]`}},
		{`g7('hello %v')`, "hello %!v(MISSING)"},
		{`var b = {foo: 521, g8}; b.g8()`, &Object{json: `[{"foo":522},null]`}},
		{`var c = {foo: 529, g9}; c.g9()`, &Object{json: `{"foo":530}`}},
		{`g10({foo: 533})`, &Object{json: `[534,{"foo":533}]`}},
	} {
		got, err := m.Eval(c.src, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if !sameGo(got, c.want) {
			t.Errorf("%s: got %s, want %s", c.src, show(got), show(c.want))
		}
	}
	for _, c := range []struct{ src, want string }{
		{`g3()`, "not enough arguments in call to g3"},
		{`g3(1, 'a', 2)`, "too many arguments in call to g3"},
		{`g3(3.5, 'a')`, "calling g3: cannot assign float64 to int"},
		{`g3(1, undefined)`, "calling g3: cannot assign tifajs.Undefined to string"},
		{`g3(1, {})`, "calling g3: cannot assign *tifajs.Object to string"},
	} {
		_, err := m.Eval(c.src, EvalGlobal)
		wantError(t, err, "TypeError", c.want)
	}
}

func TestObjectValueFree(t *testing.T) {
	m := newTestVM(t)
	v, err := m.EvalValue(`({a: 1})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	v.Free()
	if v != (Value{}) {
		t.Fatal("Free left the Value set")
	}
	v.Free() // the zero Value: nothing to do
}

func TestAtoms(t *testing.T) {
	m := newTestVM(t)
	foo, err := m.NewAtom("foo")
	if err != nil || foo == atomNull {
		t.Fatalf("NewAtom(foo): %v, %v", foo, err)
	}
	bar, err := m.NewAtom("bar")
	if err != nil || bar == atomNull || bar == foo {
		t.Fatalf("NewAtom(bar): %v, %v (foo %v)", bar, err, foo)
	}
	if again, err := m.NewAtom("foo"); err != nil || again != foo {
		t.Fatalf("NewAtom(foo) again: %v, %v", again, err)
	}
}

func TestProperties(t *testing.T) {
	m := newTestVM(t)
	obj, err := m.EvalValue(`globalThis.obj = {}; obj`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Free()
	foo := atomOrFatal(t, m, "foo")

	s, err := m.NewString("bar")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetPropertyValue(obj, foo, s); err != nil {
		t.Fatal(err)
	}
	s.Free()
	wantEval(t, m, `obj.foo;`, "bar")

	if err := m.SetProperty(obj, foo, "baz"); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `obj.foo;`, "baz")
	if err := obj.SetProperty(foo, "bar"); err != nil {
		t.Fatal(err)
	}

	v, err := m.GetPropertyValue(obj, foo)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := v.Any(); err != nil || got != "bar" {
		t.Fatalf("GetPropertyValue: %v, %v", got, err)
	}
	v.Free()
	if got, err := m.GetProperty(obj, foo); err != nil || got != "bar" {
		t.Fatalf("GetProperty: %v, %v", got, err)
	}
	if got, err := obj.Property(foo); err != nil || got != "bar" {
		t.Fatalf("Property: %v, %v", got, err)
	}
	if got, err := obj.GetProperty(UndefinedValue, foo); err != nil || got != "bar" {
		t.Fatalf("Value.GetProperty: %v, %v", got, err)
	}
	if err := obj.SetPropertyValue(foo, UndefinedValue); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `'foo' in obj && obj.foo === undefined`, true)
	if v, err := obj.GetPropertyValue(foo); err != nil || !v.IsUndefined() {
		t.Fatalf("Value.GetPropertyValue: %v, %v", v, err)
	}
}

// A write a Proxy refuses is a TypeError, as in strict code, and leaves the
// target as it was.
func TestSetPropertyRefused(t *testing.T) {
	m := newTestVM(t)
	p := evalValueOrFatal(t, m, `globalThis.t = {x: 0}; new Proxy(t, {defineProperty() { return false }})`)
	defer p.Free()
	for _, name := range []string{"x", "y"} {
		atom := atomOrFatal(t, m, name)
		var e *Error
		if err := m.SetProperty(p, atom, 1); !errors.As(err, &e) || e.Name != "TypeError" {
			t.Errorf("SetProperty %s: got %v, want a TypeError", name, err)
		}
		if err := m.SetPropertyValue(p, atom, m.NewInt(1)); !errors.As(err, &e) || e.Name != "TypeError" {
			t.Errorf("SetPropertyValue %s: got %v, want a TypeError", name, err)
		}
	}
	wantEval(t, m, `JSON.stringify(t)`, `{"x":0}`)
}

// The bytecode of a script is what the engine writes. When the engine changes
// its format, the expected bytes change with it.
func TestBytecode(t *testing.T) {
	m := newTestVM(t)
	code, err := m.Compile(`42 * 314;`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	const want = "0f000c000600a801000100020000000801aa0100000000bd2abe3a019acd29a8010400001b0600"
	if got := hex.EncodeToString(code); got != want {
		t.Errorf("bytecode\n got %s\nwant %s", got, want)
	}
	got, err := m.EvalBytecode(code)
	if err != nil || got != 13188 {
		t.Fatalf("EvalBytecode: %v, %v", got, err)
	}
	v, err := m.EvalBytecodeValue(code)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Free()
	if got, err := v.Any(); err != nil || got != 13188 {
		t.Fatalf("EvalBytecodeValue: %v, %v", got, err)
	}
}

// Value.Call keeps the this it is given.
func TestValueCall(t *testing.T) {
	m := newTestVM(t)
	f := evalValueOrFatal(t, m, `(function (a, b) { return this.base + a + b })`)
	defer f.Free()
	for _, c := range []struct {
		base, want int
	}{{100, 123}, {1000, 1023}} {
		this := evalValueOrFatal(t, m, fmt.Sprintf(`({base: %d})`, c.base))
		got, err := f.Call(this, 20, 3)
		if err != nil || got != c.want {
			t.Errorf("base %d: got %v, %v, want %d", c.base, got, err, c.want)
		}
		v, err := f.CallValue(this, 20, 3)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := v.Any(); err != nil || got != c.want {
			t.Errorf("CallValue base %d: got %v, %v", c.base, got, err)
		}
		v.Free()
		this.Free()
	}
	g := evalValueOrFatal(t, m, `(function (s) { return 'got ' + s })`)
	defer g.Free()
	if got, err := g.Call(UndefinedValue, "it"); err != nil || got != "got it" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestValueCallErrors(t *testing.T) {
	m := newTestVM(t)
	if _, err := UndefinedValue.Call(UndefinedValue); err == nil {
		t.Error("Call of a Value of no VM: no error")
	}
	if _, err := UndefinedValue.CallValue(UndefinedValue); err == nil {
		t.Error("CallValue of a Value of no VM: no error")
	}
	obj := evalValueOrFatal(t, m, `({})`)
	defer obj.Free()
	if _, err := obj.Call(UndefinedValue); err == nil || err.Error() != "cannot call a non-function" {
		t.Errorf("Call of an object: %v", err)
	}
	if _, err := obj.CallValue(UndefinedValue); err == nil || err.Error() != "cannot call a non-function" {
		t.Errorf("CallValue of an object: %v", err)
	}
	f := evalValueOrFatal(t, m, `(function () { return 1 })`)
	defer f.Free()
	other := newTestVM(t)
	this := evalValueOrFatal(t, other, `({})`)
	defer this.Free()
	if _, err := f.Call(this); !errors.Is(err, errForeignValue) {
		t.Errorf("Call with the this of another VM: %v", err)
	}
	if _, err := f.CallValue(this); !errors.Is(err, errForeignValue) {
		t.Errorf("CallValue with the this of another VM: %v", err)
	}
}

// A Go function keeps the resolve and reject of a promise, and Go calls one
// later.
func TestValueCallPromise(t *testing.T) {
	m := newTestVM(t)
	var resolve, reject Value
	if err := m.RegisterFunc("goAsync", func(res, rej Value) {
		resolve, reject = res.Dup(), rej.Dup()
	}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`new Promise((res, rej) => goAsync(res, rej)).then(v => { globalThis.out = 'ok: ' + v }, e => { globalThis.out = 'err: ' + e })`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	defer resolve.Free()
	defer reject.Free()
	if _, err := resolve.Call(UndefinedValue, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `globalThis.out`, "ok: done")
}

// A null argument is the nil of a parameter that has one, and a TypeError
// for any other.
func TestRegisterFuncNullArg(t *testing.T) {
	type S struct{ A int }
	isNil := func(v any) bool { return v == nil }
	for _, c := range []struct {
		f        any
		wantThis bool
		src      string
		want     any
		err      string
	}{
		{f: func(v any) bool { return v == nil }, src: `f(null)`, want: true},
		{f: func(v *S) bool { return v == nil }, src: `f(null)`, want: true},
		{f: func(v map[string]any) bool { return v == nil }, src: `f(null)`, want: true},
		{f: func(v []int) bool { return v == nil }, src: `f(null)`, want: true},
		{f: func(v error) bool { return v == nil }, src: `f(null)`, want: true},
		{f: func(v ...any) bool { return len(v) == 1 && v[0] == nil }, src: `f(null)`, want: true},
		{f: func(a, b any) bool { return a == nil && b == 42 }, src: `f(null, 42)`, want: true},
		{f: func(this any) bool { return isNil(this) }, wantThis: true, src: `f.call(null)`, want: true},
		{f: func(v any) bool { return v == Undefined{} }, src: `f(undefined)`, want: true},
		{f: func(string) {}, src: `f(null)`, err: "cannot assign null to string"},
		{f: func(int) {}, src: `f(null)`, err: "cannot assign null to int"},
		{f: func(bool) {}, src: `f(null)`, err: "cannot assign null to bool"},
		{f: func(float64) {}, src: `f(null)`, err: "cannot assign null to float64"},
		{f: func(S) {}, src: `f(null)`, err: "cannot assign null to"},
		{f: func(this string) {}, wantThis: true, src: `f.call(null)`, err: "cannot assign null to string"},
	} {
		m := newTestVM(t)
		if err := m.RegisterFunc("f", c.f, c.wantThis); err != nil {
			t.Fatal(err)
		}
		got, err := m.Eval(c.src, EvalGlobal)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%T %s: got %v, %v, want an error with %q", c.f, c.src, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%T %s: got %v, %v, want %v", c.f, c.src, got, err, c.want)
		}
	}
}

func TestRegisterHostFuncNullArg(t *testing.T) {
	m := newTestVM(t)
	var got []any
	mustHost(t, m, "h", func(args []any) (any, error) {
		got = args
		return len(args), nil
	})
	if n, err := m.Eval(`h(null, 42, undefined, null, 1.5)`, EvalGlobal); err != nil || n != 5 {
		t.Fatalf("got %v, %v", n, err)
	}
	if want := []any{nil, 42, Undefined{}, nil, 1.5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// A map result is an object of its entries, in the order of its keys as
// strings.
func TestRegisterFuncMapResult(t *testing.T) {
	for _, c := range []struct {
		f    any
		src  string
		want any
		err  string
	}{
		{f: func() map[string]int { return map[string]int{"a": 1, "b": 2} }, src: `JSON.stringify(f())`, want: `{"a":1,"b":2}`},
		{f: func() map[string]int { return map[string]int{} }, src: `JSON.stringify(f())`, want: `{}`},
		{f: func() any { return map[string]int{"a": 1} }, src: `JSON.stringify(f())`, want: `{"a":1}`},
		{f: func() map[string]map[string]int { return map[string]map[string]int{"in": {"q": 7}} }, src: `JSON.stringify(f())`, want: `{"in":{"q":7}}`},
		{f: func() map[string]int { return nil }, src: `f() === null`, want: true},
		{f: func() map[string]any { return map[string]any{"z": nil} }, src: `f().z === null`, want: true},
		{f: func() map[int]string { return map[int]string{1: "x", -2: "y"} }, src: `JSON.stringify(f())`, want: `{"1":"x","-2":"y"}`},
		{f: func() map[string]int { return map[string]int{"f": 6, "c": 3, "a": 1, "e": 5, "b": 2, "d": 4} }, src: `Object.keys(f()).join(',')`, want: "a,b,c,d,e,f"},
		{f: func() map[string]*big.Int { return map[string]*big.Int{"n": new(big.Int).Lsh(big.NewInt(1), 62)} }, src: `typeof f().n + ' ' + String(f().n)`, want: "bigint 4611686018427387904"},
		{f: func() map[[2]int]int { return map[[2]int]int{{1, 2}: 3} }, src: `f()`, err: "unsupported map key type"},
	} {
		m := newTestVM(t)
		if err := m.RegisterFunc("f", c.f, false); err != nil {
			t.Fatal(err)
		}
		got, err := m.Eval(c.src, EvalGlobal)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%T: got %v, %v, want an error with %q", c.f, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%T %s: got %v, %v, want %v", c.f, c.src, got, err, c.want)
		}
	}
}

// A function in a map result stays callable: each call hands over a Dup of
// it, and the function keeps its own.
func TestRegisterFuncMapValueResult(t *testing.T) {
	m := newTestVM(t)
	fn := evalValueOrFatal(t, m, `(() => 'called')`)
	defer fn.Free()
	if err := m.RegisterFunc("f", func() map[string]any { return map[string]any{"cb": fn.Dup()} }, false); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `typeof f().cb + ' ' + f().cb() + ' ' + f().cb()`, "function called called")
}

// The zero Value and UndefinedValue are undefined everywhere: as arguments,
// as this, and to Any and IsUndefined.
func TestZeroValueIsUndefined(t *testing.T) {
	m := newTestVM(t)
	for name, v := range map[string]Value{"zero": {}, "UndefinedValue": UndefinedValue} {
		if !v.IsUndefined() {
			t.Errorf("%s: IsUndefined is false", name)
		}
		if got, err := v.Any(); err != nil || got != (Undefined{}) {
			t.Errorf("%s: Any is %v, %v", name, got, err)
		}
		if got, err := m.Call(`(function (a) { return a === undefined && this === globalThis })`, v); err != nil || got != true {
			t.Errorf("%s as an argument: %v, %v", name, got, err)
		}
	}
	if m.NewInt(0).IsUndefined() {
		t.Error("the number 0 of a VM is undefined")
	}
	if got, err := m.NewInt(0).Any(); err != nil || got != 0 {
		t.Errorf("the number 0: %v, %v", got, err)
	}
}

// Dup of a Value of no VM panics: it is most often a Value used after Free.
func TestDupOfNoVMPanics(t *testing.T) {
	for _, v := range []Value{{}, UndefinedValue} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("Dup of a Value of no VM did not panic")
				}
			}()
			v.Dup()
		}()
	}
}

func TestNewValues(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		v    Value
		want any
	}{
		{m.NewInt(7), 7},
		{m.NewInt(1 << 40), float64(1 << 40)},
		{m.NewFloat64(0.25), 0.25},
	} {
		if got, err := c.v.Any(); err != nil || got != c.want {
			t.Errorf("got %s, %v, want %s", show(got), err, show(c.want))
		}
	}
	s, err := m.NewString("a\xffb")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Free()
	if got, err := s.Any(); err != nil || got != "a�b" {
		t.Errorf("NewString: %q, %v", got, err)
	}
	o, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	defer o.Free()
	if b, err := o.MarshalJSON(); err != nil || string(b) != "{}" {
		t.Errorf("NewObjectValue: %s, %v", b, err)
	}
	g := m.GlobalObject()
	defer g.Free()
	if g.VM() != m || o.VM() != m || UndefinedValue.VM() != nil {
		t.Error("VM is not the VM of the value")
	}
}

func TestStaticValues(t *testing.T) {
	if Version() != engineVersion {
		t.Errorf("Version is %q", Version())
	}
	if (Undefined{}).String() != "undefined" || (Unsupported{}).String() != "unsupported Javascript type" {
		t.Error("the strings of Undefined and Unsupported")
	}
	o := &Object{json: `{"a":1}`}
	var into struct{ A int }
	if err := o.Into(&into); err != nil || into.A != 1 || o.String() != `{"a":1}` {
		t.Errorf("Object: %v, %v", into, err)
	}
	for _, c := range []struct {
		e    *Error
		want string
	}{
		{&Error{}, "Error"},
		{&Error{Name: "TypeError"}, "TypeError"},
		{&Error{Message: "m"}, "Error: m"},
		{&Error{Name: "RangeError", Message: "m"}, "RangeError: m"},
		{&Error{engine: true, raw: "42", Name: "x"}, "42"},
	} {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%#v: Error() is %q, want %q", c.e, got, c.want)
		}
	}
	if errors.Is(&Error{}, ErrKilled) || !errors.Is(killedError(), ErrKilled) || !errors.Is(errKilledVM, ErrKilled) {
		t.Error("Is of ErrKilled")
	}
}

func TestValidUTF8(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"日本", "日本"},
		{"\x80a", "�a"},
		{"\xc0\x80", "��"},
		{"\xe4\xb8", "��"},
		{"a\xed\xa0\x80", "a���"},
	} {
		if got := validUTF8(c.in); got != c.want {
			t.Errorf("validUTF8(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	s := "valid string"
	if testing.AllocsPerRun(10, func() { s = validUTF8(s) }) != 0 {
		t.Error("validUTF8 of a valid string allocates")
	}
}

func TestWellFormed(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"\xed", "\xed"},
		{"a\xed", "a\xed"},
		{"\xed\xa0", "\xed\xa0"},
		{"a\xed\xa0", "a\xed\xa0"},
		{"\xed\x95\x9c\xed", "\xed\x95\x9c\xed"},
		{"\xed\xa0\x80\xed", "�\xed"},
		{"x\xed\xbf\xbfy\xed\xa0\x80", "x�y�"},
	} {
		if got := wellFormed([]byte(c.in)); got != c.want {
			t.Errorf("wellFormed(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// SetMemoryLimit of 0 is the smallest limit, not none, and ^uintptr(0) is
// none.
func TestSetMemoryLimitZero(t *testing.T) {
	m := newTestVM(t)
	m.SetMemoryLimit(0)
	if m.memoryLimit != 128<<10 {
		t.Fatalf("the limit of 0 is %d", m.memoryLimit)
	}
	_, err := m.Eval(`'x'.repeat(1 << 20)`, EvalGlobal)
	wantKind(t, err, outOfMemory)
	m.SetMemoryLimit(^uintptr(0))
	if m.memoryLimit != 0 {
		t.Fatalf("no limit is %d", m.memoryLimit)
	}
	wantEval(t, m, `'x'.repeat(1 << 20).length`, 1<<20)
}

func TestGlobalObject(t *testing.T) {
	m := newTestVM(t)
	g := m.GlobalObject()
	defer g.Free()
	if got, err := m.Call(`(o => o === globalThis)`, g); err != nil || got != true {
		t.Fatalf("got %v, %v", got, err)
	}
}

// EvalThis runs with this set to obj, and takes obj.
func TestEvalThis(t *testing.T) {
	m := newTestVM(t)
	obj := evalValueOrFatal(t, m, `({a: 5})`)
	if got, err := m.EvalThis(obj, `this.a`, EvalGlobal); err != nil || got != 5 {
		t.Fatalf("EvalThis: %v, %v", got, err)
	}
	obj = evalValueOrFatal(t, m, `({a: 6})`)
	v, err := m.EvalThisValue(obj, `this`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Free()
	if got, err := v.Any(); err != nil || got.(*Object).json != `{"a":6}` {
		t.Fatalf("EvalThisValue: %v, %v", got, err)
	}
	other := newTestVM(t)
	foreign := evalValueOrFatal(t, other, `({})`)
	defer foreign.Free()
	if _, err := m.EvalThis(foreign, `1`, EvalGlobal); !errors.Is(err, errForeignValue) {
		t.Fatalf("EvalThis of a Value of another VM: %v", err)
	}
	if _, err := m.EvalThisValue(foreign, `1`, EvalGlobal); !errors.Is(err, errForeignValue) {
		t.Fatalf("EvalThisValue of a Value of another VM: %v", err)
	}
}
