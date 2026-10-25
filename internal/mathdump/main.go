// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command mathdump runs engine/test/mathcmp.js on the Go engine and prints what
// runFn gives for each name, in the format of engine/test/mathdump.c. Its output
// is the reference that engine/test/math.sh compares the C tree with.
//
//	go run ./internal/mathdump engine/test/mathcmp.js NAME...
package main

import (
	"fmt"
	"os"

	"github.com/liustack/slowjs"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: mathdump SCRIPT NAME...")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "mathdump:", err)
		os.Exit(2)
	}
	vm, err := slowjs.NewVM()
	if err != nil {
		fmt.Fprintln(os.Stderr, "mathdump:", err)
		os.Exit(1)
	}
	defer vm.Close()
	if _, err := vm.Eval(string(src), slowjs.EvalGlobal); err != nil {
		fmt.Fprintln(os.Stderr, "mathdump:", err)
		os.Exit(1)
	}
	for _, name := range os.Args[2:] {
		r, err := vm.Eval(fmt.Sprintf("runFn('%s')", name), slowjs.EvalGlobal)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mathdump:", err)
			os.Exit(1)
		}
		s, ok := r.(string)
		if !ok {
			fmt.Fprintf(os.Stderr, "mathdump: runFn(%s) gave %T\n", name, r)
			os.Exit(1)
		}
		fmt.Println(s)
	}
}
