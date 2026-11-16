// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"slices"
	"time"
)

// A timed render of a program built with the tag pagesspin1 or pagesspin3 also
// waits on the clock, inside its time, for 1% or 3% of the median of the warm
// renders of its page: a slowdown of a known size with the code of the engine
// and its layout the same as the program built without the tag, to check
// what a way of timing can tell (.github/workflows/timeexp.yml, and the known
// slowdowns of docs/measuring.md). The program as it is waits for nothing:
// spinShare is 0 (spin_off.go).

// spinTarget returns share of the median of warm, as the P50 takes it (the
// upper one of an even count), or 0 when warm is empty.
func spinTarget(warm []time.Duration, share float64) time.Duration {
	if len(warm) == 0 || share == 0 {
		return 0
	}
	s := slices.Clone(warm)
	slices.Sort(s)
	return time.Duration(float64(s[len(s)/2]) * share)
}

// spinFor waits on the clock for at least d, busy, and returns how long it waited.
func spinFor(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	start := time.Now()
	for time.Since(start) < d {
	}
	return time.Since(start)
}
