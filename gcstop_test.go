// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	goruntime "runtime"
	"testing"
	"time"
)

// TestGCStopsLoopWithoutCalls runs GC while a script loops without calling
// anything. The Go interpreter is preempted by signal anywhere. The assembly
// interpreter (the default on linux/amd64) has no asynchronous safe points: GC can stop it
// only when it calls Go, and a loop like these calls Go only when the
// interrupt countdown of its backward jump reaches zero.
func TestGCStopsLoopWithoutCalls(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"goto", `let i = 0; for (;;) { i++ }`},
		{"if_true", `let i = 0; do { i++ } while (i > -1)`},
		{"if_false", `let i = 0; while (i > -1) { i++ }`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			done := make(chan error, 1)
			go func() {
				_, err := m.Eval(c.js, EvalGlobal)
				done <- err
			}()
			time.Sleep(20 * time.Millisecond)
			for range 5 {
				gc := make(chan time.Duration, 1)
				go func() {
					start := time.Now()
					goruntime.GC()
					gc <- time.Since(start)
				}()
				select {
				case d := <-gc:
					if d > time.Second {
						t.Errorf("GC took %v", d)
					}
				case <-time.After(10 * time.Second):
					m.Interrupt()
					<-done
					t.Fatal("GC did not stop the script within 10s")
				}
			}
			m.Interrupt()
			if err := <-done; err == nil {
				t.Fatal("the loop ended without the interrupt")
			}
		})
	}
}
