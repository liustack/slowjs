// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package stopsweep

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// A try that runs past its timeout and grace has a stack in what it says,
// and its CPU time and GC pauses; one that ends before has no stack.
func TestTry(t *testing.T) {
	w := watch(time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	s := w.end()
	if !strings.Contains(s, "at its timeout and grace in\n(no goroutine in Sweep)") || !strings.Contains(s, "CPU ") || !strings.Contains(s, " GC with pauses of ") {
		t.Errorf("a late try: %q", s)
	}
	if runtime.GOOS != "windows" && !strings.Contains(s, "page faults") {
		t.Errorf("a late try has no count of page faults: %q", s)
	}
	if runtime.GOOS == "windows" && !strings.Contains(s, " CPU cycles") {
		t.Errorf("a late try on windows has no count of CPU cycles: %q", s)
	}
	w = watch(time.Hour)
	if s := w.end(); strings.Contains(s, "grace") {
		t.Errorf("a try in time: %q", s)
	}
}

// On windows the CPU times of the process move once a clock tick, about
// 15.6 ms, so a try of a few milliseconds reads 0 whether the process ran or
// not. The cycles it ran tell the two apart: work counts some, and a process
// the system did not run counts none.
func TestUsageCycles(t *testing.T) {
	u := usageNow()
	if runtime.GOOS != "windows" {
		if u.cycles != -1 {
			t.Errorf("cycles %d on %s, want -1 (not asked)", u.cycles, runtime.GOOS)
		}
		return
	}
	end := time.Now().Add(5 * time.Millisecond)
	for time.Now().Before(end) {
	}
	if v := usageNow(); u.cycles < 0 || v.cycles <= u.cycles {
		t.Errorf("cycles %d, then %d after 5 ms of work, want more", u.cycles, v.cycles)
	}
}
