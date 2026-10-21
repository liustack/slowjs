// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// defineBuiltin makes a function of b on m and puts it on the global object
// as name.
func defineBuiltin(t testing.TB, m *VM, name string, b *Builtin) {
	t.Helper()
	f, err := m.NewBuiltinFunction(b)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Free()

	set, err := m.EvalValue(`(name, f) => { globalThis[name] = f }`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer set.Free()

	if _, err := set.Call(UndefinedValue, name, f); err != nil {
		t.Fatal(err)
	}
}

// evalString evaluates js on m and returns its result as a string.
func evalString(t testing.TB, m *VM, js string) string {
	t.Helper()
	v, err := m.Eval(js, EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", js, err)
	}

	return fmt.Sprint(v)
}

var (
	builtinNothing = NewBuiltin("frob", 2, func(a Args) (Value, error) { return UndefinedValue, nil })

	// builtinEcho returns its argument i, borrowed, as it is: a Builtin owns
	// what it returns, so it returns a copy.
	builtinEcho = NewBuiltin("echo", 2, func(a Args) (Value, error) {
		i, _ := a.Number(1)
		return a.Value(int(i)).Dup(), nil
	})

	// builtinDescribe reports what Args make of each argument and of this.
	builtinDescribe = NewBuiltin("describe", 0, func(a Args) (Value, error) {
		var b strings.Builder
		describe := func(i int) {
			n, isNumber := a.Number(i)
			s, isString := a.String(i)
			v, isBool := a.Bool(i)
			fmt.Fprintf(&b, "[%v %v|%q %v|%v %v|%v]", n, isNumber, s, isString, v, isBool, a.Value(i).IsUndefined())
		}
		fmt.Fprintf(&b, "%d", a.Len())
		for i := -1; i <= a.Len(); i++ {
			describe(i)
		}
		this, _ := a.This().Any()
		fmt.Fprintf(&b, " this=%v", this)
		return a.VM().NewString(b.String())
	})

	builtinString = NewBuiltin("str", 1, func(a Args) (Value, error) {
		s, _ := a.String(0)
		return a.VM().NewString(s + "\x80")
	})

	builtinThrow = NewBuiltin("fail", 2, func(a Args) (Value, error) {
		name, _ := a.String(0)
		msg, _ := a.String(1)
		switch name {
		case "plain":
			return Value{}, errors.New(msg)
		case "wrapped":
			return Value{}, fmt.Errorf("wrapped: %w", &Error{Name: "RangeError", Message: msg})
		}
		return Value{}, &Error{Name: name, Message: msg}
	})
)

// A function made of a Builtin is shaped like the engine's own built-ins, such
// as Math.max: a name and a length and nothing else of its own, not a
// constructor, and a toString that shows no source.
func TestBuiltinShape(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "frob", builtinNothing)
	for _, c := range [][2]string{
		{`JSON.stringify(Object.getOwnPropertyNames(frob))`, `JSON.stringify(Object.getOwnPropertyNames(Math.max))`},
		{`JSON.stringify(Object.getOwnPropertyDescriptor(frob, 'name'))`, `JSON.stringify({...Object.getOwnPropertyDescriptor(Math.max, 'name'), value: 'frob'})`},
		{`JSON.stringify(Object.getOwnPropertyDescriptor(frob, 'length'))`, `JSON.stringify(Object.getOwnPropertyDescriptor(Math.max, 'length'))`},
		{`Object.getPrototypeOf(frob) === Function.prototype`, `Object.getPrototypeOf(Math.max) === Function.prototype`},
		{`String(frob)`, `String(Math.max).replace('max', 'frob')`},
		{`try { new frob(); 'constructed' } catch (e) { e.constructor.name }`, `try { new Math.max(); 'constructed' } catch (e) { e.constructor.name }`},
		{`typeof frob`, `typeof Math.max`},
	} {
		if got, want := evalString(t, m, c[0]), evalString(t, m, c[1]); got != want {
			t.Errorf("%s\n\tgot  %s\n\twant %s", c[0], got, want)
		}
	}
	// The anonymous ones, like the bound format of ECMA-402, have the empty
	// name.
	defineBuiltin(t, m, "anon", NewBuiltin("", 1, func(Args) (Value, error) { return UndefinedValue, nil }))
	if got := evalString(t, m, `JSON.stringify([anon.name, anon.length, String(anon)])`); got != `["",1,"function () {\n    [native code]\n}"]` {
		t.Errorf("got %s", got)
	}
}

