// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestInterruptAfterClose calls Interrupt on a closed VM, as a watchdog does
// when the script it guards finished just before its deadline. Interrupt used
// to dereference the interrupt flag Close had freed and cleared, and panicked.
func TestInterruptAfterClose(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m.Interrupt()
}

// TestInterruptDuringClose calls Interrupt from other goroutines while the VM
// closes, and again after Close returned. Interrupt read the address of the
// flag that Close frees and then clears, so it could write to freed memory or
// dereference nil, and under -race the race detector reports the read. Every
// call must be safe and do nothing once Close has started.
func TestInterruptDuringClose(t *testing.T) {
	for range 50 {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		var closed atomic.Bool
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				for {
					after := closed.Load()
					m.Interrupt()
					if after {
						return
					}
				}
			})
		}
		time.Sleep(time.Millisecond)
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}

		closed.Store(true)
		wg.Wait()
	}
}

// TestInterruptStopsEval pins the case of ExampleVM_Interrupt that the race
// detector reported: Interrupt from another goroutine stops the script, and
// the VM is closed after the call returns. The flag the script polls is the
// only link between the two goroutines, and it lives in C memory, where the
// race detector does not see atomic operations. Close must not write what
// Interrupt reads, or the detector reports a race it cannot see is ordered.
func TestInterruptStopsEval(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		m.Interrupt()
	}()

	_, err = m.Eval(`for (;;) {}`, EvalGlobal)
	if e, ok := err.(*Error); !ok || !e.Interrupted {
		t.Fatalf("got %v, want an interrupt", err)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
