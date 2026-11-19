// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// pastDeadline is how far past a deadline a test sleeps to be sure it has
// passed.
const pastDeadline = 10 * time.Millisecond

// hookJS defines, after a script that defines loop, the values whose use from
// Go runs loop: the getter and the setter of hook.x and hook's toJSON, the
// name getter of hookErr, and the toString of every BigInt, which hookBig is.
// It starts with a semicolon, to go right after that script.
const hookJS = `;
globalThis.hook = {
	get x() { loop(); return 1 },
	set x(v) { loop() },
	toJSON() { loop(); return 1 },
};
globalThis.hookErr = new Error("e");
Object.defineProperty(hookErr, "name", { get() { loop(); return "N" } });
globalThis.hookBig = 12345678901234567890n;
BigInt.prototype.toString = function () { loop(); return "1" };
`

// hookValue reads the global name, a plain data property, which runs no
// javascript: it works past a deadline too.
func hookValue(m *VM, name string) (Value, error) {
	g := m.GlobalObject()
	defer g.Free()
	atom, err := m.NewAtom(name)
	if err != nil {
		return Value{}, err
	}
	return m.GetPropertyValue(g, atom)
}

// nestedEntries are the ways a Go function that javascript called runs
// javascript on its own VM again. Each runs the global loop, through what
// hookJS defined, and returns the error it got.
var nestedEntries = map[string]func(m *VM) error{
	"GetProperty": func(m *VM) error {
		hook, err := hookValue(m, "hook")
		if err != nil {
			return err
		}
		defer hook.Free()
		atom, err := m.NewAtom("x")
		if err != nil {
			return err
		}
		_, err = m.GetProperty(hook, atom)
		return err
	},
	"SetProperty": func(m *VM) error {
		hook, err := hookValue(m, "hook")
		if err != nil {
			return err
		}
		defer hook.Free()
		atom, err := m.NewAtom("x")
		if err != nil {
			return err
		}
		return m.SetProperty(hook, atom, 1)
	},
	"Value.Any of an object": func(m *VM) error {
		hook, err := hookValue(m, "hook")
		if err != nil {
			return err
		}
		defer hook.Free()
		_, err = hook.Any()
		return err
	},
	"Value.Any of a BigInt": func(m *VM) error {
		big, err := hookValue(m, "hookBig")
		if err != nil {
			return err
		}
		defer big.Free()
		_, err = big.Any()
		return err
	},
	"MarshalJSON": func(m *VM) error {
		hook, err := hookValue(m, "hook")
		if err != nil {
			return err
		}
		defer hook.Free()
		_, err = hook.MarshalJSON()
		return err
	},
	"ErrorFromValue": func(m *VM) error {
		e, err := hookValue(m, "hookErr")
		if err != nil {
			return err
		}
		defer e.Free()
		if got := ErrorFromValue(e); got.Uncatchable {
			return got
		}
		return errors.New("the read of the name was not stopped")
	},
	"Eval": func(m *VM) error {
		_, err := m.Eval(`loop()`, EvalGlobal)
		return err
	},
	"Call": func(m *VM) error {
		_, err := m.Call("loop")
		return err
	},
	"EvalBytecode": func(m *VM) error {
		code, err := m.Compile(`loop()`, EvalGlobal)
		if err != nil {
			return err
		}
		_, err = m.EvalBytecode(code)
		return err
	},
	"EvalBytecode of a module": func(m *VM) error {
		c, err := NewVM()
		if err != nil {
			return err
		}
		code, err := c.Compile("await null; loop()\n", EvalModule)
		c.Close()
		if err != nil {
			return err
		}
		_, err = m.EvalBytecode(code)
		return err
	},
	"ExecutePendingJobs": func(m *VM) error {
		if _, err := m.Eval(`globalThis.jobError = ''; Promise.resolve().then(loop).catch(e => { jobError = String(e) })`, EvalGlobal); err != nil {
			return err
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			return err
		}
		v, err := hookValue(m, "jobError")
		if err != nil {
			return err
		}
		defer v.Free()
		if s, err := v.Any(); err != nil || s != "" {
			return fmt.Errorf("a script caught the stop of the job: %v, %v", s, err)
		}
		return nil
	},
}

// finiteLoop is a loop that ends, many polls long.
const finiteLoop = `globalThis.loop = function () { for (let i = 0; i < 1e5; i++) {} }`

