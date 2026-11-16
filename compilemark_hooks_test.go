// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"testing"
)

// TestCompileMarkCleared: the word after the interrupt flag, which the
// engine sets from the stop of a compile to its return so that the freeing
// of its code is left out of the kill (JS_GetInterruptFlag in
// engine/quickjs.h), is 0 again after each way a compile of 50 KB ends: a
// stop at every poll of the compile, at the top level and in a direct eval
// that the code of another eval runs, and a stop in the middle with 0 to
// 4 MiB of memory left, so that some compiles end out of memory and some at
// the stop. No callback of the compile runs javascript, so no compile starts
// inside another. The VM then still kills a script that frees for longer
// than KillPolls polls after its stop.
func TestCompileMarkCleared(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`var a = 0, s = '(function(){' + 'a=a+1;'.repeat(1 << 13) + '})'; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	h := m.h
	wantClear := func(what string, err error) {
		t.Helper()
		var e *Error
		if errors.Is(err, ErrKilled) || err != nil && !errors.As(err, &e) {
			t.Fatalf("%s: got %v", what, err)
		}
		if n := h.FreeingCode(); n != 0 {
			t.Fatalf("%s: the compile ended with %v and left the word after the flag at %d", what, err, n)
		}
	}
	for _, c := range []struct{ name, call string }{
		{"an eval", `(0, eval)(s); 0`},
		{"a direct eval in an eval", `(0, eval)('eval(s)'); 0`},
	} {
		before := h.Polls()
		if _, err := m.Eval(c.call, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		span := h.Polls() - before
		stopped := 0
		for at := int64(1); at <= span; at++ {
			h.InterruptAtPoll(h.Polls() + at)
			_, err := m.Eval(c.call, EvalGlobal)
			h.InterruptAtPoll(0)
			if err != nil {
				stopped++
			}
			wantClear(fmt.Sprintf("%s stopped at poll %d of %d", c.name, at, span), err)
		}
		t.Logf("%s: %d of %d stops ended the call", c.name, stopped, span)
	}
	before := h.Polls()
	if _, err := m.Eval(`(0, eval)(s); 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	mid := (h.Polls() - before) / 2
	ends := map[string]int{}
	for room := uintptr(0); room <= 4<<20; room += 16 << 10 {
		h.InterruptAtPoll(h.Polls() + mid)
		m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
		_, err := m.Eval(`(0, eval)(s); 0`, EvalGlobal)
		m.SetMemoryLimit(^uintptr(0))
		h.InterruptAtPoll(0)
		var e *Error
		switch {
		case err == nil:
			ends["ran"]++
		case errors.As(err, &e) && e.Interrupted:
			ends["interrupted"]++
		default:
			ends[err.Error()]++
		}
		wantClear(fmt.Sprintf("a stop at poll %d with %d bytes left", mid, room), err)
	}
	t.Logf("a stop at poll %d with 0 to 4 MiB left: %v", mid, ends)
	if ends["interrupted"] == 0 || len(ends) < 2 {
		t.Fatalf("the sweep of memory left gave %v: want both stops and other ends", ends)
	}
	wantKillWhileFreeing(t, m, "after the compiles")
}
