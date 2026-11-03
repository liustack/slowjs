// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import "golang.org/x/sys/unix"

func init() {
	// Under Rosetta an amd64 process sees pages of 4 KiB, while the kernel
	// backs memory in its own pages of 16 KiB.
	n, err := unix.SysctlUint32("vm.pagesize")
	if err != nil {
		panic("hostmem: the sysctl vm.pagesize: " + err.Error())
	}
	if uintptr(n) > pageSize {
		touchPages = int(uintptr(n) / pageSize)
	}
}
