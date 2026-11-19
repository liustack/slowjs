// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build linux || darwin

package slowjs

import (
	"runtime/debug"
	"syscall"
	"testing"
	"unsafe"
)

// asciiUnits, asciiUnitsLen and units8At read no unit past the end of the
// units: units that end where a mapped page ends, with a page after it that
// cannot be read, are read for every length up to 40 and every start, all
// ASCII so that the loops go as far as they can. A read of one unit too far
// faults, which SetPanicOnFault makes a panic the test reports.
func TestUTF16ReadsStayInSource(t *testing.T) {
	size := syscall.Getpagesize()
	mem, err := syscall.Mmap(-1, 0, 2*size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Munmap(mem) })
	if err := syscall.Mprotect(mem[size:], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	out := make([]byte, 3*40)
	for n := 0; n <= 40; n++ {
		u := unsafe.Slice((*uint16)(unsafe.Pointer(&mem[size-2*n])), n)
		for i := range u {
			u[i] = 'a'
		}
		for i := 0; i <= n; i++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%d units ending at the end of a page, from %d: %v", n, i, r)
					}
				}()
				if k := asciiUnits(unsafe.Pointer(unsafe.SliceData(out)), 0, u, i); k != (n-i)&^7 {
					t.Fatalf("%d units from %d: asciiUnits %d, want %d", n, i, k, (n-i)&^7)
				}
				if k := asciiUnitsLen(u[i:]); k != (n-i)&^7 {
					t.Fatalf("%d units from %d: asciiUnitsLen %d, want %d", n, i, k, (n-i)&^7)
				}
				if i+8 <= n {
					if w0, w1 := units8At(unsafe.Pointer(unsafe.SliceData(u)), i); w0 != 'a'*lanes || w1 != 'a'*lanes {
						t.Fatalf("%d units from %d: units8At %#x %#x", n, i, w0, w1)
					}
				}
			}()
		}
	}
}
