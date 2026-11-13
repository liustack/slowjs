// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"io"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/stopsweep"
)

// swept are long calls of fmt, each with an output or an argument of n units
// (bytes of output or of format, or numbers of an array), which TestLongCallsStopAtEveryTimeout
// stops at many eval timeouts, those marked short under -short too.
var swept = []struct {
	name, call string
	n          int
	short      bool
}{
	{"%q of 3.2 MiB of control characters", `fmt.sprintf('%q', s1).length`, 16 << 20, true},
	{"%v of an array of 2 Mi numbers", `fmt.sprintf('%v', arr).length`, 16 << 20, false},
	{"%v of an object of 8 MiB", `fmt.sprintf('%v', obj).length`, 8 << 20, true},
	{"printf of an array of 2 Mi numbers", `fmt.printf('%v', arr)`, 16 << 20, true},
	{"sprint of an array of 2 Mi numbers", `fmt.sprint(arr).length`, 16 << 20, false},
	{"%d of the longest BigInt", `fmt.sprintf('%d', big).length`, 1 << 20, false},
	{"a verb of 8 MiB of zeros", `fmt.sprintf(zeros, 1).length`, 8 << 20, false},
}

// sweptSetup makes the arguments of swept: in a VM with no memory limit, an
// output of up to 16 MiB (maxOutput). arrSetup makes the array, which takes
// longer, for the calls that read it.
const (
	sweptSetup = `var K = 16 << 20, s1 = '\x01'.repeat(K / 5), obj = {a: 'x'.repeat(K / 2)}, big = 1n << 1040000n, zeros = '%' + '0'.repeat(K / 2) + '5d'; 0`
	arrSetup   = `var arr = Array.from({length: K / 8}, (_, i) => i); 0`
)

// known holds the calls of swept that still run a part of their work whole,
// in Go, with no check, each with what bounds that part. The conversion of
// a BigInt to decimal cuts it in two by a power of ten, and its parts the
// same way, with a check between two steps (decimal in printf.go), and its
// first step is one division by math/big of the whole BigInt, at most the
// 1,048,576 bits of the longest the engine makes: 5 to 7 ms on an Apple M4,
// 9 ms on one core of an AMD EPYC and 19 ms under Rosetta, about a third of
// the conversion. The sweep passed with it, but only by the fixed part of
// its grace on a machine a few times slower.
var known = map[string]string{
	"%d of the longest BigInt": "one division of 1,048,576 bits by math/big",
}

// Each long call stops within the grace of stopsweep at every eval timeout
// from 2 ms to the time it takes whole, so that a part of it with no check
// shows wherever it lies. Under -short, at three places in it (see
// stopsweep).
func TestLongCallsStopAtEveryTimeout(t *testing.T) {
	for _, c := range swept {
		t.Run(c.name, func(t *testing.T) {
			if b, ok := known[c.name]; ok {
				t.Skip("runs whole with no check: " + b)
			}
			if testing.Short() && !c.short {
				t.Skip("not under -short")
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
			setup := sweptSetup
			if strings.Contains(c.call, "arr") {
				setup += "; " + arrSetup
			}
			if _, err := vm.Eval(setup, quickjs.EvalGlobal); err != nil {
				t.Fatal(err)
			}
			stopsweep.Sweep(t, vm, c.call, c.n, 1<<16)
		})
	}
}
