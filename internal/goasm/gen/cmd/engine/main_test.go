// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestFirstGroups: a function two files define goes with the first one, as
// llvm-link keeps the first one's name, whichever compile finished last.
func TestFirstGroups(t *testing.T) {
	got := firstGroups([][]string{{"a", "dup"}, {"dup", "b"}, {"dup"}}, []string{"one", "two", "three"})
	want := map[string]string{"a": "one", "dup": "one", "b": "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestKeepNarrowFromEnv: only 1 turns the negative build on. A variable that
// reads as set but not as 1 (0, true, a space) is an error, not a build that
// treats 0 as on.
func TestKeepNarrowFromEnv(t *testing.T) {
	for v, want := range map[string]bool{"": false, "1": true} {
		if got, err := keepNarrowFromEnv(v); err != nil || got != want {
			t.Errorf("%q: %v, %v, want %v", v, got, err, want)
		}
	}
	for _, v := range []string{"0", "true", "yes", "2", " 1", "1 ", "off"} {
		if got, err := keepNarrowFromEnv(v); err == nil || got || !strings.Contains(err.Error(), "GEN_KEEP_NARROW") {
			t.Errorf("%q: accepted as %v, %v", v, got, err)
		}
	}
}

// TestReleaseError: a release build refuses each switch made for surveys,
// tests and negative builds, one at a time, and a plain build with an output
// folder goes on.
func TestReleaseError(t *testing.T) {
	plain := devSwitches{Arch: "amd64", NonFormat: defaultNonFormat}
	if err := releaseError(plain, "out"); err != nil {
		t.Fatalf("a plain release build: %v", err)
	}
	if err := releaseError(plain, ""); err == nil || !strings.Contains(err.Error(), "-out") {
		t.Fatalf("a release build with no -out: %v", err)
	}
	for name, edit := range map[string]func(*devSwitches){
		"-keep-going":                 func(d *devSwitches) { d.KeepGoing = true },
		"-loose-varargs":              func(d *devSwitches) { d.LooseVarargs = true },
		"-poison":                     func(d *devSwitches) { d.Poison = true },
		"GEN_KEEP_NARROW":             func(d *devSwitches) { d.KeepNarrow = true },
		"GEN_EDIT_LOWERED":            func(d *devSwitches) { d.EditLowered = "1" },
		"GEN_FORCE_NOSPLIT":           func(d *devSwitches) { d.ForceNoSplit = "f" },
		"-arch arm64":                 func(d *devSwitches) { d.Arch = "arm64" },
		"-non-format-variadic":        func(d *devSwitches) { d.NonFormat = "" },
		"without polls (-poll=false)": func(d *devSwitches) { d.NoPoll = true },
	} {
		d := plain
		edit(&d)
		if err := releaseError(d, "out"); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSizeLog2BeforeLLVM: a -shadow-size-log2 out of range stops the driver
// before it runs any tool or makes its work folder. The tools named here do
// not exist, so a run that got as far as one would fail with another message.
func TestSizeLog2BeforeLLVM(t *testing.T) {
	dir := t.TempDir()
	drv := filepath.Join(dir, "engine")
	build := exec.Command("go", "build", "-o", drv, ".")
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	for _, v := range []string{"-1", "0", "15", "41", "64", "100"} {
		work := filepath.Join(dir, "w"+v)
		out, err := exec.Command(drv, "-shadow-size-log2="+v, "-llvm", filepath.Join(dir, "no-llvm"), "-pass", filepath.Join(dir, "no-pass.so"),
			"-work", work).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "-shadow-size-log2="+v+" is out of range (16 to 40)") {
			t.Errorf("%s: %v\n%s", v, err, out)
		}
		if _, err := os.Stat(work); err == nil {
			t.Errorf("%s: the driver made its work folder", v)
		}
	}
}

// TestParseAlign64: -align64 takes name:offset pairs with the offset a
// multiple of 8 below 64, and nothing else.
func TestParseAlign64(t *testing.T) {
	got, err := parseAlign64(engineAlign64)
	if err != nil || !reflect.DeepEqual(got, map[string]int{"JS_CallInternal": 0, "lre_exec_backtrack": 16}) {
		t.Fatalf("the default: %v, %v", got, err)
	}
	if got, err := parseAlign64(""); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v, %v", got, err)
	}
	for _, s := range []string{"f", "f:", ":8", "f:4", "f:64", "f:-8", "f:x", "f:8,f:16"} {
		if got, err := parseAlign64(s); err == nil {
			t.Errorf("%q: accepted as %v", s, got)
		}
	}
}
