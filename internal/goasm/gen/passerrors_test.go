// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPassSizeLog2: the pass takes the same range of -shadow-size-log2 as
// CheckShadowSizeLog2, and stops outside it with one line and status 1. A
// shift of 1 by 64 or more in the pass would be undefined. It needs LLVM
// 22.1.8 (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS).
func TestPassSizeLog2(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	ir := filepath.Join(t.TempDir(), "f.ll")
	if err := os.WriteFile(ir, []byte("define i64 @f(i64 %x) {\n  ret i64 %x\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{MinShadowSizeLog2 - 1, MinShadowSizeLog2, 25, MaxShadowSizeLog2, MaxShadowSizeLog2 + 1, 64, 200} {
		out, err := exec.Command(filepath.Join(llvm, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp",
			fmt.Sprintf("-shadow-size-log2=%d", n), "-S", "-o", os.DevNull, ir).CombinedOutput()
		inRange := CheckShadowSizeLog2(n) == nil
		switch {
		case inRange && err != nil:
			t.Errorf("%d: the pass refused a size the generator takes: %v\n%s", n, err, out)
		case !inRange && (err == nil || !strings.Contains(string(out), "LLVM ERROR: -shadow-size-log2=")):
			t.Errorf("%d: the pass took a size the generator refuses, or failed otherwise: %v\n%s", n, err, out)
		case !inRange && strings.Contains(string(out), "Stack dump"):
			t.Errorf("%d: the refusal printed a stack:\n%s", n, out)
		}
	}
}
