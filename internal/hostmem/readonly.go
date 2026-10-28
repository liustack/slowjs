// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"
	"fmt"
)

// A read-only mapping holds bytes that the engine of several VMs reads at
// once, the shared bytecode of the root package: it belongs to no Memory,
// and whoever maps it counts who reads it, and unmaps it after the last.
// Nothing writes to it once MapReadOnly returns, so VMs on other goroutines
// need no lock to read it, and a write of the engine to it faults.

// ReadOnlySize is how many bytes MapReadOnly maps for n bytes: whole pages,
// at least one.
func ReadOnlySize(n int) uintptr {
	return max(alignUp(uintptr(n), pageSize), pageSize)
}

// MapReadOnly maps a copy of b that can only be read, and returns its
// address. When the OS refuses to make the pages read-only, MapReadOnly
// gives them back and returns the error. When it refuses that too, it
// panics with both errors.
func MapReadOnly(b []byte) (uintptr, error) {
	size := ReadOnlySize(len(b))
	p, err := osMap(size)
	if err != nil {
		return 0, fmt.Errorf("hostmem: map %d bytes to read only: %w", size, err)
	}
	copy(bytesAt(p, uintptr(len(b))), b)
	if err := osProtectRead(p, size); err != nil {
		if cerr := osUnmap(p, size); cerr != nil {
			panic(fmt.Errorf("hostmem: a read-only mapping cannot give back the %d bytes at %#x it holds: %w", size, p, errors.Join(err, cerr)))
		}
		return 0, fmt.Errorf("hostmem: make %d bytes read-only: %w", size, err)
	}
	return p, nil
}

// UnmapReadOnly unmaps the mapping of MapReadOnly at addr, of n bytes.
func UnmapReadOnly(addr uintptr, n int) error {
	if err := osUnmap(addr, ReadOnlySize(n)); err != nil {
		return fmt.Errorf("hostmem: unmap %d read-only bytes at %#x: %w", n, addr, err)
	}
	return nil
}
