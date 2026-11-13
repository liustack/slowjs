// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package stopsweep is for the tests of the packages that give a VM
// functions written in Go (intl, temporal, fmt and url): it stops a long call
// at many eval timeouts, so that a part of the call that does not check the
// limits shows wherever it lies in the call, not only where one fixed timeout
// falls.
//
// The grace is set from the work, not from one machine. A function of those
// packages works on an input of n units in passes, and reads vmhook.Stop
// between pieces of at most piece units of a pass. All its passes together,
// the whole call, take T, so no pass takes more than T/n a unit, and two
// reads of vmhook.Stop are at most two pieces apart, across the end of one
// pass and the start of the next: 2·piece·T/n. The grace is twice that, and
// a fixed part for what is not work on the input: Go clears an allocation
// the size of the input, a few milliseconds for 16 MiB, and with one P
// (GOMAXPROCS=1, as under taskset on one core) the goroutine of the timeout
// waits up to one preemption of the scheduler, 10 ms, for it
// (docs/testing.md, Time in a test). A slower machine takes a longer T, and
// gets a longer grace.
//
// Under -short a sweep stops a call at three places in it, a quarter, a half
// and three quarters of its time, so that it takes a few runs of the call
// on any machine.
package stopsweep // import "github.com/liustack/quickjs/internal/stopsweep"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs"
)

// The fixed part of the grace, and what one P adds to it.
const (
	fixed     = 5 * time.Millisecond
	preemptMs = 10 * time.Millisecond
)

// first is the shortest timeout of a sweep, and step how much longer each
// next one is: a part of the call with no check shows when it is longer than
// a fifth of the time before it, and longer than the grace.
const (
	first = 2 * time.Millisecond
	step  = 1.2
)

// Grace is the most a call of n units that takes whole, and reads
// vmhook.Stop every piece units of each pass, may run past its timeout.
func Grace(whole time.Duration, n, piece int) time.Duration {
	g := fixed + time.Duration(4*float64(piece)/float64(n)*float64(whole))
	if runtime.GOMAXPROCS(0) == 1 {
		g += preemptMs
	}
	return g
}

// short are the timeouts of a sweep under -short, as parts of the time of
// the whole call: three places in the call, which a part with no check of
// more than a quarter of it cannot miss, whatever the speed of the machine.
var short = []float64{0.25, 0.5, 0.75}

// timeouts are the eval timeouts a sweep of a call that takes whole stops it
// at: under -short, the parts short of whole, and otherwise every timeout
// from 2 ms up to whole, each step times longer than the one before.
func timeouts(whole time.Duration) []time.Duration {
	var r []time.Duration
	if testing.Short() {
		for _, f := range short {
			r = append(r, time.Duration(f*float64(whole)))
		}
		return r
	}
	for timeout := first; timeout < whole; timeout = time.Duration(float64(timeout) * step) {
		r = append(r, timeout)
	}
	return r
}

// Sweep calls call on vm, an expression whose work grows with an input of n
// units that the test made before, whole first, then with each eval timeout
// of timeouts, and fails t for each timeout the call ended later than the
// grace after (see the package comment), or ended with another error than
// the interrupt. A function reads vmhook.Stop at least every piece units of
// each pass.
func Sweep(t *testing.T, vm *quickjs.VM, call string, n, piece int) {
	t.Helper()
	// The first call loads what the call needs once, such as the data of a
	// locale, which is not work on the input. The next are timed, and the
	// fastest is the time of the work.
	whole := time.Hour
	for i := range 3 {
		start := time.Now()
		if _, err := vm.Eval(call, quickjs.EvalGlobal); err != nil {
			t.Fatalf("%s: %v", call, err)
		}
		if i > 0 {
			whole = min(whole, time.Since(start))
		}
	}
	grace := Grace(whole, n, piece)
	var late []string
	for _, timeout := range timeouts(whole) {
		// A late stop is taken again, twice, before it counts: once the
		// scheduler or the GC of another test may have held it.
		over := time.Hour
		for try := 0; try < 3 && over > grace; try++ {
			if err := vm.SetEvalTimeout(timeout); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			_, err := vm.Eval(call, quickjs.EvalGlobal)
			took := time.Since(start)
			if err := vm.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			if err == nil && took <= timeout {
				// The call ended before the timeout: nothing was late.
				over = 0
				break
			}
			// A call that ends with no error after its timeout is late too:
			// the timeout came in a part with no check, and nothing read
			// it until the end of the call.
			var e *quickjs.Error
			if err != nil && (!errors.As(err, &e) || !e.Interrupted) {
				t.Fatalf("%s with a timeout of %v: %v, want the interrupt", call, timeout, err)
			}
			over = min(over, took-timeout)
		}
		if over > grace {
			late = append(late, fmt.Sprintf("%v late at a timeout of %v", round(over), round(timeout)))
		}
	}
	t.Logf("%s: %v whole, a grace of %v", call, round(whole), round(grace))
	if len(late) > 0 {
		t.Errorf("%s, of %v whole with a grace of %v, stopped\n\t%s", call, round(whole), round(grace), strings.Join(late, "\n\t"))
	}
	if v, err := vm.Eval(`1+1`, quickjs.EvalGlobal); err != nil || v != 2 {
		t.Fatalf("the VM after the sweep: 1+1 got %v, %v", v, err)
	}
}

func round(d time.Duration) time.Duration { return d.Round(100 * time.Microsecond) }
