// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"

	"golang.org/x/sys/windows"
)

// CanRemap tells whether Remap can resize a chunk: windows cannot.
const CanRemap = false

// osMap reserves and commits in one step. The engine never needs a chunk to
// grow in place, so nothing is gained by reserving more than it commits.
func osMap(size uintptr) (uintptr, error) {
	return windows.VirtualAlloc(0, size, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
}

func osUnmap(addr, size uintptr) error {
	return windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
}

func osRemap(addr, size, newSize uintptr) (uintptr, error) {
	return 0, errors.ErrUnsupported
}

// osRelease says the contents of the pages no longer matter. The system takes
// them when it needs memory, and they stay committed. Decommitting would free
// them at once, but the engine would have to tell the host before it touched
// them again.
func osRelease(addr, size uintptr) error {
	_, err := windows.VirtualAlloc(addr, size, windows.MEM_RESET, windows.PAGE_READWRITE)
	return err
}

// osReserveStack reserves the stack and its guard page at an address aligned
// to the size, and commits the stack. The guard page stays reserved and not
// committed, so touching it faults. VirtualFree gives back only a whole
// reservation, so an aligned address is found by reserving twice the size,
// giving it back and reserving again at the aligned address. Another thread
// may take the address in between, and then it tries again.
func osReserveStack(size uintptr) (uintptr, error) {
	for range 16 {
		p, err := windows.VirtualAlloc(0, 2*size+pageSize, windows.MEM_RESERVE, windows.PAGE_NOACCESS)
		if err != nil {
			return 0, err
		}
		if err := windows.VirtualFree(p, 0, windows.MEM_RELEASE); err != nil {
			return 0, err
		}
		base := alignUp(p, size)
		_, err = windows.VirtualAlloc(base, size+pageSize, windows.MEM_RESERVE, windows.PAGE_NOACCESS)
		if err == windows.ERROR_INVALID_ADDRESS {
			continue
		}
		if err != nil {
			return 0, err
		}
		if _, err := windows.VirtualAlloc(base, size, windows.MEM_COMMIT, windows.PAGE_READWRITE); err != nil {
			windows.VirtualFree(base, 0, windows.MEM_RELEASE)
			return 0, err
		}
		return base, nil
	}
	return 0, errors.New("other threads took the address of the stack 16 times")
}

func osFreeStack(base, size uintptr) error {
	return windows.VirtualFree(base, 0, windows.MEM_RELEASE)
}
