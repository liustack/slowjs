// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"sync"
	"time"
	"unsafe"
)

// The engine's clocks are Go's. The gettimeofday and clock_gettime of
// modernc.org/libc are system calls, while time.Now reads the clock
// without entering the kernel where the system allows it (the vDSO on
// Linux), so Date.now cost several times as much as in other Go engines.
// internal/fixlib makes the functions of the engine that read a clock
// call these (see fixClock):
//
//   - date_now calls dateNow. fixVMClock redirects its two Date callers
//     to dateNowContext below, so they can read a VM's clock.
//   - js_random_init, which seeds Math.random of a new context, calls
//     randomSeed.
//   - get_time_ns of quickjs-libc, behind performance.now, calls
//     monoNanos.
//
// Atomics.wait still reads libc's CLOCK_REALTIME: it hands the deadline to
// libc's pthread_cond_timedwait, which measures it on that clock.

var (
	timeNow   = time.Now // tests replace it
	monoStart = time.Now()
)

// dateNow returns the time in milliseconds since 1970 UTC, rounded down,
// as date_now computes it from gettimeofday.
func dateNow() int64 {
	return timeNow().UnixMilli()
}

// randomSeed returns the state js_random_init gives the random number
// generator of a new context: the time in microseconds since 1970, or 1
// when that is 0, as the state must not be 0.
func randomSeed() uint64 {
	if s := uint64(timeNow().UnixMicro()); s != 0 {
		return s
	}
	return 1
}

// monoNanos returns the time in nanoseconds on Go's monotonic clock, which
// only goes forward whatever the wall clock does, since the package was
// loaded. get_time_ns read CLOCK_MONOTONIC, whose zero is the boot of the
// system, and gettimeofday on Windows and FreeBSD, where a change of the
// wall clock moved performance.now. On darwin, CLOCK_MONOTONIC goes on
// while the system sleeps, and Go's monotonic clock, mach_absolute_time,
// does not.
func monoNanos() int64 {
	return int64(time.Since(monoStart))
}

// clocks roots callbacks in Go memory. Runtime memory is outside Go's heap,
// so it carries only a flag, never a Go pointer. The live path does no lookup.
var clocks sync.Map // runtime uintptr -> func() time.Time

// SetClock sets a runtime's wall clock. Call only between calls into JavaScript.
// The owner must call SetClock(rt, nil) before freeing the runtime.
func SetClock(rt uintptr, now func() time.Time) {
	if now == nil {
		forkOf(rt).clock = false
		clocks.Delete(rt)
	} else {
		clocks.Store(rt, now)
		forkOf(rt).clock = true
	}
}

// Now reads a runtime's wall clock, with full time.Time precision.
func Now(rt uintptr) time.Time {
	if forkOf(rt).clock {
		return customNow(rt)
	}
	return timeNow()
}

func customNow(rt uintptr) time.Time {
	now, _ := clocks.Load(rt)
	return now.(func() time.Time)()
}

// dateNowContext shares the runtime clock with Go callers of Now. Read the
// live clock here so the default path adds no call through dateNow.
func dateNowContext(ctx uintptr) int64 {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	if forkOf(rt).clock {
		return customDateNow(rt)
	}
	return timeNow().UnixMilli()
}

func customDateNow(rt uintptr) int64 { return customNow(rt).UnixMilli() }
