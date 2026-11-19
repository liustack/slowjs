// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// NewMurmur2Function returns a function of javascript, written in C inside
// the engine, that hashes a string as murmur2 of @emotion/hash 0.9 does,
// which emotion takes of every style it serializes: the low byte of each
// UTF-16 code unit, a lone surrogate as the unit it is, and the hash in base
// 36. It answers undefined for anything but a string. A rope, a string the
// engine keeps as the pieces it was joined from, is joined first, under the
// memory limit, and an interrupt or the eval timeout stops it on a long
// string. emotion's own function calls String.prototype.charCodeAt and
// Number.prototype.toString: a host that calls this one in its place checks
// that a page has not replaced them (package tifa does). The function is
// named murmur2 and its length is 1.
func (m *VM) NewMurmur2Function() (Value, error) {
	if err := m.usable(); err != nil {
		return Value{}, err
	}
	f := m.jsNewMurmur2Function()
	if isException(f) {
		return Value{}, m.errFromException()
	}
	return Value{vm: m, v: f}, nil
}
