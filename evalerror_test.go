// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"errors"
	"strings"
	"testing"
)

// The fields of an *Error, from a syntax error, a thrown error and a thrown
// value that is no error.
func TestEvalError(t *testing.T) {
	m := newTestVM(t)
	asErr := func(err error) *Error {
		t.Helper()
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("got %v, want an *Error", err)
		}
		return e
	}

	_, err := m.Eval("function hello(name) {\n  syntax error\n}\n", EvalModule)
	e := asErr(err)
	if e.Name != "SyntaxError" || e.Message == "" || e.FileName != "<eval>" || e.LineNumber != 2 || e.ColumnNumber <= 0 || e.Stack == "" {
		t.Errorf("syntax error: %#v", e)
	}
	if e.Error() != e.Name+": "+e.Message {
		t.Errorf("syntax error: Error() is %q", e.Error())
	}

	_, err = m.Eval("\n\nnope();\n", EvalGlobal)
	e = asErr(err)
	if e.Name != "ReferenceError" || !strings.Contains(e.Message, "nope") || !strings.Contains(e.Stack, "<eval>") || e.LineNumber != 0 {
		t.Errorf("reference error: %#v", e)
	}

	_, err = m.Eval(`throw new Error('boom')`, EvalGlobal)
	e = asErr(err)
	if e.Error() != "Error: boom" || e.Name != "Error" || e.Message != "boom" {
		t.Errorf("thrown Error: %#v", e)
	}

	_, err = m.Eval(`throw 42`, EvalGlobal)
	e = asErr(err)
	if e.Error() != "42" || e.Name != "" || e.Message != "" {
		t.Errorf("thrown 42: %#v", e)
	}
}
