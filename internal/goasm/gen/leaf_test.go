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

// TestAddrTakenLeafRejected: a register-entry leaf whose address is taken has
// no unified entry an indirect call could reach, so the pass must stop. Needs
// LLVM 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestAddrTakenLeafRejected(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	ir := filepath.Join(work, "a.ll")
	out, err := exec.Command(filepath.Join(llvm, "clang"), append(append([]string{}, EngineCFlags...),
		"-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "leaf", "addr_taken.c"))...).CombinedOutput()
	if err != nil {
		t.Fatalf("clang: %v\n%s", err, out)
	}
	out, err = exec.Command(filepath.Join(llvm, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
		"-shadow-add-sp", "-shadow-leaf=myleaf", "-S", "-o", os.DevNull, ir).CombinedOutput()
	if err == nil {
		t.Fatalf("accepted an address-taken leaf:\n%s", out)
	}
	if !strings.Contains(string(out), "address is taken") {
		t.Fatalf("wrong failure: %s", out)
	}
	// A rejection is one clean line and an ordinary exit, not an abort that
	// prints the stack of opt.
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Errorf("opt ended with %v, want exit status 1", err)
	}
	if lines := strings.Split(strings.TrimSpace(string(out)), "\n"); len(lines) != 1 ||
		!strings.HasPrefix(lines[0], "LLVM ERROR: ") || strings.Contains(string(out), "Stack dump") {
		t.Errorf("a rejection printed %d lines, want one starting LLVM ERROR:\n%s", len(lines), out)
	}
}
