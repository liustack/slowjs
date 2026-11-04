// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDropPages(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bin")
	// Just written: its pages are dirty and in memory.
	if err := os.WriteFile(f, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "linux" {
		if DropPages(f) == nil {
			t.Fatal("DropPages off linux did not say it cannot")
		}
		return
	}
	if _, err := os.ReadFile(f); err != nil {
		t.Fatal(err)
	}
	n, total, err := Resident(f)
	if err != nil || total != (1<<20+os.Getpagesize()-1)/os.Getpagesize() {
		t.Fatalf("Resident = %d of %d, %v", n, total, err)
	}
	if n == 0 {
		t.Log("the file was not in memory after reading it: the test says less on this machine")
	}
	if err := DropPages(f); err != nil {
		t.Fatalf("DropPages: %v", err)
	}
	if n, _, _ := Resident(f); n != 0 {
		t.Fatalf("%d pages still in memory", n)
	}
	if err := DropPages(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("DropPages of a missing file did not fail")
	}
}

// A file whose pages are locked in memory cannot be dropped, and DropPages
// says so instead of reporting fresh pages. (Linux 7.3 drops pages that are
// only mapped, so the test locks them.)
func TestDropPagesFailsWhenAPageStays(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	testDropPagesLocked(t)
}
