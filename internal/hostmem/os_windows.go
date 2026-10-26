// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// CanRemap tells whether Remap can resize a chunk: windows cannot.
const CanRemap = false

// The calls to the OS, each of which asks fault first.

func sysAlloc(call string, addr, size uintptr, kind, prot uint32) (uintptr, error) {
	if err := faulted(call, addr, size); err != nil {
		return 0, err
	}
	return windows.VirtualAlloc(addr, size, kind, prot)
}

func sysFree(addr uintptr) error {
	if err := faulted("unmap", addr, 0); err != nil {
		return err
	}
	return windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
}

// osMap reserves and commits in one step. The engine never needs a chunk to
// grow in place, so nothing is gained by reserving more than it commits.
func osMap(size uintptr) (uintptr, error) {
	return sysAlloc("map", 0, size, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
}

func osUnmap(addr, size uintptr) error {
	return sysFree(addr)
}

func osRemap(addr, size, newSize uintptr) (uintptr, error) {
	return 0, errors.ErrUnsupported
}

// osRelease says the contents of the pages no longer matter. The system takes
// them when it needs memory, and they stay committed. Decommitting would free
// them at once, but the engine would have to tell the host before it touched
// them again.
func osRelease(addr, size uintptr) error {
	_, err := sysAlloc("release", addr, size, windows.MEM_RESET, windows.PAGE_READWRITE)
	return err
}

// osReserveStack reserves the stack and its guard page at an address aligned
// to the size, and commits the stack. The guard page stays reserved and not
// committed, so touching it faults. VirtualFree gives back only a whole
// reservation, so an aligned address is found by reserving twice the size,
// giving it back and reserving again at the aligned address. Another thread
// may take the address in between, and then it tries again, 16 times at most.
func osReserveStack(size uintptr) (uintptr, error) {
	n := 2*size + pageSize
	var err error // why the last try failed
	for range 16 {
		var p uintptr
		if p, err = sysAlloc("reserve", 0, n, windows.MEM_RESERVE, windows.PAGE_NOACCESS); err != nil {
			return 0, err
		}
		if cerr := sysFree(p); cerr != nil {
			lost(p, n, nil, cerr)
		}
		base := alignUp(p, size)
		_, err = sysAlloc("reserve", base, size+pageSize, windows.MEM_RESERVE, windows.PAGE_NOACCESS)
		if errors.Is(err, windows.ERROR_INVALID_ADDRESS) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if _, err := sysAlloc("commit", base, size, windows.MEM_COMMIT, windows.PAGE_READWRITE); err != nil {
			if cerr := sysFree(base); cerr != nil {
				lost(base, size+pageSize, err, cerr)
			}
			return 0, err
		}
		return base, nil
	}
	return 0, fmt.Errorf("other threads took the aligned address 16 times: %w", err)
}

func osFreeStack(base, size uintptr) error {
	return sysFree(base)
}
