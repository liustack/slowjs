// Copyright (c) 2026 The slowjs Authors.
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
	if err := os.WriteFile(f, make([]byte, 1<<16), 0o644); err != nil {
		t.Fatal(err)
	}
	err := DropPages(f)
	if runtime.GOOS == "linux" && err != nil {
		t.Fatalf("DropPages on linux: %v", err)
	}
	if runtime.GOOS != "linux" && err == nil {
		t.Fatal("DropPages off linux did not say it cannot")
	}
	if err := DropPages(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("DropPages of a missing file did not fail")
	}
}
