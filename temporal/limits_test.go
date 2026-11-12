// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

// A string argument longer than the cap of a string of Temporal is refused
// with a RangeError before it is read: the rope of 32 MiB is not joined, in
// a VM limited to 16 MiB. And one under the cap, in a VM with less room than
// the door counts for the call, ends the call with the out of memory error:
// the bridge does not parse it as the empty string.
func TestStringArgumentOutOfMemory(t *testing.T) {
	for _, js := range []string{
		`Temporal.PlainDate.from(s)`,
		`Temporal.Duration.from(s)`,
		`Temporal.PlainDate.from('2024-01-05').toLocaleString(s)`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `Temporal.PlainDate.from('2024-01-05').toLocaleString('en'); JSON`)
			vm.SetMemoryLimit(16 << 20)
			got, err := vm.Eval(`
				var r = "ok";
				try {
					let s = "1".repeat(1024);
					for (let i = 0; i < 15; i++) s = s + s;
					`+js+`;
				} catch (e) { r = "caught " + e; }
				r`, quickjs.EvalGlobal)
			if r, _ := got.(string); err != nil || !strings.HasPrefix(r, "caught RangeError") {
				t.Fatalf("got %v, %v, want a RangeError", got, err)
			}
			vm.SetMemoryLimit(vm.MemorySize() + 16<<10)
			_, err = vm.Eval(`var s = "1".repeat(100); try { `+js+` } catch (e) { "caught " + e }`, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
				t.Fatalf("got %v, want the uncatchable out of memory error", err)
			}
		})
	}
}

// What a VM asks Intl to format a Temporal value with stays in the caches of
// the process, which every VM shares, only within a small bound: a failed
// request with an option of 1 MiB, and a format of a list of 10,000 locales,
// each in a VM closed right after, leave a few MiB at most.
func TestFormatCachesAfterClose(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"bad calendar", `try { Temporal.PlainDate.from('2024-01-05').toLocaleString('en', {calendar: 'X'.repeat(1 << 20) + %d}) } catch (e) {}`},
		{"long list", `new Intl.DateTimeFormat(Array.from({length: 10000}, (_, i) => 'en-x-' + i.toString(36) + %d)).format(Temporal.PlainDate.from('2024-01-05'))`},
		{"long list toLocaleString", `Temporal.PlainDate.from('2024-01-05').toLocaleString(Array.from({length: 10000}, (_, i) => 'en-x-' + i.toString(36) + %d))`},
	} {
		t.Run(c.name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			const n = 40
			for i := range n {
				vm, err := quickjs.NewVM()
				if err != nil {
					t.Fatal(err)
				}
				if err := intl.Register(vm, intl.TimeZone("UTC")); err != nil {
					t.Fatal(err)
				}
				if err := Register(vm); err != nil {
					t.Fatal(err)
				}
				_, err = vm.Eval(fmt.Sprintf(c.js, i), quickjs.EvalGlobal)
				vm.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			runtime.GC()
			runtime.GC()
			runtime.ReadMemStats(&after)
			kept := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			t.Logf("%d VMs closed, the heap kept %d KiB", n, kept>>10)
			if kept > 8<<20 {
				t.Fatalf("the caches keep %d MiB after the VMs closed", kept>>20)
			}
		})
	}
}
