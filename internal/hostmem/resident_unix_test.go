// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux || darwin

package hostmem

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// resident counts the pages of [addr, addr+size) in memory, by mincore.
// x/sys has no wrapper for it, on linux or on darwin, so it is a system call.
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

// anyMapped reports whether a page of [addr, addr+size) is mapped. msync
// fails with ENOMEM on a page that is not, on linux and darwin alike
// (darwin's mincore does not fail there).
func anyMapped(t *testing.T, addr, size uintptr) bool {
	t.Helper()
	for p := addr; p < addr+size; p += pageSize {
		switch err := unix.Msync(bytesAt(p, pageSize), unix.MS_ASYNC); err {
		case nil:
			return true
		case unix.ENOMEM:
		default:
			t.Fatalf("msync at %#x: %v", p, err)
		}
	}
	return false
}
