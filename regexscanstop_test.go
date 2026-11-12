// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// One call of a regexp that finds nothing in a string of 32 Mi units stops
// within stepStopPolls polls of the request, by Interrupt or the eval
// timeout. lre_find looks for where a match can start without the
// backtracking machine, through the table of first units of the regexp
// (lre_plan in engine/libregexp.c), memchr for a first character (and then
// the rest of a literal prefix before the machine), or the
// class itself in a string of 16 bits, and reads the interrupt flag after
// every 65,536 units, 16,384 for the class in a string of 16 bits, whose
// binary search costs more for each unit: a scan that read it only at the end
// of the string would let hundreds of polls see the request. Before the table,
// the binary search of a class of a wide span such as seroval's escape in a
// string of 8 bits let 11 to 14 polls see it.
func TestRegexLongScanStops(t *testing.T) {
	for _, c := range []struct {
		name string
		arm  func(m *VM) (stop func())
	}{
		{"timeout", func(m *VM) func() {
			m.SetEvalTimeout(10 * time.Millisecond)
			return func() {}
		}},
		{"interrupt", func(m *VM) func() { return interruptAfter(m, 10*time.Millisecond) }},
	} {
		for _, w := range []struct{ name, s, re string }{
			{"table/class", `'a'.repeat(1 << 25)`, `/["&<>]/`},
			{"table/set", `'x'.repeat(1 << 25)`, `/(?:a|c)b/`},
			{"table/escape", `'a'.repeat(1 << 25)`, `/[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]/`},
			{"memchr", `'x'.repeat(1 << 25)`, `/ab/`},
			{"prefix", `'lab'.repeat(1 << 23)`, `/label:/`},
			{"16bit/class", `'\u4e2d'.repeat(1 << 24)`, `/["&<>]/`},
			{"16bit/set", `'\u4e2d'.repeat(1 << 24)`, `/(?:a|c)b/`},
			{"16bit/escape", `'\u4e2d'.repeat(1 << 24)`, `/[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]/`},
		} {
			t.Run(c.name+"/"+w.name, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(`var s = `+w.s+`; var re = `+w.re+`; 0`, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				stop := c.arm(m)
				defer stop()
				start := time.Now()
				_, err := m.Eval(`for (;;) re.test(s)`, EvalGlobal)
				checkStopped(t, m, err, time.Since(start), stepStopPolls, "")
			})
		}
	}
}
