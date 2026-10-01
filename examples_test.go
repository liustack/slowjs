// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "modernc.org/quickjs"

// https://sangupta.com/tech/parsing-typescript-in-go-with-quickjs.html

import (
	"fmt"
	"time"
)

// JSON marshalling.
func ExampleObject_MarshalJSON() {
	m, _ := NewVM()
	defer m.Close()
	obj, _ := m.Eval("obj = {a: 42+314, b: 'foo'}; obj;", EvalGlobal)
	s, _ := (obj.(*Object).MarshalJSON())
	fmt.Printf("%s\n", s)
	// Output:
	// {"a":356,"b":"foo"}
}

// JSON marshalling.
func ExampleObject_String() {
	m, _ := NewVM()
	defer m.Close()
	obj, _ := m.Eval("obj = {a: 42+314, b: 'foo'}; obj;", EvalGlobal)
	fmt.Printf("%s\n", obj)
	// Output:
	// {"a":356,"b":"foo"}
}

// JSON unmarshalling into native Go struct.
func ExampleObject_Into() {
	m, _ := NewVM()
	defer m.Close()
	obj, _ := m.Eval("obj = {a: 42+314, b: 'foo'}; obj;", EvalGlobal)
	var dst struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	obj.(*Object).Into(&dst)
	fmt.Printf("%#v\n", dst)
	// Output:
	// struct { A int "json:\"a\""; B string "json:\"b\"" }{A:356, B:"foo"}
}

// Call a Javascript function.
func ExampleVM_Call_function() {
	m, _ := NewVM()
	defer m.Close()
	fmt.Println(m.Call("parseInt", "1234"))
	// Output:
	// 1234 <nil>
}

// Call a Javascript method.
func ExampleVM_Call_method() {
	m, _ := NewVM()
	defer m.Close()
	fmt.Println(m.Call("Math.abs", -1234))
	// Output:
	// 1234 <nil>
}

// Getting exception.
func ExampleVM_Eval_exception() {
	m, _ := NewVM()
	defer m.Close()
	fmt.Println(m.Eval("throw new Error('failed');", EvalGlobal))
	// Output:
	// <nil> Error: failed
}

// Evaluate a simple Javascript expression.
func ExampleVM_Eval_expression() {
	m, _ := NewVM()
	defer m.Close()
	fmt.Println(m.Eval("1+2", EvalGlobal))
	// Output:
	// 3 <nil>
}

// Object example.
func ExampleVM_Eval_object() {
	m, _ := NewVM()
	defer m.Close()
	fmt.Println(m.Eval("obj = {a: 42+314, b: 'foo'}; obj;", EvalGlobal))
	// Output:
	// {"a":356,"b":"foo"} <nil>
}

// Call error returning Go function from Javascript.
func ExampleVM_RegisterFunc_error() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(a int) error {
		if a < 0 {
			return fmt.Errorf("negative")
		}
		return nil
	}, false)
	fmt.Println(m.Eval("gofunc(-1)", EvalGlobal))
	fmt.Println(m.Eval("gofunc(1)", EvalGlobal))
	// Output:
	// negative <nil>
	// <nil> <nil>
}

// Call multiple return Go function from Javascript.
func ExampleVM_RegisterFunc_multipleReturn() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(a, b int) (int, int) { return 10 * a, 100 * b }, false)
	fmt.Println(m.Eval("gofunc(2, 3)", EvalGlobal))
	// Output:
	// [20,300] <nil>
}

// Call single return Go function from Javascript.
func ExampleVM_RegisterFunc_singleReturn() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(a, b, c int) int { return a + b*c }, false)
	fmt.Println(m.Eval("gofunc(2, 3, 5)", EvalGlobal))
	// Output:
	// 17 <nil>
}

// Call void Go function from Javascript.
func ExampleVM_RegisterFunc_void() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(a, b, c int) { fmt.Println(a + b*c) }, false)
	fmt.Println(m.Eval("gofunc(2, 3, 5)", EvalGlobal))
	// Output:
	// 17
	// undefined <nil>
}

// Passing undefined Javascript 'this' to a Go function.
func ExampleVM_RegisterFunc_thisNull() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(this any) any { return this }, true)
	fmt.Println(m.Eval("gofunc()", EvalGlobal))
	// Output:
	// undefined <nil>
}

// Passing undefined Javascript 'this' to a Go function.
func ExampleVM_RegisterFunc_thisNull2() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(this any, n int) (any, int) { return this, 10 * n }, true)
	fmt.Println(m.Eval("gofunc(42)", EvalGlobal))
	// Output:
	// [null,420] <nil>
}

// Passing Javascript 'this' to a Go function.
func ExampleVM_RegisterFunc_thisNonNull() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(this any) any { return this }, true)
	fmt.Println(m.Eval("var obj = { foo: 314, method: gofunc }; obj.method()", EvalGlobal))
	// Output:
	// {"foo":314} <nil>
}

// Passing Javascript 'this' to a Go function.
func ExampleVM_RegisterFunc_thisNonNull2() {
	m, _ := NewVM()
	defer m.Close()
	m.RegisterFunc("gofunc", func(this any, n int) (any, int) { return this, 10 * n }, true)
	fmt.Println(m.Eval("var obj = { foo: 314, method: gofunc }; obj.method(42)", EvalGlobal))
	// Output:
	// [{"foo":314},420] <nil>
}

// Enabling the module loader.
func ExampleVM_SetDefaultModuleLoader() {
	m, _ := NewVM()
	defer m.Close()
	m.SetDefaultModuleLoader()
	// testdata/power.js:
	//  export const name = "Power";
	//
	//  export function square(x) {
	//  	return x*x;
	//  }
	//
	//  export function cube(x) {
	//  	return x*x*x;
	//  }
	m.Eval("import * as Power from './testdata/power.js'; globalThis.Power = Power;", EvalModule)
	fmt.Println(m.Eval("[Power.square(2), Power.cube(2)];", EvalGlobal))

	// Output:
	// [4,8] <nil>
}

func ExampleVM_SetEvalTimeout() {
	m, _ := NewVM()
	defer m.Close()

	for _, timeout := range []time.Duration{100 * time.Millisecond, time.Second, 3 * time.Second} {
		m.SetEvalTimeout(timeout)
		t0 := time.Now()
		r, err := m.Eval(`
function f() {
	var sink;
	for (var i = 0; i < 10000; i++) {
		sink += 42;
		sink -= 42;
	}
}

(function() {
	for (var i = 0; i < 10000; i++) {
		f();
	}
	return 42;
})();
`, EvalGlobal)
		d := time.Since(t0)
		min := timeout / 2
		max := timeout * 3 / 2
		fmt.Println(r, err, timeout, d >= min && d <= max)
	}

	// Output:
	// <nil> InternalError: interrupted 100ms true
	// <nil> InternalError: interrupted 1s true
	// <nil> InternalError: interrupted 3s true
}

func ExampleVM_Interrupt() {
	const timeout = time.Second
	m, _ := NewVM()
	defer m.Close()

	go func() {
		time.Sleep(timeout)
		m.Interrupt()
	}()

	t0 := time.Now()
	r, err := m.Eval(`
function f() {
	var sink;
	for (var i = 0; i < 10000; i++) {
		sink += 42;
		sink -= 42;
	}
}

(function() {
	for (var i = 0; i < 10000; i++) {
		f();
	}
	return 42;
})();
`, EvalGlobal)
	d := time.Since(t0)
	step := timeout / 5
	d = d / step * step
	fmt.Println(r, err, d)

	// Output:
	// <nil> InternalError: interrupted 1s
}
