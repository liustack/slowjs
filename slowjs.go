// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package slowjs is a JavaScript engine written for Go programs that render
// pages on a server: pure Go, with no cgo and no shared library, so that it
// builds and cross-compiles as any Go package does. It runs ES2023, modules,
// async generators, Proxy and BigInt among it. The engine, which started
// from QuickJS, is written in C and ships as Go assembly that is checked in,
// so building the package needs no C compiler.
//
// # Intl and time zones
//
// The engine has no Intl of its own, and its toLocale* methods look at no
// locale and no timeZone. The Register function of package
// github.com/liustack/slowjs/intl adds every Intl constructor to a VM and
// replaces toLocale* and localeCompare with ones that use them. The one of
// package github.com/liustack/slowjs/temporal adds Temporal. Without a locale
// they use en-US, and without a timeZone UTC. The time zone data is built in:
// the zoneinfo of the host is never read.
//
// # Platforms
//
// The package builds for darwin/amd64, darwin/arm64, linux/amd64,
// linux/arm64, windows/amd64 and windows/arm64. A build for any other target
// stops with an error that names these.
//
// # Bytecode across versions
//
// The bytecode of [VM.Compile] belongs to the release of QuickJS the engine
// is. Moving to QuickJS 2026-06-04 changed its format, so bytecode an older
// version compiled no longer loads, and so did the move to 2025-09-13 before
// it. Compile again after an upgrade.
//
// # Links
//
//   - QuickJS in C: https://bellard.org/quickjs
//   - ES2023: https://tc39.es/ecma262/2023/
package slowjs // import "github.com/liustack/slowjs"

import (
	"encoding/json"
	"errors"
)

// Both kinds of JSON a Go program reads from javascript go through
// encoding/json as they are.
var (
	_ json.Marshaler = (*Object)(nil)
	_ json.Marshaler = (*Value)(nil)
)

// Unsupported is the Go value of a javascript value that has no Go form, such
// as a symbol.
type Unsupported struct{}

// String returns "unsupported Javascript type".
func (u Unsupported) String() string { return "unsupported Javascript type" }

// Undefined is the Go value of javascript's undefined. As an argument it is
// undefined again.
type Undefined struct{}

// String returns "undefined".
func (u Undefined) String() string { return "undefined" }

// Object is the Go value of a javascript object, array or function: the JSON
// that JSON.stringify made of it when it was converted. It is a copy of what
// the object held then, not the object, so two Objects of one object are not
// the same and comparing them tells nothing. A value JSON.stringify gives
// undefined for, such as a function, has the JSON text undefined, which is
// not JSON.
//
// The zero Object has no JSON at all: passed to javascript, as an argument
// or a result, it fails to parse.
type Object struct {
	json string
}

// String returns the JSON of o.
func (o *Object) String() string { return o.json }

// MarshalJSON returns the JSON of o. Its error is always nil.
func (o *Object) MarshalJSON() ([]byte, error) { return []byte(o.json), nil }

// Into decodes the JSON of o into v, as encoding/json.Unmarshal does.
func (o *Object) Into(v any) error { return json.Unmarshal([]byte(o.json), v) }

// Version returns the release of QuickJS the engine is, such as
// "2026-06-04".
func Version() string { return engineVersion }

// Atom is the number by which a VM knows a property name (see
// [VM.NewAtom]). It is a uint32.
type Atom = uint32

// Strip says what the functions a VM compiles leave out (see [VM.SetStrip]).
type Strip int

const (
	// StripSource leaves out the source text of each function, so that its
	// toString shows [native code].
	StripSource Strip = 1
	// StripDebug leaves out the file names and the line and column numbers,
	// so that a stack frame shows only the name of its function, and the
	// source text too.
	StripDebug Strip = 2
)

// The flags of the eval methods: a script, or an ES module. A program may add
// other flag bits of QuickJS, which the engine is given as they are.
const (
	EvalGlobal = 0
	EvalModule = 1
)

// evalTypeMask is the bits of the flags that say what kind of code an eval
// runs.
const evalTypeMask = 3

// DefaultShapeCache is how many shapes a new VM keeps (see
// [VM.SetShapeCache]).
const DefaultShapeCache = 2048

// DefaultRegExpCache and DefaultRegExpCacheBytes are how many compiled
// regexps a new VM keeps at most, and in how many bytes (see
// [VM.SetRegExpCache]).
const (
	DefaultRegExpCache      = 512
	DefaultRegExpCacheBytes = 1 << 20
)

// UndefinedValue is javascript's undefined as a [Value]. It belongs to no VM,
// so every VM takes it, it holds no reference, and freeing it does nothing.
// The zero Value is undefined too.
var UndefinedValue = Value{v: undefined}

// ErrKilled matches the error of a call whose script did not stop in time
// after the eval timeout or [VM.Interrupt] and was killed, with
// [errors.Is]. Such a VM is good only for [VM.Close].
var ErrKilled = errors.New("slowjs: the script did not stop in time and was killed")

// HostFunc is a Go function for [VM.RegisterHostFunc]: it gets the arguments
// of the call as Go values, and returns the one value of the call, or an
// error to throw.
type HostFunc func(args []any) (any, error)
