// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"testing"
	"time"
)

// spin is a busy loop of 300 ms in javascript. A VM with a 10 ms eval timeout
// has to stop it after about 10 ms, whichever method of the Go API ran it.
const spin = `function spin() { const t = Date.now(); while (Date.now() - t < 300) {} }`

// TestAPIEntriesHaveTheEvalTimeout runs javascript from every method of the Go
// API that can: a getter, a setter, a proxy trap, toJSON, a replaced
// BigInt.prototype.toString, and the toString or the getter that reads the
// error an engine call threw. Each must end with the interrupted error at the
// eval timeout, as Eval and Call do, and not run to the end of the loop.
func TestAPIEntriesHaveTheEvalTimeout(t *testing.T) {
	const timeout = 10 * time.Millisecond
	const limit = 150 * time.Millisecond

	setup := spin + `
globalThis.hostile = {
  get x() { spin(); return 42 },
  set x(v) { spin() },
  toJSON() { spin(); return 1 },
};
globalThis.getterObj = { get g() { spin(); return 1 }, y: 2 };
globalThis.proxy = new Proxy({}, {
  get() { spin(); return 7 },
  set() { spin(); return true },
  getPrototypeOf() { spin(); return null },
  ownKeys() { spin(); return [] },
});
globalThis.frozen = Object.freeze({ a: 1 });
globalThis.makeBigInt = () => 12345678901234567890n;
globalThis.errorWithGetter = new TypeError("x");
Object.defineProperty(globalThis.errorWithGetter, "name", { get() { spin(); return "N" } });
globalThis.errorWithToString = new Error("y");
globalThis.errorWithToString.toString = function () { spin(); return "T" };
globalThis.fixed = 1;
Object.defineProperty(globalThis, "fixed", { value: 1, configurable: false });
`

	// readsError names the cases where the loop runs while the error of a call is
	// read. A field the timeout cut off is left empty (see ErrorFromValue), so
	// the call ends with that error and not with the interrupted one.
	readsError := map[string]bool{
		"SetProperty of a frozen object, error read": true,
		"RegisterFunc error read":                    true,
		"RegisterCopyProps error read":               true,
		"RegisterEscapeText error read":              true,
	}
	// ErrorFromValue has no error result.
	noResult := map[string]bool{
		"ErrorFromValue name getter": true,
		"ErrorFromValue toString":    true,
	}

	cases := []struct {
		name string
		// patch runs in the page before the timeout is set, and may be empty.
		patch string
		run   func(t *testing.T, m *VM) error
	}{
		{"GetProperty", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			_, err := m.GetProperty(obj, atomOrFatal(t, m, "x"))
			return err
		}},
		{"GetPropertyValue", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			v, err := m.GetPropertyValue(obj, atomOrFatal(t, m, "x"))
			if err == nil {
				v.Free()
			}
			return err
		}},
		{"Value.Property", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			_, err := obj.Property(atomOrFatal(t, m, "x"))
			return err
		}},
		{"Value.GetPropertyValue", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			v, err := obj.GetPropertyValue(atomOrFatal(t, m, "x"))
			if err == nil {
				v.Free()
			}
			return err
		}},
		{"SetProperty", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			return m.SetProperty(obj, atomOrFatal(t, m, "x"), 1)
		}},
		{"SetPropertyValue", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			return m.SetPropertyValue(obj, atomOrFatal(t, m, "x"), m.NewInt(1))
		}},
		{"Value.SetProperty", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			return obj.SetProperty(atomOrFatal(t, m, "x"), 1)
		}},
		{"Value.SetPropertyValue", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			return obj.SetPropertyValue(atomOrFatal(t, m, "x"), m.NewInt(1))
		}},
		{"GetProperty of a proxy", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "proxy")
			defer obj.Free()
			_, err := m.GetProperty(obj, atomOrFatal(t, m, "z"))
			return err
		}},
		{"SetProperty of a proxy", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "proxy")
			defer obj.Free()
			return m.SetProperty(obj, atomOrFatal(t, m, "z"), 1)
		}},
		{"SetProperty of a frozen object, error read", `Object.defineProperty(TypeError.prototype, "name", { get() { spin(); return "TypeError" }, configurable: true })`, func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "frozen")
			defer obj.Free()
			return m.SetProperty(obj, atomOrFatal(t, m, "b"), 1)
		}},
		{"Value.MarshalJSON", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "hostile")
			defer obj.Free()
			_, err := obj.MarshalJSON()
			return err
		}},
		{"Value.Any of an object", "", func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "getterObj")
			defer obj.Free()
			_, err := obj.Any()
			return err
		}},
		{"Value.Any of a BigInt", `BigInt.prototype.toString = function () { spin(); return "1" }`, func(t *testing.T, m *VM) error {
			v := evalValueOrFatal(t, m, "makeBigInt()")
			defer v.Free()
			_, err := v.Any()
			return err
		}},
		{"GetProperty of a BigInt", `BigInt.prototype.toString = function () { spin(); return "1" }`, func(t *testing.T, m *VM) error {
			obj := evalValueOrFatal(t, m, "({ b: 12345678901234567890n })")
			defer obj.Free()
			_, err := m.GetProperty(obj, atomOrFatal(t, m, "b"))
			return err
		}},
		{"ErrorFromValue name getter", "", func(t *testing.T, m *VM) error {
			e := evalValueOrFatal(t, m, "errorWithGetter")
			defer e.Free()
			return ErrorFromValue(e)
		}},
		{"ErrorFromValue toString", "", func(t *testing.T, m *VM) error {
			e := evalValueOrFatal(t, m, "errorWithToString")
			defer e.Free()
			return ErrorFromValue(e)
		}},
		{"RegisterFunc error read", `Object.defineProperty(TypeError.prototype, "name", { get() { spin(); return "TypeError" }, configurable: true })`, func(t *testing.T, m *VM) error {
			return m.RegisterFunc("fixed", func() int { return 1 }, false)
		}},
		{"RegisterCopyProps error read", `Object.defineProperty(TypeError.prototype, "name", { get() { spin(); return "TypeError" }, configurable: true })`, func(t *testing.T, m *VM) error {
			return m.RegisterCopyProps("fixed")
		}},
		{"RegisterEscapeText error read", `Object.defineProperty(TypeError.prototype, "name", { get() { spin(); return "TypeError" }, configurable: true })`, func(t *testing.T, m *VM) error {
			return m.RegisterEscapeText("fixed")
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()

			if _, err := m.Eval(setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if c.patch != "" {
				if _, err := m.Eval(c.patch, EvalGlobal); err != nil {
					t.Fatal(err)
				}
			}
			if err := m.SetEvalTimeout(timeout); err != nil {
				t.Fatal(err)
			}

			start := time.Now()
			err = c.run(t, m)
			elapsed := time.Since(start)

			var e *Error
			switch {
			case noResult[c.name]:
			case readsError[c.name]:
				if err == nil {
					t.Errorf("the call returned no error")
				}
			case !errors.As(err, &e) || !e.Interrupted:
				t.Errorf("the error is %v, want the interrupted error", err)
			}
			if elapsed > limit {
				t.Errorf("took %v, the eval timeout is %v", elapsed, timeout)
			}
		})
	}
}

