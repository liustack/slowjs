// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "modernc.org/quickjs"

import (
	"errors"
	"strings"
	"testing"
)

func TestEvalError(t *testing.T) { testEvalError(t) }

// testEvalError is the worker behind TestEvalError; it is also run under the
// memgrind allocator by TestMemgrind2 to guard ErrorFromValue against leaks, so
// it must create and Close its own VM.
func testEvalError(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	// Parse/syntax error: Name, Message, FileName, LineNumber, ColumnNumber and
	// Stack are all populated.
	_, err = m.Eval("function hello(name) {\n  syntax error\n}\n", EvalModule)
	if err == nil {
		t.Fatal("expected a syntax error")
	}

	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("syntax: want *Error, got %T", err)
	}

	if g, w := e.Name, "SyntaxError"; g != w {
		t.Errorf("syntax Name = %q, want %q", g, w)
	}
	if e.Message == "" {
		t.Errorf("syntax Message is empty")
	}
	if g, w := e.FileName, "<eval>"; g != w {
		t.Errorf("syntax FileName = %q, want %q", g, w)
	}
	if g, w := e.LineNumber, 2; g != w {
		t.Errorf("syntax LineNumber = %d, want %d", g, w)
	}
	if e.ColumnNumber <= 0 {
		t.Errorf("syntax ColumnNumber = %d, want > 0", e.ColumnNumber)
	}
	if e.Stack == "" {
		t.Errorf("syntax Stack is empty")
	}
	// Error() must equal Name+": "+Message, i.e. what the engine has always
	// returned, so pre-existing Error()-string comparisons keep working.
	if g, w := e.Error(), e.Name+": "+e.Message; g != w {
		t.Errorf("syntax Error() = %q, want %q", g, w)
	}

	// Runtime error: Name, Message and Stack are set, but the flat FileName /
	// LineNumber / ColumnNumber fields are not - the location lives in Stack.
	_, err = m.Eval("\n\nnope();\n", EvalGlobal)
	if !errors.As(err, &e) {
		t.Fatalf("runtime: want *Error, got %T", err)
	}
	if g, w := e.Name, "ReferenceError"; g != w {
		t.Errorf("runtime Name = %q, want %q", g, w)
	}
	if !strings.Contains(e.Message, "nope") {
		t.Errorf("runtime Message = %q, want it to mention 'nope'", e.Message)
	}
	if !strings.Contains(e.Stack, "<eval>") {
		t.Errorf("runtime Stack = %q, want it to mention '<eval>'", e.Stack)
	}
	if e.LineNumber != 0 {
		t.Errorf("runtime LineNumber = %d, want 0 (runtime errors carry no flat location)", e.LineNumber)
	}

	// A proper Error object round-trips Name/Message and the classic Error()
	// string unchanged.
	_, err = m.Eval("throw new Error('boom')", EvalGlobal)
	if !errors.As(err, &e) {
		t.Fatalf("throw Error: want *Error, got %T", err)
	}
	if g, w := e.Error(), "Error: boom"; g != w {
		t.Errorf("throw Error() = %q, want %q", g, w)
	}
	if g, w := e.Name, "Error"; g != w {
		t.Errorf("throw Name = %q, want %q", g, w)
	}
	if g, w := e.Message, "boom"; g != w {
		t.Errorf("throw Message = %q, want %q", g, w)
	}

	// Throwing a non-Error value still yields an *Error whose Error() is the
	// value's string form; the structured Error fields stay empty.
	_, err = m.Eval("throw 42", EvalGlobal)
	if !errors.As(err, &e) {
		t.Fatalf("throw 42: want *Error, got %T", err)
	}
	if g, w := e.Error(), "42"; g != w {
		t.Errorf("throw 42 Error() = %q, want %q", g, w)
	}
	if e.Name != "" || e.Message != "" {
		t.Errorf("throw 42: want empty Name/Message, got Name=%q Message=%q", e.Name, e.Message)
	}
}
