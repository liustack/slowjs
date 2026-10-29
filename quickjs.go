// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package slowjs is a pure Go embeddable Javascript engine. It supports the
// ECMA script 14 ([ES2023]) specification including modules, asynchronous
// generators, proxies and BigInt.
//
// See also the original [C QuickJS] library.
//
// # Internationalization
//
// The engine has no Intl. ECMA-402 is not implemented by the C library this
// package wraps, so a page or bundle reaching for Intl.DateTimeFormat gets a
// ReferenceError, and the toLocale* methods it ships ignore both their locale
// and their timeZone argument. Package [github.com/liustack/slowjs/intl] adds
// every Intl constructor, with the data of every locale Chromium has, and
// replaces those toLocale* methods and localeCompare, for a VM that opts in by
// calling its Register. Package [github.com/liustack/slowjs/temporal] adds
// Temporal the same way. With no locale argument the locale is en-US, and with
// no timeZone option the zone is UTC. Both use the time zone data built into
// the module and never read the host's zoneinfo.
//
// # Notes
//
// Parts of the documentation were copied from the quickjs documentation, see
// LICENSE-LIBQUICKJS for details.
//
// # Supported platforms and architectures
//
// The engine is C compiled to Go assembly, checked in, so building needs no
// C compiler. It runs on these combinations of GOOS and GOARCH, and a build
// for any other stops with an error that names them
//
//	OS      Arch
//	-------------
//	darwin  arm64
//	linux   amd64
//
// # Change Log
//
// 2026-07-27: Upgrade to QuickJS release 2026-06-04. The bytecode format
// changed, code produced by [VM.Compile] using an older release will no longer
// load.
//
// 2025-10-10: Upgrade to QuickJS release 2025-09-13.
//
// [C QuickJS]: https://bellard.org/quickjs
// [ES2023]: https://tc39.es/ecma262/2023/
package slowjs // import "github.com/liustack/slowjs"

import (
	"encoding/json"
)

var (
	_ json.Marshaler = (*Object)(nil)
	_ json.Marshaler = (*Value)(nil)
)

// Unsupported represents an unsupported Javascript value.
type Unsupported struct{}

// String implements fmt.Stringer.
func (u Unsupported) String() string {
	return "unsupported Javascript type"
}

// Undefined represents the Javascript value "undefined".
type Undefined struct{}

// String implements fmt.Stringer.
func (u Undefined) String() string {
	return "undefined"
}

// Object represents the value of a Javascript object, but not the javascript
// object instance itself. Do not compare instances of Object.
type Object struct {
	json string
}

// String implements fmt.Stringer.
func (o *Object) String() string {
	return o.json
}

// MarshalJSON implements encoding/json.Marshaler.
func (o *Object) MarshalJSON() (r []byte, err error) {
	return []byte(o.json), nil
}

// Into helps you quickly deserialize an Object into a native Go type.
// This uses encoding/json.Unmarshal internally, so the target type must be
// compatible with that package.
func (o *Object) Into(v any) error {
	return json.Unmarshal([]byte(o.json), v)
}

// Version returns the underlying QuickJS version.
func Version() string {
	return engineVersion
}
