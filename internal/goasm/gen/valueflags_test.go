// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// TestValueFlagsCopyInEightByteMoves: a copy of a JSValue (16 bytes in two
// words) is two moves of 8 bytes with EngineValueFlags, so that a load of it
// can be forwarded from the two 8-byte stores that wrote it, and one SSE move
// without them. Needs LLVM (SLOWJS_LLVM).
func TestValueFlagsCopyInEightByteMoves(t *testing.T) {
	llvm := os.Getenv("SLOWJS_LLVM")
	if llvm == "" {
		t.Skip("set SLOWJS_LLVM")
	}
	asm := func(flags []string) map[string]string {
		args := append(append([]string{}, EngineCFlags...), flags...)
		out, err := exec.Command(filepath.Join(llvm, "clang"),
			append(args, "-S", "-o", "-", filepath.Join("testdata", "valueflags", "copy.c"))...).CombinedOutput()
		if err != nil {
			t.Fatalf("clang: %v\n%s", err, out)
		}
		// One function's text per name, from its label to its .Lfunc_end.
		m := map[string]string{}
		for _, name := range []string{"copy_fields", "copy_struct"} {
			re := regexp.MustCompile(`(?s)\n` + name + `:[^\n]*\n(.*?)\n\.Lfunc_end`)
			f := re.FindStringSubmatch(string(out))
			if f == nil {
				t.Fatalf("no %s in the assembly:\n%s", name, out)
			}
			m[name] = f[1]
		}
		return m
	}
	mem8 := regexp.MustCompile(`(?m)^\s+movq\s.*\(%r\w+\)`)
	sse := regexp.MustCompile(`\b(movups|movupd|movdqu|movaps|movapd|movdqa)\b`)
	base := asm(nil)
	value := asm(EngineValueFlags)
	for _, name := range []string{"copy_fields", "copy_struct"} {
		if !sse.MatchString(base[name]) {
			t.Errorf("%s: without the flags LLVM no longer copies with an SSE move, so this test shows nothing:\n%s", name, base[name])
		}
		if sse.MatchString(value[name]) {
			t.Errorf("%s: an SSE move with the flags:\n%s", name, value[name])
		}
		if n := len(mem8.FindAllString(value[name], -1)); n != 4 {
			t.Errorf("%s: %d 8-byte moves to or from memory with the flags, want 4 (two loads, two stores):\n%s", name, n, value[name])
		}
	}
}

// TestValueFlagsFor: quickjs.c on amd64 gets EngineValueFlags, nothing else
// does, the small libc and arm64 included.
func TestValueFlagsFor(t *testing.T) {
	if got := ValueFlagsFor("amd64", "quickjs"); len(got) != len(EngineValueFlags) {
		t.Errorf("quickjs on amd64: %v, want %v", got, EngineValueFlags)
	}
	for _, c := range [][2]string{{"amd64", "libc"}, {"amd64", "libregexp"}, {"amd64", "cutils"}, {"arm64", "quickjs"}} {
		if got := ValueFlagsFor(c[0], c[1]); len(got) != 0 {
			t.Errorf("%s %s: %v, want none", c[0], c[1], got)
		}
	}
}
