// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command linkprobe uses Temporal and nothing else, for
// TestLinksOnlyReadData to look at what it links.
package main

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/temporal"
)

func main() {
	vm, err := quickjs.NewVM()
	if err != nil {
		panic(err)
	}
	defer vm.Close()
	if err := temporal.Register(vm); err != nil {
		panic(err)
	}
	if _, err := vm.Eval(`Temporal.Now.zonedDateTimeISO("Europe/Paris").toString()`, quickjs.EvalGlobal); err != nil {
		panic(err)
	}
}
