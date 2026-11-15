// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"runtime"
	"testing"

	"github.com/liustack/slowjs"
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
		vm, err := slowjs.NewVM()
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
