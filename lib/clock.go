// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

import "time"

// The engine's clocks are Go's. The gettimeofday and clock_gettime of
// modernc.org/libc are system calls, while time.Now reads the clock
// without entering the kernel where the system allows it (the vDSO on
// Linux), so Date.now cost several times as much as in other Go engines.
// internal/fixlib makes the functions of the engine that read a clock
// call these (see fixClock):
//
//   - date_now, behind Date.now, new Date() and Date(), calls dateNow.
//   - js_random_init, which seeds Math.random of a new context, calls
//     randomSeed.
//   - get_time_ns and get_time_ms of quickjs-libc, behind performance.now,
//     os.now and the timers of the os module, call monoNanos.
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
// loaded. get_time_ns and get_time_ms read CLOCK_MONOTONIC, whose zero is
// the boot of the system, and gettimeofday on Windows and FreeBSD, where a
// change of the wall clock moved the timers of the os module. On darwin,
// CLOCK_MONOTONIC goes on while the system sleeps, and Go's monotonic
// clock, mach_absolute_time, does not.
func monoNanos() int64 {
	return int64(time.Since(monoStart))
}
