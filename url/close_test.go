// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package url

import (
	"runtime"
	"testing"

	"github.com/liustack/tifajs"
)

// manyVMs is how many VMs TestRegisterManyVMs makes: more than a third of
// the 32768 Builtins a process may have, so that a Register that made three
// of its own would run out of them.
const manyVMs = 20000

// heapAfterGC returns the bytes of the Go heap in use after a collection.
func heapAfterGC() uint64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestRegisterManyVMs makes, uses and closes 20,000 VMs with Register, as a
// server that makes the VMs of its pool again for days does. The Builtins of
// the package are made once, and what a VM keeps goes when it closes: no
// Builtin is made, nothing of a closed VM stays in the package, and the Go
// heap does not grow with the VMs.
func TestRegisterManyVMs(t *testing.T) {
	use := `var u = new URL("/p?q#h", "http://localhost"); u.pathname = "/x" + u.pathname; u.href`
	var before uint64
	for i := range manyVMs {
		if i == manyVMs/10 {
			before = heapAfterGC()
		}
		vm, err := tifajs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if err := Register(vm); err != nil {
			vm.Close()
			t.Fatal(err)
		}
		if got := evalString(t, vm, use); got != "http://localhost/x/p?q#h" {
			vm.Close()
			t.Fatalf("VM %d: %s", i, got)
		}
		vm.Close()
	}
	if n := keptStates(); n != 0 {
		t.Errorf("%d VMs closed, and the package keeps the state of %d", manyVMs, n)
	}
	// What a VM kept is under 4 KiB, so 18,000 VMs that each left theirs
	// behind would grow the heap by tens of MB.
	if after := heapAfterGC(); after > before+1<<20 {
		t.Errorf("the Go heap grew from %d to %d bytes over %d VMs", before, after, manyVMs-manyVMs/10)
	}
}

// keptStates is how many VMs the package keeps a state of.
func keptStates() int {
	n := 0
	states.Range(func(any, any) bool { n++; return true })
	return n
}

// TestRegisterFailsAgain calls Register 10,000 times on a VM where it fails
// after url.js has started: a page made URL a property that cannot be
// redefined. A failed Register leaves one state and one close function for
// the VM, however often it is tried, and Close removes the state.
func TestRegisterFailsAgain(t *testing.T) {
	vm, err := tifajs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	evalString(t, vm, `Object.defineProperty(globalThis, "URL", {value: 0}); ""`)
	if err := Register(vm); err == nil {
		t.Fatal("Register took a VM whose URL cannot be redefined")
	}
	before := heapAfterGC()
	for range 10000 {
		if err := Register(vm); err == nil {
			t.Fatal("Register took a VM whose URL cannot be redefined")
		}
	}
	// a close function and its place in the list of the VM take 24 bytes
	// or more, so 10,000 of them would take 240 KB
	if after := heapAfterGC(); after > before+64<<10 {
		t.Errorf("10,000 failed Registers grew the Go heap from %d to %d bytes", before, after)
	}
	if n := keptStates(); n != 1 {
		t.Errorf("the package keeps %d states for one VM", n)
	}
	vm.Close()
	if n := keptStates(); n != 0 {
		t.Errorf("after Close the package keeps %d states", n)
	}
}
