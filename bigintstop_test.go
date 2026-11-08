// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
	"time"
)

// The eval timeout stops one long operation on BigInts of up to a million
// bits within a few polls: the loops whose work grows as the square of the
// length (a product, a division, a power, a conversion to and from a string
// in a radix that is not a power of two) and the loops over each digit read
// the interrupt flag as they go. Before, String(10n ** 300000n) ran on for
// about 2 s, past the grace of the kill, and left the VM good only for
// Close. The call ends with the uncatchable interrupt and the VM goes on.
func TestBigIntWorkStops(t *testing.T) {
	const (
		// 300,000 decimal digits, about 1,000,000 bits
		big = `var x = 10n ** 300000n`
		// two numbers of half a million bits, every bit set
		halves = `var a = 2n ** 500000n - 1n, b = 2n ** 499999n + 12345n`
		// a number of a million bits, and one of half a million
		whole = `var w = 2n ** 1000000n - 1n, h = 2n ** 500000n + 12345n`
	)
	for _, c := range []struct{ name, setup, call string }{
		{"String", big, `String(x)`},
		{"toString", big, `x.toString()`},
		{"toString radix 7", big, `x.toString(7)`},
		{"toString radix 2", big, `x.toString(2)`},
		{"template literal", big, "`${x}`"},
		{"String of a negative", big + `; x = -x`, `String(x)`},
		{"power", ``, `10n ** 300000n`},
		{"power of a long base", `var p = 3n ** 10000n`, `p ** 60n`},
		{"product", halves, `a * b`},
		{"product of negatives", halves, `(-a) * (-b)`},
		{"quotient", whole, `w / h`},
		{"remainder", whole, `w % h`},
		{"BigInt of a decimal string", `var s = '9'.repeat(300000)`, `BigInt(s)`},
		{"compared with a string", big + `; var xs = '1' + '0'.repeat(300000)`, `x == xs`},
		{"less than a string", big + `; var xs = '1' + '0'.repeat(300000)`, `x < xs`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			_, err := m.Eval(`for (;;) `+c.call, EvalGlobal)
			took := time.Since(start)
			if errors.Is(err, ErrKilled) {
				t.Fatalf("killed after %v, want the interrupt", took)
			}
			var e *Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
				t.Fatalf("got %v after %v, want interrupted", err, took)
			}
			if n := m.h.StopPolls(); n > bigIntStopPolls {
				t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the call started)", n, bigIntStopPolls, took)
			}
			t.Logf("stopped after %v, %d polls saw the request", took, m.h.StopPolls())
			if err := m.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
				t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
			}
		})
	}
}

// bigIntStopPolls is the most polls TestBigIntWorkStops lets see the request:
// the loops read the flag every 32,768 limbs (of 32 bits) or digits they go
// through, a step of up to a few polls where a limb takes a division, and
// the error, the free of the operands and the result made so far take a few
// more.
const bigIntStopPolls = 8
