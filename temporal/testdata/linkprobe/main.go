// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Command linkprobe uses Temporal and nothing else, for
// TestLinksOnlyReadData to look at what it links.
package main

import (
	"github.com/liustack/tifajs"
	"github.com/liustack/tifajs/temporal"
)

func main() {
	vm, err := tifajs.NewVM()
	if err != nil {
		panic(err)
	}
	defer vm.Close()
	if err := temporal.Register(vm); err != nil {
		panic(err)
	}
	if _, err := vm.Eval(`Temporal.Now.zonedDateTimeISO("Europe/Paris").toString()`, tifajs.EvalGlobal); err != nil {
		panic(err)
	}
}
