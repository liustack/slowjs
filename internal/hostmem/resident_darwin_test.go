// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func init() {
	// Under Rosetta an amd64 process sees pages of 4 KiB, while the kernel
	// backs memory in its own pages of 16 KiB.
	n, err := unix.SysctlUint32("vm.pagesize")
	if err != nil {
		panic("hostmem: the sysctl vm.pagesize: " + err.Error())
	}
	if uintptr(n) > pageSize {
		touchPages = int(uintptr(n) / pageSize)
	}
}

// written is resident, for a block (resident_linux_test.go).
func written(t *testing.T, addr, size uintptr) int {
	t.Helper()
	return resident(t, addr, size)
}

// resident counts the pages of [addr, addr+size) in memory, by mincore.
// x/sys has no wrapper for it on darwin, so it is a system call.
func resident(t *testing.T, addr, size uintptr) int {
	t.Helper()
	vec := make([]byte, (size+pageSize-1)/pageSize)
	_, _, e := unix.Syscall(unix.SYS_MINCORE, addr, size, uintptr(unsafe.Pointer(&vec[0])))
	if e != 0 {
		t.Fatalf("mincore of %d bytes at %#x: %v", size, addr, e)
	}
	n := 0
	for _, v := range vec {
		n += int(v & 1)
	}
	return n
}
