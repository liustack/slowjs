// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/beastjs"
)

// newVM is NewVM for an example, which stops at an error.
func newVM() *beastjs.VM {
	m, err := beastjs.NewVM()
	if err != nil {
		panic(err)
	}
	return m
}

// object is the result of an object literal, as the examples of Object use it.
func object() *beastjs.Object {
	m := newVM()
	defer m.Close()
	v, err := m.Eval(`({a: 42 + 314, b: 'foo'})`, beastjs.EvalGlobal)
	if err != nil {
		panic(err)
	}
	return v.(*beastjs.Object)
}

func ExampleObject_MarshalJSON() {
	b, err := object().MarshalJSON()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	// Output:
	// {"a":356,"b":"foo"}
}

func ExampleObject_String() {
	fmt.Printf("%s\n", object())
	// Output:
	// {"a":356,"b":"foo"}
}

func ExampleObject_Into() {
	var s struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	if err := object().Into(&s); err != nil {
		panic(err)
	}
	fmt.Printf("%#v\n", s)
	// Output:
	// struct { A int "json:\"a\""; B string "json:\"b\"" }{A:356, B:"foo"}
}

func ExampleVM_Call_function() {
	m := newVM()
	defer m.Close()
	fmt.Println(m.Call("parseInt", "1234"))
	// Output:
	// 1234 <nil>
}

func ExampleVM_Call_method() {
	m := newVM()
	defer m.Close()
	fmt.Println(m.Call("Math.abs", -1234))
	// Output:
	// 1234 <nil>
}

func ExampleVM_Eval_exception() {
	m := newVM()
	defer m.Close()
	fmt.Println(m.Eval(`throw new Error('failed')`, beastjs.EvalGlobal))
	// Output:
	// <nil> Error: failed
}

func ExampleVM_Eval_expression() {
	m := newVM()
	defer m.Close()
	fmt.Println(m.Eval(`1 + 2`, beastjs.EvalGlobal))
	// Output:
	// 3 <nil>
}

func ExampleVM_Eval_object() {
	m := newVM()
	defer m.Close()
	fmt.Println(m.Eval(`({a: 42 + 314, b: 'foo'})`, beastjs.EvalGlobal))
	// Output:
	// {"a":356,"b":"foo"} <nil>
}

// A Go error a function returns is a string to javascript, not a throw.
func ExampleVM_RegisterFunc_error() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("check", func(a int) error {
		if a < 0 {
			return errors.New("negative")
		}
		return nil
	}, false); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`check(-1)`, beastjs.EvalGlobal))
	fmt.Println(m.Eval(`check(1)`, beastjs.EvalGlobal))
	// Output:
	// negative <nil>
	// <nil> <nil>
}

// Several results are an array.
func ExampleVM_RegisterFunc_multipleReturn() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("scale", func(a, b int) (int, int) { return 10 * a, 100 * b }, false); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`scale(2, 3)`, beastjs.EvalGlobal))
	// Output:
	// [20,300] <nil>
}

func ExampleVM_RegisterFunc_singleReturn() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("madd", func(a, b, c int) int { return a + b*c }, false); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`madd(2, 3, 5)`, beastjs.EvalGlobal))
	// Output:
	// 17 <nil>
}

// A function with no result returns undefined.
func ExampleVM_RegisterFunc_void() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("show", func(a, b, c int) { fmt.Println(a + b*c) }, false); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`show(2, 3, 5)`, beastjs.EvalGlobal))
	// Output:
	// 17
	// undefined <nil>
}

// Called on its own, a function that wants this gets undefined.
func ExampleVM_RegisterFunc_thisNull() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("gofunc", func(this any) any { return this }, true); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`gofunc()`, beastjs.EvalGlobal))
	// Output:
	// undefined <nil>
}

// Undefined in an array is null in its JSON.
func ExampleVM_RegisterFunc_thisNull2() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("gofunc", func(this any, a int) (any, int) { return this, 10 * a }, true); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`gofunc(42)`, beastjs.EvalGlobal))
	// Output:
	// [null,420] <nil>
}

// Called as a method, it gets the object, here as its JSON.
func ExampleVM_RegisterFunc_thisNonNull() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("gofunc", func(this any) any { return this }, true); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`({foo: 314, gofunc}).gofunc()`, beastjs.EvalGlobal))
	// Output:
	// {"foo":314} <nil>
}

func ExampleVM_RegisterFunc_thisNonNull2() {
	m := newVM()
	defer m.Close()
	if err := m.RegisterFunc("gofunc", func(this any, a int) (any, int) { return this, 10 * a }, true); err != nil {
		panic(err)
	}
	fmt.Println(m.Eval(`({foo: 314, gofunc}).gofunc(42)`, beastjs.EvalGlobal))
	// Output:
	// [{"foo":314},420] <nil>
}

// Each call has the whole timeout, and a loop that never ends stops when it
// runs out.
func ExampleVM_SetEvalTimeout() {
	m := newVM()
	defer m.Close()
	for _, d := range []time.Duration{100 * time.Millisecond, time.Second, 3 * time.Second} {
		m.SetEvalTimeout(d)
		start := time.Now()
		v, err := m.Eval(`for (;;) {}`, beastjs.EvalGlobal)
		took := time.Since(start)
		fmt.Println(v, err, d, took >= d/2 && took <= d*3/2)
	}
	// Output:
	// <nil> InternalError: interrupted 100ms true
	// <nil> InternalError: interrupted 1s true
	// <nil> InternalError: interrupted 3s true
}

// Another goroutine stops a script that would run for ever.
func ExampleVM_Interrupt() {
	m := newVM()
	defer m.Close()
	start := time.Now()
	go func() {
		time.Sleep(time.Second)
		m.Interrupt()
	}()
	v, err := m.Eval(`for (;;) {}`, beastjs.EvalGlobal)
	fmt.Println(v, err, time.Since(start).Truncate(200*time.Millisecond))
	// Output:
	// <nil> InternalError: interrupted 1s
}
