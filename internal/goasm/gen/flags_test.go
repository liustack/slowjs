// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestFlagsComeFromFlagsSh: the portable C flags have one source,
// engine/test/flags.sh. EngineCFlags and ARM64CFlags hold all of them (read
// here from the text of the file, apart from the way init runs it in sh),
// arm64 holds the target flags of the file and amd64 does not, and none of
// the generator's own flags is also one of those, so a flag cannot live in
// two places. cycles.sh, which compiles at -O0 for the call graph, repeats the
// generator's IR-shaping flags in a line of its own: that line and the
// generator's list must agree, except for the flags named here.
func TestFlagsComeFromFlagsSh(t *testing.T) {
	if FlagsErr != nil {
		t.Fatal(FlagsErr)
	}
	root := filepath.Join("..", "..", "..", "engine", "test")
	text := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	m := regexp.MustCompile(`(?m)^engine_flags="([^"]*)"$`).FindStringSubmatch(text("flags.sh"))
	if m == nil {
		t.Fatal(`no engine_flags="..." line in flags.sh`)
	}
	shared := strings.Fields(m[1])
	if len(shared) < 8 {
		t.Fatalf("flags.sh has only %v", shared)
	}
	has := func(list []string, f string) bool {
		for _, x := range list {
			if x == f {
				return true
			}
		}
		return false
	}
	for _, f := range shared {
		for name, list := range map[string][]string{"EngineCFlags": EngineCFlags, "ARM64CFlags": ARM64CFlags} {
			if !has(list, f) {
				t.Errorf("%s lacks %s, a flag of flags.sh", name, f)
			}
		}
		for name, list := range map[string][]string{"engineOnly": engineOnly, "arm64Only": arm64Only} {
			if has(list, f) {
				t.Errorf("%s repeats %s, which flags.sh gives", name, f)
			}
		}
	}
	// The target flags of flags.sh: arm64 only.
	if !has(ARM64CFlags, "-mno-outline-atomics") || has(EngineCFlags, "-mno-outline-atomics") {
		t.Errorf("-mno-outline-atomics must come with arm64 and not with amd64: %v / %v", ARM64CFlags, EngineCFlags)
	}
	if !regexp.MustCompile(`(?s)aarch64\* \| arm64\*\) echo "-mno-outline-atomics"`).MatchString(text("flags.sh")) {
		t.Error("flags.sh no longer gives -mno-outline-atomics for arm64: update this test with it")
	}

	// cycles.sh: the line of flags it compiles with.
	cm := regexp.MustCompile(`(?m)^flags="([^"]*)"$`).FindStringSubmatch(text("cycles.sh"))
	if cm == nil {
		t.Fatal(`no flags="..." line in cycles.sh`)
	}
	cycles := strings.Fields(strings.ReplaceAll(cm[1], "$engine_flags", ""))
	// It compiles at -O0 and for kcfi, and asks no backend feature.
	notInCycles := map[string]bool{"-O2": true, "-Xclang": true, "-target-feature": true, "+slow-two-mem-ops": true}
	notInGen := map[string]bool{"-O0": true, "-fsanitize=kcfi": true}
	for _, f := range cycles {
		if !notInGen[f] && !has(engineOnly, f) {
			t.Errorf("cycles.sh compiles with %s, which the generator's flags do not have", f)
		}
	}
	for _, f := range engineOnly {
		if !notInCycles[f] && !has(cycles, f) {
			t.Errorf("the generator compiles with %s, which cycles.sh does not", f)
		}
	}
	if !strings.Contains(text("cycles.sh"), `. "$here/flags.sh"`) {
		t.Error("cycles.sh does not source flags.sh")
	}
	for _, name := range []string{"build.sh", "check-symbols.sh", "libc-test.sh"} {
		if !strings.Contains(text(name), `. "$here/flags.sh"`) {
			t.Errorf("%s does not source flags.sh", name)
		}
	}
}

// TestSharedCFlagsErrors: a flags file that cannot be read or sets nothing is
// an error, not an empty list.
func TestSharedCFlagsErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, err := SharedCFlags("", "x86_64-linux-gnu"); err == nil {
		t.Error("an empty path was taken")
	}
	if _, err := SharedCFlags(filepath.Join(dir, "none.sh"), "x86_64-linux-gnu"); err == nil {
		t.Error("a missing file was taken")
	}
	if _, err := SharedCFlags(write("empty.sh", "x=1\n"), "x86_64-linux-gnu"); err == nil {
		t.Error("a file with no engine_flags was taken")
	}
	got, err := SharedCFlags(write("ok.sh", "engine_flags=\"-a -b\"\nengine_target_flags() { echo \"-t-$1\"; }\n"), "x86_64-linux-gnu")
	if err != nil || strings.Join(got, " ") != "-a -b -t-x86_64-linux-gnu" {
		t.Errorf("a small file: %v, %v", got, err)
	}
}

// TestFlagsSameFloats: the engine's C is compiled to give the same numbers
// on every target. It contracts no multiply and add into one rounding
// (-ffp-contract=off: aarch64 would fuse them, amd64 without FMA does not)
// and takes no flag that lets the compiler reorder, approximate or assume
// away floating-point results.
func TestFlagsSameFloats(t *testing.T) {
	for name, flags := range map[string][]string{"amd64": EngineCFlags, "arm64": ARM64CFlags} {
		if !slices.Contains(flags, "-ffp-contract=off") {
			t.Errorf("%s: no -ffp-contract=off in %v", name, flags)
		}
		for _, f := range flags {
			for _, bad := range []string{"-ffast-math", "-funsafe-math-optimizations", "-fassociative-math", "-freciprocal-math",
				"-ffinite-math-only", "-fno-honor-nans", "-fno-honor-infinities", "-fno-signed-zeros", "-fapprox-func",
				"-ffp-contract=fast", "-ffp-contract=on", "-ffp-model=fast", "-Ofast", "-mfma"} {
				if f == bad || strings.HasPrefix(f, bad+"=") {
					t.Errorf("%s: %s", name, f)
				}
			}
		}
	}
}
