// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package pages

import (
	"testing"
	"time"
)

// The wait of a timed render is the share of the median of the warm renders.
func TestSpinTarget(t *testing.T) {
	ms := time.Millisecond
	for _, c := range []struct {
		warm  []time.Duration
		share float64
		want  time.Duration
	}{
		{[]time.Duration{30 * ms, 10 * ms, 20 * ms}, 0.01, 200 * time.Microsecond},
		{[]time.Duration{40 * ms, 10 * ms, 20 * ms, 30 * ms}, 0.03, 900 * time.Microsecond}, // the upper median, as the P50
		{nil, 0.01, 0},
		{[]time.Duration{10 * ms}, 0, 0},
	} {
		if got := spinTarget(c.warm, c.share); got != c.want {
			t.Errorf("spinTarget(%v, %v) = %v, want %v", c.warm, c.share, got, c.want)
		}
	}
}

// spinFor waits at least as long as it is asked, and says how long it waited.
func TestSpinFor(t *testing.T) {
	if got := spinFor(0); got != 0 {
		t.Errorf("spinFor(0) = %v", got)
	}
	start := time.Now()
	got := spinFor(2 * time.Millisecond)
	if got < 2*time.Millisecond || got > time.Since(start) {
		t.Errorf("spinFor(2ms) = %v, the call took %v", got, time.Since(start))
	}
}
