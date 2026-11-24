// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import (
	"math"
	"testing"
	"time"
)

// A timeout further away than the clock reaches is no timeout at all. The
// deadline used to wrap around to the past, so the call was interrupted at
// once.
func TestHugeEvalTimeout(t *testing.T) {
	for _, d := range []time.Duration{math.MaxInt64, math.MaxInt64 - time.Second, 290 * 365 * 24 * time.Hour} {
		m := newTestVM(t)
		m.SetEvalTimeout(d)
		if _, err := m.Eval(`let s = 0; for (let i = 0; i < 1e6; i++) s += i; s`, EvalGlobal); err != nil {
			t.Fatalf("timeout %v: %v", d, err)
		}
	}
}

// BenchmarkClock times one read of each clock javascript has: Date.now and
// new Date(). ns/op is per read.
func BenchmarkClock(b *testing.B) {
	for _, c := range []struct{ name, call string }{
		{"DateNow", `Date.now()`},
		{"NewDate", `new Date()`},
	} {
		b.Run(c.name, func(b *testing.B) {
			m := newCallVM(b, "", c.call)
			b.ReportAllocs()
			b.ResetTimer()
			if _, err := m.Call("loop", b.N); err != nil {
				b.Fatal(err)
			}
		})
	}
}
