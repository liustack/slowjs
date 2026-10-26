// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import "golang.org/x/sys/unix"

// CanRemap tells whether Remap can resize a chunk: linux has mremap.
const CanRemap = true

// stackFlags: the shadow stack takes no swap space up front (MAP_NORESERVE),
// and no transparent huge pages (MAP_STACK, from linux 6.7), which would make
// 2 MiB resident at the first touch.
const stackFlags = unix.MAP_NORESERVE | unix.MAP_STACK

// releaseAdvice drops the pages at once. They read as zero when next touched.
const releaseAdvice = unix.MADV_DONTNEED

func osRemap(addr, size, newSize uintptr) (uintptr, error) {
	if err := faulted("remap", addr, size); err != nil {
		return 0, err
	}
	p, err := unix.MremapPtr(pointer(addr), size, nil, newSize, unix.MREMAP_MAYMOVE)
	return uintptr(p), err
}
