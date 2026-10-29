// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/sjdev"
)

func TestParsePositions(t *testing.T) {
	q, err := ParsePositions("quick")
	if err != nil || len(q) != 4 {
		t.Fatalf("quick = %v, %v", q, err)
	}
	a, err := ParsePositions("all")
	if err != nil || len(a) != 8 {
		t.Fatalf("all = %v, %v", a, err)
	}
	got, err := ParsePositions("0.0, 32.16")
	if err != nil || len(got) != 2 || got[1] != (Position{32, 16}) {
		t.Fatalf("list = %v, %v", got, err)
	}
	for _, bad := range []string{"16.0", "0.4", "0", "a.b", "0.0,0.0", "0.0.0"} {
		if _, err := ParsePositions(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if s := (Position{32, 8}).String(); s != "32.8" {
		t.Errorf("String = %s", s)
	}
	if BinName("base", Position{0, 16}) != "base@0.16" {
		t.Error("BinName")
	}
	ops := Position{32, 16}.XformOps("JS_CallInternal")
	if !slices.Equal(ops, []string{"shift:32", "front:JS_CallInternal:16"}) {
		t.Errorf("ops = %v", ops)
	}
}

func fakeTree(t *testing.T) string {
	t.Helper()
	tree := t.TempDir()
	tree, _ = filepath.EvalSymlinks(tree)
	dir := filepath.Join(tree, "internal", "cengine")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "cutils_amd64.s"), []byte("TEXT a\n\tRET\nTEXT b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "quickjs_amd64.s"), []byte("TEXT ·JS_CallInternal(SB)\n\tPCALIGN $64\n\tRET\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "engine_decl_amd64.go"), []byte("package cengine\n"), 0o644)
	os.WriteFile(filepath.Join(tree, "go.mod"), []byte("module m\n\ngo 1.27.0\n"), 0o644)
	return tree
}

// fakeXform does what xform does to the two places: it adds text after the first RET
// of cutils and after the PCALIGN of the interpreter.
func fakeXform(ctx context.Context, dir string, ops []string) error {
	for _, op := range ops {
		parts := strings.Split(op, ":")
		file, marker := "cutils_amd64.s", "\tRET\n"
		if parts[0] == "front" {
			file, marker = "quickjs_amd64.s", "PCALIGN $64\n"
		}
		n := parts[len(parts)-1]
		if n == "0" {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, file))
		s := string(b)
		i := strings.Index(s, marker) + len(marker)
		os.WriteFile(filepath.Join(dir, file), []byte(s[:i]+"NOP"+n+"\n"+s[i:]), 0o644)
	}
	return nil
}

func TestResolveFront(t *testing.T) {
	tree := fakeTree(t)
	got, err := ResolveFront(tree, "JS_CallInternal")
	if err != nil || got != "JS_CallInternal" {
		t.Fatalf("ResolveFront = %q, %v", got, err)
	}
	// A packed function: only NAME_block has a body.
	p := filepath.Join(tree, "internal/cengine/quickjs_amd64.s")
	os.WriteFile(p, []byte("TEXT ·JS_CallInternal_block(SB), $496-32\n\tPCALIGN $64\n\tRET\n"), 0o644)
	if got, err = ResolveFront(tree, "JS_CallInternal"); err != nil || got != "JS_CallInternal_block" {
		t.Fatalf("ResolveFront = %q, %v", got, err)
	}
	if _, err = ResolveFront(tree, "nope"); err == nil {
		t.Error("a missing function accepted")
	}
}

func TestMakeOverlayLeavesTheTreeAlone(t *testing.T) {
	tree := fakeTree(t)
	before, _ := os.ReadFile(filepath.Join(tree, "internal/cengine/cutils_amd64.s"))
	ov, err := MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{32, 16}, "JS_CallInternal", fakeXform)
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Replace) != 2 {
		t.Fatalf("overlay = %v", ov.Replace)
	}
	after, _ := os.ReadFile(filepath.Join(tree, "internal/cengine/cutils_amd64.s"))
	if !bytes.Equal(before, after) {
		t.Fatal("the tree changed")
	}
	key := filepath.Join(tree, "internal/cengine/cutils_amd64.s")
	moved, err := os.ReadFile(ov.Replace[key])
	if err != nil || !strings.Contains(string(moved), "NOP32") {
		t.Errorf("overlay file: %q, %v", moved, err)
	}

	// Position 0.0 changes nothing, and that is not an error.
	ov, err = MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{}, "JS_CallInternal", fakeXform)
	if err != nil || len(ov.Replace) != 0 {
		t.Errorf("0.0: %v, %v", ov, err)
	}
	// A position that moves nothing is an error: the xform did not find its place.
	nop := func(context.Context, string, []string) error { return nil }
	if _, err := MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{32, 0}, "JS_CallInternal", nop); err == nil {
		t.Error("a position that changed no file was accepted")
	}
	// No generated engine is an error.
	if _, err := MakeOverlay(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "w"), Position{}, "x", fakeXform); err == nil {
		t.Error("a tree without an engine was accepted")
	}
}

