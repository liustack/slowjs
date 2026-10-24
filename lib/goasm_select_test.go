// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

import (
	"go/build"
	"slices"
	"strings"
	"testing"
)

// TestOneInterpreterPerBuild checks the build constraints of the goasm
// files: on linux/amd64 a build uses the generated interpreter unless the
// slowjs_nogoasm tag is given, every other target and slowjs_nogoasm use the
// Go interpreter, and the old goasm tag changes nothing. Each build gets
// either all the goasm files or goasm_disabled.go, never both or neither.
func TestOneInterpreterPerBuild(t *testing.T) {
	asm := []string{"goasm_bridges_linux_amd64.go", "goasm_bridges_linux_amd64.s", "goasm_linux_amd64.go", "goasm_run_linux_amd64.s"}
	off := []string{"goasm_disabled.go"}
	for _, target := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"} {
		for _, tags := range [][]string{nil, {"goasm"}, {"slowjs_nogoasm"}, {"goasm", "slowjs_nogoasm"}} {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH, _ = strings.Cut(target, "/")
			ctx.CgoEnabled = false
			ctx.BuildTags = tags
			pkg, err := ctx.ImportDir(".", 0)
			if err != nil {
				t.Fatalf("%s %v: %v", target, tags, err)
			}
			var got []string
			for _, f := range slices.Concat(pkg.GoFiles, pkg.SFiles) {
				if strings.HasPrefix(f, "goasm_") {
					got = append(got, f)
				}
			}
			slices.Sort(got)
			want := off
			if target == "linux/amd64" && !slices.Contains(tags, "slowjs_nogoasm") {
				want = asm
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s with tags %v builds %v, want %v", target, tags, got, want)
			}
			hasTest := slices.Contains(pkg.TestGoFiles, "goasm_linux_amd64_test.go")
			if hasTest != slices.Equal(want, asm) {
				t.Errorf("%s with tags %v: goasm_linux_amd64_test.go included %v, want %v", target, tags, hasTest, !hasTest)
			}
		}
	}
}
