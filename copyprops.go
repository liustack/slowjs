// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"strings"
)

// RegisterCopyProps defines a function named name on the global object that
// does the work of the loop with which React's createElement and cloneElement
// copy the keys of their config into the props of an element:
//
//	for (propName in config)
//	  hasOwnProperty.call(config, propName) && "key" !== propName &&
//	    "__self" !== propName && "__source" !== propName &&
//	    (props[propName] = config[propName])
//
// (and for cloneElement also "ref" when config.ref is undefined). It is
// written in C inside the engine and copies all the keys in one call, a few
// times faster than the loop.
//
// The function takes four arguments: config, props, React's hasOwnProperty
// (the Object.prototype.hasOwnProperty it took when it loaded) and whether
// the call is cloneElement's. It answers true when it copied, and false when
// it changed nothing and the script has to run its loop. It copies only
// where its result and the loop's cannot differ in anything a script can
// see: config an ordinary object whose prototype is Object.prototype, with
// no more than 32 own keys, none of them an array index, __proto__ or stack,
// and every key the loop reads a plain data property (a getter, a setter, a
// proxy, a symbol key and a key that is not enumerable never change the
// result, or send it to the loop); props an ordinary object that can be
// extended, with the same prototype and only plain writable data properties;
// Object.prototype with no setter and no read-only property but __proto__;
// and hasOwnProperty.call the built-in function calling the built-in. It
// throws only when the engine has no memory, with the error that adding the
// keys would have thrown.
//
// A page that replaces any of these after React has loaded, or gives
// Object.prototype a setter or a read-only property, only sends the next
// calls to the loop. The function is not on the global object unless a host
// calls this. The property is writable, configurable and not enumerable, and
// it replaces a configurable property of that name, as RegisterFunc does. A
// property that is not configurable (undefined, NaN, Infinity, a var or
// function of a script) is kept and RegisterCopyProps returns an error, as it
// does when the global object is not extensible. The name is the one
// javascript sees: invalid UTF-8 is replaced as Go decodes it (see Eval), and
// a NUL byte in it is an error. Each call makes a function of its own, and
// calling it again with the same name makes another one that takes the place
// of the first.
func (m *VM) RegisterCopyProps(name string) error {
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
	f, err := m.jsNewCopyPropsFunction(jsName)
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
