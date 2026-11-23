// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import "testing"

// panicJSON is an argument whose JSON panics.
type panicJSON struct{}

func (panicJSON) MarshalJSON() ([]byte, error) { panic("boom") }

// A panic that leaves a conversion halfway, from a Go function javascript
// called while a result converts or from the JSON of an argument, frees
// what the conversion held: the result, the value a replaced toString
// returned, and the arguments converted before. The VM goes on, and frees
// every block when it closes.
func TestPanicInConversionLeaksNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		run  func(t *testing.T, m *VM)
	}{
		{"no conversion", func(t *testing.T, m *VM) {
			m.Eval(`boom()`, EvalGlobal)
		}},
		{"result toJSON", func(t *testing.T, m *VM) {
			m.Eval(`({ payload: new ArrayBuffer(1 << 20), toJSON() { boom() } })`, EvalGlobal)
		}},
		{"result of Call", func(t *testing.T, m *VM) {
			m.Call(`(() => ({ payload: new ArrayBuffer(1 << 20), toJSON() { boom() } }))`)
		}},
		{"BigInt toString", func(t *testing.T, m *VM) {
			m.Eval(`BigInt.prototype.toString = function () { return { payload: new ArrayBuffer(1 << 20), toString() { boom() } } }; 1n`, EvalGlobal)
		}},
		{"arguments", func(t *testing.T, m *VM) {
			large := evalValueOrFatal(t, m, `new ArrayBuffer(1 << 20)`)
			defer large.Free()
			m.Call(`(function (a, b) {})`, large, "a string of its own", panicJSON{})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			if err := m.RegisterHostFunc("boom", func([]any) (any, error) { panic("boom") }); err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if p := recover(); p != "boom" {
						t.Errorf("recovered %v, want the panic boom", p)
					}
				}()
				c.run(t, m)
			}()
			wantEval(t, m, `1 + 1`, 2)
			if n := closeLeaked(t, m); n != 0 {
				t.Errorf("the runtime freed with %d blocks still in use", n)
			}
		})
	}
}
