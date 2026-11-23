// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRegisterHostFunc(t *testing.T) {
	m := newTestVM(t)
	mustHost(t, m, "join", func(args []any) (any, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = fmt.Sprint(a)
		}
		return strings.Join(parts, "-"), nil
	})
	mustHost(t, m, "sum", func(args []any) (any, error) {
		total := 0.0
		for _, a := range args {
			switch x := a.(type) {
			case int:
				total += float64(x)
			case float64:
				total += x
			default:
				return nil, fmt.Errorf("not a number: %v", a)
			}
		}
		return total, nil
	})
	mustHost(t, m, "boom", func([]any) (any, error) { return nil, errors.New("kaboom") })
	mustHost(t, m, "nada", func([]any) (any, error) { return nil, nil })

	wantEval(t, m, `join("a", 1, true)`, "a-1-true")
	// One result is one value, not an array of one.
	wantEval(t, m, `Array.isArray(join("a"))`, false)
	wantEval(t, m, `sum(1, 2, 3.5)`, 6.5)
	wantEval(t, m, `try { boom(); 'no throw' } catch (e) { e.message.includes('kaboom') }`, true)
	if _, err := m.Eval(`boom()`, EvalGlobal); err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("boom() uncaught: %v", err)
	}
	wantEval(t, m, `nada() == null`, true)
}

func TestRegisterHostFuncValidation(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterHostFunc("nofn", nil); err == nil {
		t.Error("a nil function: no error")
	}
	add := func(args []any) (any, error) { return args[0].(int) + args[1].(int), nil }
	if err := m.RegisterHostFunc("add", add); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHostFunc("add", add); err == nil {
		t.Error("the same name again: no error")
	}
	if err := m.RegisterFunc("id", func(x int) int { return x }, false); err != nil {
		t.Fatal(err)
	}
	wantEval(t, m, `add(2, 3) + id(5)`, 10)
}

// The message of a Go error is text, never a format: what looks like a verb
// in it comes out as it is, and only its own bytes are read.
func TestThrowMessage(t *testing.T) {
	for _, msg := range []string{
		"boom-trailing-garbage"[:4],
		"100% done",
		"bad value: %s",
		"bad value: %d",
		"literal %% here",
		"%p %x %lu",
		"%n",
	} {
		m := newTestVM(t)
		mustHost(t, m, "boom", func([]any) (any, error) { return nil, errors.New(msg) })
		_, err := m.Eval(`boom()`, EvalGlobal)
		if err == nil || err.Error() != "TypeError: "+msg {
			t.Errorf("%q: got %v", msg, err)
		}
	}
}
