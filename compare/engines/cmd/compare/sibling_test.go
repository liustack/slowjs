// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"slices"
	"testing"
	"time"
)

func TestDisturbed(t *testing.T) {
	for _, c := range []struct {
		ticks, share float64
		want         bool
	}{
		{1, 0.10, false}, // one stray tick in a 0.1 s run
		{2, 0.50, false},
		{3, 0.06, true},
		{3, 0.05, false},
		{40, 0.04, false}, // a long run with a little noise
		{40, 0.20, true},
	} {
		if got := disturbed(c.ticks, c.share); got != c.want {
			t.Errorf("disturbed(%v, %v) = %v, want %v", c.ticks, c.share, got, c.want)
		}
	}
}

func TestSiblingBusy(t *testing.T) {
	ticks, share := siblingBusy([2]float64{100, 1000}, [2]float64{104, 1040})
	if ticks != 4 || share != 0.1 {
		t.Fatalf("got %v ticks, share %v", ticks, share)
	}
	if ticks, share := siblingBusy([2]float64{}, [2]float64{}); ticks != 0 || share != 0 {
		t.Fatalf("no readings: got %v, %v", ticks, share)
	}
}

func TestArgvNice(t *testing.T) {
	got := argv(1<<30, "0", 19, "bin/x", []string{"t262"})
	tail := []string{"taskset", "-c", "0", "nice", "-n", "19", "bin/x", "t262"}
	if len(got) < len(tail) || !slices.Equal(got[len(got)-len(tail):], tail) {
		t.Fatalf("argv = %q", got)
	}
}

func TestPreempted(t *testing.T) {
	ms := time.Millisecond
	for _, c := range []struct {
		cpu, wall time.Duration
		want      bool
	}{
		{0, time.Second, false}, // no CPU reading
		{980 * ms, time.Second, false},
		{850 * ms, time.Second, true},
		{60 * ms, 100 * ms, false}, // under 50 ms lost
		{18 * time.Second, 37 * time.Second, true},
	} {
		if got := preempted(c.cpu, c.wall); got != c.want {
			t.Errorf("preempted(%v, %v) = %v, want %v", c.cpu, c.wall, got, c.want)
		}
	}
}
