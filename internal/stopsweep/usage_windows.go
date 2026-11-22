// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package stopsweep

import (
	"syscall"
	"time"
	"unsafe"
)

var queryProcessCycleTime = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryProcessCycleTime")

// usageNow is the CPU time of the process so far and the CPU cycles its
// threads ran. The times move once a clock tick, about 15.6 ms, so a try of
// a few milliseconds may read none of them: the cycles show whether the
// process ran at all. Windows does not count page faults here.
func usageNow() usage {
	u := usage{faults: -1, cycles: -1}
	p, err := syscall.GetCurrentProcess()
	if err != nil {
		return u
	}
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(p, &created, &exited, &kernel, &user) == nil {
		u.user = filetime(user)
		u.sys = filetime(kernel)
	}
	var c uint64
	if r, _, _ := queryProcessCycleTime.Call(uintptr(p), uintptr(unsafe.Pointer(&c))); r != 0 {
		u.cycles = int64(c)
	}
	return u
}

// filetime is a FILETIME that counts a duration, in units of 100 ns.
func filetime(f syscall.Filetime) time.Duration {
	return time.Duration(uint64(f.HighDateTime)<<32|uint64(f.LowDateTime)) * 100
}
