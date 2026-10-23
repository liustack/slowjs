// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	goruntime "runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	lib "github.com/liustack/quickjs/lib"
	_ "golang.org/x/mod/modfile" // generator.go
	util "modernc.org/fileutil/ccgo"
)

var (
	goarch   = goruntime.GOARCH
	goos     = goruntime.GOOS
	memgrind bool
	target   = fmt.Sprintf("%s/%s", goos, goarch)
)

//lint:ignore U1000 debug helper
func stack() []byte { return debug.Stack() }

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestEval(t *testing.T) {
	t.Run("eval1", testEval1)
	t.Run("eval2", testEval2)
	t.Run("eval3", testEval3)
}

func testEval1(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		js string
		v  any
	}{
		{"42*314;", 42 * 314},
		{"'foo'+'bar';", "foobar"},
		{"42 < 314;", true},
		{"null;", nil},
		{"undefined;", Undefined{}},
		{"throw new Error('FAIL')", fmt.Errorf("Error: FAIL")},
	} {
		v, err := m.Eval(test.js, EvalGlobal)
		t.Logf("%s: %T(%[1]v) %v", test.js, v, err)
		if err != nil {
			switch x := test.v.(type) {
			case error:
				if g, e := err.Error(), x.Error(); g != e {
					t.Fatalf("FAIL %s: %v %v", test.js, g, e)
				}

				continue
			default:
				t.Errorf("FAIL %s: %v", test.js, err)
				continue
			}
		}

		if g, e := v, test.v; g != e {
			t.Fatalf("FAIL %s: %T(%[1]v) %T(%[2]v)", test.js, g, e)
		}
	}
}

func testEval2(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		js string
		v  any
		sv string
	}{
		{"BigInt('1234567890123456789')", newBigInt(t, "1234567890123456789"), "1234567890123456789"},
		{"BigInt('-1234567890123456789')", newBigInt(t, "-1234567890123456789"), "-1234567890123456789"},
		{"1234567890123456789n", newBigInt(t, "1234567890123456789"), "1234567890123456789"},
		{"-1234567890123456789n", newBigInt(t, "-1234567890123456789"), "-1234567890123456789"},
	} {
		v, err := m.Eval(test.js, EvalGlobal)
		t.Logf("%s: %T(%[1]v) %v", test.js, v, err)
		if err != nil {
			t.Errorf("FAIL %s: %v", test.js, err)
			continue
		}

		if g, e := fmt.Sprintf("%T", v), fmt.Sprintf("%T", test.v); g != e {
			t.Errorf("FAIL %s: %T(%[1]v) %T(%[2]v)", test.js, g, e)
			continue
		}

		if g, e := fmt.Sprint(v), test.sv; g != e {
			t.Errorf("FAIL %v: %T(%[1]v) %T(%[2]v)", test.js, g, e)
			continue
		}
	}
}

func testEval3(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		js string
		v  string
	}{
		{`a = {}; a;`, `{}`},
		{`a = {foo: 'bar', baz: "qux", i: 42}; a;`, `{"foo":"bar","baz":"qux","i":42}`},
		{`a = []; a;`, `[]`},
		{`a = [1, 2, 3]; a;`, `[1,2,3]`},
	} {
		v, err := m.Eval(test.js, EvalGlobal)
		t.Logf("js=`%s`: v=%T(%[2]v) err=%T(%[3]v)", test.js, v, err)
		if err != nil {
			t.Errorf("FAIL js=`%s`: err=%v", test.js, err)
			continue
		}

		switch x := v.(type) {
		case *Object:
			if g, e := string(x.json), test.v; g != e {
				t.Errorf("got=`%s` expected=`%s`", g, e)
			}
		default:
			t.Errorf("unexpected result type: %T", x)
		}
	}
}

func TestCall0(t *testing.T) {
	t.Run("call1", testCall1)
	t.Run("call2", testCall2)
	t.Run("call3", testCall3)
	t.Run("call4", testCall4)
}

func testCall1(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		js string
	}{
		{`42;`},
		{`var a = 42; a;`},
		{`var a = { }; a;`},
	} {
		v, err := m.Call(test.js)
		t.Logf("js=`%s`: v=%T(%[2]v) err=%T(%[3]v)", test.js, v, err)
		if err == nil {
			t.Errorf("FAIL js=`%s`: expected non nil err", test.js)
		}
	}
}

