// Copyright (c) 2026 The liustack/quickjs Authors.
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

	"github.com/liustack/quickjs/internal/measure"
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
	if err != nil || len(got) != 2 || got[1] != (Position{32, 16, 0}) {
		t.Fatalf("list = %v, %v", got, err)
	}
	for _, bad := range []string{"16.0", "0.4", "0", "a.b", "0.0,0.0", "0.0.0"} {
		if _, err := ParsePositions(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if s := (Position{32, 8, 0}).String(); s != "32.8" {
		t.Errorf("String = %s", s)
	}
	if BinName("base", Position{0, 16, 0}) != "base@0.16" {
		t.Error("BinName")
	}
	ops := Position{32, 16, 0}.XformOps("JS_RegvmRun", "")
	if !slices.Equal(ops, []string{"shift:32", "front:JS_RegvmRun:16"}) {
		t.Errorf("ops = %v", ops)
	}
	// The second shift goes after the regexp matcher, between the first shift and the front.
	ops = Position{64, 8, 2208}.XformOps("JS_RegvmRun_block", "dbuf_init2")
	if !slices.Equal(ops, []string{"shift:64:dbuf_init2", "shift:2208:" + Matcher, "front:JS_RegvmRun_block:8"}) {
		t.Errorf("ops = %v", ops)
	}
	ops = Position{0, 0, 32}.XformOps("JS_RegvmRun_block", "")
	if !slices.Equal(ops, []string{"shift:0", "shift:32:" + Matcher, "front:JS_RegvmRun_block:0"}) {
		t.Errorf("ops = %v", ops)
	}
	for _, c := range []struct {
		p Position
		s string
	}{{Position{0, 0, 0}, "0.0"}, {Position{64, 8, 2208}, "64.8.2208"}, {Position{0, 56, 32}, "0.56.32"}} {
		if c.p.String() != c.s {
			t.Errorf("String of %v = %s, want %s", c.p, c.p.String(), c.s)
		}
		if q, err := ParsePosition(c.s); err != nil || q != c.p {
			t.Errorf("ParsePosition(%s) = %v, %v", c.s, q, err)
		}
	}
	for _, bad := range []string{"0.0.16x", "0.0.8", "0.0.8192", "0.0.-32", "0.0.32.0", "0.0.0"} {
		if _, err := ParsePosition(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func fakeTree(t *testing.T) string {
	t.Helper()
	tree := t.TempDir()
	tree, _ = filepath.EvalSymlinks(tree)
	dir := filepath.Join(tree, "internal", "cengine")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "cutils_amd64.s"), []byte("TEXT a\n\tRET\nTEXT b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "quickjs_amd64.s"), []byte("TEXT ·JS_RegvmRun(SB)\n\tPCALIGN $64\n\tRET\n"), 0o644)
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
		if parts[0] == "shift" {
			n = parts[1]
		}
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
	got, err := ResolveFront(tree, "JS_RegvmRun")
	if err != nil || got != "JS_RegvmRun" {
		t.Fatalf("ResolveFront = %q, %v", got, err)
	}
	// A packed function: only NAME_block has a body.
	p := filepath.Join(tree, "internal/cengine/quickjs_amd64.s")
	os.WriteFile(p, []byte("TEXT ·JS_RegvmRun_block(SB), $496-32\n\tPCALIGN $64\n\tRET\n"), 0o644)
	if got, err = ResolveFront(tree, "JS_RegvmRun"); err != nil || got != "JS_RegvmRun_block" {
		t.Fatalf("ResolveFront = %q, %v", got, err)
	}
	if _, err = ResolveFront(tree, "nope"); err == nil {
		t.Error("a missing function accepted")
	}
}

func TestMakeOverlayLeavesTheTreeAlone(t *testing.T) {
	tree := fakeTree(t)
	before, _ := os.ReadFile(filepath.Join(tree, "internal/cengine/cutils_amd64.s"))
	ov, err := MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{32, 16, 0}, "JS_RegvmRun", "", fakeXform)
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
	ov, err = MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{}, "JS_RegvmRun", "", fakeXform)
	if err != nil || len(ov.Replace) != 0 {
		t.Errorf("0.0: %v, %v", ov, err)
	}
	// A position that moves nothing is an error: the xform did not find its place.
	nop := func(context.Context, string, []string) error { return nil }
	if _, err := MakeOverlay(context.Background(), tree, filepath.Join(t.TempDir(), "w"), Position{32, 0, 0}, "JS_RegvmRun", "", nop); err == nil {
		t.Error("a position that changed no file was accepted")
	}
	// No generated engine is an error.
	if _, err := MakeOverlay(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "w"), Position{}, "x", "", fakeXform); err == nil {
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
	os.WriteFile(filepath.Join(app, "go.mod"), []byte("module app\n\ngo 1.27.0\n"), 0o644)
	out := filepath.Join(t.TempDir(), "bin")
	var cmds []measure.Cmd
	run := func(ctx context.Context, c measure.Cmd, _ io.Writer) measure.Result {
		cmds = append(cmds, c)
		return measure.Result{}
	}
	var log bytes.Buffer
	err := Build(context.Background(), BuildOptions{Side: "base", Tree: tree, App: app, Pkg: "./test/ssr", Test: true,
		Positions: []Position{{32, 16, 0}, {0, 0, 0}}, Out: out, Pad: "live"}, fakeXform, run, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if len(cmds) != 2 {
		t.Fatalf("%d commands", len(cmds))
	}
	// The formal binary is built first.
	if !strings.HasSuffix(cmds[0].Args[slices.Index(cmds[0].Args, "-o")+1], "base@0.0") {
		t.Errorf("first command: %v", cmds[0])
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

// TestBuildWorkFileTakesTheHigherGoVersion: a go.work whose go line is below
// the go line of one of its modules does not load, so the build of an app that
// needs a newer Go than the tree takes the app's go line, and the other way
// round.
func TestBuildWorkFileTakesTheHigherGoVersion(t *testing.T) {
	for _, c := range []struct{ tree, app, want string }{
		{"1.26.6", "1.27.0", "1.27.0"},
		{"1.27.0", "1.26.6", "1.27.0"},
		{"1.26.10", "1.26.6", "1.26.10"},
	} {
		tree := fakeTree(t)
		os.WriteFile(filepath.Join(tree, "go.mod"), []byte("module m\n\ngo "+c.tree+"\n"), 0o644)
		app := t.TempDir()
		os.WriteFile(filepath.Join(app, "go.mod"), []byte("module app\n\ngo "+c.app+"\n"), 0o644)
		out := filepath.Join(t.TempDir(), "bin")
		var work string
		run := func(ctx context.Context, cmd measure.Cmd, _ io.Writer) measure.Result {
			for _, e := range cmd.Env {
				if v, ok := strings.CutPrefix(e, "GOWORK="); ok {
					work = v
				}
			}
			return measure.Result{}
		}
		var log bytes.Buffer
		err := Build(context.Background(), BuildOptions{Side: "base", Tree: tree, App: app, Pkg: "./test/ssr", Test: true,
			Positions: []Position{{0, 0, 0}}, Out: out, Pad: "live"}, fakeXform, run, &log)
		if err != nil {
			t.Fatalf("tree %s, app %s: %v\n%s", c.tree, c.app, err, log.String())
		}
		text, err := os.ReadFile(work)
		if err != nil || !strings.HasPrefix(string(text), "go "+c.want+"\n") {
			t.Errorf("tree %s, app %s: go.work = %q, %v, want go %s", c.tree, c.app, text, err, c.want)
		}
	}
}

func TestBuildInTheTreeWithoutAnApp(t *testing.T) {
	tree := fakeTree(t)
	var cmds []measure.Cmd
	run := func(ctx context.Context, c measure.Cmd, _ io.Writer) measure.Result {
		cmds = append(cmds, c)
		return measure.Result{}
	}
	err := Build(context.Background(), BuildOptions{Side: "x", Tree: tree, Pkg: "./internal/cengine/cmd/jsrun", Positions: []Position{{0, 0, 0}}, Out: t.TempDir()}, fakeXform, run, io.Discard)
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

func TestProtocolLayouts(t *testing.T) {
	ps := Protocol()
	if len(ps) != 16 || ps[0] != Formal {
		t.Fatalf("protocol = %v", ps)
	}
	fronts, shifts := map[int]int{}, map[int]bool{}
	for _, p := range ps {
		fronts[p.Front]++
		shifts[p.Shift] = true
		if _, err := ParsePosition(p.String()); err != nil {
			t.Error(err)
		}
		// Only the formal layout has no shift. A shift of 32 can be taken up
		// entirely by the alignment of the regexp matcher, any other multiple of
		// 32 moves it (by a multiple of 64 within 32 of the shift).
		if p != Formal && (p.Shift < 64 || p.Shift%32 != 0) {
			t.Errorf("%s: shift %d", p, p.Shift)
		}
	}
	// 16 shifts, in different places of the page, none 512 bytes or more from
	// the next one (the code from the first engine function to the matcher
	// moves by exactly the shift).
	if len(shifts) != 16 {
		t.Errorf("%d different shifts", len(shifts))
	}
	var offs []int
	for s := range shifts {
		offs = append(offs, s%4096)
	}
	checkSpread(t, "the shifts", offs)
	// every front of the 64-byte block twice
	for f := 0; f < 64; f += 8 {
		if fronts[f] != 2 {
			t.Errorf("front %d %d times", f, fronts[f])
		}
	}
	if len(Full()) != 16 {
		t.Error("full")
	}
	for _, bad := range []string{"48.0", "0.60", "8192.0", "-32.0", "0.12"} {
		if _, err := ParsePosition(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

// checkSpread wants the places in a page all different and each no 512 bytes
// or more from the next, the last from the first around the page.
func checkSpread(t *testing.T, what string, offs []int) {
	t.Helper()
	slices.Sort(offs)
	if len(slices.Compact(slices.Clone(offs))) != len(offs) {
		t.Errorf("%s share a place in the page: %v", what, offs)
	}
	for i, o := range offs {
		next := offs[(i+1)%len(offs)] + 4096*b2i(i+1 == len(offs))
		if next-o >= 512 {
			t.Errorf("no place of %s between %d and %d of the page", what, o, next%4096)
		}
	}
}

// TestProtocolSections: the three sections of the engine have their own place
// modulo 64, and each is one of two, since Go starts a function on 32. The code
// from the first engine function to the matcher follows the shift, the code
// from the matcher to the interpreter the second shift (the matcher and the
// interpreter take up a multiple of 64 of either, so the sum is no use), and the
// code after the interpreter the front. In each half of the set, each of the
// first two is 32 bytes into the block in 4 layouts, and each of the 5 two-way
// choices (shift, shift2 and the 3 bits of the front) is independent of the
// others: any two have 2 layouts with both set. The places in the page of the
// code of the first two sections, taken as the shift and the shift plus the
// second shift, are 16 different ones about 256 bytes apart.
func TestProtocolSections(t *testing.T) {
	ps := Protocol()
	if ps[0].Shift2 != 0 {
		t.Errorf("the formal layout has a second shift: %v", ps[0])
	}
	var between []int
	var all [2][2]int
	for h, half := range [2][]Position{ps[:8], ps[8:]} {
		var col [5][8]bool
		for i, p := range half {
			if p.Shift2%32 != 0 || p.Shift2 < 0 || p.Shift2 >= MaxShift {
				t.Errorf("second shift of %s", p)
			}
			if p.Shift2 == 0 && p != Formal {
				t.Errorf("%s has no second shift", p)
			}
			if p.Front != 8*i {
				t.Errorf("%s: the %dth of a half has front %d", p, i, p.Front)
			}
			col[0][i], col[1][i] = p.Shift%64 == 32, p.Shift2%64 == 32
			for b := range 3 {
				col[2+b][i] = i>>b&1 == 1
			}
			all[b2i(col[0][i])][b2i(col[1][i])]++
			between = append(between, (p.Shift+p.Shift2)%4096)
		}
		for x := range col {
			n := 0
			for _, v := range col[x] {
				n += b2i(v)
			}
			if n != 4 {
				t.Errorf("half %d: choice %d is set in %d layouts", h, x, n)
			}
			for y := x + 1; y < len(col); y++ {
				n := 0
				for i := range 8 {
					n += b2i(col[x][i] && col[y][i])
				}
				if n != 2 {
					t.Errorf("half %d: choices %d and %d are both set in %d layouts", h, x, y, n)
				}
			}
		}
	}
	if all != [2][2]int{{4, 4}, {4, 4}} {
		t.Errorf("shift and second shift 32 bytes into the block: %v", all)
	}
	checkSpread(t, "the code between the matcher and the interpreter", between)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
