// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"errors"

	"golang.org/x/sys/unix"
)

// CanRemap tells whether Remap can resize a chunk: darwin has no mremap.
const CanRemap = false

const stackFlags = 0

// releaseAdvice lets the system take the pages when it needs memory, with
// nothing to do before they are used again. Until then they stay counted in
// the memory of the process. MADV_FREE_REUSABLE would stop counting them at
// once, but needs MADV_FREE_REUSE before the engine touches them again, which
// the engine cannot tell the host.
const releaseAdvice = unix.MADV_FREE

func osRemap(addr, size, newSize uintptr) (uintptr, error) {
	return 0, errors.ErrUnsupported
}
