// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux || darwin

package hostmem

import "golang.org/x/sys/unix"

// The calls to the OS, each of which asks faulted first: a test can fail them,
// and in the normal build faulted does nothing. The *Ptr forms of x/sys keep
// no table of mappings, so VMs on other goroutines share no lock.

func sysMmap(call string, size uintptr, prot, flags int) (uintptr, error) {
	if err := faulted(call, 0, size); err != nil {
		return 0, err
	}
	p, err := unix.MmapPtr(-1, 0, nil, size, prot, unix.MAP_PRIVATE|unix.MAP_ANON|flags)
	return uintptr(p), err
}

func sysMunmap(addr, size uintptr) error {
	if err := faulted("unmap", addr, size); err != nil {
		return err
	}
	return unix.MunmapPtr(pointer(addr), size)
}

func sysMprotect(addr, size uintptr, prot int) error {
	if err := faulted("protect", addr, size); err != nil {
		return err
	}
	return unix.Mprotect(bytesAt(addr, size), prot)
}

func sysMadvise(call string, addr, size uintptr, advice int) error {
	if err := faulted(call, addr, size); err != nil {
		return err
	}
	return unix.Madvise(bytesAt(addr, size), advice)
}

func osMap(size uintptr) (uintptr, error) {
	return sysMmap("map", size, unix.PROT_READ|unix.PROT_WRITE, 0)
}

func osUnmap(addr, size uintptr) error {
	return sysMunmap(addr, size)
}

func osRelease(addr, size uintptr) error {
	return sysMadvise("release", addr, size, releaseAdvice)
}

// osReserveStack maps twice the size without access, which holds a region
// aligned to the size and the guard page above it, unmaps what lies around
// them, opens the region to reading and writing, and keeps huge pages off it.
// The guard page keeps no access.
func osReserveStack(size uintptr) (uintptr, error) {
	n := 2*size + pageSize
	start, err := sysMmap("reserve", n, unix.PROT_NONE, stackFlags)
	if err != nil {
		return 0, err
	}
	// [lo, hi) is what the setup still holds. Once a part is unmapped, another
	// thread may map there at once, so a failure later unmaps only [lo, hi).
	lo, hi := start, start+n
	undo := func(err error) (uintptr, error) {
		if cerr := osUnmap(lo, hi-lo); cerr != nil {
			lost(lo, hi-lo, err, cerr)
		}
		return 0, err
	}
	base := alignUp(start, size)
	end := base + size + pageSize
	if base > lo {
		if err := osUnmap(lo, base-lo); err != nil {
			return undo(err)
		}
		lo = base
	}
	if hi > end {
		if err := osUnmap(end, hi-end); err != nil {
			return undo(err)
		}
		hi = end
	}
	if err := sysMprotect(base, size, unix.PROT_READ|unix.PROT_WRITE); err != nil {
		return undo(err)
	}
	if err := noHugePages(base, size); err != nil {
		return undo(err)
	}
	return base, nil
}

func osFreeStack(base, size uintptr) error {
	return osUnmap(base, size+pageSize)
}

func osProtectRead(addr, size uintptr) error {
	return sysMprotect(addr, size, unix.PROT_READ)
}
