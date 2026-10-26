// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux || darwin

package hostmem

import "golang.org/x/sys/unix"

// The *Ptr forms of x/sys keep no table of mappings, so VMs on other
// goroutines share no lock.

func osMap(size uintptr) (uintptr, error) {
	p, err := unix.MmapPtr(-1, 0, nil, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	return uintptr(p), err
}

func osUnmap(addr, size uintptr) error {
	return unix.MunmapPtr(pointer(addr), size)
}

// osReserveStack maps twice the size without access, which holds a region
// aligned to the size and the guard page above it, unmaps what lies around
// them, and opens the region to reading and writing. The guard page keeps no
// access.
func osReserveStack(size uintptr) (uintptr, error) {
	n := 2*size + pageSize
	p, err := unix.MmapPtr(-1, 0, nil, n, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON|stackFlags)
	if err != nil {
		return 0, err
	}
	start := uintptr(p)
	base := alignUp(start, size)
	end := base + size + pageSize
	if base > start {
		err = osUnmap(start, base-start)
	}
	if err == nil && start+n > end {
		err = osUnmap(end, start+n-end)
	}
	if err == nil {
		err = unix.Mprotect(bytesAt(base, size), unix.PROT_READ|unix.PROT_WRITE)
	}
	if err != nil {
		// Unmapping what is no longer mapped does nothing.
		osUnmap(start, n)
		return 0, err
	}
	return base, nil
}

func osFreeStack(base, size uintptr) error {
	return osUnmap(base, size+pageSize)
}
