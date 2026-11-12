// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	gen "github.com/liustack/quickjs/internal/goasm/gen"
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
	plain := devSwitches{NonFormat: defaultNonFormat, PostLink: "auto", PostArgs: strings.Join(gen.PostLinkOptArgs, " ")}
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
	if runtime.GOOS == "windows" {
		drv += ".exe"
	}
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
	if err != nil || !reflect.DeepEqual(got, map[string]int{"lre_exec_backtrack": 16}) {
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

// TestArchError: arm64 lowers the whole engine with -mode pn only, and
// refuses the switches its lowering does not do (-poison, -loose-varargs),
// which would otherwise build an engine as if they had not been given.
func TestArchError(t *testing.T) {
	for _, c := range []struct {
		arch, mode    string
		poison, loose bool
		want          string
	}{
		{"amd64", "body", false, false, ""},
		{"amd64", "pn", true, true, ""},
		{"arm64", "pn", false, false, ""},
		{"riscv64", "pn", false, false, "amd64 or arm64"},
		{"arm64", "body", false, false, "-mode pn only"},
		{"arm64", "pn", true, false, "-poison"},
		{"arm64", "pn", false, true, "-loose-varargs"},
	} {
		err := archError(c.arch, c.mode, c.poison, c.loose)
		if (c.want == "") != (err == nil) || err != nil && !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

// TestGoFileHeaders: with -build-tag the Go files the driver writes require
// the tag, the keep file together with goasmkeep and the negative build file
// together with its own. Without it they carry only their own.
func TestGoFileHeaders(t *testing.T) {
	defer func(old string) { *buildTag = old }(*buildTag)
	for _, c := range []struct{ tag, decl, keep, neg string }{
		{"", "", "//go:build goasmkeep\n", "//go:build !goasmnegative\n"},
		{"quickjs_cengine", "//go:build quickjs_cengine\n", "//go:build quickjs_cengine && goasmkeep\n", "//go:build quickjs_cengine && !goasmnegative\n"},
	} {
		*buildTag = c.tag
		if got := declBuildLine(); got != c.decl {
			t.Errorf("tag %q: decl line %q, want %q", c.tag, got, c.decl)
		}
		if got := keepAll(nil, nil, nil); !strings.Contains(got, c.keep) {
			t.Errorf("tag %q: keep file lacks %q:\n%s", c.tag, c.keep, got)
		}
		if got := negativeBuildFile(); !strings.Contains(got, c.neg) {
			t.Errorf("tag %q: negative file lacks %q", c.tag, c.neg)
		}
	}
}

// TestMissingNamesFormat: the list of stub names is written as gofmt writes
// it, whether it is empty (a release build) or not, so that the checked-in
// engine_decl_<arch>.go passes gofmt -s.
func TestMissingNamesFormat(t *testing.T) {
	for _, c := range []struct {
		stubs []gen.Meta
		want  string
	}{
		{nil, "var missingNames = []string{}\n"},
		{[]gen.Meta{{Name: "a"}, {Name: "b"}}, "var missingNames = []string{\n\t\"a\",\n\t\"b\",\n}\n"},
	} {
		got := missingNamesDecl(c.stubs)
		if got != c.want {
			t.Errorf("%d stubs: got %q, want %q", len(c.stubs), got, c.want)
		}
		src := "package p\n\n" + got
		if f, err := format.Source([]byte(src)); err != nil || string(f) != src {
			t.Errorf("%d stubs: gofmt changes %q to %q (%v)", len(c.stubs), src, f, err)
		}
	}
}

// TestCloneOf: llvm-link adds a number to a static whose name another file
// also uses, the optimizer over the linked module adds .specialized and a
// number to a function it clones, and both can follow each other. A name of C
// that only ends in digits is not a copy.
func TestCloneOf(t *testing.T) {
	for name, want := range map[string]string{
		"cr_add_interval.156":            "cr_add_interval",
		"mark_children.specialized.2":    "mark_children",
		"mark_children.specialized.2.7":  "mark_children",
		"f.12.specialized.3":             "f",
		"js_sub_string2":                 "",
		"memcpy":                         "",
		"js_string_pad.specialized":      "",
		"a.b":                            "",
		"mark_children.specialized.2x":   "",
		"lre_exec_backtrack.specialized": "",
	} {
		got, ok := cloneOf(name)
		if want == "" {
			if ok || got != name {
				t.Errorf("%s: cloneOf = %q, %v, want it left alone", name, got, ok)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%s: cloneOf = %q, %v, want %q", name, got, ok, want)
		}
	}
}

// TestCloneGroups: a copy of a function goes with the file of the function it
// is a copy of, lowered or a stub, and a function some file defined keeps the
// group of that file.
func TestCloneGroups(t *testing.T) {
	group := map[string]string{"mark_children": "quickjs", "cr_add_interval": "libregexp", "walk.specialized.1": "other"}
	metas := []gen.Meta{{Name: "mark_children.specialized.2"}, {Name: "mark_children.specialized.5"}, {Name: "walk.specialized.1"}, {Name: "lonely"}}
	stubs := []gen.Meta{{Name: "cr_add_interval.156"}, {Name: "unknown.specialized.1"}}
	cloneGroups(group, metas, stubs)
	for name, want := range map[string]string{
		"mark_children.specialized.2": "quickjs",
		"mark_children.specialized.5": "quickjs",
		"cr_add_interval.156":         "libregexp",
		"walk.specialized.1":          "other",
		"lonely":                      "",
		"unknown.specialized.1":       "",
	} {
		if got := group[name]; got != want {
			t.Errorf("group of %s = %q, want %q", name, got, want)
		}
	}
}

// TestPostLinkOn: on and off say so, auto follows gen.PostLinkOptFor, and
// anything else is an error before any work is done.
func TestPostLinkOn(t *testing.T) {
	for _, c := range []struct {
		mode, arch string
		want       bool
	}{{"on", "amd64", true}, {"on", "arm64", true}, {"off", "amd64", false}, {"off", "arm64", false},
		{"auto", "amd64", gen.PostLinkOptFor("amd64")}, {"auto", "arm64", gen.PostLinkOptFor("arm64")}} {
		if got, err := postLinkOn(c.mode, c.arch); err != nil || got != c.want {
			t.Errorf("%s %s: %v, %v, want %v", c.mode, c.arch, got, err, c.want)
		}
	}
	for _, mode := range []string{"", "yes", "ON", "1", "true"} {
		if _, err := postLinkOn(mode, "amd64"); err == nil || !strings.Contains(err.Error(), "-postlink-opt") {
			t.Errorf("%q: %v", mode, err)
		}
	}
}

// TestReleaseErrorPostLink: a release build runs the optimizer as the
// generator says, with its own arguments: it refuses a choice of its own.
func TestReleaseErrorPostLink(t *testing.T) {
	args := strings.Join(gen.PostLinkOptArgs, " ")
	if err := releaseError(devSwitches{NonFormat: defaultNonFormat, PostLink: "auto", PostArgs: args}, "out"); err != nil {
		t.Errorf("the default switches: %v", err)
	}
	// An empty value is a choice too: -postlink-opt-args= would run opt with
	// no pipeline at all.
	for _, c := range []struct {
		name string
		d    devSwitches
	}{
		{"-postlink-opt other than auto", devSwitches{NonFormat: defaultNonFormat, PostLink: "off", PostArgs: args}},
		{"-postlink-opt other than auto", devSwitches{NonFormat: defaultNonFormat, PostLink: "", PostArgs: args}},
		{"-postlink-opt-args", devSwitches{NonFormat: defaultNonFormat, PostLink: "auto", PostArgs: args + " -force-specialization"}},
		{"-postlink-opt-args", devSwitches{NonFormat: defaultNonFormat, PostLink: "auto", PostArgs: ""}},
	} {
		if err := releaseError(c.d, "out"); err == nil || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%+v: %v", c.d, err)
		}
	}
}
