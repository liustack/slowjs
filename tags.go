// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"strings"
)

// RegisterStartTag defines a function named name on the global object that
// writes the start tag of an element the way React DOM's server renderer does
// in pushStartGenericElement (react-dom-server-legacy.browser.production.js
// of react-dom 19.3), with the functions it calls: startChunkForTag,
// pushAttribute, pushStyleAttribute, escapeTextForBrowser and the others. It
// is written in C inside the engine and writes the whole tag in one call, a
// few times faster than those functions.
//
// The function takes six arguments: target, props, type and formatContext,
// those of pushStartGenericElement, then React's hasOwnProperty (the
// Object.prototype.hasOwnProperty it took when it loaded) and decline, an
// object only the caller holds. It pushes onto target one string, the start
// tag with its attributes and, when props.children is a string, the escaped
// text after it, and returns what pushStartGenericElement returns: null when
// children is a string, else props.children, or null when there is none.
// React pushes the same text in several strings. When it cannot be sure to
// write what React writes it returns decline, having changed nothing, and the
// script runs React's function. That is the case for an unusual target (not a
// plain array, an index property ever put on Array.prototype or
// Object.prototype, where push would set it, or Array.prototype.push
// replaced), a tag React refuses, a formatContext whose viewTransition is not
// null, props or a style that are not plain objects of Object.prototype with
// at most 32 keys, a getter, an array index key, dangerouslySetInnerHTML, the
// rare attributes xlinkHref, capture, download, cols, rows, size, span,
// rowSpan and start, a value that React would turn into a string by calling
// its toString (an object) or a bigint, a javascript: URL, a name of the
// default branch that is not ASCII or is 64 characters or more, a style name
// that is not ASCII and does not start with --, a tag that would be too long,
// and hasOwnProperty.call that is not the built-in function calling the
// built-in. The built-ins are found by their C function, so one replaced
// before registering is not taken for them, and registering runs no script.
// It assumes that the built-in methods of String, RegExp, Map and Set that
// React's functions call are still those of the engine. It throws only when
// the engine has no memory, or when an interrupt or the eval timeout stops it
// on a long string, and target is then unchanged.
//
// The function is not on the global object unless a host calls this. The
// property is writable, configurable and not enumerable, and it replaces a
// configurable property of that name, as RegisterFunc does. A property that
// is not configurable (undefined, NaN, Infinity, a var or function of a
// script) is kept and RegisterStartTag returns an error, as it does when the
// global object is not extensible. The name is the one javascript sees:
// invalid UTF-8 is replaced as Go decodes it (see Eval), and a NUL byte in it
// is an error. Each call makes a function of its own, and calling it again
// with the same name makes another one that takes the place of the first.
func (m *VM) RegisterStartTag(name string) error {
	return m.registerTagFunc(name, m.jsNewStartTagFunction)
}

// RegisterEndTag defines a function named name on the global object that
// gives the end tag of an element the way React DOM's server renderer does in
// endChunkForTag: "</" + tag + ">" for its argument tag. It is written in C
// inside the engine and keeps the end tag of each string it was given, so that
// the same string gives the same end tag again without making it. An argument
// that is not a string becomes one as in "</" + tag, and the function throws
// what that throws. The rules of the name and of the property are those of
// RegisterStartTag.
func (m *VM) RegisterEndTag(name string) error {
	return m.registerTagFunc(name, m.jsNewEndTagFunction)
}

func (m *VM) registerTagFunc(name string, make func(string) (jsValue, error)) error {
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
	f, err := make(jsName)
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
