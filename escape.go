// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"strings"
)

// RegisterEscapeText defines a function named name on the global object that
// escapes text for HTML the way React DOM's string renderer does
// (escapeTextForBrowser in react-dom-server-legacy.browser.production.js).
// It is written in C inside the engine, so a call costs about what a call of
// an empty JavaScript function costs, and it does the work of the whole
// function of React: a script that calls it in place of the function of React
// gets the same text.
//
// The argument becomes a string as in "" + text: a string stays as it is,
// numbers, booleans and bigints become their text, an object goes through
// valueOf and toString (or Symbol.toPrimitive) with the hint "default", and a
// symbol or a toString that throws makes the call throw the same error. Then
// each of & < > " and ' becomes &amp; &lt; &gt; &quot; or &#x27;, one UTF-16
// code unit at a time, so a lone surrogate goes through as it is. The string
// it was given comes back when it has none of the five. Called with no
// argument it answers "undefined". A string of more than 1,073,741,823 units
// once escaped throws a RangeError, as the function of React does when it
// builds that string. An interrupt or the eval timeout stops it on a long
// string.
//
// The function is not on the global object unless a host calls this. The
// property is writable, configurable and not enumerable, and it replaces a
// configurable property of that name, as RegisterFunc does. A property that
// is not configurable (undefined, NaN, Infinity, a var or function of a
// script) is kept and RegisterEscapeText returns an error, as it does when
// the global object is not extensible. The name is the one javascript sees:
// invalid UTF-8 is replaced as Go decodes it (see Eval), and a NUL byte in it
// is an error. Each call makes a function of its own, and calling it again
// with the same name makes another one that takes the place of the first.
func (m *VM) RegisterEscapeText(name string) error {
	if err := m.usable(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("func name cannot be empty")
	}
	jsName := validUTF8(name)
	if strings.IndexByte(jsName, 0) >= 0 {
		return fmt.Errorf("func name cannot contain a NUL byte: %q", name)
	}
	f, err := m.jsNewEscapeTextFunction(jsName)
	if err == nil && isException(f) {
		err = m.errFromException()
	}
	if err != nil {
		return fmt.Errorf("cannot register func %s: %w", name, err)
	}
	if err := m.defineGlobalFunc(jsName, f); err != nil {
		return fmt.Errorf("cannot register func %s: %w", name, err)
	}
	return nil
}
