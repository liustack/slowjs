// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func testDropPagesLocked(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(f, make([]byte, 1<<16), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	m, err := syscall.Mmap(int(h.Fd()), 0, 1<<16, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Munmap(m)
	if err := syscall.Mlock(m); err != nil {
		t.Skipf("mlock: %v", err)
	}
	defer syscall.Munlock(m)
	if err := DropPages(f); err == nil {
		t.Fatal("DropPages of a file locked in memory said its pages were dropped")
	}
}
