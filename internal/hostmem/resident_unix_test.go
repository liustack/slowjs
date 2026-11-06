// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build linux || darwin

package hostmem

import (
	"testing"

	"golang.org/x/sys/unix"
)

// anyMapped reports whether a page of [addr, addr+size) is mapped. msync
// fails with ENOMEM on a page that is not, on linux and darwin alike
// (darwin's mincore does not fail there).
func anyMapped(t *testing.T, addr, size uintptr) bool {
	t.Helper()
	for p := addr; p < addr+size; p += pageSize {
		switch err := unix.Msync(bytesAt(p, pageSize), unix.MS_ASYNC); err {
		case nil:
			return true
		case unix.ENOMEM:
		default:
			t.Fatalf("msync at %#x: %v", p, err)
		}
	}
	return false
}
