// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import "testing"

// A template object read back from bytecode is frozen as a parsed one is: the
// strings array and its raw array are not extensible and their length can be
// neither written nor configured (upstream QuickJS 88130c0).
func TestBytecodeTemplateObjectSealed(t *testing.T) {
	const src = `
function tag(s) { return s; }
var s = tag` + "`a${1}b`" + `;
JSON.stringify([Object.isFrozen(s), Object.isFrozen(s.raw),
  Object.getOwnPropertyDescriptor(s, "length").writable,
  Object.getOwnPropertyDescriptor(s, "length").configurable,
  Object.getOwnPropertyDescriptor(s.raw, "length").writable])`
	const want = `[true,true,false,false,false]`
	code := compileOrFatal(t, newTestVM(t), src)
	m := newTestVM(t)
	got, err := m.EvalBytecode(code)
	if err != nil || got != want {
		t.Fatalf("EvalBytecode: %v, %v, want %s", got, err, want)
	}
}
