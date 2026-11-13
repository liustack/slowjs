// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The tests here run on the function heads the generator writes now (the
// checked-in engine), which have the room check of windows/amd64 between
// NO_LOCAL_POINTERS and the PCALIGN of the body.

// copyEngine copies the generated amd64 files of internal/cengine into a new
// directory.
func copyEngine(t *testing.T) string {
	t.Helper()
	src, err := filepath.Glob("../../../cengine/*_amd64.s")
	if err != nil || len(src) == 0 {
		t.Fatalf("no generated amd64 files: %v", err)
	}
	d := t.TempDir()
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		write(t, d, filepath.Base(p), string(b))
	}
	return d
}

// engineFile returns the text of the file of d that has function name.
func engineFile(t *testing.T, d, name string) (string, string) {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(d, "*_amd64.s"))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "\nTEXT ·"+name+"(SB)") {
			return filepath.Base(p), string(b)
		}
	}
	t.Fatalf("function %s not found", name)
	return "", ""
}

func TestHeadMatchesEveryGeneratedFunctionOnce(t *testing.T) {
	d := copyEngine(t)
	files, _ := filepath.Glob(filepath.Join(d, "*_amd64.s"))
	n := 0
	for _, p := range files {
		// The files of the host and the room check are written by hand in the
		// generator, with no PCALIGN of a body.
		if base := filepath.Base(p); base == "engine_amd64.s" || strings.Contains(base, "room") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, m := range textLine.FindAllStringSubmatch(s, -1) {
			if got := len(head(m[1]).FindAllStringIndex(s, -1)); got != 1 {
				t.Errorf("%s in %s: %d matches of its head", m[1], filepath.Base(p), got)
			}
			n++
		}
	}
	if n < 1000 {
		t.Fatalf("only %d functions checked", n)
	}
}

func TestFrontOnGeneratedHead(t *testing.T) {
	d := copyEngine(t)
	file, before := engineFile(t, d, "JS_RegvmRun_block")
	// The interpreter keeps a pool that XORPD reads after its code: a front
	// that is not a multiple of 16 would take it off its 16 bytes.
	if err := apply(d, []string{"front:JS_RegvmRun_block:8"}); err == nil || !strings.Contains(err.Error(), "multiple of 16") {
		t.Fatalf("a front of 8 on the interpreter: %v", err)
	}
	if read(t, d, file) != before {
		t.Fatal("a refused front changed the file")
	}
	run(t, d, "front:JS_RegvmRun_block:16")
	after := read(t, d, file)
	i := strings.Index(before, "TEXT ·JS_RegvmRun_block(SB)")
	j := strings.Index(before[i:], "\tPCALIGN $")
	k := i + j + strings.Index(before[i+j:], "\n") + 1 // the line after the PCALIGN
	want := before[:k] + nopBytes(16) + before[k:]
	if after != want {
		t.Fatalf("front changed more or less than 16 bytes of no-ops after the PCALIGN of the body")
	}
	// The room check stays in front of the PCALIGN, where the generator put it.
	if !strings.Contains(after, "#endif\n\tPCALIGN $") {
		t.Fatal("the room check does not end right before the PCALIGN")
	}
}

func TestBodyOnGeneratedHead(t *testing.T) {
	d := copyEngine(t)
	file, before := engineFile(t, d, "JS_RegvmRun_block")
	run(t, d, "body:128:JS_RegvmRun_block")
	after := read(t, d, file)
	if strings.Count(after, "PCALIGN $128") != 1 {
		t.Fatalf("want one PCALIGN $128, got %d", strings.Count(after, "PCALIGN $128"))
	}
	i := strings.Index(after, "TEXT ·JS_RegvmRun_block(SB)")
	if !strings.Contains(after[i:i+400], "#endif\n\tPCALIGN $128\n") {
		t.Fatal("the PCALIGN of the body was not replaced in place")
	}
	// the PCALIGN of the body as the generator wrote it: $64 with -align64,
	// $16 without
	j := strings.Index(before, "TEXT ·JS_RegvmRun_block(SB)")
	was := regexp.MustCompile(`#endif\n\tPCALIGN (\$\d+)\n`).FindStringSubmatch(before[j : j+400])
	if was == nil {
		t.Fatal("no PCALIGN of the body after the room check")
	}
	if strings.Replace(after, "PCALIGN $128", "PCALIGN "+was[1], 1) != before {
		t.Fatalf("body changed more than the alignment of the body (the function had PCALIGN %s)", was[1])
	}
}

func TestFrontOnHeadWithoutRoomCheckOrWithOnlyIt(t *testing.T) {
	d := t.TempDir()
	write(t, d, "a_amd64.s", "TEXT ·f(SB), NOSPLIT, $0-0\n#ifdef GOOS_windows\n\tMOVQ (TLS), R14\n\tLEAQ -1(SP), R12\n#endif\n\tPCALIGN $16\n\tRET\n\n"+
		"TEXT ·g(SB), $8-0\n\tNO_LOCAL_POINTERS\n\tPCALIGN $16\n\tRET\n")
	run(t, d, "front:f:8", "front:g:8")
	got := read(t, d, "a_amd64.s")
	if strings.Count(got, "BYTE $0x0f") != 2 {
		t.Fatalf("nops not put in both functions:\n%s", got)
	}
	// An unclosed room block must not let the match run into the next function.
	write(t, d, "b_amd64.s", "TEXT ·h(SB), $0-0\n#ifdef GOOS_windows\n\tNOP\nTEXT ·i(SB), $0-0\n\tPCALIGN $16\n\tRET\n")
	if err := apply(d, []string{"front:h:8"}); err == nil {
		t.Fatal("a head with an unclosed block matched")
	}
}