func TestBuildArgs(t *testing.T) {
	o := BuildOptions{Pkg: "./test/ssr", Test: true, Tags: "testhooks", PGO: "off"}
	got := strings.Join(o.BuildArgs("/o.json", "/b"), " ")
	if got != "test -c -overlay /o.json -pgo=off -buildvcs=false -tags testhooks -o /b ./test/ssr" {
		t.Errorf("args = %s", got)
	}
	o.Test = false
	if !strings.HasPrefix(strings.Join(o.BuildArgs("/o.json", "/b"), " "), "build -overlay") {
		t.Error("not a plain build")
	}
}

func TestBuildCheck(t *testing.T) {
	ok := BuildOptions{Side: "a", Pkg: "./x", Out: "o"}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	for name, mod := range map[string]func(*BuildOptions){
		"side": func(o *BuildOptions) { o.Side = "a b" },
		"pkg":  func(o *BuildOptions) { o.Pkg = "" },
		"out":  func(o *BuildOptions) { o.Out = "" },
	} {
		o := ok
		mod(&o)
		if o.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestBuildMakesOneBinaryPerPosition(t *testing.T) {
	tree := fakeTree(t)
	app := t.TempDir()
	out := filepath.Join(t.TempDir(), "bin")
	var cmds []sjdev.Cmd
	run := func(ctx context.Context, c sjdev.Cmd, _ io.Writer) sjdev.Result {
		cmds = append(cmds, c)
		return sjdev.Result{}
	}
	var log bytes.Buffer
	err := Build(context.Background(), BuildOptions{Side: "base", Tree: tree, App: app, Pkg: "./test/ssr", Test: true,
		Positions: []Position{{0, 0}, {32, 16}}, Out: out}, fakeXform, run, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if len(cmds) != 2 {
		t.Fatalf("%d commands", len(cmds))
	}
	c := cmds[1]
	if c.Name != "go" || c.Args[0] != "test" || !strings.HasSuffix(c.Args[slices.Index(c.Args, "-o")+1], "base@32.16") {
		t.Errorf("command: %v", c)
	}
	appReal, _ := filepath.EvalSymlinks(app)
	if c.Dir != appReal {
		t.Errorf("dir = %s, want %s", c.Dir, appReal)
	}
	var work string
	for _, e := range c.Env {
		if v, ok := strings.CutPrefix(e, "GOWORK="); ok {
			work = v
		}
	}
	text, err := os.ReadFile(work)
	if err != nil || !strings.Contains(string(text), "go 1.27.0") || !strings.Contains(string(text), "use "+tree) || !strings.Contains(string(text), "use "+appReal) {
		t.Errorf("go.work = %q, %v", text, err)
	}
	var ov Overlay
	ovFile := c.Args[slices.Index(c.Args, "-overlay")+1]
	b, _ := os.ReadFile(ovFile)
	if err := json.Unmarshal(b, &ov); err != nil || len(ov.Replace) != 2 {
		t.Errorf("overlay file %s: %v %v", ovFile, ov, err)
	}
	if cmds[0].Args[slices.Index(cmds[0].Args, "-overlay")+1] == ovFile {
		t.Error("two positions share an overlay file")
	}
}

func TestBuildInTheTreeWithoutAnApp(t *testing.T) {
	tree := fakeTree(t)
	var cmds []sjdev.Cmd
	run := func(ctx context.Context, c sjdev.Cmd, _ io.Writer) sjdev.Result {
		cmds = append(cmds, c)
		return sjdev.Result{}
	}
	err := Build(context.Background(), BuildOptions{Side: "x", Tree: tree, Pkg: "./internal/cengine/cmd/jsrun", Positions: []Position{{0, 0}}, Out: t.TempDir()}, fakeXform, run, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cmds[0].Dir != tree || cmds[0].Args[0] != "build" {
		t.Errorf("command: %+v", cmds[0])
	}
	for _, e := range cmds[0].Env {
		if strings.HasPrefix(e, "GOWORK=") {
			t.Error("GOWORK set without an app")
		}
	}
}

func TestGenerated(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a_amd64.s", "engine_decl_amd64.go", "engine_keep_amd64.go", "b_arm64.s", "engine_decl_arm64.go", "vm.go", "hammer_g.s"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	got, err := Generated(dir, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range got {
		names = append(names, filepath.Base(g))
	}
	if strings.Join(names, ",") != "a_amd64.s,engine_decl_amd64.go,engine_keep_amd64.go" {
		t.Errorf("Generated = %v", names)
	}
}
