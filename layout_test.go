// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
)

// layoutProblems returns the breaks of the layout rule in files (paths as
// git ls-files prints them): engine/ holds no Go file, and a folder with Go
// files holds no C file.
func layoutProblems(files []string) []string {
	exts := map[string]map[string]bool{}
	for _, f := range files {
		d := path.Dir(f)
		if exts[d] == nil {
			exts[d] = map[string]bool{}
		}
		exts[d][path.Ext(f)] = true
	}
	var bad []string
	for d, e := range exts {
		if (d == "engine" || strings.HasPrefix(d, "engine/")) && e[".go"] {
			bad = append(bad, d+" has Go files")
		}
		if e[".go"] && e[".c"] {
			bad = append(bad, d+" has Go files and C files")
		}
	}
	slices.Sort(bad)
	return bad
}

func TestLayoutProblems(t *testing.T) {
	ok := []string{"engine/quickjs.c", "engine/test/host.c", "internal/cengine/vm.go", "internal/goasm/allocas.cpp"}
	if got := layoutProblems(ok); len(got) != 0 {
		t.Errorf("layout of a good tree: %v", got)
	}
	if got := layoutProblems(append(slices.Clone(ok), "engine/libc/wrap.go")); !slices.Equal(got, []string{"engine/libc has Go files"}) {
		t.Errorf("a Go file in engine/libc: %v", got)
	}
	if got := layoutProblems(append(slices.Clone(ok), "internal/cengine/shim.c")); !slices.Equal(got, []string{"internal/cengine has Go files and C files"}) {
		t.Errorf("a C file next to Go files: %v", got)
	}
}

// TestRepositoryLayout checks the committed files: engine/ is the C tree and
// has no Go file, and no Go package folder holds a C file.
func TestRepositoryLayout(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Skip("not a git checkout:", err)
	}
	if bad := layoutProblems(strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")); len(bad) != 0 {
		t.Errorf("layout rule broken: %v", bad)
	}
}