// Args read the arguments as they are, with no conversion: a number only
// from a number, a string only from a string, and nothing past the last one.
func TestBuiltinArgs(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "describe", builtinDescribe)
	defineBuiltin(t, m, "echo", builtinEcho)
	for _, c := range [][2]string{
		{`describe()`, `0[0 false|"" false|false false|true][0 false|"" false|false false|true] this=undefined`},
		{`describe(1, 2.5, -0, NaN)`, `4[0 false|"" false|false false|true][1 true|"" false|false false|false][2.5 true|"" false|false false|false]` +
			`[-0 true|"" false|false false|false][NaN true|"" false|false false|false][0 false|"" false|false false|true] this=undefined`},
		{`describe('a', 'b' + 'c'.repeat(300).slice(0, 2), 'é😀', '\ud800x')`, `4[0 false|"" false|false false|true][0 false|"a" true|false false|false]` +
			`[0 false|"bcc" true|false false|false][0 false|"é😀" true|false false|false][0 false|"�x" true|false false|false][0 false|"" false|false false|true] this=undefined`},
		{`describe(true, false, '1', new Number(1), undefined)`, `5[0 false|"" false|false false|true][0 false|"" false|true true|false][0 false|"" false|false true|false]` +
			`[0 false|"1" true|false false|false][0 false|"" false|false false|false][0 false|"" false|false false|true][0 false|"" false|false false|true] this=undefined`},
		{`describe.call('me')`, `0[0 false|"" false|false false|true][0 false|"" false|false false|true] this=me`},
		{`const o = {}; echo(o, 0) === o`, `true`},
		{`echo('x', 7) === undefined`, `true`},
	} {
		if got := evalString(t, m, c[0]); got != c[1] {
			t.Errorf("%s\n\tgot  %s\n\twant %s", c[0], got, c[1])
		}
	}
}

// A string a Builtin returns reaches javascript as NewString makes it: invalid
// UTF-8 as U+FFFD.
func TestBuiltinString(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "str", builtinString)
	long := strings.Repeat("长", 5000)
	for _, s := range []string{"", "ascii", "中文 😀", long} {
		got := evalString(t, m, fmt.Sprintf(`JSON.stringify(str(%q))`, s))
		if want := fmt.Sprintf("%q", s+"�"); got != want && got != strings.ReplaceAll(want, `�`, "�") {
			t.Errorf("str(%.20q): got %.40s, want %.40s", s, got, want)
		}
	}
}

// WTF8 gives a string as the engine holds it: a lone surrogate as the three
// bytes of its code point, which is not UTF-8, so that a Builtin can tell
// '\ud800' from '\ufffd' and from '\udc00', and a pair as its one character.
func TestBuiltinWTF8(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "bytes", NewBuiltin("bytes", 1, func(a Args) (Value, error) {
		s, ok := a.WTF8(0)
		return a.VM().NewString(fmt.Sprintf("%v %x", ok, s))
	}))
	for _, c := range [][2]string{
		{`bytes('')`, `true `},
		{`bytes('a\0b')`, `true 610062`},
		{`bytes('\ud800x')`, `true eda08078`},
		{`bytes('x\udc00')`, `true 78edb080`},
		{`bytes('\udbff\udfff')`, `true f48fbfbf`},
		{`bytes('\ufffd')`, `true efbfbd`},
		{`bytes('\ud83d' + '\ude00')`, `true f09f9880`},
		{`bytes(1)`, `false `},
		{`bytes(new String('a'))`, `false `},
	} {
		if got := evalString(t, m, c[0]); got != c[1] {
			t.Errorf("%s: got %s, want %s", c[0], got, c[1])
		}
	}
}

