// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command evalonly evaluates one script and prints the result. It uses no
// other API, so TestQuickJSLibcNotLinked can read from its symbols what a
// program that only evaluates javascript links.
package main

import (
	"fmt"
	"os"

	"github.com/liustack/quickjs"
)

func main() {
	m, err := quickjs.NewVM()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer m.Close()
	v, err := m.Eval("[1, 2, 3].map(x => x * 2).join()", quickjs.EvalGlobal)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(v)
}
