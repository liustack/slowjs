// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const cutils = "TEXT ·utf8_decode(SB), $16-24\n\tPCALIGN $16\n\tBYTE $0xc3\n\tRET\n\n" +
	"TEXT ·other(SB), $0-8\n\tPCALIGN $16\n\tBYTE $0xc3\n\tRET\n\n"

const engine = "TEXT ·JS_CallInternal(SB), $352-24\n\tNO_LOCAL_POINTERS\n\tPCALIGN $16\n\tBYTE $0x55\n\tRET\n\n"

func setup(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	write(t, d, "cutils_amd64.s", cutils)
	write(t, d, "engine_amd64.s", engine)
	return d
}

func write(t *testing.T, d, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, d, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(d, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t *testing.T, d string, ops ...string) {
	t.Helper()
	if err := apply(d, ops); err != nil {
		t.Fatal(err)
	}
}

func TestShiftPadsAfterTheRETOfTheFirstFunction(t *testing.T) {
	d := setup(t)
	run(t, d, "shift:32")
	first, second, _ := strings.Cut(cutils, "TEXT ·other")
	want := strings.Replace(first, "\tRET\n", "\tRET\n"+strings.Repeat("\tBYTE $0x90\n", 32), 1) + "TEXT ·other" + second
	if got := read(t, d, "cutils_amd64.s"); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if read(t, d, "engine_amd64.s") != engine {
		t.Fatal("shift changed engine_amd64.s")
	}
}

func TestShiftZeroChangesNothing(t *testing.T) {
	d := setup(t)
	run(t, d, "shift:0")
	if read(t, d, "cutils_amd64.s") != cutils {
		t.Fatal("shift:0 changed cutils_amd64.s")
	}
}

func TestFrontPutsTheNOPAfterThePCALIGNOfTheNamedFunction(t *testing.T) {
	d := setup(t)
	run(t, d, "front:JS_CallInternal:16")
	want := strings.Replace(engine, "\tPCALIGN $16\n", "\tPCALIGN $16\n"+nopBytes(16), 1)
	if got := read(t, d, "engine_amd64.s"); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if read(t, d, "cutils_amd64.s") != cutils {
		t.Fatal("front changed cutils_amd64.s")
	}
}

func TestFrontNOPSizesAreExact(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 9, 16, 24, 63} {
		d := setup(t)
		run(t, d, "front:JS_CallInternal:"+itoa(n))
		bytes := strings.Count(read(t, d, "engine_amd64.s"), "BYTE $0x") - 1
		if bytes != n {
			t.Fatalf("front %d added %d bytes", n, bytes)
		}
	}
}

func TestEightBytesAreOneMultiByteNOP(t *testing.T) {
	want := "\tBYTE $0x0f\n\tBYTE $0x1f\n\tBYTE $0x84\n\tBYTE $0x00\n\tBYTE $0x00\n\tBYTE $0x00\n\tBYTE $0x00\n\tBYTE $0x00\n"
	if got := nopBytes(8); got != want {
		t.Fatalf("got\n%s", got)
	}
}

// A body may have no tab before NO_LOCAL_POINTERS and PCALIGN.
func TestFrontOnTheUnindentedForm(t *testing.T) {
	d := setup(t)
	write(t, d, "run_amd64.s", "TEXT ·goasmRun(SB), $352-24\nNO_LOCAL_POINTERS\nPCALIGN $16\nRET\n")
	run(t, d, "front:goasmRun:8")
	want := "TEXT ·goasmRun(SB), $352-24\nNO_LOCAL_POINTERS\nPCALIGN $16\n" + nopBytes(8) + "RET\n"
	if got := read(t, d, "run_amd64.s"); got != want {
		t.Fatalf("got\n%s", got)
	}
}

func TestFrontOnAMissingFunctionFails(t *testing.T) {
	d := setup(t)
	if err := apply(d, []string{"front:nothing:8"}); err == nil || !strings.Contains(err.Error(), "found 0 times") {
		t.Fatalf("got %v", err)
	}
}

func TestShiftThenFrontApplyInOrder(t *testing.T) {
	d := setup(t)
	run(t, d, "shift:32", "front:JS_CallInternal:8")
	if n := strings.Count(read(t, d, "cutils_amd64.s"), "BYTE $0x90"); n != 32 {
		t.Fatalf("%d NOP bytes in cutils_amd64.s", n)
	}
	if !strings.Contains(read(t, d, "engine_amd64.s"), nopBytes(8)) {
		t.Fatal("no front NOP")
	}
}

func TestBodyReplacesTheAlignment(t *testing.T) {
	d := setup(t)
	run(t, d, "body:64:JS_CallInternal")
	if got, want := read(t, d, "engine_amd64.s"), strings.Replace(engine, "PCALIGN $16", "PCALIGN $64", 1); got != want {
		t.Fatalf("got\n%s", got)
	}
}

// The generator puts the interpreter's body on 64 bytes: front still finds it.
func TestFrontAfterABodyOn64Bytes(t *testing.T) {
	d := setup(t)
	write(t, d, "engine_amd64.s", strings.Replace(engine, "PCALIGN $16", "PCALIGN $64", 1))
	run(t, d, "front:JS_CallInternal:8")
	want := strings.Replace(engine, "\tPCALIGN $16\n", "\tPCALIGN $64\n"+nopBytes(8), 1)
	if got := read(t, d, "engine_amd64.s"); got != want {
		t.Fatalf("got\n%s", got)
	}
}

func TestEndalignOnlyTheNamedFunction(t *testing.T) {
	d := setup(t)
	run(t, d, "endalign:32:other")
	text := read(t, d, "cutils_amd64.s")
	if strings.Count(text, "PCALIGN $32") != 1 || strings.Index(text, "TEXT ·other") > strings.Index(text, "PCALIGN $32") {
		t.Fatalf("got\n%s", text)
	}
	if !strings.HasSuffix(text, "\tRET\n\tPCALIGN $32\n\n") {
		t.Fatalf("the PCALIGN is not right after the RET of other:\n%s", text)
	}
}

func TestEndalignOnAMissingFunctionFails(t *testing.T) {
	d := setup(t)
	if err := apply(d, []string{"endalign:64:nothing"}); err == nil {
		t.Fatal("no error")
	}
}

func TestUnknownOpStops(t *testing.T) {
	d := setup(t)
	if err := apply(d, []string{"wobble:1"}); err == nil || !strings.Contains(err.Error(), "unknown op") {
		t.Fatalf("got %v", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
