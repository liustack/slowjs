// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
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
	w = watch(time.Hour)
	if s := w.end(); strings.Contains(s, "grace") {
		t.Errorf("a try in time: %q", s)
	}
}
