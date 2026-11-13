// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package vmhook lets the packages of this module that give a VM functions
// written in Go (intl, temporal, fmt and url) hold the work those functions do in
// Go to the limits of the VM: its eval timeout, Interrupt and its memory
// limit, which the engine's polls and allocator see only for the work done
// in the engine. Package quickjs sets the functions at initialization. They
// take the *quickjs.VM as an any, as this package cannot import package
// quickjs, which imports it.
//
// Each is for a Go function that javascript called, while it runs on the
// goroutine of the call, and panics anywhere else.
package vmhook // import "github.com/liustack/quickjs/internal/vmhook"

var (
	// Stop reports whether the call into javascript in progress on vm is to
	// end: Interrupt asked it to stop, its eval timeout passed, or an
	// uncatchable error is already pending, such as running out of memory
	// in reading an argument. When it is true the uncatchable error is
	// pending, "interrupted" for a timeout or an Interrupt, as the engine's
	// polls throw it, and the Go function returns at once: the call ends
	// with that error whatever it returns. A Go function whose loop runs as
	// long as a script makes it, such as one over a list the script passed,
	// calls Stop every so often, about every millisecond of work or less.
	Stop func(vm any) bool

	// Room returns how many bytes vm may still take under the limit of
	// SetMemoryLimit, or -1 when it has no limit. A Go function that builds
	// a result of a size a script controls checks it before building it.
	Room func(vm any) int64

	// OutOfMemory throws the uncatchable out of memory error in vm, as the
	// engine does when an allocation passes the limit, and the call ends
	// with it whatever the Go function returns.
	OutOfMemory func(vm any)

	// Chars returns the characters of argument i of args, a quickjs.Args, when
	// it is a string, as the engine holds them: in latin1, one byte each, when
	// all are below U+0100, or in utf16 otherwise, lone surrogates as they are.
	// Nothing is copied: they must not be changed, and are valid until release
	// is called, which the Go function does before it returns. A rope is joined
	// first, by the engine, under the memory limit of the VM: ok is false when
	// that fails, with the error that ends the call pending, as for an argument
	// that is not a string.
	Chars func(args any, i int) (latin1 []byte, utf16 []uint16, release func(), ok bool)

	// LongWTF8 is Args.WTF8 for argument i of args, a quickjs.Args, a long
	// string the Go function uses only until it calls give: a long one is
	// in memory mapped for the call (hostmem.Take), and nothing may keep the
	// string, or a part of it, after give. It is not a string Go can keep,
	// as one the function returns or stores. When ok is false give does
	// nothing.
	LongWTF8 func(args any, i int) (s string, give func(), ok bool)
)
