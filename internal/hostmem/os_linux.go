// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"

	"golang.org/x/sys/unix"
)

// CanRemap tells whether Remap can resize a chunk: linux has mremap.
const CanRemap = true

// stackFlags: the shadow stack takes no swap space up front (MAP_NORESERVE).
// MAP_STACK keeps transparent huge pages off it only from linux 6.8 and the
// 6.7.6 stable update, so noHugePages does that on older kernels too.
const stackFlags = unix.MAP_NORESERVE | unix.MAP_STACK

// noHugePages keeps transparent huge pages off the shadow stack, which would
// make 2 MiB resident at its first touch. Many kernels in use are older than
// 6.8, and some set huge pages to always. It accepts EINVAL: on an anonymous
// mapping of our own, aligned to pages, the one reason left for it is a kernel
// built without huge pages, and then there is nothing to keep off.
func noHugePages(addr, size uintptr) error {
	err := sysMadvise("nohugepage", addr, size, unix.MADV_NOHUGEPAGE)
	if errors.Is(err, unix.EINVAL) {
		return nil
	}
	return err
}

// releaseAdvice drops the pages at once. They read as zero when next touched.
const releaseAdvice = unix.MADV_DONTNEED

func osRemap(addr, size, newSize uintptr) (uintptr, error) {
	if err := faulted("remap", addr, size); err != nil {
		return 0, err
	}
	p, err := unix.MremapPtr(pointer(addr), size, nil, newSize, unix.MREMAP_MAYMOVE)
	return uintptr(p), err
}
