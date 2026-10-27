// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && cenginehammer && amd64

package cengine

import (
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"unsafe"
)

// hammerG returns the runtime's g of the calling goroutine (hammer_g.s).
func hammerG() uintptr

// stackPreempt is the runtime's value of a stack guard that asks the next
// stack check to preempt the goroutine.
const stackPreempt = ^uintptr(1313)

// TestPreemptionHammer runs scripts in new VMs while another goroutine keeps
// asking for a preemption: it sets the stack guard of the goroutine in the
// engine to stackPreempt, so the next generated function's stack check goes
// through morestack into the scheduler, which may move the goroutine to
// another thread. It counts the requests it makes, which is more than the
// preemptions that happen. The results
// must not change and no block may be left. It writes into the runtime's g
// (its stack guard is the third word), which the runtime does not promise,
// hence the tag of its own:
//
//	go test -tags slowjs_cengine,cenginehammer -run Hammer ./internal/cengine
//
// CENGINE_HAMMER_N is the number of runs, 300 by default.
func TestPreemptionHammer(t *testing.T) {
	n := 300
	if s := os.Getenv("CENGINE_HAMMER_N"); s != "" {
		var err error
		if n, err = strconv.Atoi(s); err != nil {
			t.Fatal(err)
		}
	}
	scripts := []struct{ src, want string }{
		{`function fib(n){return n<2?n:fib(n-1)+fib(n-2)} fib(15)`, `610`},
		{`JSON.stringify([..."hammer"].map((c,i)=>({[c]:i*1.5})))`, `[{"h":0},{"a":1.5},{"m":3},{"m":4.5},{"e":6},{"r":7.5}]`},
		{`/(\d+)-(\d+)/.exec("x10-20y").slice(1).join("|") + String(10n**20n)`, `10|20100000000000000000000`},
	}
	var in atomic.Bool
	var requests atomic.Int64 // writes of stackPreempt, not preemptions that were made
	guard := (*uintptr)(unsafe.Pointer(hammerG() + 16))
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if in.Load() {
				atomic.StoreUintptr(guard, stackPreempt)
				requests.Add(1)
			}
			for i := 0; i < 1000; i++ {
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := range n {
		s := scripts[i%len(scripts)]
		v := newVM(t, Options{Seed: 1})
		in.Store(true)
		got, err := v.Eval(s.src, "hammer.js")
		in.Store(false)
		if err != nil || got != s.want {
			v.Host().MarkBroken()
			v.Close()
			t.Fatalf("run %d with preemption requests: %q, %v (want %q)", i, got, err, s.want)
		}
		closeVM(t, v)
	}
	// A request is a write of stackPreempt to the stack guard. The scheduler
	// answers it at the next stack check, and requests that arrive before it
	// has answered one merge, so this counts what was asked, not the
	// preemptions that happened.
	t.Logf("%d runs, %d preemption requests", n, requests.Load())
	if requests.Load() == 0 {
		t.Fatal("no preemption was requested")
	}
}