func testCall2(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		js string
		v  any
	}{
		{`function f() { return null; }; f`, nil},
		{`function f() { return undefined; }; f`, Undefined{}},
		{`function f() { return "foo"; }; f`, "foo"},
		{`function f() { return 42; }; f`, 42},
		{`function f() { return true; }; f`, true},
		{`function f() { return 3.14; }; f`, 3.14},
		{`function f() { return 1n; }; f`, 1},
		{`function f() { return {1:2,3:4}; }; f`, `{"1":2,"3":4}`},
	} {
		v, err := m.Call(test.js)
		t.Logf("js=`%s`: v=%T(%[2]v) err=%T(%[3]v)", test.js, v, err)
		if err != nil {
			t.Errorf("FAIL js=`%s`: err=%v", test.js, err)
		}

		switch x := v.(type) {
		case nil:
			if test.v != nil {
				t.Errorf("expected nil")
			}
		case Undefined, string, int, bool, float64:
			if g, e := x, test.v; g != e {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *big.Int:
			if g, e := x, big.NewInt(int64(test.v.(int))); g.Cmp(e) != 0 {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *Object:
			if g, e := x.json, test.v; g != e {
				t.Errorf("got %v, expected %v", g, e)
			}
		default:
			panic(todo("%T", x))
		}
	}
}

func testCall3(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	type T struct {
		A int
		B string
	}

	for _, test := range []struct {
		js   string
		args []any
		v    any
	}{
		{`function f(a) { return a; }; f`, nil, Undefined{}},
		{`function f(a) { return a; }; f`, []any{nil}, nil},
		{`function f(a) { return a; }; f`, []any{Undefined{}}, Undefined{}},
		{`function f(a) { return a; }; f`, []any{123}, 123},
		{`function f(a) { return a; }; f`, []any{int64(1234567890123)}, 1.234567890123e+12},
		{`function f(a) { return a; }; f`, []any{true}, true},
		{`function f(a) { return a; }; f`, []any{"foo"}, "foo"},
		{`function f(a) { return a; }; f`, []any{big.NewInt(42)}, 42},
		{`function f(a, b, c, d) { return {a: a, b: b, c: c, d: d}; }; f`, []any{"aa", 11, "cc", 22}, `{"a":"aa","b":11,"c":"cc","d":22}`},
		{`function f(a) { return a; }; f`, []any{T{11, "aa"}}, `{"A":11,"B":"aa"}`},
		{`function f(a) { return a.A; }; f`, []any{T{11, "aa"}}, 11},
		{`function f(a) { return a; }; f`, []any{&T{11, "aa"}}, `{"A":11,"B":"aa"}`},
		{`var obj={"a":42, foo() { return obj.a; }}; obj.foo`, nil, 42},
	} {
		v, err := m.Call(test.js, test.args...)
		t.Logf("js=`%s`: v=%T(%[2]v) err=%T(%[3]v)", test.js, v, err)
		if err != nil {
			t.Errorf("FAIL js=`%s`: err=%v", test.js, err)
		}

		switch x := v.(type) {
		case nil:
			if test.v != nil {
				t.Errorf("expected nil")
			}
		case Undefined, string, int, bool, float64:
			if g, e := x, test.v; g != e {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *big.Int:
			if g, e := x, big.NewInt(int64(test.v.(int))); g.Cmp(e) != 0 {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *Object:
			if g, e := x.json, test.v; g != e {
				t.Errorf("got %v, expected %v", g, e)
			}
		default:
			panic(todo("%T", x))
		}
	}
}

func testCall4(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	v, err := m.EvalValue(`
function f(o) {
	return o.a;
}

obj = {a:42,g:function(){return this;}};
obj;
`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer v.Free()

	w, err := m.Call("f", v)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := w, 42; g != e {
		t.Fatal(g, e)
	}

	x, err := m.CallValue("obj.g", v)
	if err != nil {
		t.Fatal(err)
	}

	defer x.Free()

	if g, e := tag(x.v), lib.EJS_TAG_OBJECT; g != int32(e) {
		t.Fatal(g, e)
	}

	y, err := m.value(x.v, false)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := fmt.Sprint(y), `{"obj":{"a":42}}`; g != e {
		t.Fatal(g, e)
	}
}

func newBigInt(t *testing.T, s string) *big.Int {
	n := big.NewInt(0)
	if _, ok := n.SetString(s, 10); !ok {
		t.Fatalf("big.Int.SetString(%q, 10) failed", s)
	}

	return n
}

func TestMem(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}

	if !memgrind {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)

		defer cancel()

		t.Log(time.Now().Format(time.DateTime))
		if _, err := util.Shell(ctx, "go", "test", fmt.Sprintf("-short=%v", testing.Short()), "-v", "-tags", "libc.memgrind", "-timeout", "12h", "-run", "TestMemgrind"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegisterGoFunc(t *testing.T) {
	t.Run("fnFail", testRegisterGoFuncMustFail)
	t.Run("fnOK", testRegisterGoFuncOK)
}

func testRegisterGoFuncMustFail(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, test := range []struct {
		wantThis bool
		f        any
	}{
		{false, 42},       // Not a function
		{true, func() {}}, // Wants this but no parameters
	} {
		switch err := m.RegisterFunc("myfunc", test.f, test.wantThis); {
		case err != nil:
			t.Logf("registering function '%T(%[1]v)' failed as expected: err=%v", test, err)
		default:
			t.Errorf("registering should have failed: %T", test.f)
		}
	}
}

func testRegisterGoFuncOK(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.Eval("obj = {a: 463, b: 'foo'}; obj;", EvalGlobal)
	if err != nil {
		t.Fatalf("obj: %v", err)
	}

	foo, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	type T struct {
		A int
		B string
	}

	for i, test := range []struct {
		nm       string
		wantThis bool
		f        any
		args     []any
		call     string
		v        any
	}{
		{"", false, func() {}, nil, "", Undefined{}},
		{"", false, func() any { return nil }, nil, "", nil},
		{"", false, func() any { return Undefined{} }, nil, "", Undefined{}},
		{"", false, func() any { return fmt.Errorf("abc") }, nil, "", "abc"},
		{"", false, func() any { return error(nil) }, nil, "", nil},
		{"", false, func() error { return fmt.Errorf("abc") }, nil, "", "abc"},
		{"", false, func() error { return nil }, nil, "", nil},
		{"", false, func() *int { return nil }, nil, "", nil},
		{"", false, func() any { i := 495; return &i }, nil, "", 495},
		{"", false, func() *int { i := 496; return &i }, nil, "", 496},
		{"", false, func() bool { return true }, nil, "", true},
		{"", false, func() bool { return false }, nil, "", false},
		{"", false, func() any { return true }, nil, "", true},
		{"", false, func() float64 { return 0.5 }, nil, "", 0.5},
		{"", false, func() float32 { return 2.5 }, nil, "", 2.5},
		{"", false, func() string { return "2.5" }, nil, "", "2.5"},
		{"", false, func() any { return big.NewInt(500) }, nil, "", 500},
		{"", false, func() *big.Int { return big.NewInt(501) }, nil, "", 501},
		{"", false, func() any { return obj }, nil, "", `{"a":463,"b":"foo"}`},
		{"", false, func() any { return []any{42, "foo"} }, nil, "", `[42,"foo"]`},
		{"", false, func() []any { return []any{42, "foo"} }, nil, "", `[42,"foo"]`},
		{"", false, func() []int { return []int{42, 314} }, nil, "", `[42,314]`},
		{"", false, func() []any { return []any{42, obj, "foo"} }, nil, "", `[42,{"a":463,"b":"foo"},"foo"]`},
		{"", false, func() (int, string) { return 42, "foo" }, nil, "", `[42,"foo"]`},
		{"", true, func(any) {}, nil, "", Undefined{}},
		{"", false, func() any { return T{511, "foo"} }, nil, "", `{"A":511,"B":"foo"}`},
		{"g1", true, func(this any) any { return this }, nil, "var a = {foo: 512, bar: g1}; a.bar();", `{"foo":512}`},
		{"g2", false, func(in any) any { return in }, nil, "g2(obj)", `{"a":463,"b":"foo"}`},
		{"g3", false, func(i int, s string) (string, int) { return s, i }, nil, "g3(514, 'foo')", `["foo",514]`},
		{"g4", false, func(in ...any) []any { return in }, nil, "g4(515, 'foo')", `[515,"foo"]`},
		{"g5", false, func(s string, args ...any) string { return fmt.Sprintf(s, args...) }, nil, "g5('hello %v %q', 516, 'foo')", `hello 516 "foo"`},
		{"g6", false, func(in ...any) []any { return in }, nil, "g6()", `[]`},
		{"g7", false, func(s string, args ...any) string { return fmt.Sprintf(s, args...) }, nil, "g7('hello %v')", `hello %!v(MISSING)`},
		{"g8", true, func(this Value) (any, error) {
			this.SetProperty(foo, 522)
			return this.Any()
		}, nil, "var a = {foo: 5220, bar: g8}; a.bar();", `[{"foo":522},null]`},
		{"g9", false, func(v Value) (any, error) {
			v.SetProperty(foo, 526)
			return v.Any()
		}, nil, "var a = {foo: 5260, bar: g9}; g9(a);", `[{"foo":526},null]`},
		{"g10", true, func(this Value) Value {
			this.SetProperty(foo, 530)
			return this.Dup()
		}, nil, "var a = {foo: 5300, bar: g10}; a.bar();", `{"foo":530}`},
		{"g11", false, func(a Value) (int, Value) {
			a.SetProperty(foo, 533)
			return 534, a.Dup()
		}, nil, "var a = {foo: 42, bar: g11}; g11(a);", `[534,{"foo":533}]`},

		// A Go boolean/integer must be a boolean/number for Javascript itself,
		// not merely a value that decodes back to the Go original. Both
		// payloads live in the low 32 bits of the JSValue union, which on a big
		// endian target is not where its first four bytes are.
		{"g12", false, func() bool { return true }, nil, "g12() === true;", true},
		{"g13", false, func() bool { return false }, nil, "g13() === false;", true},
		{"g14", false, func() int { return 542 }, nil, "g14() === 542;", true},
	} {
		nm := test.nm
		if nm == "" {
			nm = fmt.Sprintf("f%v", i)
		}
		if err := m.RegisterFunc(nm, test.f, test.wantThis); err != nil {
			t.Errorf("registering function '%T(%[1]v)': %v", test, err)
			continue
		}

		call := test.call
		if call == "" {
			call = fmt.Sprintf("%s()", nm)
		}
		rv, err := m.Eval(call, EvalGlobal)
		t.Logf("%s: %T(%[2]v)", call, rv)
		if err != nil {
			t.Errorf("calling %s: err=%v", nm, err)
			continue
		}

		switch x := rv.(type) {
		case nil:
			if test.v != nil {
				t.Errorf("expected nil")
			}
		case Undefined, string, int, bool, float64:
			if g, e := x, test.v; g != e {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *big.Int:
			if g, e := x, big.NewInt(int64(test.v.(int))); g.Cmp(e) != 0 {
				t.Errorf("got %v, expected %v", g, e)
			}
		case *Object:
			if g, e := x.json, test.v; g != e {
				t.Errorf("%s: got %v, expected %v", nm, g, e)
			}
		default:
			panic(todo("%T", x))
		}
	}
}

func TestEvalValue(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	v, err := m.EvalValue("obj = {foo:42,bar:'baz'}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer v.Free()
}

func TestAtom(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	n, err := m.NewAtom("foo")
	t.Logf("n=%v err=%v", n, err)
	if n == lib.MJS_ATOM_NULL || err != nil {
		t.Fatal("FAIL")
	}

	n2, err := m.NewAtom("bar")
	t.Logf("n2=%v err=%v", n2, err)
	if n2 == lib.MJS_ATOM_NULL || n2 == n || err != nil {
		t.Fatal("FAIL")
	}
}

func TestSetPropertyValue(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.EvalValue("var obj = {}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	atom, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	n, err := m.NewString("bar")
	if err != nil {
		t.Fatal(err)
	}

	defer n.Free()

	if err := m.SetPropertyValue(obj, atom, n); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval("obj.foo;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "bar"; g != e {
		t.Fatal(g, e)
	}
}

func TestSetProperty(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.EvalValue("var obj = {}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	atom, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	if err := m.SetProperty(obj, atom, "bar"); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval("obj.foo;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "bar"; g != e {
		t.Fatal(g, e)
	}
}

func TestGetPropertyValue(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.EvalValue("var obj = {}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	atom, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	n, err := m.NewString("bar")
	if err != nil {
		t.Fatal(err)
	}

	defer n.Free()

	if err := m.SetPropertyValue(obj, atom, n); err != nil {
		t.Fatal(err)
	}

	v, err := m.GetPropertyValue(obj, atom)
	if err != nil {
		t.Fatal(err)
	}

	gv, err := m.value(v.v, true)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := gv, "bar"; g != e {
		t.Fatal(g, e)
	}
}

func TestGetProperty(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.EvalValue("var obj = {}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	atom, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	n, err := m.NewString("bar")
	if err != nil {
		t.Fatal(err)
	}

	defer n.Free()

	if err := m.SetPropertyValue(obj, atom, n); err != nil {
		t.Fatal(err)
	}

	v, err := m.GetProperty(obj, atom)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "bar"; g != e {
		t.Fatal(g, e)
	}
}

func TestBytecode(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	bytecode, err := m.Compile("42 * 314;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	// Hello future reader: this will change if you update the QuickJS version.
	// You just need to update the expected value below!
	// You can also use this chance to document that something changed.
	// See: https://gitlab.com/cznic/quickjs/-/merge_requests/3#note_2862847634
	//
	// 2026-07-27, QuickJS release 2026-06-04 (libquickjs v0.13.0): The
	// serialized function header gained a var_ref_count field, that's the extra
	// zero byte, and variable definitions now record var_ref_idx instead of
	// scope_level. Opcodes were renumbered, so the code of this script,
	// push_i8 42; push_i16 314; mul; set_loc0; return, is now encoded as
	// bb 2a bc 3a 01 98 cb 28, previously bd 2a be 3a 01 9a cd 28.
	hex := hex.EncodeToString(bytecode)
	if g, e := hex, "05000c000600a801000100020000000801aa01000000bb2abc3a0198cb28a8010400001b0600"; g != e {
		t.Fatalf("got %s, expected %s", g, e)
	}

	v, err := m.EvalBytecode(bytecode)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, 42*314; g != e {
		t.Fatal(g, e)
	}
}

func TestCustomModuleLoader(t *testing.T) {
	vm, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()

	// An in-memory filesystem for our mock modules
	modules := map[string]string{
		"math_module": `
			export function multiply(a, b) {
				return a * b;
			}
		`,
		"string_module": `
			export const greeting = "Hello from QuickJS!";
		`,
	}

	normalizeCalled := false
	loaderCalled := false

	vm.SetModuleLoader(
		func(m *VM, name string) (string, error) {
			loaderCalled = true
			if src, ok := modules[name]; ok {
				return src, nil
			}
			return "", fmt.Errorf("module not found: %s", name)
		},
		func(m *VM, baseName, name string) (string, error) {
			normalizeCalled = true
			// A real normalizer would resolve relative paths against baseName.
			// For this test, we just pass the requested name through.
			return name, nil
		},
	)

	// Happy Path: Successfully load and execute modules
	script := `
		import { multiply } from "math_module";
		import { greeting } from "string_module";

		// Export results to the global object so we can assert them from Go
		globalThis.testMathResult = multiply(6, 7);
		globalThis.testStringResult = greeting;
	`

	_, err = vm.Eval(script, EvalModule)
	if err != nil {
		t.Fatalf("EvalModule failed: %v", err)
	}

	if !normalizeCalled {
		t.Error("expected normalize callback to be called, but it was not")
	}

	if !loaderCalled {
		t.Error("expected loader callback to be called, but it was not")
	}

	// Verify the math module executed correctly
	mathRes, err := vm.Eval("testMathResult", EvalGlobal)
	if err != nil {
		t.Fatalf("failed to evaluate testMathResult: %v", err)
	}
	if v, ok := mathRes.(int); !ok || v != 42 {
		t.Errorf("expected math result 42, got %v (type %T)", mathRes, mathRes)
	}

	// Verify the string module executed correctly
	strRes, err := vm.Eval("testStringResult", EvalGlobal)
	if err != nil {
		t.Fatalf("failed to evaluate testStringResult: %v", err)
	}
	if v, ok := strRes.(string); !ok || v != "Hello from QuickJS!" {
		t.Errorf("expected string result 'Hello from QuickJS!', got %v", strRes)
	}

	// Error Path: Attempt to load a module that doesn't exist
	missingScript := `
		import { something } from "missing_module";
	`
	_, err = vm.Eval(missingScript, EvalModule)
	if err == nil {
		t.Fatal("expected error when loading missing module, got nil")
	}

	// Ensure the error message propagated properly from the Go loader to the JS Exception
	expectedErrFragment := "module not found: missing_module"
	if !strings.Contains(err.Error(), expectedErrFragment) && !strings.Contains(err.Error(), "module load failed") {
		t.Errorf("expected error to contain '%s', got: %v", expectedErrFragment, err)
	}

	// Teardown: Verify we can remove the module loader without crashing
	vm.SetModuleLoader(nil, nil)

	// FIX: Request a completely new module name so QuickJS doesn't use the cache
	_, err = vm.Eval(`import { multiply } from "new_missing_module";`, EvalModule)
	if err == nil {
		t.Fatal("expected error after removing custom module loader, got nil")
	}
}

func TestValueCall(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	f, err := m.EvalValue("(function(a, b) { return this.base + a + b; });", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Free()

	this, err := m.EvalValue("({base: 100});", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer this.Free()

	v, err := f.Call(this, 20, 3)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, 123; g != e {
		t.Fatal(g, e)
	}

	// 'this' is honored, not silently replaced by the global object.
	other, err := m.EvalValue("({base: 1000});", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer other.Free()

	if v, err = f.Call(other, 20, 3); err != nil {
		t.Fatal(err)
	}

	if g, e := v, 1023; g != e {
		t.Fatal(g, e)
	}

	// CallValue returns a Value, like EvalValue does.
	w, err := f.CallValue(this, 20, 3)
	if err != nil {
		t.Fatal(err)
	}

	defer w.Free()

	x, err := w.Any()
	if err != nil {
		t.Fatal(err)
	}

	if g, e := x, 123; g != e {
		t.Fatal(g, e)
	}

	// UndefinedValue is accepted as 'this'.
	h, err := m.EvalValue("(function(s) { return 'got ' + s; });", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer h.Free()

	if v, err = h.Call(UndefinedValue, "it"); err != nil {
		t.Fatal(err)
	}

	if g, e := v, "got it"; g != e {
		t.Fatal(g, e)
	}
}

func TestValueCallErrors(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	m2, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m2.Close()

	// A Value not associated with a VM, eg. UndefinedValue, reports an error
	// instead of panicking on the nil vm.
	if _, err := UndefinedValue.Call(UndefinedValue); err == nil {
		t.Error("expected an error calling a Value not associated with a VM")
	}

	if _, err := UndefinedValue.CallValue(UndefinedValue); err == nil {
		t.Error("expected an error calling a Value not associated with a VM")
	}

	obj, err := m.EvalValue("({});", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	if _, err := obj.Call(UndefinedValue); err == nil {
		t.Error("expected an error calling a non-function")
	}

	if _, err := obj.CallValue(UndefinedValue); err == nil {
		t.Error("expected an error calling a non-function")
	}

	// A reference counted 'this' from a different VM is rejected the same way
	// convertArgs rejects such an argument.
	f, err := m.EvalValue("(function() { return this.x; });", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Free()

	alien, err := m2.EvalValue("({x: 42});", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer alien.Free()

	if _, err := f.Call(alien); err == nil {
		t.Error("expected an error using a 'this' from a different VM")
	}

	if _, err := f.CallValue(alien); err == nil {
		t.Error("expected an error using a 'this' from a different VM")
	}
}

// TestValueCallPromise covers the use case Value.Call was added for: settling a
// promise from Go using the resolve/reject functions handed to a registered Go
// function. They outlive that function only because it retains them using Dup.
func TestValueCallPromise(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	var resolve, reject Value
	if err := m.RegisterFunc("goAsync", func(res, rej Value) {
		resolve = res.Dup()
		reject = rej.Dup()
	}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Eval(`
globalThis.out = 'pending';
new Promise(function(res, rej) { goAsync(res, rej); }).then(
	function(v) { globalThis.out = 'ok: ' + v; },
	function(e) { globalThis.out = 'err: ' + e; }
);
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	defer reject.Free()
	defer resolve.Free()

	if _, err := resolve.Call(UndefinedValue, "done"); err != nil {
		t.Fatal(err)
	}

	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval("globalThis.out;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "ok: done"; g != e {
		t.Fatal(g, e)
	}
}

func TestValueProperty(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	obj, err := m.EvalValue("var obj = {}; obj;", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	atom, err := m.NewAtom("foo")
	if err != nil {
		t.Fatal(err)
	}

	n, err := m.NewString("bar")
	if err != nil {
		t.Fatal(err)
	}

	defer n.Free()

	if err := m.SetPropertyValue(obj, atom, n); err != nil {
		t.Fatal(err)
	}

	v, err := obj.Property(atom)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "bar"; g != e {
		t.Fatal(g, e)
	}
}

// TestRegisterFuncNullArg covers passing Javascript null to a function registered
// by RegisterFunc. That used to panic in reflect for every parameter type except
// Value, and the panic leaked memory even when recovered, so there was no way to
// work around it. null now converts to a Go nil where nil is a valid value of the
// parameter type and is a type error otherwise, like undefined already was.
func TestRegisterFuncNullArg(t *testing.T) {
	type T struct{ A int }

	for i, test := range []struct {
		f        any
		wantThis bool
		call     string
		v        any
		err      string
	}{
		// nil is a valid value of the parameter type: null converts to it.
		{func(v any) bool { return v == nil }, false, "f(null)", true, ""},
		{func(v *T) bool { return v == nil }, false, "f(null)", true, ""},
		{func(v map[string]any) bool { return v == nil }, false, "f(null)", true, ""},
		{func(v []int) bool { return v == nil }, false, "f(null)", true, ""},
		{func(v error) bool { return v == nil }, false, "f(null)", true, ""},
		{func(v ...any) bool { return len(v) == 1 && v[0] == nil }, false, "f(null)", true, ""},
		{func(a, b any) bool { return a == nil && b == 42 }, false, "f(null, 42)", true, ""},

		// 'this' can be null as well, a registered function does not coerce it
		// to the global object.
		{func(this any) bool { return this == nil }, true, "f.call(null)", true, ""},

		// undefined stays distinct from null.
		{func(v any) bool { return v == Undefined{} }, false, "f(undefined)", true, ""},

		// nil is not a valid value of the parameter type: a catchable type
		// error, neither a silent zero value nor a panic.
		{func(string) {}, false, "f(null)", nil, "cannot assign null to string"},
		{func(int) {}, false, "f(null)", nil, "cannot assign null to int"},
		{func(bool) {}, false, "f(null)", nil, "cannot assign null to bool"},
		{func(float64) {}, false, "f(null)", nil, "cannot assign null to float64"},
		{func(T) {}, false, "f(null)", nil, "cannot assign null to"},
		{func(this string) {}, true, "f.call(null)", nil, "cannot assign null to string"},
	} {
		func() {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}

			defer m.Close()

			if err := m.RegisterFunc("f", test.f, test.wantThis); err != nil {
				t.Errorf("%v: %s: RegisterFunc: %v", i, test.call, err)
				return
			}

			v, err := m.Eval(test.call, EvalGlobal)
			switch {
			case test.err != "":
				if err == nil {
					t.Errorf("%v: %s: expected an error containing %q, got %v", i, test.call, test.err, v)
					return
				}

				if !strings.Contains(err.Error(), test.err) {
					t.Errorf("%v: %s: got error %q, expected it to contain %q", i, test.call, err, test.err)
				}
			default:
				if err != nil {
					t.Errorf("%v: %s: %v", i, test.call, err)
					return
				}

				if g, e := v, test.v; g != e {
					t.Errorf("%v: %s: got %v, expected %v", i, test.call, g, e)
				}
			}
		}()
	}
}

// TestRegisterHostFuncNullArg is TestRegisterFuncNullArg for the RegisterHostFunc
// shape, func(args []any) (any, error). Every argument lands in an any there, so
// null is always valid and must stay distinct from undefined.
func TestRegisterHostFuncNullArg(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	var got []any
	if err := m.RegisterHostFunc("h", func(args []any) (any, error) {
		got = args
		return len(args), nil
	}); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval("h(null, 42, undefined, null)", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, 4; g != e {
		t.Fatal(g, e)
	}

	if g, e := len(got), 4; g != e {
		t.Fatal(g, e)
	}

	if got[0] != nil || got[3] != nil {
		t.Errorf("null arguments did not arrive as nil: %#v", got)
	}

	if g, e := got[1], 42; g != e {
		t.Error(g, e)
	}

	if g, e := got[2], (Undefined{}); g != e {
		t.Errorf("undefined must stay distinct from null: got %#v, expected %#v", g, e)
	}
}

// TestRegisterFuncMapResult covers returning a Go map from a registered function.
// That used to fail with "internal error: map". Map values are converted one by
// one, the way slice elements are and unlike struct fields, which go through a
// JSON round trip that turns a *big.Int into a double and fails outright on a
// Value.
func TestRegisterFuncMapResult(t *testing.T) {
	big62 := new(big.Int).Lsh(big.NewInt(1), 62) // exceeds 2**53, unrepresentable as a double

	for i, test := range []struct {
		f    any
		call string
		v    any
		err  string
	}{
		{func() map[string]int { return map[string]int{"a": 1, "b": 2} }, "JSON.stringify(f())", `{"a":1,"b":2}`, ""},
		{func() map[string]int { return map[string]int{} }, "JSON.stringify(f())", "{}", ""},
		{func() any { return map[string]int{"a": 1} }, "JSON.stringify(f())", `{"a":1}`, ""},
		{func() map[string]map[string]int { return map[string]map[string]int{"in": {"q": 7}} }, "JSON.stringify(f())", `{"in":{"q":7}}`, ""},

		// A nil map is null, like a nil pointer and a nil interface are.
		{func() map[string]int { return nil }, "f() === null", true, ""},

		// A nil map value is null too.
		{func() map[string]any { return map[string]any{"z": nil} }, "f().z === null", true, ""},

		// Integer keys stringify the way they do in JSON.
		{func() map[int]string { return map[int]string{1: "x", -2: "y"} }, "JSON.stringify(f())", `{"1":"x","-2":"y"}`, ""},

		// Property order is observable in Javascript while Go map iteration order
		// is randomized, so keys are sorted. With six of them an unsorted result
		// would match by chance once in 720 runs.
		{
			func() map[string]int {
				return map[string]int{"e": 5, "b": 2, "d": 4, "a": 1, "c": 3, "f": 6}
			},
			"Object.keys(f()).join(',')", "a,b,c,d,e,f", "",
		},

		// Converted one by one: a *big.Int stays a BigInt and keeps its precision.
		// The same value in a struct field comes back as a lossy double.
		{func() map[string]any { return map[string]any{"n": big62} }, "typeof f().n + ' ' + String(f().n)", "bigint " + big62.String(), ""},

		// A key Javascript has no name for is a catchable error, not a panic.
		{func() map[[2]int]int { return map[[2]int]int{{1, 2}: 3} }, "f()", nil, "unsupported map key type"},
	} {
		func() {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}

			defer m.Close()

			if err := m.RegisterFunc("f", test.f, false); err != nil {
				t.Errorf("%v: %s: RegisterFunc: %v", i, test.call, err)
				return
			}

			v, err := m.Eval(test.call, EvalGlobal)
			switch {
			case test.err != "":
				if err == nil {
					t.Errorf("%v: %s: expected an error containing %q, got %v", i, test.call, test.err, v)
					return
				}

				if !strings.Contains(err.Error(), test.err) {
					t.Errorf("%v: %s: got error %q, expected it to contain %q", i, test.call, err, test.err)
				}
			default:
				if err != nil {
					t.Errorf("%v: %s: %v", i, test.call, err)
					return
				}

				if g, e := v, test.v; g != e {
					t.Errorf("%v: %s: got %v, expected %v", i, test.call, g, e)
				}
			}
		}()
	}
}

// TestRegisterFuncMapValueResult is TestRegisterFuncMapResult for a Value in a
// map. It stays a live reference, so a function put in a map is still callable
// from Javascript. Like jsArray, jsMap takes over the reference it is given, so
// the Go function has to Dup per call.
func TestRegisterFuncMapValueResult(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	fn, err := m.EvalValue("(function() { return 'called'; });", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer fn.Free()

	if err := m.RegisterFunc("f", func() map[string]any {
		return map[string]any{"cb": fn.Dup()}
	}, false); err != nil {
		t.Fatal(err)
	}

	// Called more than once to show the per-call Dup keeps 'fn' usable.
	v, err := m.Eval("typeof f().cb + ' ' + f().cb() + ' ' + f().cb()", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := v, "function called called"; g != e {
		t.Fatal(g, e)
	}
}
