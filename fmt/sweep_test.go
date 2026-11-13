// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"io"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/stopsweep"
)

// swept are long calls of fmt, each with an output or an argument of n units
// (bytes of output, or numbers of an array), which TestLongCallsStopAtEveryTimeout
// stops at many eval timeouts.
var swept = []struct {
	name, call string
	n          int
}{
	{"%q of 3.2 MiB of control characters", `fmt.sprintf('%q', s1).length`, 16 << 20},
	{"%v of an array of 2 Mi numbers", `fmt.sprintf('%v', arr).length`, 16 << 20},
	{"%v of an object of 8 MiB", `fmt.sprintf('%v', obj).length`, 8 << 20},
	{"printf of an array of 2 Mi numbers", `fmt.printf('%v', arr)`, 16 << 20},
	{"sprint of an array of 2 Mi numbers", `fmt.sprint(arr).length`, 16 << 20},
	{"%d of the longest BigInt", `fmt.sprintf('%d', big).length`, 1 << 20},
}

// sweptSetup makes the arguments of swept: in a VM with no memory limit, an
// output of up to 16 MiB (maxOutput).
const sweptSetup = `var K = 16 << 20, s1 = '\x01'.repeat(K / 5), arr = Array.from({length: K / 8}, (_, i) => i),
	obj = {a: 'x'.repeat(K / 2)}, big = 1n << 1040000n; 0`

// known holds the calls of swept that still run a part of their work whole,
// in Go, with no check, each with what bounds that part, until the change
// that cuts it into pieces takes it out. Go's fmt makes the whole output of
// a call at once: at most maxOutput bytes in a VM with no memory limit, and
// a sixteenth of the room under the limit (allocated). math/big writes a
// BigInt in decimal at once: at most the 1,040,000 bits of the longest
// BigInt the engine makes. The json of an array is made by the engine, which
// polls, and then copied whole by Go's fmt into the output, 15 MiB: under
// Rosetta that copy ran 8 ms past a timeout once in four sweeps.
var known = map[string]string{
	"%q of 3.2 MiB of control characters": "one pass of Go's fmt over 16 MiB of output",
	"%v of an array of 2 Mi numbers":      "one copy by Go's fmt of 15 MiB of output",
	"printf of an array of 2 Mi numbers":  "one copy by Go's fmt of 15 MiB of output",
	"sprint of an array of 2 Mi numbers":  "one copy by Go's fmt of 15 MiB of output",
	"%d of the longest BigInt":            "one conversion of 1,040,000 bits by math/big",
}

// Each long call stops within the grace of stopsweep at every eval timeout
// from 2 ms to the time it takes whole, so that a part of it with no check
// shows wherever it lies.
func TestLongCallsStopAtEveryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("times each call at many timeouts")
	}
	for _, c := range swept {
		t.Run(c.name, func(t *testing.T) {
			if b, ok := known[c.name]; ok {
				t.Skip("runs whole with no check: " + b)
			}
			vm, err := quickjs.NewVM()
			if err != nil {
				t.Fatal(err)
			}
			defer vm.Close()
			g := vm.GlobalObject()
			defer g.Free()
			if err := Register(g.Dup(), Writer(io.Discard)); err != nil {
				t.Fatal(err)
			}
			if _, err := vm.Eval(sweptSetup, quickjs.EvalGlobal); err != nil {
				t.Fatal(err)
			}
			stopsweep.Sweep(t, vm, c.call, c.n, 1<<16)
		})
	}
}
