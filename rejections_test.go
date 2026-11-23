// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs // import "github.com/liustack/tifajs"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	// The next read runs with no deadline, which a busy machine could pass.
	if err := m.SetEvalTimeout(0); err != nil {
		t.Fatal(err)
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

// textOK reports whether s, a text of a reason, is at most rejectionText
// bytes of UTF-8 cut where no character is cut, followed by cutMark when
// the whole text was longer, and starts as whole would.
func textOK(s, whole string) bool {
	body, cut := strings.CutSuffix(s, cutMark)
	if !cut {
		return s == whole && len(whole) <= rejectionText
	}
	return len(whole) > rejectionText && len(body) <= rejectionText &&
		len(body) > rejectionText-utf8.UTFMax && strings.HasPrefix(whole, body)
}

// TestRejectionsText: each text of a reason Go reads, its string, name,
// message, stack and file name, is cut to its first rejectionText bytes of
// UTF-8 or a few fewer, where no character is cut, and marked as cut.
func TestRejectionsText(t *testing.T) {
	m := newRejectionsVM(t, 8)
	if _, err := m.Eval(`
var a = "x".repeat(1 << 20), w = "中".repeat(1 << 20), p = "😀".repeat(1 << 18);
Promise.reject(a);
Promise.reject(w);
Promise.reject(p);
var e = new Error(w); e.name = p; e.stack = a; e.fileName = w;
Promise.reject(e);
Promise.reject({ toString() { return p } });
Promise.reject("é".repeat(5000) + "short");
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 6 {
		t.Fatalf("%d reasons, %v", len(reasons), err)
	}
	a, w, p := strings.Repeat("x", 1<<20), strings.Repeat("中", 1<<20), strings.Repeat("😀", 1<<18)
	wants := []struct{ raw, name, message, stack, file string }{
		{a, "", "", "", ""},
		{w, "", "", "", ""},
		{p, "", "", "", ""},
		{p + ": " + w, p, w, a, w},
		{p, "", "", "", ""},
		{strings.Repeat("é", 5000) + "short", "", "", "", ""},
	}
	for i, e := range reasons {
		want := wants[i]
		for _, f := range []struct{ what, got, want string }{
			{"text", e.Error(), want.raw}, {"Name", e.Name, want.name}, {"Message", e.Message, want.message},
			{"Stack", e.Stack, want.stack}, {"FileName", e.FileName, want.file},
		} {
			if !utf8.ValidString(f.got) || strings.ContainsRune(f.got, utf8.RuneError) || !textOK(f.got, f.want) {
				t.Errorf("reason %d, %s: %d bytes, want the first %d bytes or a few fewer of %d, marked", i, f.what, len(f.got), rejectionText, len(f.want))
			}
		}
	}
}

// allocated returns the bytes Go allocated while f ran.
func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestRejectionsGoMemory: what one read makes Go allocate is bounded by
// reasonsRoom and the number of reasons times rejectionRoom, however long
// the reasons are and however many references they share:
// MaxTrackedRejections references to one string of 1 MiB, in a VM limited to
// 64 MiB, read 1 GiB into Go before the texts were cut.
func TestRejectionsGoMemory(t *testing.T) {
	m := newRejectionsVM(t, MaxTrackedRejections)
	m.SetMemoryLimit(64 << 20)
	if _, err := m.Eval(fmt.Sprintf(`var s = "x".repeat(1 << 20); for (var i = 0; i < %d; i++) Promise.reject(s)`, MaxTrackedRejections), EvalGlobal); err != nil {
		t.Fatal(err)
	}
	var reasons []*Error
	var err error
	n := allocated(func() { reasons, _, err = m.UnhandledRejections() })
	if err != nil || len(reasons) != MaxTrackedRejections {
		t.Fatalf("%d reasons, %v", len(reasons), err)
	}
	if bound := uint64(reasonsRoom + MaxTrackedRejections*rejectionRoom); n > bound {
		t.Fatalf("Go allocated %d bytes, more than %d", n, bound)
	}
}

// TestRejectionsRoom: a read takes the room for each reason under the
// memory limit of the VM before it reads it. When it has none left, it ends
// with the out of memory error, and the reasons it did not read stay kept
// for the next read.
func TestRejectionsRoom(t *testing.T) {
	m := newRejectionsVM(t, 16)
	// One string for all, so that taking a reason frees little of the VM.
	if _, err := m.Eval(`var y = "y".repeat(1 << 16); for (var i = 0; i < 16; i++) Promise.reject(y)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(m.MemorySize() + uintptr(3*rejectionRoom+rejectionRoom/2))
	first, _, err := m.UnhandledRejections()
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory || len(first) == 0 || len(first) >= 16 {
		t.Fatalf("%d reasons, %v, want some and the out of memory error", len(first), err)
	}
	m.SetMemoryLimit(^uintptr(0))
	rest, _, err := m.UnhandledRejections()
	if err != nil || len(first)+len(rest) != 16 {
		t.Fatalf("%d then %d reasons, %v, want 16 in all", len(first), len(rest), err)
	}
	for i, e := range append(first, rest...) {
		if !strings.HasSuffix(e.Error(), cutMark) {
			t.Errorf("reason %d: not cut", i)
		}
	}
}

// TestRejectionsReadSweep: a read of many reasons, each of which runs
// javascript for about a millisecond, under eval timeouts from 1 to 32 ms,
// ends by the timeout plus the grace of a kill with the interrupt or with
// all of them, never killed. The reasons it did not take stay kept: only
// the one it was reading when the timeout came is lost.
func TestRejectionsReadSweep(t *testing.T) {
	const n = 64
	for _, d := range []time.Duration{0, time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond, 8 * time.Millisecond, 16 * time.Millisecond, 32 * time.Millisecond} {
		m := newRejectionsVM(t, n)
		if _, err := m.Eval(fmt.Sprintf(`for (let i = 0; i < %d; i++) Promise.reject({
  get message() { const end = Date.now() + 1; while (Date.now() < end) {} return "m" + i }
})`, n), EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if err := m.SetEvalTimeout(d); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		first, _, err := m.UnhandledRejections()
		took := time.Since(start)
		if d > 0 && took > d+50*time.Millisecond {
			t.Errorf("timeout %v: the read took %v", d, took)
		}
		var e *Error
		lost := 0
		if err != nil {
			if !errors.As(err, &e) || !e.Interrupted || errors.Is(err, ErrKilled) {
				t.Fatalf("timeout %v: %v, want the interrupt", d, err)
			}
			lost = 1
		}
		if err := m.SetEvalTimeout(0); err != nil {
			t.Fatal(err)
		}
		rest, _, err := m.UnhandledRejections()
		if err != nil {
			t.Fatal(err)
		}
		if got := len(first) + len(rest); got != n && got != n-lost {
			t.Errorf("timeout %v: %d then %d reasons, want %d, or %d when one was being read", d, len(first), len(rest), n, n-1)
		}
		if got := evalString(t, m, `String(1 + 1)`); got != "2" {
			t.Errorf("timeout %v: after the read %q", d, got)
		}
	}
}

// TestRejectionsLongReasonsTimeout is the case of the review: 64 references
// to a string of 4 Mi characters, read under an eval timeout of 1 ms, read
// 256 MiB into Go before the texts were cut, with no read of the stop. Now
// a read makes Go allocate at most reasonsRoom and 64 times rejectionRoom,
// and ends with all of them or the interrupt.
func TestRejectionsLongReasonsTimeout(t *testing.T) {
	m := newRejectionsVM(t, 64)
	if _, err := m.Eval(`var s = "x".repeat(4 << 20); for (var i = 0; i < 64; i++) Promise.reject(s)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEvalTimeout(time.Millisecond); err != nil {
		t.Fatal(err)
	}
	var err error
	n := allocated(func() { _, _, err = m.UnhandledRejections() })
	var e *Error
	if err != nil && (!errors.As(err, &e) || !e.Interrupted) {
		t.Fatalf("%v, want none or the interrupt", err)
	}
	if bound := uint64(reasonsRoom + 64*rejectionRoom); n > bound {
		t.Fatalf("Go allocated %d bytes, more than %d", n, bound)
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

// TestRejectionsStopBetween: the stop is read before each reason, also when
// reading the reasons runs no javascript that would read it: called from a
// Go function after Interrupt, a read takes none, and all stay kept.
func TestRejectionsStopBetween(t *testing.T) {
	m := newRejectionsVM(t, 8)
	var reasons []*Error
	var readErr error
	defineBuiltin(t, m, "readAll", NewBuiltin("readAll", 0, func(a Args) (Value, error) {
		a.VM().Interrupt()
		reasons, _, readErr = a.VM().UnhandledRejections()
		return UndefinedValue, nil
	}))
	if _, err := m.Eval(`for (var i = 0; i < 8; i++) Promise.reject("r" + i)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	_, err := m.Eval(`readAll()`, EvalGlobal)
	var e *Error
	if len(reasons) != 0 || !errors.As(readErr, &e) || !e.Interrupted {
		t.Fatalf("%d reasons, %v, want none and the interrupt", len(reasons), readErr)
	}
	if !errors.As(err, &e) || !e.Interrupted {
		t.Fatalf("the call: %v, want the interrupt", err)
	}
	rest, _, err := m.UnhandledRejections()
	if err != nil || len(rest) != 8 {
		t.Fatalf("after: %d reasons, %v, want 8", len(rest), err)
	}
}

// TestRejectionsReadAlone: a read in progress is the only one. A getter of a
// reason that reaches UnhandledRejections or TrackRejections of the same VM
// through a Go function gets an error, and nothing changes: the limit stays,
// and what is kept stays kept for the next read.
func TestRejectionsReadAlone(t *testing.T) {
	m := newRejectionsVM(t, 4)
	var nestedRead, track0, track1 error
	var nestedReasons []*Error
	defineBuiltin(t, m, "nested", NewBuiltin("nested", 0, func(a Args) (Value, error) {
		nestedReasons, _, nestedRead = a.VM().UnhandledRejections()
		track0 = a.VM().TrackRejections(0)
		track1 = a.VM().TrackRejections(1)
		return UndefinedValue, nil
	}))
	if _, err := m.Eval(`
Promise.reject({ get message() { nested(); return "outer" } });
Promise.reject("second");
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 2 || reasons[0].Message != "outer" || reasons[1].Error() != "second" {
		t.Fatalf("%d reasons, %v, want outer and second", len(reasons), err)
	}
	if nestedRead == nil || nestedReasons != nil || track0 == nil || track1 == nil {
		t.Fatalf("inside a read: UnhandledRejections %d reasons, %v, TrackRejections(0) %v, (1) %v, want errors",
			len(nestedReasons), nestedRead, track0, track1)
	}
	// The limit is still 4: none of the calls inside took effect.
	got, dropped := messages(t, m, `for (var i = 1; i <= 4; i++) Promise.reject(i)`)
	if strings.Join(got, ",") != "1,2,3,4" || dropped != 0 {
		t.Fatalf("after: %q, %d dropped, want 1 to 4 kept", got, dropped)
	}
}

// TestRejectionsWideMemory: what a read makes Go allocate stays under what it
// takes from the room, also for texts of 16-bit characters that are mostly
// ASCII, with no memory kept from earlier conversions (two collections empty
// sync.Pool). The conversion into a buffer of 3 bytes a unit, then a copy,
// took 106,526 bytes for the five texts of such a reason, more than the room
// taken for it.
func TestRejectionsWideMemory(t *testing.T) {
	const n = 64
	m := newRejectionsVM(t, n)
	if _, err := m.Eval(fmt.Sprintf(`var s = "a".repeat(8192) + "中";
for (var i = 0; i < %d; i++) Promise.reject({ toString() { return s }, name: s, message: s, stack: s, fileName: s })`, n), EvalGlobal); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.GC()
	var reasons []*Error
	var err error
	got := allocated(func() { reasons, _, err = m.UnhandledRejections() })
	if err != nil || len(reasons) != n {
		t.Fatalf("%d reasons, %v", len(reasons), err)
	}
	if bound := uint64(reasonsRoom + n*rejectionRoom); got > bound {
		t.Fatalf("Go allocated %d bytes, more than the %d taken from the room", got, bound)
	}
	for i, e := range reasons {
		if !textOK(e.Stack, strings.Repeat("a", 8192)+"中") {
			t.Fatalf("reason %d: Stack of %d bytes", i, len(e.Stack))
		}
	}
}

// TestRejectionsGetterRoom: each text takes its room when it is read, after
// the getters that ran before it. A getter of the stack that allocates the
// room away makes the read end with the out of memory error before it reads
// the file name, where the room was taken once, before the reason.
func TestRejectionsGetterRoom(t *testing.T) {
	m := newRejectionsVM(t, 4)
	limit := m.MemorySize() + 4<<20
	m.SetMemoryLimit(limit)
	defineBuiltin(t, m, "room", NewBuiltin("room", 0, func(a Args) (Value, error) {
		return a.VM().NewFloat64(float64(limit - a.VM().MemorySize())), nil
	}))
	if _, err := m.Eval(`
var s = "f".repeat(8192), hold = [];
Promise.reject({ name: "n", message: "m", get stack() { while (room() > 6144) hold.push(new Uint8Array(room() - 4096)); return "s" }, fileName: s });
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory || len(reasons) != 0 {
		t.Fatalf("%d reasons, %v, want none and the out of memory error", len(reasons), err)
	}
}

// TestRejectionsErrorCut: the error a read ends with is cut as the texts of
// the reasons are. A getter that runs out of memory in a function with a
// name of 128 KiB made the Stack of that error 128 KiB, past what a read
// returns otherwise.
func TestRejectionsErrorCut(t *testing.T) {
	m := newRejectionsVM(t, 4)
	m.SetMemoryLimit(8 << 20)
	if _, err := m.Eval(`
function big() { return new ArrayBuffer(1 << 30) }
Object.defineProperty(big, "name", { value: "n".repeat(128 << 10) });
Promise.reject({ get message() { big(); return "m" } });
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	_, _, err := m.UnhandledRejections()
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory {
		t.Fatalf("%v, want the out of memory error", err)
	}
	if !strings.Contains(e.Stack, "nnnn") {
		t.Fatalf("Stack %q, want the frame of big", e.Stack[:min(len(e.Stack), 100)])
	}
	for _, f := range []struct{ what, s string }{{"text", e.Error()}, {"Message", e.Message}, {"Stack", e.Stack}} {
		if len(f.s) > rejectionText+len(cutMark) {
			t.Errorf("%s: %d bytes, more than %d", f.what, len(f.s), rejectionText+len(cutMark))
		}
	}
}

// TestRejectionsOddReasons: a reason that cannot be made a string, or whose
// reads throw, is read with the texts it gives, "" for the others, and the
// read goes on to the next reason: a Symbol, a revoked Proxy, a Proxy whose
// trap throws, and a toString that throws a primitive.
func TestRejectionsOddReasons(t *testing.T) {
	m := newRejectionsVM(t, 8)
	if _, err := m.Eval(`
Promise.reject(Symbol("s"));
var r = Proxy.revocable({}, {}); r.revoke(); Promise.reject(r.proxy);
Promise.reject(new Proxy({}, { get() { throw new Error("trap") } }));
Promise.reject({ toString() { throw 42 }, message: "m" });
Promise.reject("last");
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 5 {
		t.Fatalf("%d reasons, %v, want 5", len(reasons), err)
	}
	for i, e := range reasons[:3] {
		if e.raw != "" || e.Name != "" || e.Message != "" || e.Stack != "" || e.Uncatchable {
			t.Errorf("reason %d: %+v, want no text", i, e)
		}
	}
	if e := reasons[3]; e.raw != "" || e.Message != "m" {
		t.Errorf("toString that throws: %+v, want the message alone", e)
	}
	if e := reasons[4]; e.Error() != "last" {
		t.Errorf("after them: %q", e.Error())
	}
	if got := evalString(t, m, `String(1 + 1)`); got != "2" {
		t.Errorf("after the read %q", got)
	}
}

// TestRejectionsLoneSurrogateCut: a lone surrogate is U+FFFD, three bytes,
// and the cut keeps it when it fits and leaves it out when it does not.
func TestRejectionsLoneSurrogateCut(t *testing.T) {
	m := newRejectionsVM(t, 8)
	if _, err := m.Eval(`
Promise.reject("a".repeat(8189) + "\uD800" + "bcd");
Promise.reject("a".repeat(8190) + "\uDC00" + "bcd");
Promise.reject("a".repeat(8189) + "\uDC00");
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reasons, _, err := m.UnhandledRejections()
	if err != nil || len(reasons) != 3 {
		t.Fatalf("%d reasons, %v", len(reasons), err)
	}
	a := func(n int) string { return strings.Repeat("a", n) }
	for i, want := range []string{a(8189) + "�" + cutMark, a(8190) + cutMark, a(8189) + "�"} {
		if got := reasons[i].Error(); got != want {
			t.Errorf("reason %d: %d bytes ending %q, want %d ending %q", i, len(got), got[max(0, len(got)-12):], len(want), want[len(want)-12:])
		}
	}
}

// TestRejectionsInterruptGetter: Interrupt from another goroutine while a
// getter of a reason runs ends the read with the interrupt, and the reasons
// after it stay kept.
func TestRejectionsInterruptGetter(t *testing.T) {
	m := newRejectionsVM(t, 4)
	ready := make(chan struct{})
	defineBuiltin(t, m, "ready", NewBuiltin("ready", 0, func(a Args) (Value, error) {
		close(ready)
		return UndefinedValue, nil
	}))
	if _, err := m.Eval(`
Promise.reject({ get message() { ready(); for (;;) {} } });
Promise.reject("after");
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	go func() {
		<-ready
		m.Interrupt()
	}()
	reasons, _, err := m.UnhandledRejections()
	var e *Error
	if !errors.As(err, &e) || !e.Interrupted || len(reasons) != 0 {
		t.Fatalf("%d reasons, %v, want none and the interrupt", len(reasons), err)
	}
	reasons, _, err = m.UnhandledRejections()
	if err != nil || len(reasons) != 1 || reasons[0].Error() != "after" {
		t.Fatalf("after: %d reasons, %v, want after", len(reasons), err)
	}
}
