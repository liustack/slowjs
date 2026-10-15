// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/liustack/quickjs"
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
const hostFuncsSeen = `Object.getOwnPropertyNames(globalThis).filter(n => n.startsWith('__quickjs_intl')).length`

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
// Intl. That use fails with a javascript exception, and once memory is back
// the next use installs.
func TestInstallFailsOnLowMemory(t *testing.T) {
	// firstUseWith reports what the first use answers on a fresh VM limited to limit
	// bytes, and how many installs it started.
	firstUseWith := func(limit uintptr) (*quickjs.VM, string, int64) {
		vm := newVM(t)
		// The engine creates some builtins, JSON among them, on first touch,
		// and one it fails to create for want of memory stays undefined for
		// good. The shim relies on JSON, so the page has touched it already,
		// as any page that has read its data has.
		eval(t, vm, `JSON`)
		vm.SetMemoryLimit(limit)
		before := shimInstalls.Load()
		v, err := vm.Eval(firstUse, quickjs.EvalGlobal)
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
		t.Run(fmt.Sprint(limit), func(t *testing.T) { evalTests(t, vm, afterFailedInstall) })
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
