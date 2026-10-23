// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

// firstUse touches Intl for the first time and reports the outcome as a value,
// so that a failed install shows up as a string instead of an Eval error.
const firstUse = `(() => {
	try {
		return (1234.5).toLocaleString('en-US');
	} catch (e) {
		return 'error: ' + e;
	}
})()`

// hostFuncsSeen counts the host functions a page can see on the global object.
const hostFuncsSeen = `Object.getOwnPropertyNames(globalThis).filter(n => n.startsWith('__slowjs_intl')).length`

// afterFailedInstall are the checks a VM whose install failed has to pass once
// the cause is gone: nothing left behind, and the next use installs.
var afterFailedInstall = [][2]string{
	{hostFuncsSeen, "0"},
	{`typeof Object.getOwnPropertyDescriptor(globalThis, 'Intl').get`, "function"},
	{`typeof Intl`, "object"},
	{`(1234.5).toLocaleString('en-US')`, "1,234.5"},
	{`new Date(0).toLocaleDateString('de')`, "1.1.1970"},
	{`new Intl.NumberFormat('fr').format(1234.5)`, "1 234,5"},
	{hostFuncsSeen, "0"},
}

// TestInstallFailsOnLowMemory runs out of memory during the first use of
// Intl. That use fails, and once memory is back the next use installs.
// Running out of memory is no exception stub.js sees, like an interrupt
// (TestInstallInterrupted): an install it cuts short is rolled back by the
// next use, so the checks start there.
func TestInstallFailsOnLowMemory(t *testing.T) {
	// firstUseWith reports what the first use answers on a fresh VM limited to limit
	// bytes, and how many installs it started.
	firstUseWith := func(limit uintptr) (*slowjs.VM, string, int64) {
		vm := newVM(t)
		// The engine creates some builtins, JSON among them, on first touch,
		// and one it fails to create for want of memory stays undefined for
		// good. The shim relies on JSON, so the page has touched it already,
		// as any page that has read its data has.
		eval(t, vm, `JSON`)
		vm.SetMemoryLimit(limit)
		before := shimInstalls.Load()
		v, err := vm.Eval(firstUse, slowjs.EvalGlobal)
		got := fmt.Sprint(v)
		if err != nil {
			got = "error: " + err.Error()
		}
		return vm, got, shimInstalls.Load() - before
	}

	// Find the smallest limit the first use works with.
	lo, hi := uintptr(128<<10), uintptr(16<<20)
	if _, got, _ := firstUseWith(hi); got != "1,234.5" {
		t.Fatalf("first use with %d bytes: %s", hi, got)
	}
	for hi-lo > 512 {
		mid := (lo + hi) / 2
		if _, got, _ := firstUseWith(mid); got == "1,234.5" {
			hi = mid
		} else {
			lo = mid
		}
	}

	// Below it the first use fails, somewhere in the install or before it.
	failedInstalls := 0
	for limit := hi - 512; limit > hi-64<<10; limit -= 1 << 10 {
		vm, got, installs := firstUseWith(limit)
		if got == "1,234.5" {
			continue
		}
		if !strings.HasPrefix(got, "error: ") {
			t.Fatalf("first use with %d bytes: %q", limit, got)
		}
		if installs != 0 {
			failedInstalls++
		}
		vm.SetMemoryLimit(1 << 30)
		t.Run(fmt.Sprint(limit), func(t *testing.T) { evalTests(t, vm, afterFailedInstall[2:]) })
	}
	if failedInstalls == 0 {
		t.Fatal("no first use failed inside the install")
	}
}

// deepFirstUse recurses until the stack runs out and makes the first use of
// Intl on the way back, at every depth until one works. It reports the result
// and whether any use failed first.
const deepFirstUse = `
	const failures = [];
	let result;
	function down() {
		try {
			down();
		} catch (e) {
		}
		if (result === undefined) {
			try {
				result = (1234.5).toLocaleString('en-US');
			} catch (e) {
				failures.push(e);
			}
		}
	}
	down();
	result + ' ' + (failures.length > 0)`

// TestInstallFailsOnDeepStack makes the first use of Intl at every depth
// below the stack limit, deepest first. The deep ones fail with a javascript
// exception, the first one with room to spare installs.
func TestInstallFailsOnDeepStack(t *testing.T) {
	vm := newVM(t)
	vm.SetMaxStackSize(400)
	before := shimInstalls.Load()
	evalTests(t, vm, [][2]string{{deepFirstUse, "1,234.5 true"}})
	evalTests(t, vm, afterFailedInstall[2:])
	if n := shimInstalls.Load() - before; n < 2 {
		t.Fatalf("the shim was installed %d times, want a failed install before the one that worked", n)
	}
}

