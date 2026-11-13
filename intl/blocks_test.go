// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"runtime"
	"testing"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/hostmem"
)

// blockCalls are long calls that hold a copy of their text, in a block
// (hostmem.Take), and kept is how many blocks a call leaves held: the text
// of a Segments object, which its cursor holds until the VM drops it.
var blockCalls = []struct {
	name, setup, call string
	kept              int64
}{
	{"the first word of a long text",
		`var s = 'hello world '.repeat(N / 12), g = new Intl.Segmenter('en', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`, 1},
	{"compare of two long strings",
		`var a = 'ab'.repeat(N / 2) + 'c', b = 'ab'.repeat(N / 2) + 'd', c = new Intl.Collator('en')`,
		`c.compare(a, b)`, 0},
	{"compare of two long strings with lone surrogates",
		`var a = 'ab'.repeat(N / 4) + '\ud800' + 'ab'.repeat(N / 4) + 'c', b = 'ab'.repeat(N / 4) + '\ud800' + 'ab'.repeat(N / 4) + 'd', c = new Intl.Collator('en')`,
		`c.compare(a, b)`, 0},
	{"compare of two long runs of accents",
		`var a = 'a' + '\u0323\u0301\u0300'.repeat(N / 3), b = 'b' + '\u0323\u0301\u0300'.repeat(N / 3), c = new Intl.Collator('en')`,
		`c.compare(a, b)`, 0},
	{"a list of one long item",
		`var l = ['x'.repeat(N), 'y'], f = new Intl.ListFormat('en')`,
		`f.format(l).length`, 0},
	{"toLocaleUpperCase in Turkish",
		`var s = 'istanbul '.repeat(N / 9)`, `s.toLocaleUpperCase('tr').length`, 0},
	{"toLocaleLowerCase in Turkish of I and marks",
		`var s = 'I' + '̣'.repeat(N - 1)`, `s.toLocaleLowerCase('tr').length`, 0},
	{"toLocaleUpperCase in Greek",
		`var s = 'άέή'.repeat(N / 3)`, `s.toLocaleUpperCase('el').length`, 0},
}

// A long call holds its copy of the text in a block, not in Go's heap: Go
// allocates far less than the text in it, and the call gives back every
// block it made when it returns, but for the text of a cursor, which the VM
// gives back when it drops the cursor or is closed. A Go allocation the size
// of the text is cleared in one call, with no check, while the OS hands over
// its pages: 30 ms for 16 MiB on the virtual M1 of GitHub's macos-15 runner,
// past the timeout of the call.
func TestLongCallsUseBlocks(t *testing.T) {
	const n = 4 << 20
	for _, c := range blockCalls {
		t.Run(c.name, func(t *testing.T) {
			held := hostmem.Blocks()
			vm, err := quickjs.NewVM()
			if err != nil {
				t.Fatal(err)
			}
			if err := Register(vm, TimeZone("UTC")); err != nil {
				t.Fatal(err)
			}
			setUp(t, vm, c.setup, n)
			eval(t, vm, c.call)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			eval(t, vm, c.call)
			runtime.ReadMemStats(&after)
			if a := after.TotalAlloc - before.TotalAlloc; a > n/4 {
				t.Errorf("%s: Go allocated %d bytes for a text of %d units, want at most %d", c.call, a, n, n/4)
			}
			// Each call made a Segments object, with a cursor of its own,
			// and the VM may keep one block more for its next call.
			if got := hostmem.Blocks() - held; got != 2*c.kept && got != 2*c.kept+1 {
				t.Errorf("%s: %d blocks held after two calls, want %d, or one more the VM keeps", c.call, got, 2*c.kept)
			}
			vm.Close()
			if got := hostmem.Blocks() - held; got != 0 {
				t.Errorf("%s: %d blocks held after the VM was closed, want 0", c.call, got)
			}
		})
	}
}

// A call stopped at its timeout, which ends in a panic of the Go function
// it is in, gives back the blocks it made, at every place of the call.
func TestStoppedCallsGiveBlocks(t *testing.T) {
	const n = 4 << 20
	for _, c := range blockCalls {
		t.Run(c.name, func(t *testing.T) {
			first := hostmem.Blocks()
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			eval(t, vm, c.call)
			start := time.Now()
			eval(t, vm, c.call)
			whole := time.Since(start)
			for _, f := range []float64{0.1, 0.3, 0.5, 0.7, 0.9} {
				held := hostmem.Blocks()
				if err := vm.SetEvalTimeout(time.Duration(f * float64(whole))); err != nil {
					t.Fatal(err)
				}
				_, err := vm.Eval(c.call, quickjs.EvalGlobal)
				if err := vm.SetEvalTimeout(0); err != nil {
					t.Fatal(err)
				}
				// A stopped Segments call drops its cursor. One that ends
				// keeps it, and the VM drops its first when it has four.
				// The VM may keep one block more for its next call.
				want := int64(1)
				if err == nil {
					want += c.kept
				}
				if got := hostmem.Blocks() - held; got > want {
					t.Errorf("%s with a timeout at %.0f%% of its time (%v): %d more blocks held, want at most %d", c.call, 100*f, err, got, want)
				}
			}
			vm.Close()
			if got := hostmem.Blocks() - first; got != 0 {
				t.Errorf("%s: %d blocks held after the VM was closed, want 0", c.call, got)
			}
		})
	}
}
