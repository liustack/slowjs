// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newRejectionsVM(t *testing.T, max int) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if max > 0 {
		if err := m.TrackRejections(max); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

// messages runs src and its jobs and returns the messages of the
// rejections left unhandled, and how many were dropped.
func messages(t *testing.T, m *VM, src string) ([]string, int) {
	t.Helper()
	if _, err := m.Eval(src, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	reasons, dropped, err := m.UnhandledRejections()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range reasons {
		got = append(got, e.Error())
	}
	return got, dropped
}

// TestRejectionsOff: a VM keeps no rejection unless it is asked to.
func TestRejectionsOff(t *testing.T) {
	m := newRejectionsVM(t, 0)
	got, dropped := messages(t, m, `Promise.reject(new Error("x"))`)
	if got != nil || dropped != 0 {
		t.Fatalf("got %q, %d dropped", got, dropped)
	}
}

// TestRejections: the promises rejected with no handler, oldest first, read
// as errors are: a handler attached later takes one off, and a promise that
// then or an async function made counts as any other.
func TestRejections(t *testing.T) {
	m := newRejectionsVM(t, 8)
	got, dropped := messages(t, m, `
function f() { return Promise.reject(new TypeError("one")) }
f();
var p2 = Promise.reject(new Error("two"));
Promise.reject(3).catch(() => {});
Promise.reject(42);
(async () => { throw new RangeError("five") })();
new Promise((_, r) => r("six")).then(() => {});
(async () => { try { await Promise.reject(7) } catch (e) {} })();
p2.catch(() => {});
`)
	want := []string{"TypeError: one", "42", "RangeError: five", "six"}
	if strings.Join(got, "|") != strings.Join(want, "|") || dropped != 0 {
		t.Fatalf("got %q, %d dropped, want %q", got, dropped, want)
	}
	if got, _ := messages(t, m, `0`); got != nil {
		t.Fatalf("kept again after they were taken: %q", got)
	}

	_, err := m.Eval(`function g() { return Promise.reject(new Error("stack")) } g()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 1 {
		t.Fatalf("%v, %v", reasons, err)
	}
	if e := reasons[0]; e.Name != "Error" || e.Message != "stack" || !strings.Contains(e.Stack, "at g (") || e.Uncatchable {
		t.Fatalf("%+v", e)
	}
}

// TestRejectionsMax: at most max are kept, the others counted, and turning
// tracking off forgets them.
func TestRejectionsMax(t *testing.T) {
	m := newRejectionsVM(t, 2)
	got, dropped := messages(t, m, `for (var i = 1; i <= 5; i++) Promise.reject(i)`)
	if strings.Join(got, ",") != "1,2" || dropped != 3 {
		t.Fatalf("got %q, %d dropped", got, dropped)
	}
	if _, err := m.Eval(`Promise.reject(1)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.TrackRejections(0); err != nil {
		t.Fatal(err)
	}
	got, dropped = messages(t, m, `Promise.reject(2)`)
	if got != nil || dropped != 0 {
		t.Fatalf("after off: %q, %d dropped", got, dropped)
	}
	for _, max := range []int{-1, MaxTrackedRejections + 1} {
		if err := m.TrackRejections(max); err == nil {
			t.Errorf("TrackRejections(%d): no error", max)
		}
	}
	if err := m.TrackRejections(MaxTrackedRejections); err != nil {
		t.Fatal(err)
	}
}

// TestRejectionsReadStops: reading a reason runs javascript (a getter of its
// message), which the eval timeout ends. The reasons left stay kept, and
// the VM goes on.
func TestRejectionsReadStops(t *testing.T) {
	m := newRejectionsVM(t, 4)
	if _, err := m.Eval(`
Promise.reject({ get message() { for (;;) {} } });
Promise.reject(new Error("after"));
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEvalTimeout(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	_, _, err := m.UnhandledRejections()
	var e *Error
	if !errors.As(err, &e) || !e.Interrupted {
		t.Fatalf("got %v, want the timeout", err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 1 || reasons[0].Message != "after" {
		t.Fatalf("%v, %v", reasons, err)
	}
}

// TestRejectionsClose: a VM closed with rejections kept closes, and the
// methods panic after Close as the others do. That the engine frees the kept
// entries is checked by test_rejections_max of engine/test/unit.c, which
// frees a runtime that still keeps some and fails on a leak.
func TestRejectionsClose(t *testing.T) {
	m := newRejectionsVM(t, 4)
	if _, err := m.Eval(`Promise.reject({ big: new Array(1e5).fill(1) })`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func(){
		func() { m.UnhandledRejections() },
		func() { m.TrackRejections(1) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("no panic after Close")
				}
			}()
			call()
		}()
	}
}

// TestRejectionsReadRange: a read takes the rejections kept when it starts,
// at most max. Those made while it reads, by a getter of a reason, wait
// for the next read, after their jobs could attach a handler: a getter
// that rejects its own reason again made one read take them all, 100 from
// max 1, and never end without a counter.
func TestRejectionsReadRange(t *testing.T) {
	m := newRejectionsVM(t, 1)
	if _, err := m.Eval(`
var remaining = 100;
var reason = { get message() { if (--remaining > 0) Promise.reject(reason); return "again" } };
Promise.reject(reason);
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 3; k++ {
		reasons, dropped, err := m.UnhandledRejections()
		if err != nil || len(reasons) != 1 || dropped != 0 {
			t.Fatalf("read %d: %d reasons, %d dropped, %v, want 1", k, len(reasons), dropped, err)
		}
	}

	m = newRejectionsVM(t, 1)
	if err := m.SetEvalTimeout(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`
var forever = { get message() { Promise.reject(forever); return "again" } };
Promise.reject(forever);
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 3; k++ {
		reasons, _, err := m.UnhandledRejections()
		if err != nil || len(reasons) != 1 {
			t.Fatalf("forever, read %d: %d reasons, %v, want 1", k, len(reasons), err)
		}
	}

	m = newRejectionsVM(t, 4)
	if _, err := m.Eval(`
Promise.reject({ get message() {
  const p = Promise.reject(new Error("late"));
  Promise.resolve().then(() => p.catch(() => {}));
  return "first";
} });
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 1 || reasons[0].Message != "first" {
		t.Fatalf("%d reasons, %v, want first alone", len(reasons), err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	if reasons, _, err := m.UnhandledRejections(); err != nil || len(reasons) != 0 {
		t.Fatalf("after the jobs: %d reasons, %v, want none", len(reasons), err)
	}
}
