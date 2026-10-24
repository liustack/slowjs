// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package engine puts every JavaScript engine of the comparison behind one
// small interface. Each engine lives in a file of its own, behind a build tag
// (eng_<name>), so that a benchmark binary links exactly one engine.
package engine

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Options configure a new VM.
type Options struct {
	// Print receives what the script passes to print(). Nil drops it.
	Print func(string)
	// MemoryLimit, in bytes, if the engine has one. 0 is no limit.
	MemoryLimit int64
	// Intl asks for Intl where the engine makes it opt-in.
	Intl bool
	// ModuleDir is where module specifiers resolve, for engines that load
	// modules from files.
	ModuleDir string
	// Interruptible asks for Interrupt to work, for engines where it costs
	// speed and is off unless asked. No engine of the comparison needs it now.
	Interruptible bool
	// Clock asks for globalThis.__jsbench_now(), milliseconds from a Go
	// monotonic clock, where the adapter has it (slowjs), to time
	// parts of a render from inside the script.
	Clock bool
}

// Program is a compiled script, or the source for engines that do not
// compile ahead.
type Program any

// VM is one engine instance with its own global object.
type VM interface {
	// Eval runs src as a global (sloppy) script and returns String() of its
	// completion value.
	Eval(name, src string) (string, error)
	// Compile prepares src for Run.
	Compile(name, src string) (Program, error)
	// Run runs a program from Compile, of this VM or, when the engine says
	// SharedPrograms, of any VM of the engine.
	Run(p Program) (string, error)
	// Call calls globalThis[fn]() and returns String() of the result.
	Call(fn string) (string, error)
	// RunJobs drains the promise job queue.
	RunJobs() error
	// Interrupt stops the running script; safe from another goroutine.
	Interrupt()
	Close()
}

// ModuleVM is a VM that runs ES modules. Imports resolve in Options.ModuleDir.
type ModuleVM interface {
	EvalModule(name, src string) error
}

// Engine describes one engine.
type Engine struct {
	Name   string // short name used on the command line and in results
	Module string // Go module path
	New    func(Options) (VM, error)
	// SharedPrograms is true when a Program compiled once can run on other
	// VMs, so that cold start can skip parsing.
	SharedPrograms bool
	// CompileShared compiles a Program usable on every VM of the engine.
	// Nil when SharedPrograms is false.
	CompileShared func(name, src string) (Program, error)
	// Interrupt, MemoryLimit and Modules say what the engine's API offers.
	Interrupt   bool
	MemoryLimit bool
	Modules     bool
	// Notes on how the adapter uses the engine, printed in the results.
	Notes []string
}

var registered *Engine

func register(e *Engine) {
	if registered != nil {
		panic("two engines in one binary: " + registered.Name + " and " + e.Name)
	}
	registered = e
}

// Get returns the engine linked into this binary.
func Get() (*Engine, error) {
	if registered == nil {
		return nil, errors.New("no engine: build with -tags eng_<name>")
	}
	return registered, nil
}

// JSError is an exception that reached Go, with the name of its error type
// when the engine tells it.
type JSError struct {
	Name    string // "SyntaxError", "TypeError", ...; may be empty
	Message string
	Compile bool // thrown while compiling
}

func (e *JSError) Error() string {
	if e.Name == "" {
		return e.Message
	}
	return e.Name + ": " + e.Message
}

// errorNames are the error types test262 expects.
var errorNames = []string{"SyntaxError", "TypeError", "ReferenceError", "RangeError",
	"EvalError", "URIError", "Test262Error", "AggregateError", "SuppressedError", "Error"}

// Classify wraps err as a JSError, taking the error type from the start of
// the message ("TypeError: ...") when the engine gives no name.
func Classify(err error, compile bool) error {
	if err == nil {
		return nil
	}
	var je *JSError
	if errors.As(err, &je) {
		return err
	}
	msg := err.Error()
	name := ""
	for _, n := range errorNames {
		if strings.HasPrefix(msg, n+":") || strings.HasPrefix(msg, n+" ") || msg == n {
			name = n
			break
		}
	}
	if name == "" {
		for _, n := range errorNames {
			if strings.Contains(msg, n+":") {
				name = n
				break
			}
		}
	}
	return &JSError{Name: name, Message: strings.TrimPrefix(strings.TrimPrefix(msg, name+":"), " "), Compile: compile}
}

// PrintShim defines print() on top of the Go function each adapter installs
// as __jsbench_print, so that every engine converts the argument the same way.
const PrintShim = `(function (g) { g.print = function print() { var s = ""; for (var i = 0; i < arguments.length; i++) { if (i) s += " "; s += String(arguments[i]); } __jsbench_print(s); }; })(this);`

// Watchdog interrupts vm after d unless stopped.
func Watchdog(vm VM, d time.Duration) (stop func()) {
	if d <= 0 {
		return func() {}
	}
	t := time.AfterFunc(d, vm.Interrupt)
	return func() { t.Stop() }
}

// Str formats a Go value from an engine the way String() would for the
// common cases.
func Str(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// Host is a VM that runs a page the way the application's host does, with a
// render and the housekeeping after it of its own (slowjs-tifa). The tifa
// page runs use it when a VM has it, and otherwise call startRender through
// the glue.
type Host interface {
	// Loaded is called once the bundle is loaded.
	Loaded() error
	// Render runs startRender(input), the promise jobs and the read of the
	// result, and returns the HTML.
	Render(input string) (string, error)
	// Settle runs after each render that succeeded, outside its wall time and
	// inside its CPU time.
	Settle() error
	// Counters report how often the host did what it does, for the results.
	Counters() map[string]int
}
