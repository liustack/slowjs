// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build unix

package stopsweep

import (
	"syscall"
	"time"
)

// usageNow is the CPU time of the process so far, and the pages the OS has
// handed it at their first touch (its minor page faults).
func usageNow() usage {
	var u syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &u); err != nil {
		return usage{faults: -1}
	}
	return usage{
		user:   time.Duration(u.Utime.Nano()),
		sys:    time.Duration(u.Stime.Nano()),
		faults: int64(u.Minflt),
	}
}
