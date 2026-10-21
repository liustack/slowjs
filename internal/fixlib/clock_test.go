// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On every target, the functions that read a clock call lib/clock.go, and
// only Atomics.wait reads a clock of libc, outside the functions kept for
// fixlib to check.
func TestClockEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, f := range clockFuncs {
			body, err := funcBody(b, funcHead(f.name), f.name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(clockReads(body)) != 0 || !bytes.Contains(body, []byte(clockNote(f.name))) {
				t.Errorf("%s: %s does not read Go's clock:\n%s", name, f.name, body)
			}
		}
		calls := len(clockReads(b))
		wait, err := funcBody(b, funcHead(clockWait), clockWait)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(clockReads(wait)); n != 1 || calls != len(clockFuncs)+1 {
			t.Errorf("%s: %d reads of a clock of libc, %d in %s, want %d and 1", name, calls, n, clockWait, len(clockFuncs)+1)
		}
	}
}

// fixClock leaves a fixed file as it is, and puts the change back into a
// file without it, on every target. restoreClock gives back the text ccgo
// writes.
func TestClockIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte(clockKept+"(")) {
			t.Fatalf("%s: readEngine leaves the change in", name)
		}
		fixed, err := fixClock(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixClock changed nothing", name)
		}
		again, err := fixClock(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreClock(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreClock does not give the text back", name)
		}
	}
}

// A change to a function that reads a clock, a new read of a clock of
// libc, or a part of the change stops fixClock.
func TestClockRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixClock([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, old, new, err string
		in                  string
	}{
		{"date_now in seconds", "*int64(1000) + int64((*(*Ttimeval)(unsafe.Pointer(bp))).Ftv_usec/libc.Int32FromInt32(1000))", "*int64(1) + int64(0)", "changed (fingerprint", b},
		{"get_time_ms on the wall clock", "libc.Xclock_gettime(tls, int32(E_CLOCK_MONOTONIC), bp)\n\treturn libc.Int64FromUint64(libc.Uint64FromInt64((*(*Ttimespec)(unsafe.Pointer(bp))).Ftv_sec)*uint64(1000) ", "libc.Xclock_gettime(tls, int32(E_CLOCK_REALTIME), bp)\n\treturn libc.Int64FromUint64(libc.Uint64FromInt64((*(*Ttimespec)(unsafe.Pointer(bp))).Ftv_sec)*uint64(1000) ", "changed (fingerprint", b},
		{"another read of the time", "\nfunc _js_date_constructor(", "\nfunc _js_new_clock(tls *libc.TLS) {\n\tlibc.Xtime(tls, 0)\n}\n\nfunc _js_date_constructor(", "the engine reads the time some new way", b},
		{"the address of a clock", "\nfunc _js_date_constructor(", "\nfunc _js_new_clock(tls *libc.TLS) {\n\t_ = __ccgo_fp(libc.Xgettimeofday)\n}\n\nfunc _js_date_constructor(", "the engine reads the time some new way", b},
		{"a clock of Windows", "\nfunc _js_date_constructor(", "\nfunc _js_new_clock(tls *libc.TLS) {\n\tlibc.XQueryPerformanceCounter(tls, 0)\n}\n\nfunc _js_date_constructor(", "the engine reads the time some new way", b},
		{"mingw's time", "\nfunc _js_date_constructor(", "\nfunc _js_new_clock(tls *libc.TLS) {\n\t__time64(tls, 0)\n}\n\nfunc _js_date_constructor(", "the engine reads the time some new way", b},
		{"a second clock in Atomics.wait", "\t\tlibc.Xclock_gettime(tls, int32(E_CLOCK_REALTIME), bp+24)\n", "\t\tlibc.Xgettimeofday(tls, bp+24, 0)\n\t\tlibc.Xclock_gettime(tls, int32(E_CLOCK_REALTIME), bp+24)\n", "_js_atomics_wait: 2 reads of a clock of libc", b},
		{"another timed wait", "\nfunc _js_date_constructor(", "\nfunc _js_new_wait(tls *libc.TLS) {\n\tlibc.Xpthread_cond_timedwait(tls, 0, 0, 0)\n}\n\nfunc _js_date_constructor(", "uses of pthread_cond_timedwait", b},
		{"a function gone upstream", "\nfunc _get_time_ms(tls *libc.TLS)", "\nfunc _get_time_ms_gone(tls *libc.TLS)", "_get_time_ms is gone: upstream removed it, so it must leave clockFuncs", b},
		{"a part of the change", "\treturn monoNanos()\n", "\treturn monoNanos() + 1\n", "does not follow the function fixClock adds", string(fixed)},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixClock([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
