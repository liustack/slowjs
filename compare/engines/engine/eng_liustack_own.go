// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build eng_liustack

package engine

import (
	"github.com/liustack/beastjs"
	"github.com/liustack/beastjs/intl"
	"github.com/liustack/beastjs/temporal"
	"github.com/liustack/beastjs/url"
)

const liustackName = "liustack-beastjs"

var liustackNotes = []string{
	"Intl, Temporal and URL come from the packages intl, temporal and url of the module, registered only when a test asks for them.",
}

// registerGoPackages gives the VM what the Go packages next to the engine add.
func registerGoPackages(vm *beastjs.VM) error {
	if err := intl.Register(vm); err != nil {
		return err
	}
	if err := temporal.Register(vm); err != nil {
		return err
	}
	return url.Register(vm)
}
