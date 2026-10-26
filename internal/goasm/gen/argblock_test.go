// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestArgBlockShared checks that the calls of one function share one
// argument block: three(x) makes three packed calls of 56 bytes of
// arguments, so its shadow frame is one 64-byte block. It needs LLVM 22.1.8
// (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS).
func TestArgBlockShared(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	ir := filepath.Join(t.TempDir(), "a.ll")
	out, err := exec.Command(filepath.Join(llvm, "clang"), append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...),
		"-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "argblock.c"))...).CombinedOutput()
	if err != nil {
		t.Fatalf("clang: %v\n%s", err, out)
	}
	out, err = exec.Command(filepath.Join(llvm, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
		"-shadow-add-sp", "-shadow-max-int-args=6", "-shadow-report", "-S", "-o", os.DevNull, ir).CombinedOutput()
	if err != nil {
		t.Fatalf("opt: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PACKED many") {
		t.Fatalf("many not packed:\n%s", out)
	}
	if !strings.Contains(string(out), "SHADOW three 64\n") {
		t.Fatalf("want one 64-byte block for three:\n%s", out)
	}
}
