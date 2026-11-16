// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package tifa

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

// heapAfterGC returns the bytes of the Go heap in use after a collection.
func heapAfterGC() uint64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestRegisterManyVMs makes, uses and closes 20,000 VMs set up by Register,
// as tifa makes the VMs of its pool again over days. Nothing Register gives
// a VM is made once for each VM in Go and kept: a Builtin a VM made would
// stay in the table of the process, which holds 32768, and what a closed VM
// kept would grow the Go heap.
func TestRegisterManyVMs(t *testing.T) {
	const n = 20000
	use := `var u = new URL("/p?q#h", "http://localhost"); u.pathname = "/x" + u.pathname;
		u.href + " " + __tifa_murmur2__("color:red;") + " " + new Intl.NumberFormat("en-US").format(1234.5)`
	var want any // what the first VM gives, which each other gives too
	var before uint64
	for i := range n {
		if i == n/10 {
			before = heapAfterGC()
		}
		vm, err := slowjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if err := vm.SetEvalTimeout(time.Minute); err != nil {
			vm.Close()
			t.Fatal(err)
		}
		if err := Register(vm); err != nil {
			vm.Close()
			t.Fatal(err)
		}
		got, err := vm.Eval(use, slowjs.EvalGlobal)
		vm.Close()
		if i == 0 {
			want = got
			if s, ok := got.(string); !ok || !strings.HasPrefix(s, "http://localhost/x/p?q#h ") || !strings.HasSuffix(s, " 1,234.5") {
				t.Fatalf("%v, %v", got, err)
			}
		}
		if err != nil || got != want {
			t.Fatalf("VM %d: %v, %v, want %s", i, got, err, want)
		}
	}
	if after := heapAfterGC(); after > before+1<<20 {
		t.Errorf("the Go heap grew from %d to %d bytes over %d VMs", before, after, n-n/10)
	}
}
