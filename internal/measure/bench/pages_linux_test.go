// Copyright (c) 2026 The liustack/quickjs Authors.
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

// skipWherePagesStay skips the test when dir is on a filesystem whose files
// live only in memory (tmpfs, ramfs), told by the type statfs gives and not by
// the error DropPages returns. There no page can be dropped, so DropPages
// fails and bench run stops with that error, as it should. The test of the
// drop needs a folder on a disk.
func skipWherePagesStay(t *testing.T, dir string) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		t.Fatal(err)
	}
	var fs string
	switch uint32(st.Type) {
	case 0x01021994: // TMPFS_MAGIC
		fs = "tmpfs"
	case 0x858458f6: // RAMFS_MAGIC
		fs = "ramfs"
	default:
		return
	}
	t.Skipf("%s is on %s, which keeps every page of a file in memory, so no page can be dropped there: run with TMPDIR set to a folder on a disk", dir, fs)
}

func testDropPagesLocked(t *testing.T) {
	dir := t.TempDir()
	skipWherePagesStay(t, dir)
	f := filepath.Join(dir, "bin")
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
