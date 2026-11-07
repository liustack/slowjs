// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package vmhook lets the packages of this module that give a VM functions
// written in Go (intl, temporal and fmt) hold the work those functions do in
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
)
