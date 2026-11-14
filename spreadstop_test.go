// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import "testing"

// A spread of a string with more characters than the memory limit leaves
// room for ends with the out of memory error, and so does Object.assign of
// it. The VM goes on. The stop of a long copy is TestSpreadCopyStops
// (spreadstop_hooks_test.go).
func TestSpreadStringOutOfMemory(t *testing.T) {
	for _, call := range []string{`({..."x".repeat(1 << 22)})`, `Object.assign({}, "x".repeat(1 << 22))`} {
		m := newTestVM(t)
		m.SetMemoryLimit(32 << 20)
		_, err := m.Eval(call, EvalGlobal)
		wantOutOfMemory(t, call, err)
		wantNoPendingException(t, m)
		m.SetMemoryLimit(^uintptr(0))
		if v, err := m.Eval(`JSON.stringify({..."ab"})`, EvalGlobal); err != nil || v != `{"0":"a","1":"b"}` {
			t.Fatalf("%s: the next call got %v, %v", call, v, err)
		}
	}
}
