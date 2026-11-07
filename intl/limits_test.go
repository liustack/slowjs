// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"testing"
	"time"

	"github.com/liustack/quickjs"
)

// The work Intl does in Go for a script stays inside the limits of its VM:
// the memory limit and the eval timeout end the call, as they do in the
// engine, whatever the script asks of Intl.

// evalGuarded evaluates src on vm in a goroutine, and returns its result, its
// error, what it panicked with and how long it took. A call that has not
// returned by deadline fails the test.
func evalGuarded(t *testing.T, vm *quickjs.VM, src string, deadline time.Duration) (res any, err error, pan any, elapsed time.Duration) {
	t.Helper()
	type out struct {
		res any
		err error
		pan any
	}
	ch := make(chan out, 1)
	start := time.Now()
	go func() {
		var o out
		defer func() {
			o.pan = recover()
			ch <- o
		}()
		o.res, o.err = vm.Eval(src, quickjs.EvalGlobal)
	}()
	select {
	case o := <-ch:
		return o.res, o.err, o.pan, time.Since(start)
	case <-time.After(deadline):
		t.Fatalf("no answer after %v", deadline)
	}
	return
}

// wantUncatchable checks that err ended the call as running out of memory,
// or as an interrupt when interrupted is set.
func wantUncatchable(t *testing.T, err error, interrupted bool) {
	t.Helper()
	var e *quickjs.Error
	if !errors.As(err, &e) || !e.Uncatchable || e.Interrupted != interrupted || e.OutOfMemory == interrupted {
		t.Fatalf("got %v (%#v), want the uncatchable error of %s", err, e, map[bool]string{false: "running out of memory", true: "an interrupt"}[interrupted])
	}
}

// A string argument that the VM has no memory to read ends the call with the
// out of memory error, in every Builtin: none takes it for the empty string
// and goes on, and none panics. The string is a rope of 32 MiB, which reading
// has to join, in a VM limited to 16 MiB.
func TestStringArgumentOutOfMemory(t *testing.T) {
	for _, js := range []string{
		`new Intl.NumberFormat('en').format(s)`,
		`new Intl.NumberFormat('en').formatToParts(s)`,
		`new Intl.NumberFormat(s)`,
		`Intl.getCanonicalLocales(s)`,
		`new Intl.Collator('en').compare(s, 'a')`,
		`s.localeCompare('a')`,
		`new Intl.Segmenter('en').segment(s).containing(0)`,
		`s.toLocaleUpperCase('tr')`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `new Intl.NumberFormat('en').format(1); 'x'.toLocaleUpperCase('tr'); JSON`)
			vm.SetMemoryLimit(16 << 20)
			_, err, pan, _ := evalGuarded(t, vm, `
				var r = "ok";
				try {
					let s = "1".repeat(1024);
					for (let i = 0; i < 15; i++) s = s + s;
					`+js+`;
				} catch (e) { r = "caught " + e; }
				r`, time.Minute)
			if pan != nil {
				t.Fatalf("panic: %v", pan)
			}
			wantUncatchable(t, err, false)
		})
	}
}
