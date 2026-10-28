// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// A built-in that works on a long input in one call must read the interrupt
// flag as it goes. If it does not, the call runs to its end, or to the grace of
// the kill (50 ms), which ends the VM instead of the call: the Interrupt did
// not stop the script, the kill did.

import (
	"errors"
	"testing"
	"time"
)

func TestInterruptStopsLongBuiltins(t *testing.T) {
	// Each script runs one call of the built-in that takes longer than the grace
	// (60 to 400 ms), over and over. What the call made is small enough to free
	// in a few milliseconds: the free of a result with millions of items is not
	// the call, has no flag to read and can pass the grace by itself.
	for _, c := range []struct{ name, js string }{
		// The first replace of a VM takes the generic path, which calls exec and so reads the flag. The
		// rest, after one has run, take the one that matches in a loop of its own.
		{"replace", `'a'.replace(/a/g, 'b'); const s = 'ab'.repeat(1 << 21); for (;;) { s.replace(/a/g, 'cc') }`},
		{"fill", `for (;;) { new Array(1 << 21).fill(0) }`},
		{"join", `const a = new Array(1 << 23).fill('x'); for (;;) { a.join(',') }`},
		{"normalize", `const s = 'éå'.repeat(1 << 20); for (;;) { s.normalize('NFC') }`},
	} {
		t.Run(c.name, func(t *testing.T) {
			for trial := range 3 {
				m, err := NewVM()
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() {
					_, err := m.Eval(c.js, EvalGlobal)
					done <- err
				}()
				// Long enough for the setup, and a different place of the call each time.
				time.Sleep(time.Second + time.Duration(trial)*70*time.Millisecond)
				begin := time.Now()
				m.Interrupt()
				err = <-done
				took := time.Since(begin)
				var e *Error
				switch {
				case errors.Is(err, ErrKilled):
					t.Errorf("trial %d: the script did not stop and was killed after %v", trial, took)
				case !errors.As(err, &e) || !e.Interrupted:
					t.Errorf("trial %d: %v", trial, err)
				case took > stopLimit*4/5:
					t.Errorf("trial %d: stopped %v after Interrupt, want under %v", trial, took, stopLimit*4/5)
				}
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			}
		})
	}
}