// An error a Builtin returns is thrown: an *Error as the constructor its Name
// says, if javascript has that one, and anything else as a TypeError, as
// RegisterHostFunc throws it.
func TestBuiltinErrors(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "fail", builtinThrow)
	for _, c := range [][2]string{
		{`'RangeError', 'too far'`, "RangeError true too far"},
		{`'TypeError', 'wrong'`, "TypeError true wrong"},
		{`'SyntaxError', 'bad'`, "SyntaxError true bad"},
		{`'ReferenceError', 'who'`, "ReferenceError true who"},
		{`'InternalError', 'oops'`, "InternalError true oops"},
		{`'Error', 'plain error'`, "Error true plain error"},
		{`'NoSuchError', 'odd'`, "Error true odd"},
		{`'plain', 'from Go'`, "TypeError true from Go"},
		{`'wrapped', 'inside'`, "RangeError true inside"},
	} {
		js := fmt.Sprintf(`try { fail(%s); 'no error' } catch (e) { [e.constructor.name, e instanceof globalThis[e.constructor.name], e.message].join(' ') }`, c[0])
		if got := evalString(t, m, js); got != c[1] {
			t.Errorf("fail(%s): got %q, want %q", c[0], got, c[1])
		}
	}
	// The error has a stack, as the engine's own errors do, which starts at
	// the Builtin whatever the error's name.
	if got := evalString(t, m, `function f() { fail('RangeError', 'x') } try { f() } catch (e) { e.stack.includes('at f') }`); got != "true" {
		t.Errorf("no stack: %s", got)
	}
	// The frames up to stack, which every call has alike.
	evalString(t, m, `function stack(name) { try { fail(name, 'x') } catch (e) { return e.stack.split('\n').slice(0, 2).join('\n') } }`)
	for _, name := range []string{"Error", "", "NoSuchError", "RangeError"} {
		js := fmt.Sprintf(`stack(%q) === stack('TypeError') && stack(%[1]q).startsWith('    at fail (native)')`, name)
		if got := evalString(t, m, js); got != "true" {
			t.Errorf("%s: %s", js, evalString(t, m, fmt.Sprintf(`stack(%q)`, name)))
		}
	}
	// An Error is an Error, with its whole message: the engine cuts only
	// those of the other constructors.
	long := strings.Repeat("ab", 200)
	js := fmt.Sprintf(`try { fail('', %q) } catch (e) { [Object.getPrototypeOf(e) === Error.prototype, e.constructor === Error, e.message.length, Object.getOwnPropertyNames(e).join()].join(' ') }`, long)
	if got := evalString(t, m, js); got != "true true 400 message,stack" {
		t.Errorf("got %s", got)
	}
}

// An Error made in Go reads as javascript's Error.prototype.toString writes
// it.
func TestErrorString(t *testing.T) {
	for _, c := range []struct {
		e    Error
		want string
	}{
		{Error{Message: "x"}, "Error: x"},
		{Error{Name: "RangeError", Message: "too far"}, "RangeError: too far"},
		{Error{Name: "TypeError"}, "TypeError"},
		{Error{}, "Error"},
	} {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.e, got, c.want)
		}
	}
}

// The zero Value a Builtin returns is undefined, and a Value returned with an
// error is freed (TestMemgrind).
func TestBuiltinZeroAndError(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "zero", NewBuiltin("zero", 0, func(Args) (Value, error) { return Value{}, nil }))
	defineBuiltin(t, m, "both", NewBuiltin("both", 0, func(a Args) (Value, error) {
		v, err := a.VM().NewString(strings.Repeat("x", 100))
		if err != nil {
			return Value{}, err
		}

		return v, errors.New("and an error")
	}))
	for _, c := range [][2]string{
		{`zero() === undefined && typeof zero() === 'undefined'`, "true"},
		{`try { both(); 'returned' } catch (e) { e.constructor.name + ': ' + e.message }`, "TypeError: and an error"},
	} {
		if got := evalString(t, m, c[0]); got != c[1] {
			t.Errorf("%s: got %s, want %s", c[0], got, c[1])
		}
	}
}

// A Value of another VM is not returned: the engine would free it in the
// wrong runtime. Nor is it freed, which would use the other VM, maybe on
// another goroutine: it stays its owner's. A number counts too.
func TestBuiltinValueOfAnotherVM(t *testing.T) {
	other := newTestVM(t)
	s, err := other.NewString("not mine")
	if err != nil {
		t.Fatal(err)
	}

	defer s.Free()

	m := newTestVM(t)
	defineBuiltin(t, m, "theirs", NewBuiltin("theirs", 0, func(Args) (Value, error) { return s, nil }))
	defineBuiltin(t, m, "theirNumber", NewBuiltin("theirNumber", 0, func(Args) (Value, error) { return other.NewInt(1), nil }))
	for _, f := range []string{"theirs", "theirNumber"} {
		if got := evalString(t, m, `try { `+f+`(); 'returned' } catch (e) { e.constructor.name }`); got != "TypeError" {
			t.Errorf("%s: got %s", f, got)
		}
	}
	if got, err := s.Any(); err != nil || got != "not mine" {
		t.Errorf("the Value of the other VM is now %q, %v", got, err)
	}
}

