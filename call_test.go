// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
)

// Call and CallValue evaluate the function expression before calling it. An
// exception thrown while doing so must come back as that exception, not as
// "cannot call a non-function", and must not stay pending in the context.
func TestCallReportsFunctionLookupException(t *testing.T) {
	type call func(m *VM, function string) error
	for _, c := range []struct {
		name string
		call call
	}{
		{"Call", func(m *VM, function string) error { _, err := m.Call(function); return err }},
		{"CallValue", func(m *VM, function string) error {
			v, err := m.CallValue(function)
			if err == nil {
				v.Free()
			}
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}

			defer m.Close()

			if _, err := m.Eval("function f() { return 42 }", EvalGlobal); err != nil {
				t.Fatal(err)
			}

			m.SetMemoryLimit(4 << 20)
			for _, tc := range []struct{ function, name, message string }{
				// Running out of memory while evaluating the expression.
				{"new Uint8Array(8 << 20).fill(1) && f", "InternalError", "out of memory"},
				// A name that does not resolve.
				{"nosuchfunction", "ReferenceError", "'nosuchfunction' is not defined"},
			} {
				err := c.call(m, tc.function)
				var e *Error
				if !errors.As(err, &e) || e.Name != tc.name || e.Message != tc.message {
					t.Errorf("%s: got error %v, want %s: %s", tc.function, err, tc.name, tc.message)
				}
			}

			// The VM is still usable and no stale exception is reported.
			if err := c.call(m, "f"); err != nil {
				t.Fatalf("f after failed calls: %v", err)
			}
		})
	}
}