func evalValueOrFatal(t *testing.T, m *VM, src string) Value {
	t.Helper()
	v, err := m.EvalValue(src, EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

func atomOrFatal(t *testing.T, m *VM, s string) Atom {
	t.Helper()
	a, err := m.NewAtom(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestErrorFromValueTimeout reads an error whose name getter and toString run
// for 300 ms, in a VM with a 10 ms eval timeout. The fields it could not read
// are left empty, the others are read, no exception stays pending, and the
// next call, also another ErrorFromValue, works. The same inside a Go
// function called from a script, where the outer call's time runs.
func TestErrorFromValueTimeout(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.Eval(spin+`
globalThis.slowName = new TypeError("m1");
Object.defineProperty(slowName, "name", { get() { spin(); return "N" } });
globalThis.slowString = new RangeError("m2");
slowString.toString = function () { spin(); return "T" };
globalThis.plain = new SyntaxError("m3");`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEvalTimeout(10 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	read := func(name string) *Error {
		t.Helper()
		v := evalValueOrFatal(t, m, name)
		defer v.Free()
		start := time.Now()
		e := ErrorFromValue(v)
		if d := time.Since(start); d > 150*time.Millisecond {
			t.Errorf("reading %s took %v", name, d)
		}
		return e
	}
	check := func(e *Error, name, message, raw string) {
		t.Helper()
		if e.Name != name || e.Message != message || e.Error() != raw {
			t.Errorf("got name %q, message %q, string %q, want %q, %q, %q", e.Name, e.Message, e.Error(), name, message, raw)
		}
		wantNoPendingException(t, m)
		got := read("plain")
		if got.Name != "SyntaxError" || got.Message != "m3" || got.Error() != "SyntaxError: m3" {
			t.Errorf("the next read gave %q, %q, %q", got.Name, got.Message, got.Error())
		}
		if r, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || r != 2 {
			t.Errorf("the next call gave %v, %v", r, err)
		}
	}
	check(read("slowName"), "", "m1", "")
	check(read("slowString"), "RangeError", "m2", "")
	if e := read("plain"); e.Error() != "SyntaxError: m3" {
		t.Errorf("a plain error reads as %q", e.Error())
	}

	// Inside a Go function the call around it keeps the time.
	var inner *Error
	mustHost(t, m, "readNested", func([]any) (any, error) {
		v := evalValueOrFatal(t, m, "slowName")
		defer v.Free()
		inner = ErrorFromValue(v)
		return nil, nil
	})
	if err := m.SetEvalTimeout(300 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// The script is stopped by the same deadline, so the host function reads
	// what it can of the name before it.
	start := time.Now()
	r, err := m.Eval(`readNested(); spin(); "done"`, EvalGlobal)
	if d := time.Since(start); d > 700*time.Millisecond {
		t.Errorf("the call took %v", d)
	}
	wantInterrupted(t, r, err)
	if inner == nil || inner.Message != "m1" {
		t.Errorf("the nested read gave %#v", inner)
	}
	wantNoPendingException(t, m)
	// A read that returns before the deadline leaves the deadline of the call
	// around it running: the script after it is interrupted, not finished.
	var quick *Error
	mustHost(t, m, "readQuick", func([]any) (any, error) {
		v := evalValueOrFatal(t, m, "plain")
		defer v.Free()
		quick = ErrorFromValue(v)
		return nil, nil
	})
	if err := m.SetEvalTimeout(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	r, err = m.Eval(`readQuick(); spin(); "done"`, EvalGlobal)
	if d := time.Since(start); d > 290*time.Millisecond {
		t.Errorf("the call took %v, the timeout is 200 ms and spin runs 300 ms", d)
	}
	if quick == nil || quick.Name != "SyntaxError" || quick.Message != "m3" {
		t.Errorf("the quick read gave %#v", quick)
	}
	wantInterrupted(t, r, err)
	wantNoPendingException(t, m)
	if err := m.SetEvalTimeout(0); err != nil {
		t.Fatal(err)
	}
	if r, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || r != 2 {
		t.Errorf("the next call gave %v, %v", r, err)
	}
}

// wantInterrupted requires that an Eval ended with the interrupted error and
// no result.
func wantInterrupted(t *testing.T, r any, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || !e.Interrupted {
		t.Errorf("the call gave %v, %v, want the interrupted error", r, err)
	}
	if r != nil {
		t.Errorf("the call gave the result %v", r)
	}
}
