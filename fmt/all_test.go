// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/slowjs/fmt"

import (
	"fmt"
	"os"
	"testing"

	"github.com/liustack/slowjs"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func ExampleRegister_printf() {
	vm, err := slowjs.NewVM()
	if err != nil {
		panic(err)
	}

	defer vm.Close()

	g := vm.GlobalObject()

	defer g.Free()

	if err := Register(g.Dup()); err != nil {
		panic(err)
	}

	v, err := vm.Eval(`fmt.printf('Math.PI=%v\n', Math.PI)`, slowjs.EvalGlobal)
	if err != nil {
		panic(err)
	}

	fmt.Print(v)
	// Output:
	// Math.PI=3.141592653589793
	// [26,null]
}

func ExampleRegister_sprint() {
	vm, err := slowjs.NewVM()
	if err != nil {
		panic(err)
	}

	defer vm.Close()

	g := vm.GlobalObject()

	defer g.Free()

	if err := Register(g.Dup()); err != nil {
		panic(err)
	}

	v, err := vm.Eval(`fmt.sprint([2*21, 'foo', {bar: Math.exp(1)}])`, slowjs.EvalGlobal)
	if err != nil {
		panic(err)
	}

	fmt.Print(v)
	// Output:
	// [42,"foo",{"bar":2.718281828459045}]
}
