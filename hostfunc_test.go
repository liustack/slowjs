// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mustHost registers fn as a host function or fails the test.
func mustHost(t *testing.T, m *VM, name string, fn HostFunc) {
	t.Helper()
	if err := m.RegisterHostFunc(name, fn); err != nil {
		t.Fatalf("RegisterHostFunc(%q): %v", name, err)
	}
}

// TestRegisterHostFunc covers the host-function calling convention: every JS
// argument arrives in a single []any, a single value is returned (not
// array-wrapped as RegisterFunc does for a (value, error) pair), a non-nil error
// is thrown as a catchable JS exception, and a nil value becomes JS null.
func TestRegisterHostFunc(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// All JS arguments collected into one []any; a single string is returned.
	mustHost(t, m, "join", func(args []any) (any, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = fmt.Sprint(a)
		}
		return strings.Join(parts, "-"), nil
	})
	// Numeric arguments arrive as int (integral) or float64; a number is returned.
	mustHost(t, m, "sum", func(args []any) (any, error) {
		s := 0.0
		for _, a := range args {
			switch v := a.(type) {
			case int:
				s += float64(v)
			case float64:
				s += v
			}
		}
		return s, nil
	})
	// A non-nil error is thrown as a JS exception.
	mustHost(t, m, "boom", func(args []any) (any, error) {
		return nil, fmt.Errorf("kaboom")
	})
	// A nil value returns JS null/undefined.
	mustHost(t, m, "nada", func(args []any) (any, error) {
		return nil, nil
	})

	check := func(expr string, want any) {
		t.Helper()
		r, err := m.Eval(expr, EvalGlobal)
		if err != nil {
			t.Fatalf("Eval(%q): %v", expr, err)
		}
		if r != want {
			t.Errorf("Eval(%q) = %#v, want %#v", expr, r, want)
		}
	}

	// Single value, NOT the array [value, error] that a (any, error)
	// RegisterFunc would produce.
	check(`join("a", 1, true)`, "a-1-true")
	check(`Array.isArray(join("a"))`, false)
	check(`sum(1, 2, 3.5)`, 6.5)
	check(`nada() == null`, true)

	// The error surfaces as a thrown, catchable exception carrying its message.
	r, err := m.Eval(`(() => { try { boom(); return "no throw"; } catch (e) { return "caught:" + e.message; } })()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := r.(string); !strings.HasPrefix(s, "caught:") || !strings.Contains(s, "kaboom") {
		t.Errorf("boom() should throw with its message; got %#v", r)
	}

	// An uncaught host error propagates out of Eval as a Go error.
	if _, err := m.Eval(`boom()`, EvalGlobal); err == nil {
		t.Error("uncaught host error should surface as an Eval error")
	}
}

// TestRegisterHostFuncValidation covers the registration guards and that host
// and reflection-based functions coexist.
func TestRegisterHostFuncValidation(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.RegisterHostFunc("nilfn", nil); err == nil {
		t.Error("RegisterHostFunc(nil) should error")
	}
	ok := func(args []any) (any, error) { return nil, nil }
	if err := m.RegisterHostFunc("dup", ok); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHostFunc("dup", ok); err == nil {
		t.Error("registering a duplicate name should error")
	}

	// Host and reflection-based functions coexist in the same VM.
	if err := m.RegisterFunc("add", func(a, b int) int { return a + b }, false); err != nil {
		t.Fatal(err)
	}
	mustHost(t, m, "id", func(args []any) (any, error) {
		if len(args) == 0 {
			return nil, nil
		}
		return args[0], nil
	})
	if r, err := m.Eval(`add(2, 3) + id(5)`, EvalGlobal); err != nil || r != 10 {
		t.Errorf("mixed RegisterFunc/RegisterHostFunc = %#v, %v; want 10", r, err)
	}
}

// throwMsg calls a host function failing with errors.New(msg) and returns the
// resulting error string.
func throwMsg(t *testing.T, msg string) string {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	mustHost(t, m, "boom", func(args []any) (any, error) { return nil, errors.New(msg) })
	if _, err = m.Eval(`boom()`, EvalGlobal); err == nil {
		t.Fatal("boom() did not throw")
	}
	return err.Error()
}

// TestThrowMessage checks that a message produced on the Go side reaches
// Javascript verbatim.
//
// It crosses into C through JS_ThrowTypeError, a printf-style function, so it
// has to travel as a NUL terminated C string argument of a literal "%s" format.
// Handing a Go string to libc.VaList instead puts the 16 byte {ptr, len} header
// in the va_list; C reads the first word as a char* and, Go strings not being
// NUL terminated, runs past the end of the string appending whatever sits next
// to it on the Go heap. Making the message itself the format is not an option
// either: messages embed module names and error texts originating in the
// script, where "%n" would become a write primitive.
func TestThrowMessage(t *testing.T) {
	for _, test := range []struct{ name, msg string }{
		// A substring, so that what follows it in memory is known, and not a NUL.
		{"overread", ("boom" + "-trailing-garbage")[:len("boom")]},
		{"percent", "100% done"},
		{"percentS", "bad value: %s"},
		{"percentD", "bad value: %d"},
		{"percentPercent", "literal %% here"},
		{"percentMix", "%p %x %lu"},
		{"percentN", "%n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if g, e := throwMsg(t, test.msg), "TypeError: "+test.msg; g != e {
				t.Errorf("message = %q; want %q", g, e)
			}
		})
	}
}
