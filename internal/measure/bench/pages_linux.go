// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// DropPages writes the file out (fsync, since the kernel keeps dirty pages),
// asks the kernel to drop its cached pages (POSIX_FADV_DONTNEED), and checks
// with mincore that none is left in memory. It fails when one is: the advice
// is only advice. The next process that maps the file reads it again into
// pages the kernel picks, which may or may not be the same ones.
func DropPages(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync %s: %w", path, err)
	}
	const dontneed = 4
	if _, _, e := syscall.Syscall6(syscall.SYS_FADVISE64, f.Fd(), 0, 0, dontneed, 0, 0); e != 0 {
		return fmt.Errorf("fadvise %s: %w", path, e)
	}
	n, total, err := Resident(path)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%s: %d of its %d pages are still in memory after DONTNEED (locked, or in use)", path, n, total)
	}
	return nil
}

// Resident returns how many pages of the file are in the page cache, of how
// many, by mapping it and asking mincore.
func Resident(path string) (n, total int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	size := int(st.Size())
	if size == 0 {
		return 0, 0, nil
	}
	m, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return 0, 0, fmt.Errorf("mmap %s: %w", path, err)
	}
	defer syscall.Munmap(m)
	page := os.Getpagesize()
	total = (size + page - 1) / page
	vec := make([]byte, total)
	if _, _, e := syscall.Syscall(syscall.SYS_MINCORE, uintptr(unsafe.Pointer(&m[0])), uintptr(size), uintptr(unsafe.Pointer(&vec[0]))); e != 0 {
		return 0, 0, fmt.Errorf("mincore %s: %w", path, e)
	}
	for _, b := range vec {
		if b&1 != 0 {
			n++
		}
	}
	return n, total, nil
}
