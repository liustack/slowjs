// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package url gives a [quickjs.VM] the globals URL and URLSearchParams, as
// the URL standard (https://url.spec.whatwg.org/) defines them, with the
// parsing done in Go.
//
// QuickJS has neither. A bundle that needs them, such as a router that
// resolves links with new URL(path, base) on every render, carries a
// polyfill, which costs 40 to 90 µs a URL on this engine. With [Register] the
// same call takes about 4 µs. A polyfill that tests for a native URL first,
// as core-js does, then does not install itself.
//
// What a script can use is the whole of the standard: the constructor with
// and without a base, URL.canParse and URL.parse, every getter and setter of
// URL, searchParams linked to the URL both ways, toString and toJSON, and
// URLSearchParams with its constructor forms (a string, a sequence of pairs,
// a record), append, delete and has with an optional value, get, getAll, set,
// sort, forEach, the iterators, toString and size.
//
// The parser is internal/whatwgurl, which follows the standard of 2026 and
// passes the web-platform-tests URL data (see docs/changes/url.md for
// the list of cases it does not). Hosts are processed by IDNA with Unicode
// 17 tables.
//
// # Limits
//
// No string longer than 65,536 UTF-16 code units (its length in javascript)
// goes from a script into Go, and none comes back, so that what Go does for a
// call is over in a few milliseconds and its memory is known before it starts:
//
//   - new URL throws a TypeError, URL.parse is null and URL.canParse false for
//     an input or a base longer than that, as given, before spaces are trimmed,
//     and for one whose href would be longer than that (the href is the input
//     percent-encoded, so 7,281 CJK characters in a path make one too long);
//   - a setter leaves the URL as it was when the value is longer than that or
//     the href would be, and the href setter throws a TypeError;
//   - the host of a special scheme (http, https, ws, wss, ftp, file), which
//     IDNA processes, is not a host when it is not ASCII and longer than 1,024
//     units after percent-decoding, or once IDNA maps it, or once it is
//     normalized, and the host of another scheme, which IDNA does not process,
//     has no limit but the href's;
//   - a name or a value of a query string longer than that, as written, when it
//     has a + or a %, and one whose encoding would be longer than that, is a
//     RangeError, of the constructor of URLSearchParams and of toString;
//   - append, delete, set and sort of the searchParams of a URL throw a
//     RangeError, and change neither the list nor the URL, when the href would
//     be longer than that.
//
// Within these, the eval timeout and [quickjs.VM.Interrupt] end the script at
// the next call of Go, which looks at the stop flag as it starts, or at the
// engine's next poll, whichever comes first, and a call checks the room under the
// memory limit ([quickjs.VM.SetMemoryLimit]) for what Go may take before it
// reads a string, and ends with the out of memory error when it does not fit.
// The lists of URLSearchParams are split, sorted and walked in javascript, under
// the engine's own checks. A URL, and the list of a URLSearchParams, are
// javascript objects: they take their memory from the VM and give it back with
// the garbage collector. Go keeps nothing of them between calls, but for the
// buffer in which package quickjs converts a long string, which holds the
// bytes of the last one it read, at most 192 KiB through this package, until
// the next one (see docs/changes/url.md).
//
// # Differences from a browser
//
// URL.createObjectURL and URL.revokeObjectURL are not there, as the VM has no
// Blob. The messages of the TypeErrors are worded differently, though the
// errors are the same. A punycode label of more than 1024 characters is
// refused, as golang.org/x/net/idna refuses it.
package url // import "github.com/liustack/quickjs/url"

import (
	_ "embed"
	"fmt"
	"strconv"
	"sync"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/whatwgurl"
)

//go:embed url.js
var shim string

// script is the javascript of this package compiled to bytecode once per
// process and loaded from there into every VM: the bytecode does not depend
// on the VM that made it, so VMs on any number of goroutines share one copy of
// the bytes, and a VM copies them into its own memory when it loads them.
var script struct {
	mu   sync.Mutex
	code []byte // nil until a compile works
}

// bytecode returns the script compiled. A compile that fails is not kept: the
// next call tries again.
func bytecode() ([]byte, error) {
	script.mu.Lock()
	defer script.mu.Unlock()
	if script.code == nil {
		code, err := compile(shim)
		if err != nil {
			return nil, err
		}
		script.code = code
	}
	return script.code, nil
}

// compile compiles src on a VM of its own, as package intl does: the VM that
// needs the bytecode may be short of memory, stack or time, and a compile
// there would fail for reasons that have nothing to do with src. The
// functions are compiled without their source and debug information: they
// stand for built-ins, whose toString shows [native code].
func compile(src string) ([]byte, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}
	defer vm.Close()
	vm.SetStrip(quickjs.StripDebug)
	return vm.Compile(src, quickjs.EvalGlobal)
}

type config struct{}

// An Option configures [Register]. There is none yet: the type is here so
// that Register has the shape of intl.Register and temporal.Register.
type Option func(*config)

// marker is the well known symbol the script stamps on the global object of a
// VM it has registered, keyed by this package's import path.
const marker = "github.com/liustack/quickjs/url"

// Register installs URL and URLSearchParams on vm's global object. See the
// package documentation for what they do and what they do not.
//
// Call it on every VM that is to have them, before any code of the page runs
// on vm: it takes then the built-ins its code uses, so that a page changing
// Array.prototype, Object.prototype, String.prototype and the like afterwards
// changes nothing URL does. A script that replaces the global URL, or a
// member of URL.prototype, replaces it for itself and nothing else.
//
// Registering a second time on the same VM is an error, as it is for
// intl.Register. Nothing has to be released: vm.Close does it all. It costs
// one load of the script's bytecode (compiled once per process) and the
// making of the two classes.
func Register(vm *quickjs.VM, opts ...Option) error {
	switch already, err := registered(vm); {
	case err != nil:
		return err
	case already:
		return fmt.Errorf("url: this VM already has URL; a VM created to replace a realm needs its own Register")
	}
	// The tables of the normalization that IDNA uses are built once per process,
	// here and not in the first call of a script that needs them, which would
	// allocate them outside what the room of its VM was checked for.
	whatwgurl.Warm()
	c := &config{}
	for _, o := range opts {
		o(c)
	}
	code, err := bytecode()
	if err != nil {
		return err
	}
	f, err := vm.EvalBytecodeValue(code)
	if err != nil {
		return err
	}
	defer f.Free()
	args := make([]any, 0, len(bridge))
	for _, b := range bridge {
		v, err := vm.NewBuiltinFunction(b)
		if err != nil {
			return err
		}
		defer v.Free()
		args = append(args, v)
	}
	_, err = f.Call(quickjs.UndefinedValue, args...)
	return err
}

// registered reports whether this package has already installed URL on vm.
func registered(vm *quickjs.VM) (bool, error) {
	v, err := vm.Eval("globalThis[Symbol.for("+strconv.Quote(marker)+")] === true", quickjs.EvalGlobal)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}
