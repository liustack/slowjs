// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package pages renders tifa's pages for the counts of callgrind and the timings of go tool
// measure bench (see docs/measuring.md): one VM set up as tifa sets up each VM of its pool, the
// bundle loaded once, each page rendered a few times to warm up, then the timed renders, with
// tifa's collections between renders. BenchmarkPages, in pages_test.go, is the program: build it
// with go test -c, or with go tool measure bench build -pkg . -test.
//
// Before the bundle runs, the guard finds every name __tifa_* the bundle holds and checks that the
// VM has each one as tifa gives it (Guard): a hook a new bundle asks for, which a VM set up here
// does not have, stops the run instead of letting the bundle fall back to React's javascript.
//
// It is a module of its own, so that go tool measure bench build -app can build it against any
// tree that has package tifa, such as the base of a comparison.
package pages

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
	"github.com/liustack/slowjs/temporal"
	"github.com/liustack/slowjs/tifa"
	"github.com/liustack/slowjs/url"
)

// Mode is how a VM is set up.
type Mode string

const (
	// Tifa is a VM set up as tifa's internal/ssr sets up each VM of its pool (Load and RenderVM of
	// renderer.go, the collections of pool.go): the memory limit, a call depth of 10000, the eval
	// timeout, tifa.Register, the bundle loaded as shared bytecode, a render as RenderVM makes it,
	// and after each collection outside a render the jobs it queued (runFinalizers).
	Tifa Mode = "tifa"
	// StandIn is the program that measured the pages of v0.31.1 and before, as it was: a VM with
	// the memory limit and no other, __tifa_murmur2__ and the four functions of the engine but not
	// the random bytes and the async context, a stand-in Intl and URL in place of intl, temporal and url
	// (the preludes of StandInPreludes), the bundle evaluated as source,
	// the collections of tifa's pool without giving memory back, and a render that calls
	// startRender by name. Its numbers compare with those of that program.
	StandIn Mode = "standin"
)

const (
	// defaultMemory is tifa's ssr.vm_memory.
	defaultMemory = 128 << 20
	// stackDepth is tifa's ssr.vm_stack_depth.
	stackDepth = 10000
	// defaultTimeout is tifa's ssr.timeout. Any timeout above 0 makes the engine do the same
	// work: only when it ends a call depends on it.
	defaultTimeout = 3 * time.Second
)

// standInHooks are the hooks the stand-in mode registers, in its order.
var standInHooks = []string{"__tifa_murmur2__", "__tifa_escape", "__tifa_copy_props", "__tifa_start_tag", "__tifa_end_tag"}

// Setup is a mode, with or without the hooks of tifa.
type Setup struct {
	Mode Mode
	// Bare leaves out every hook of tifa.Hooks, for a study of what React's javascript does in
	// their place. The rest of the mode stays.
	Bare bool
}

// Name is the name of the setup in the output: the mode, with -bare after it when Bare.
func (s Setup) Name() string {
	if s.Bare {
		return string(s.Mode) + "-bare"
	}
	return string(s.Mode)
}

// Prefix is what the names of the Benchmark lines start with after Pages/: the name and a
// slash, and nothing for the stand-in mode, whose lines keep the names of the program it
// reproduces.
func (s Setup) Prefix() string {
	if s == (Setup{Mode: StandIn}) {
		return ""
	}
	return s.Name() + "/"
}

// Registered returns the hooks the setup registers, in their order.
func (s Setup) Registered() []string {
	switch {
	case s.Bare:
		return nil
	case s.Mode == StandIn:
		return standInHooks
	}
	return tifa.Hooks()
}

// Describe says in one line how the setup makes a VM, for the output of a run.
func (s Setup) Describe(limit uintptr, timeout time.Duration) string {
	lim := "no memory limit"
	if limit > 0 {
		lim = fmt.Sprintf("memory limit %d", limit)
	}
	switch s.Mode {
	case Tifa:
		what := "tifa.Register (url, intl, temporal and the hooks of tifa.Hooks)"
		if s.Bare {
			what = "url, intl and temporal as tifa registers them, no tifa hook"
		}
		return fmt.Sprintf("mode %s: %s, %s, call depth %d, eval timeout %v, the bundle as shared bytecode, a render as tifa's RenderVM, tifa's collections with memory given back after a full one, and the jobs a collection queued run after it", s.Name(), what, lim, stackDepth, timeout)
	default:
		what := "__tifa_murmur2__ and the four functions of the engine"
		if s.Bare {
			what = "no tifa hook"
		}
		return fmt.Sprintf("mode %s: %s, a stand-in Intl and URL and no temporal, %s, the engine's call depth and no eval timeout, the bundle as source, startRender called by name, tifa's collections without giving memory back", s.Name(), what, lim)
	}
}

// NewVM makes a VM of the setup, ready for its preludes and its bundle: limit is the memory
// limit (0 for none), timeout the eval timeout of the tifa mode.
func (s Setup) NewVM(limit uintptr, timeout time.Duration) (vm *slowjs.VM, err error) {
	vm, err = slowjs.NewVM()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			vm.Close()
			vm = nil
		}
	}()
	switch s.Mode {
	case Tifa:
		if limit > 0 {
			vm.SetMemoryLimit(limit)
		}
		vm.SetMaxStackSize(stackDepth)
		if err := vm.SetEvalTimeout(timeout); err != nil {
			return nil, err
		}
		if !s.Bare {
			return vm, tifa.Register(vm)
		}
		if err := url.Register(vm); err != nil {
			return nil, err
		}
		if err := intl.Register(vm); err != nil {
			return nil, err
		}
		return vm, temporal.Register(vm)
	case StandIn:
		if limit > 0 {
			vm.SetMemoryLimit(limit)
		}
		for _, h := range s.Registered() {
			if err := tifa.RegisterHook(vm, h); err != nil {
				return nil, err
			}
		}
		return vm, nil
	}
	return nil, fmt.Errorf("pages: no mode %q (tifa or standin)", s.Mode)
}

// StandInPreludes are the preludes of the stand-in mode in the checkout repo: the host stand-ins
// of hostjs.js, the Intl of engine/pgo/pages/fakeintl.js and the URL of
// engine/pgo/pages/fakeurl.js. The program the mode reproduces had no URL prelude: the bundles
// it measured carried core-js's URL, and those of tifa 63a2099e and later ask the host for one.
func StandInPreludes(repo string) []string {
	return []string{
		filepath.Join(repo, "internal", "measure", "pages", "hostjs.js"),
		filepath.Join(repo, "engine", "pgo", "pages", "fakeintl.js"),
		filepath.Join(repo, "engine", "pgo", "pages", "fakeurl.js"),
	}
}