// A Builtin belongs to no VM: every VM makes its functions of it, and Args
// say which VM called. VMs on several goroutines call it at once.
func TestBuiltinAcrossVMs(t *testing.T) {
	var mu sync.Mutex
	seen := map[*VM]int{}
	b := NewBuiltin("count", 0, func(a Args) (Value, error) {
		mu.Lock()
		seen[a.VM()]++
		n := seen[a.VM()]
		mu.Unlock()
		return a.VM().NewInt(n), nil
	})
	const vms, calls = 8, 200
	var wg sync.WaitGroup
	made := make([]*VM, vms)
	for i := range vms {
		m := newTestVM(t)
		made[i] = m
		defineBuiltin(t, m, "count", b)
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := m.Eval(fmt.Sprintf(`let n; for (let i = 0; i < %d; i++) n = count(); n`, calls), EvalGlobal)
			if err != nil || v != calls {
				t.Errorf("got %v, %v, want %d", v, err, calls)
			}
		}()
	}
	wg.Wait()
	for _, m := range made {
		if seen[m] != calls {
			t.Errorf("a VM was called %d times, want %d", seen[m], calls)
		}
	}
}

// A VM closed gives its place to the next one, and the functions of a
// Builtin find the VM that calls them.
func TestBuiltinManyVMs(t *testing.T) {
	b := NewBuiltin("id", 0, func(a Args) (Value, error) { return a.This().Dup(), nil })
	for i := range 3000 {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		defineBuiltin(t, m, "id", b)
		if got := evalString(t, m, fmt.Sprintf(`id.call(%d)`, i)); got != fmt.Sprint(i) {
			t.Fatalf("got %s, want %d", got, i)
		}
		m.Close()
	}
}

// A panic in a Builtin goes up to the Go code that called javascript, as one
// in a function of RegisterHostFunc does, and no catch sees it.
func TestBuiltinPanic(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "boom", NewBuiltin("boom", 0, func(Args) (Value, error) { panic("boom") }))
	p := evalRecover(m, `globalThis.caught = false; try { boom() } catch (e) { caught = true }`)
	if p != "boom" {
		t.Fatalf("got panic %v, want boom", p)
	}
	if got := evalString(t, m, `caught`); got != "false" {
		t.Fatal("page code caught the panic")
	}
	wantTimeoutWorks(t, m)
}

// An interrupt in javascript a Builtin evaluates ends the call that got there,
// whatever the Builtin returns.
func TestBuiltinUncatchable(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "nested", NewBuiltin("nested", 0, func(a Args) (Value, error) {
		a.VM().Interrupt()
		_, err := a.VM().Eval(`for (;;);`, EvalGlobal)
		return a.VM().NewString(fmt.Sprint(err))
	}))
	defineBuiltin(t, m, "nestedError", NewBuiltin("nestedError", 0, func(a Args) (Value, error) {
		a.VM().Interrupt()
		_, err := a.VM().Eval(`for (;;);`, EvalGlobal)
		return Value{}, err
	}))
	defineBuiltin(t, m, "nestedOtherError", NewBuiltin("nestedOtherError", 0, func(a Args) (Value, error) {
		a.VM().Interrupt()
		a.VM().Eval(`for (;;);`, EvalGlobal)
		return Value{}, &Error{Name: "RangeError", Message: "not this one"}
	}))
	for _, f := range []string{"nested", "nestedError", "nestedOtherError"} {
		_, err := m.Eval(`globalThis.seen = []; try { `+f+`() } catch (e) { seen.push('catch') }`, EvalGlobal)
		wantError(t, err, "InternalError", "interrupted")
		wantKind(t, err, interrupted)
		if got := evalString(t, m, `seen.join()`); got != "" {
			t.Fatalf("%s: page code saw the interrupt: %s", f, got)
		}
	}
}

func TestNewBuiltinPanics(t *testing.T) {
	ok := func(Args) (Value, error) { return UndefinedValue, nil }
	for _, c := range []struct {
		name   string
		length int
		fn     func(Args) (Value, error)
	}{
		{"nul\x00", 0, ok},
		{"long", 256, ok},
		{"negative", -1, ok},
		{"nil", 0, nil},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewBuiltin(%q, %d) did not panic", c.name, c.length)
				}
			}()
			NewBuiltin(c.name, c.length, c.fn)
		}()
	}
}

