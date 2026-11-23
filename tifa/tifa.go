// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package tifa gives a [tifajs.VM] what tifa (github.com/liustack/tifa)
// gives each VM of its pool before it runs a bundle: URL and
// URLSearchParams, Intl and Temporal, and the functions named __tifa_* that
// the bundles tifa builds ask for.
//
// tifajs is made for tifa, so tifa, the tifajs command and the program that
// measures tifa's pages (internal/measure/pages) all set a VM up with
// [Register], and what a VM of each has is the same.
//
// The functions named __tifa_* are those of [Hooks]:
//
//   - __tifa_random_bytes__(n): n bytes from crypto/rand, 0 to 65536, in
//     base64. The bundle's crypto.getRandomValues and crypto.randomUUID read
//     them.
//   - __tifa_murmur2__(s): the hash of @emotion/hash of the string s, by
//     the engine ([tifajs.VM.NewMurmur2Function]), or undefined for anything
//     else, which emotion then hashes in javascript.
//   - __tifa_escape, __tifa_copy_props, __tifa_start_tag and __tifa_end_tag:
//     the functions of the engine that do the work of React's escape, its
//     copy of props and the tags React DOM writes ([tifajs.VM.RegisterEscapeText],
//     [tifajs.VM.RegisterCopyProps], [tifajs.VM.RegisterStartTag],
//     [tifajs.VM.RegisterEndTag]).
//   - __tifa_async_context__: an object with get and set, the async context
//     of the VM ([tifajs.GetAsyncContext], [tifajs.SetAsyncContext]), which
//     the AsyncLocalStorage of tifa's host script keeps its stores in.
//
// The host script of a bundle takes __tifa_random_bytes__ and
// __tifa_async_context__ and deletes them from the global object before any
// code of the app runs. The others stay.
//
// What stays in tifa: the memory limit, the call depth and the eval timeout
// of a VM, the checks of tifa dev and of its emit check build, the bundle and
// the collections between renders.
package tifa

import (
	"fmt"

	"github.com/liustack/tifajs"
	"github.com/liustack/tifajs/intl"
	"github.com/liustack/tifajs/temporal"
	"github.com/liustack/tifajs/url"
)

// hooks are the globals of Hooks with the function that defines each, in the
// order Register defines them, which is tifa's.
var hooks = [...]struct {
	name     string
	register func(*tifajs.VM) error
}{
	{"__tifa_random_bytes__", registerRandom},
	{"__tifa_murmur2__", registerHash},
	{"__tifa_escape", func(vm *tifajs.VM) error { return vm.RegisterEscapeText("__tifa_escape") }},
	{"__tifa_copy_props", func(vm *tifajs.VM) error { return vm.RegisterCopyProps("__tifa_copy_props") }},
	{"__tifa_start_tag", func(vm *tifajs.VM) error { return vm.RegisterStartTag("__tifa_start_tag") }},
	{"__tifa_end_tag", func(vm *tifajs.VM) error { return vm.RegisterEndTag("__tifa_end_tag") }},
	{"__tifa_async_context__", registerAsyncContext},
}

// Hooks returns the names of the globals named __tifa_* that Register
// defines, in the order it defines them. Each is a function, but for
// __tifa_async_context__, an object with the functions get and set.
func Hooks() []string {
	names := make([]string, len(hooks))
	for i, h := range hooks {
		names[i] = h.name
	}
	return names
}

// Register sets vm up as tifa sets up each VM of its pool, in tifa's order:
// URL and URLSearchParams (url.Register), Intl (intl.Register), Temporal
// (temporal.Register), then the hooks of [Hooks]. Call it after the eval
// timeout is set, as url.Register asks, and before the bundle runs.
//
// Register runs once on a VM: a second call fails, as url.Register refuses a
// VM that has URL. When a step fails, vm keeps what the steps before it
// defined: close it.
func Register(vm *tifajs.VM) error {
	if err := url.Register(vm); err != nil {
		return err
	}
	if err := intl.Register(vm); err != nil {
		return err
	}
	if err := temporal.Register(vm); err != nil {
		return err
	}
	for _, h := range hooks {
		if err := h.register(vm); err != nil {
			return fmt.Errorf("tifa: %s: %w", h.name, err)
		}
	}
	return nil
}

// RegisterHook defines on vm the one global of [Hooks] named name, as
// Register defines it. It is for a VM that is to have only some of them,
// such as a test of one, or a measurement that leaves the others out on
// purpose.
func RegisterHook(vm *tifajs.VM, name string) error {
	for _, h := range hooks {
		if h.name == name {
			if err := h.register(vm); err != nil {
				return fmt.Errorf("tifa: %s: %w", name, err)
			}
			return nil
		}
	}
	return fmt.Errorf("tifa: no hook %q", name)
}
