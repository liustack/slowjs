// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"testing"
	"time"
)

// hostCases are host functions that run into an interrupt or out of memory
// in javascript they evaluate, and either return the error or drop it.
var hostCases = []struct {
	name             string
	nested           string
	ignore           bool
	errName, message string
	kind             errorKind
	setup            func(m *VM)
}{
	{"timeout", `for (;;);`, false, "InternalError", "interrupted", interrupted, func(m *VM) { m.SetEvalTimeout(50 * time.Millisecond) }},
	{"timeout dropped", `for (;;);`, true, "InternalError", "interrupted", interrupted, func(m *VM) { m.SetEvalTimeout(50 * time.Millisecond) }},
	{"interrupt", `stop(); for (;;);`, false, "InternalError", "interrupted", interrupted, func(m *VM) { m.RegisterFunc("stop", m.Interrupt, false) }},
	{"out of memory", `grow()`, false, "InternalError", "out of memory", outOfMemory, func(m *VM) { m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20) }},
	{"out of memory dropped", `grow()`, true, "InternalError", "out of memory", outOfMemory, func(m *VM) { m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20) }},
}

// An interrupt or running out of memory in javascript that a host function
// evaluates ends the whole call, as it does in the calling script: page code
// around the host function cannot catch it, whatever the host function does
// with the error, and the call returns it.
func TestUncatchableThroughHostFunc(t *testing.T) {
	for _, h := range hostCases {
		for _, c := range uncatchableCases {
			t.Run(h.name+"/"+c.name, func(t *testing.T) {
				m := newTestVM(t)
				var nestedErr error
				if err := m.RegisterHostFunc("host", func(args []any) (any, error) {
					_, err := m.Eval(h.nested, EvalGlobal)
					nestedErr = err
					if h.ignore {
						return "fine", nil
					}
					return nil, err
				}); err != nil {
					t.Fatal(err)
				}

				if _, err := m.Eval(`globalThis.seen = []; globalThis.grow = () => { const a = []; for (;;) a.push('x'.repeat(256) + a.length) }`, EvalGlobal); err != nil {
					t.Fatal(err)
				}

				// Page code reaches the host function through fail().
				js := `globalThis.fail = () => { host(); seen.push('after host') }; ` + c.js
				h.setup(m)
				err, seen := runUncatchable(t, m, js)
				wantError(t, err, h.errName, h.message)
				wantKind(t, err, h.kind)
				wantKind(t, nestedErr, h.kind)
				if seen != "" {
					t.Fatalf("page code saw the error: %s", seen)
				}
				wantNoPendingException(t, m)
			})
		}
	}
}

// The same holds for the module loader and normalizer: a timeout in
// javascript they evaluate ends the import, even when they drop the error.
func TestUncatchableThroughModuleLoader(t *testing.T) {
	for _, which := range []string{"loader", "normalize"} {
		t.Run(which, func(t *testing.T) {
			m := newTestVM(t)
			run := func(m *VM) { m.Eval(`for (;;);`, EvalGlobal) }
			m.SetModuleLoader(
				func(m *VM, name string) (string, error) {
					if which == "loader" {
						run(m)
					}
					return `export default 1`, nil
				},
				func(m *VM, base, name string) (string, error) {
					if which == "normalize" {
						run(m)
					}
					return name, nil
				},
			)
			if _, err := m.Eval(`globalThis.seen = []`, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			m.SetEvalTimeout(50 * time.Millisecond)
			err, seen := runUncatchable(t, m, `import('m').then(() => seen.push('loaded'), () => seen.push('rejection'))`)
			wantError(t, err, "InternalError", "interrupted")
			wantKind(t, err, interrupted)
			if seen != "" {
				t.Fatalf("page code saw the error: %s", seen)
			}
			wantNoPendingException(t, m)
		})
	}
}