// TestCompileOnDeepStack makes the first use of Intl in the process deep in
// the stack. The compile does not depend on the stack of the VM that needs it,
// and the process is left with a working shim.
func TestCompileOnDeepStack(t *testing.T) {
	saved := shimScript.code
	shimScript.code = nil
	t.Cleanup(func() { shimScript.code = saved })

	vm := newVM(t)
	vm.SetMaxStackSize(400)
	evalTests(t, vm, [][2]string{{deepFirstUse, "1,234.5 true"}})
	evalTests(t, newVM(t), [][2]string{{firstUse, "1,234.5"}})
}

// TestCompileRetries checks that a failed compile is not kept: the next eval
// compiles again. A good one is kept, and VMs starting together share it.
func TestCompileRetries(t *testing.T) {
	src := `syntax error(`
	s := &script{src: &src}
	vm := newVM(t)
	if err := s.eval(vm); err == nil {
		t.Fatal("a script with a syntax error evaluated")
	}

	src = `globalThis.evaluated = (globalThis.evaluated || 0) + 1`
	if err := s.eval(vm); err != nil {
		t.Fatalf("the compile was not retried: %v", err)
	}

	code := s.code
	if err := s.eval(vm); err != nil {
		t.Fatal(err)
	}
	if &s.code[0] != &code[0] {
		t.Fatal("a good compile was not kept")
	}
	evalTests(t, vm, [][2]string{{`evaluated`, "2"}})

	src2 := `globalThis.evaluated = 1`
	s2 := &script{src: &src2}
	var wg sync.WaitGroup
	codes := make([][]byte, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, err := s2.bytecode()
			if err != nil {
				t.Error(err)
			}
			codes[i] = code
		}()
	}
	wg.Wait()
	for _, code := range codes[1:] {
		if &code[0] != &codes[0][0] {
			t.Fatal("VMs starting together compiled more than once")
		}
	}
}

// TestInstallKeepsInterrupt makes the first use of Intl, which evaluates the
// shim from inside the page's call, between an Interrupt and the engine's
// next look at it. The request stays pending and stops the page.
func TestInstallKeepsInterrupt(t *testing.T) {
	vm := newVM(t)
	if err := vm.RegisterHostFunc("interrupt", func([]any) (any, error) {
		vm.Interrupt()
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	// A lost request leaves the loop below running: stop it after a while,
	// and fail.
	done := make(chan struct{})
	defer close(done)
	var lost atomic.Bool
	go func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			lost.Store(true)
			vm.Interrupt()
		}
	}()

	_, err := vm.Eval(`interrupt(); (1234.5).toLocaleString('en-US'); for (;;) {}`, slowjs.EvalGlobal)
	if err == nil || lost.Load() {
		t.Fatalf("got %v, lost the interrupt request: %v", err, lost.Load())
	}
	evalTests(t, vm, [][2]string{{`(1234.5).toLocaleString('en-US')`, "1,234.5"}})
}

// TestInstallInterrupted interrupts the first use of Intl at every point of a
// whole period of the engine's interrupt checks, a fresh VM for each. An
// interrupt is no exception a catch or finally sees, so one that lands in
// stub.js's own code cuts the install short with nothing rolled back, the
// host functions possibly in sight: the next use has to notice, clear up and
// start over.
func TestInstallInterrupted(t *testing.T) {
	const period = 10100 // a little over the engine's 10000 between checks
	step := 1
	if raceEnabled {
		// stub.js's code between the checks spans about 20 points, so a
		// coarser scan still lands in it, in well under a minute.
		step = 11
	}
	before := interruptedInstalls.Load()
	for k := 0; k <= period; k += step {
		vm, err := slowjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}

		if err := Register(vm, TimeZone("UTC")); err != nil {
			t.Fatal(err)
		}

		if err := vm.RegisterHostFunc("interrupt", func([]any) (any, error) {
			vm.Interrupt()
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Spend k checks' worth of the engine's countdown, so that the
		// interrupt below lands k instructions later.
		if _, err := vm.Eval(fmt.Sprintf(`for (let i = 0; i < %d; i++) {}`, k), slowjs.EvalGlobal); err != nil {
			t.Fatal(err)
		}

		vm.Eval(`interrupt(); (1234.5).toLocaleString('en-US')`, slowjs.EvalGlobal)
		v, err := vm.Eval(`[(1234.5).toLocaleString('en-US'), new Intl.NumberFormat('de').format(1234.5), `+hostFuncsSeen+`].join(' ')`, slowjs.EvalGlobal)
		vm.Close()
		if got := fmt.Sprint(v); err != nil || got != "1,234.5 1.234,5 0" {
			t.Fatalf("interrupted %d instructions later: got %q, %v", k, got, err)
		}
	}
	t.Logf("%d interrupts landed in stub.js", interruptedInstalls.Load()-before)
	if interruptedInstalls.Load() == before {
		t.Fatal("no interrupt landed in stub.js")
	}
}
