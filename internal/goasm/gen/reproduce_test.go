// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// reproduceSources writes three C files whose functions call each other across
// files, with jump tables and string constants, in numbers that a small
// -max-file turns into several .s files for each file.
func reproduceSources(t *testing.T, dir string) string {
	t.Helper()
	const files, funcs = 3, 24
	var paths []string
	for f := 0; f < files; f++ {
		var b strings.Builder
		// A static function of the same name in every file: the linker renames
		// the later ones (helper.1, helper.2), which go with the group of the first.
		fmt.Fprintf(&b, "__attribute__((noinline)) static long helper(long a) { return a * %d + %d; }\n", f+5, f*f+1)
		for k := 0; k < funcs; k++ {
			next := fmt.Sprintf("long r%d_%d(long);\n", (f+1)%files, k)
			if k > 0 {
				next += fmt.Sprintf("long r%d_%d(long);\n", f, k-1)
			}
			b.WriteString(next)
			fmt.Fprintf(&b, "const char *const s%d_%d = \"string %d of file %d\";\n", f, k, k, f)
			fmt.Fprintf(&b, "long r%d_%d(long a) {\n\tswitch (a & 7) {\n", f, k)
			for c := 0; c < 6; c++ {
				fmt.Fprintf(&b, "\tcase %d: a = a * %d + %d; break;\n", c, c+3, k+c)
			}
			b.WriteString("\tdefault: a ^= 0x5a5a;\n\t}\n")
			if k > 0 {
				fmt.Fprintf(&b, "\treturn r%d_%d(a) + r%d_%d(a + 1) + (long)s%d_%d[a & 3];\n}\n", f, k-1, (f+1)%files, k, f, k)
			} else {
				fmt.Fprintf(&b, "\treturn helper(a) + (long)s%d_%d[a & 3];\n}\n", f, k)
			}
		}
		p := filepath.Join(dir, fmt.Sprintf("part%d.c", f))
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return strings.Join(paths, ",")
}

// readPackage returns every file of a generated package by name.
func readPackage(t *testing.T, dir string) map[string]string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		m[e.Name()] = string(b)
	}
	return m
}

// TestOutputReproducible: the same C gives the same package, byte for byte,
// whatever the number of compile jobs and whichever compile finishes first,
// and the work folder is not written into any file. The three runs use jobs of
// 1, 4 and 4, and a bound on the size of a .s file small enough for several
// files in each group, for amd64 and for arm64. It needs LLVM 22.1.8
// (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS).
func TestOutputReproducible(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	for _, arch := range [][]string{nil, {"-arch", "arm64", "-mode", "pn"}} {
		name := "amd64"
		if arch != nil {
			name = "arm64"
		}
		t.Run(name, func(t *testing.T) { checkReproducible(t, llvm, pass, arch) })
	}
}

func checkReproducible(t *testing.T, llvm, pass string, arch []string) {
	work := t.TempDir()
	src := reproduceSources(t, work)
	run := driverRunner(t, work, llvm, pass)
	var pkgs []map[string]string
	for i, jobs := range []string{"1", "4", "4"} {
		name := fmt.Sprintf("run%d", i)
		if out, err := run(name, src, nil, append([]string{"-j", jobs, "-max-file", "6000"}, arch...)...); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		pkgs = append(pkgs, readPackage(t, filepath.Join(work, "out-"+name)))
	}
	parts := 0
	for name := range pkgs[0] {
		if strings.HasPrefix(name, "part") && strings.HasSuffix(name, ".s") {
			parts++
		}
	}
	if parts < 6 {
		t.Fatalf("only %d code files for three C files: the bound does not split them", parts)
	}
	// The static helpers of the three files share a name; the linker renames
	// the later ones, and they go with the group of the first.
	isHelper := regexp.MustCompile(`(?m)^TEXT ·helper(_\d+)?\(SB\)`)
	nHelpers := 0
	for n, text := range pkgs[0] {
		if k := len(isHelper.FindAllString(text, -1)); k > 0 {
			nHelpers += k
			if !strings.HasPrefix(n, "part0_") {
				t.Errorf("%d helpers in %s, want them in the files of part0", k, n)
			}
		}
	}
	if nHelpers != 3 {
		t.Errorf("%d helpers, want 3", nHelpers)
	}
	for i := 1; i < len(pkgs); i++ {
		var names []string
		for n := range pkgs[0] {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(pkgs[i]) != len(pkgs[0]) {
			t.Errorf("run %d wrote %d files, run 0 wrote %d", i, len(pkgs[i]), len(pkgs[0]))
		}
		for _, n := range names {
			got, ok := pkgs[i][n]
			if !ok {
				t.Errorf("run %d has no %s", i, n)
			} else if got != pkgs[0][n] {
				t.Errorf("%s differs between run 0 and run %d", n, i)
			}
			if strings.Contains(pkgs[0][n], work) {
				t.Errorf("%s holds the path of the work folder", n)
			}
		}
	}
}