// The deadline of a call is a point in time, set when the outermost call
// starts. A Call or ExecutePendingJobs that kept the one an Eval set before a
// long idle time was interrupted at once.
func TestCallRearmsEvalTimeout(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`function quick() { for (let i = 0; i < 1e5; i++) {} } function runaway() { for (;;) {} }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.SetEvalTimeout(50 * time.Millisecond)
	if _, err := m.Eval(`0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	// The setting changes, the deadline of the last call is long past.
	m.SetEvalTimeout(10 * time.Second)
	if _, err := m.Call("quick"); err != nil {
		t.Fatalf("Call after an idle time: %v", err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatalf("ExecutePendingJobs after an idle time: %v", err)
	}
	m.SetEvalTimeout(50 * time.Millisecond)
	if _, err := m.Call("runaway"); err == nil {
		t.Fatal("a call that runs forever returned")
	}
}

// The same for Value.Call and Value.CallValue.
func TestValueCallRearmsEvalTimeout(t *testing.T) {
	m := newTestVM(t)
	quick := evalValueOrFatal(t, m, `(function () { for (let i = 0; i < 1e5; i++) {} })`)
	defer quick.Free()
	runaway := evalValueOrFatal(t, m, `(function () { for (;;) {} })`)
	defer runaway.Free()
	stale := func() {
		t.Helper()
		m.SetEvalTimeout(50 * time.Millisecond)
		if _, err := m.Eval(`0`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50*time.Millisecond + pastDeadline)
		m.SetEvalTimeout(10 * time.Second)
	}
	stale()
	if _, err := quick.Call(UndefinedValue); err != nil {
		t.Fatalf("Value.Call after an idle time: %v", err)
	}
	stale()
	v, err := quick.CallValue(UndefinedValue)
	if err != nil {
		t.Fatalf("Value.CallValue after an idle time: %v", err)
	}
	v.Free()
	m.SetEvalTimeout(50 * time.Millisecond)
	if _, err := runaway.Call(UndefinedValue); err == nil {
		t.Fatal("a call that runs forever returned")
	}
}

// Javascript a host function runs on its VM has the time left to the outer
// call, not a timeout of its own: past the deadline both fail. The next
// outer call has the whole timeout again.
func TestNestedEvalKeepsTimeout(t *testing.T) {
	for name, entry := range nestedEntries {
		t.Run(name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(finiteLoop+hookJS, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			const timeout = 300 * time.Millisecond
			var nestedErr error
			mustHost(t, m, "nested", func([]any) (any, error) {
				time.Sleep(timeout + pastDeadline)
				nestedErr = entry(m)
				return nil, nil
			})
			m.SetEvalTimeout(timeout)
			_, err := m.Eval(`nested(); loop(); 'done'`, EvalGlobal)
			if nestedErr == nil {
				t.Error("the nested call ran past the deadline of the outer one")
			}
			if err == nil {
				t.Error("the outer call ran past its deadline")
			}
			m.SetEvalTimeout(10 * time.Second)
			if _, err := m.Eval(`loop(); 'done'`, EvalGlobal); err != nil {
				t.Fatalf("the next call: %v", err)
			}
		})
	}
}

// An Interrupt asked before a host function runs javascript stays pending
// through that nested call: the nested call does not clear it, and the outer
// script stops at it, well before the watchdog.
func TestNestedEvalKeepsInterrupt(t *testing.T) {
	for name, entry := range nestedEntries {
		t.Run(name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(finiteLoop+hookJS, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			mustHost(t, m, "nested", func([]any) (any, error) {
				m.Interrupt()
				entry(m)
				return nil, nil
			})
			const watchdog = 5 * time.Second
			start := time.Now()
			w := time.AfterFunc(watchdog, m.Interrupt)
			_, err := m.Eval(`nested(); for (;;) {}`, EvalGlobal)
			w.Stop()
			if err == nil {
				t.Fatal("the outer call returned")
			}
			if time.Since(start) >= watchdog {
				t.Fatal("only the watchdog stopped the outer call: the nested call cleared the Interrupt")
			}
			if _, err := m.Eval(`loop(); 'done'`, EvalGlobal); err != nil {
				t.Fatalf("the next call: %v", err)
			}
		})
	}
}

// stopRegexp runs src on a new VM after setup, stopped 'after' later by the
// eval timeout or Interrupt, and requires an interrupted error within
// 500 ms.
func stopRegexp(t *testing.T, setup, src string, after time.Duration) {
	t.Helper()
	for _, how := range []string{"timeout", "Interrupt"} {
		m := newTestVM(t)
		if _, err := m.Eval(setup, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if how == "timeout" {
			m.SetEvalTimeout(after)
		} else {
			defer time.AfterFunc(after, m.Interrupt).Stop()
		}
		start := time.Now()
		_, err := m.Eval(src, EvalGlobal)
		took := time.Since(start)
		if err == nil || !strings.Contains(err.Error(), "interrupted") {
			t.Errorf("%s %s: got %v, want interrupted", how, src, err)
		}
		if took > 500*time.Millisecond {
			t.Errorf("%s %s: stopped after %v", how, src, took)
		}
	}
}

// A regexp looks for where a match can start, past the backtracking
// matcher, and that search reads the interrupt flag as the matcher does.
func TestRegexScanStops(t *testing.T) {
	for _, c := range []string{"a", "中"} {
		for _, re := range []string{`/["&<>]/`, `/ab/`, `/中b/`, `/(?:a|c)b/`, `/(?:中|c)b/`} {
			stopRegexp(t, `globalThis.s = '`+c+`'.repeat(2 ** 22)`, `const re = `+re+`; for (;;) re.test(s)`, 10*time.Millisecond)
		}
	}
}

// A greedy x+ that pushes no state counts its steps and reads the interrupt
// flag too. The string starts with the literal that ends every match, so the
// search for it finds one and the matcher runs, and then holds only the
// repeated character: from each start x+ takes the rest of the string and
// gives it back one step at a time, and one test runs long.
func TestRegexMatchStops(t *testing.T) {
	for _, c := range []struct{ re, lit, char string }{
		{`/a+b/`, "b", "a"},
		{`/a+b/u`, "b", "a"},
		{`/中+b/`, "b", "中"},
		{`/A+b/i`, "b", "a"},
		{`/\w+@/`, "@", "a"},
		{`/(a)+b/`, "b", "a"},
		{`/(?:a|c)+b/`, "b", "a"},
	} {
		setup := `globalThis.s = '` + c.lit + `' + '` + c.char + `'.repeat(2 ** 17)`
		stopRegexp(t, setup, c.re+`.test(s)`, 20*time.Millisecond)
	}
}
