// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"os"
	"syscall"
)

// DropPages asks the kernel to drop the cached pages of a file
// (POSIX_FADV_DONTNEED), so that the next process that maps it gets new
// physical pages. Two copies of one binary keep their own physical pages
// for as long as they stay cached, and those can make one copy steadily
// faster than the other.
func DropPages(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	const dontneed = 4
	if _, _, e := syscall.Syscall6(syscall.SYS_FADVISE64, f.Fd(), 0, 0, dontneed, 0, 0); e != 0 {
		return e
	}
	return nil
}
