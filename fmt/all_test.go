// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt // import "modernc.org/quickjs/fmt"

import (
	"fmt"
	"os"
	"testing"

	"modernc.org/quickjs"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func ExampleRegister_printf() {
	m, _ := quickjs.NewVM()
	defer m.Close()
	g := m.GlobalObject()
	defer g.Free()
	Register(g.Dup())
	v, _ := m.Eval("fmt.printf('Math.PI=%v\\n', Math.PI)", quickjs.EvalGlobal)
	fmt.Print(v)
	// Output:
	// Math.PI=3.141592653589793
	// [26,null]
}

func ExampleRegister_sprint() {
	m, _ := quickjs.NewVM()
	defer m.Close()
	g := m.GlobalObject()
	defer g.Free()
	Register(g.Dup())
	v, _ := m.Eval("fmt.sprint([2*21, 'foo', {bar: Math.exp(1)}])", quickjs.EvalGlobal)
	fmt.Print(v)
	// Output:
	// [42,"foo",{"bar":2.718281828459045}]
}