func BenchmarkBuiltinCall(b *testing.B) {
	m, err := NewVM()
	if err != nil {
		b.Fatal(err)
	}

	defer m.Close()

	defineBuiltin(b, m, "add", NewBuiltin("add", 2, func(a Args) (Value, error) {
		x, _ := a.Number(0)
		y, _ := a.Number(1)
		return a.VM().NewFloat64(x + y), nil
	}))
	defineBuiltin(b, m, "hello", NewBuiltin("hello", 1, func(a Args) (Value, error) {
		s, _ := a.String(0)
		return a.VM().NewString("hello " + s)
	}))
	if err := m.RegisterHostFunc("addHost", func(args []any) (any, error) {
		return args[0].(int) + args[1].(int), nil
	}); err != nil {
		b.Fatal(err)
	}

	for _, c := range []struct{ name, body string }{
		{"number", `add(i, 1)`},
		{"string", `hello('world')`},
		{"RegisterHostFunc", `addHost(i, 1)`},
	} {
		b.Run(c.name, func(b *testing.B) {
			if _, err := m.Eval(`function run(n) { for (let i = 0; i < n; i++) `+c.body+` }`, EvalGlobal); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			if _, err := m.Eval(fmt.Sprintf(`run(%d)`, b.N), EvalGlobal); err != nil {
				b.Fatal(err)
			}
		})
	}
}

// NewArray makes an array as the literal ['a', 'b', ...] makes one: own,
// writable, enumerable, configurable elements, Array.prototype its prototype,
// and no setter a page put on Array.prototype called. It borrows its
// elements. The expected results are what Node 24 gives for the literal.
func TestNewArray(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "pair", NewBuiltin("pair", 2, func(a Args) (Value, error) {
		x, _ := a.String(0)
		y, _ := a.String(1)
		vx, err := a.VM().NewString(x)
		if err != nil {
			return Value{}, err
		}

		defer vx.Free()

		vy, err := a.VM().NewString(y)
		if err != nil {
			return Value{}, err
		}

		defer vy.Free()

		return a.VM().NewArray(vx, vy, a.Value(1), UndefinedValue)
	}))
	defineBuiltin(t, m, "empty", NewBuiltin("empty", 0, func(a Args) (Value, error) { return a.VM().NewArray() }))
	defineBuiltin(t, m, "zeros", NewBuiltin("zeros", 0, func(a Args) (Value, error) { return a.VM().NewArray(Value{}, a.VM().NewInt(0)) }))
	for _, test := range [][2]string{
		{`JSON.stringify(pair('a', 'b'))`, `["a","b","b",null]`},
		{`pair('a', 'b').length`, "4"},
		{`Array.isArray(pair()) && Object.getPrototypeOf(pair()) === Array.prototype`, "true"},
		{`JSON.stringify(Object.getOwnPropertyDescriptor(pair('a', 'b'), 1))`, `{"value":"b","writable":true,"enumerable":true,"configurable":true}`},
		{`Object.keys(pair('a', 'b')).join()`, "0,1,2,3"},
		{`3 in pair()`, "true"},
		{`empty().length`, "0"},
		{`const z = zeros(); typeof z[0] + ' ' + typeof z[1] + ' ' + z.length`, "undefined number 2"},
		{`let n = 0; Object.defineProperty(Array.prototype, 0, {set() { n++ }, configurable: true}); const r = pair('x', 'y'); delete Array.prototype[0]; n + ' ' + r[0]`, "0 x"},
	} {
		if got := evalString(t, m, test[0]); got != test[1] {
			t.Errorf("%s: got %s, want %s", test[0], got, test[1])
		}
	}
}

// NewArray refuses a Value of another VM, a number too, and leaves what it
// was given to its owners.
func TestNewArrayValueOfAnotherVM(t *testing.T) {
	other := newTestVM(t)
	m := newTestVM(t)
	theirs, err := other.NewString("not mine")
	if err != nil {
		t.Fatal(err)
	}

	defer theirs.Free()

	mine, err := m.NewString("mine")
	if err != nil {
		t.Fatal(err)
	}

	defer mine.Free()

	for _, v := range []Value{theirs, other.NewInt(1)} {
		if _, err := m.NewArray(mine, v); err == nil {
			t.Fatal("NewArray took a Value of another VM")
		}
	}
	for v, want := range map[*Value]string{&theirs: "not mine", &mine: "mine"} {
		if got, err := v.Any(); err != nil || got != want {
			t.Errorf("got %q, %v, want %q", got, err, want)
		}
	}
}
