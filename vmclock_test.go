// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	goruntime "runtime"
	"testing"
	"time"
	"weak"
)

func TestVMClockTimeout(t *testing.T) {
	vm := newTestVM(t)
	at := time.Unix(-100000, 0)
	vm.SetClock(func() time.Time { return at })
	if got := vm.Now(); !got.Equal(at) {
		t.Fatalf("Now = %v", got)
	}
	if err := vm.SetEvalTimeout(20 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := vm.Eval(`for (;;) { Date.now(); }`, EvalGlobal)
	wantError(t, err, "InternalError", "interrupted")
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("fixed clock delayed timeout: %s", elapsed)
	}
}

// Runtime memory cannot root Go callbacks. The registry must hold a callback
// while the VM uses it, and release its captures on reset and Close.
func TestVMClockReleasesCallback(t *testing.T) {
	for _, how := range []string{"reset", "replace", "close"} {
		t.Run(how, func(t *testing.T) {
			vm, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			t.Cleanup(func() {
				if !closed {
					vm.Close()
				}
			})
			ref := func() weak.Pointer[[1024]byte] {
				p := new([1024]byte)
				vm.SetClock(func() time.Time { goruntime.KeepAlive(p); goruntime.KeepAlive(vm); return time.Unix(1, 0) })
				return weak.Make(p)
			}()
			goruntime.GC()
			if ref.Value() == nil {
				t.Fatal("clock capture was collected while registered")
			}
			switch how {
			case "reset":
				vm.SetClock(nil)
			case "replace":
				vm.SetClock(func() time.Time { return time.Unix(2, 0) })
			case "close":
				if err := vm.Close(); err != nil {
					t.Fatal(err)
				}
				closed = true
			}
			goruntime.GC()
			goruntime.GC()
			if ref.Value() != nil {
				t.Fatal("clock retained its capture after " + how)
			}
			goruntime.KeepAlive(vm)
		})
	}
}
