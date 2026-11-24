// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestPhiNamesBlockTwice: the pass moves the locals of testdata/phi/switch.ll,
// whose phis name one block three times (three cases of a switch), onto the
// shadow stack. Each entry of a phi for one block must hold the same value, or
// LLVM's verifier stops opt. The pass must give each phi one address per
// incoming block, made in that block, at the offset of the local it replaces:
// %a at 0, %b at 4. It needs LLVM 22.1.8 (BEASTJS_LLVM) and the built pass
// (BEASTJS_SHADOW_PASS).
func TestPhiNamesBlockTwice(t *testing.T) {
	llvm, pass := os.Getenv("BEASTJS_LLVM"), os.Getenv("BEASTJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set BEASTJS_LLVM and BEASTJS_SHADOW_PASS")
	}
	shadow := filepath.Join(t.TempDir(), "switch.shadow.ll")
	out, err := exec.Command(filepath.Join(llvm, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
		"-shadow-add-sp", "-shadow-report", "-S", "-o", shadow, filepath.Join("testdata", "phi", "switch.ll")).CombinedOutput()
	if err != nil {
		t.Fatalf("opt: %v\n%s", err, out)
	}
	ir, err := os.ReadFile(shadow)
	if err != nil {
		t.Fatal(err)
	}
	label := regexp.MustCompile(`^([\w.]+):`)
	def := regexp.MustCompile(`^\s+(%[\w.]+) = (.*)$`)
	entry := regexp.MustCompile(`\[ (%[\w.]+), %([\w.]+) \]`)
	block := map[string]string{} // value -> the block that defines it
	body := map[string]string{}  // value -> its instruction
	phis := map[string]string{}  // phi -> its line
	cur := ""
	for _, line := range strings.Split(string(ir), "\n") {
		if m := label.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if m := def.FindStringSubmatch(line); m != nil {
			block[m[1]], body[m[1]] = cur, m[2]
			if strings.HasPrefix(m[2], "phi ") {
				phis[m[1]] = line
			}
		}
	}
	for _, tc := range []struct {
		phi  string
		want map[string]string // incoming block -> the address it must get
	}{
		{"%p", map[string]string{"sw": "getelementptr i8, ptr %sp, i64 0", "other": "getelementptr i8, ptr %sp, i64 4"}},
		{"%q", map[string]string{"sw": "getelementptr i8, ptr %sp, i64 4", "other": "getelementptr i8, ptr %sp, i64 0"}},
	} {
		line, ok := phis[tc.phi]
		if !ok {
			t.Errorf("no phi %s", tc.phi)
			continue
		}
		seen := map[string]string{}
		n := 0
		for _, m := range entry.FindAllStringSubmatch(line, -1) {
			v, from := m[1], m[2]
			n++
			if prev, ok := seen[from]; ok && prev != v {
				t.Errorf("%s names %s with %s and %s: %s", tc.phi, from, prev, v, line)
			}
			seen[from] = v
			if body[v] != tc.want[from] || block[v] != from {
				t.Errorf("%s from %s: %s = %q in %q, want %q in %q", tc.phi, from, v, body[v], block[v], tc.want[from], from)
			}
		}
		if n != 4 || len(seen) != 2 {
			t.Errorf("%s: %d entries for %d blocks, want 4 for 2: %s", tc.phi, n, len(seen), line)
		}
	}
	if t.Failed() {
		t.Logf("pass output:\n%s", ir)
	}
}
