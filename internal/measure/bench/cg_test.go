// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/measure"
)

func listing(total int, fns map[string]int) string {
	var b strings.Builder
	b.WriteString("-- Auto-annotated source\n")
	fmt.Fprintf(&b, "%d (100.0%%)  PROGRAM TOTALS\n", total)
	for k, v := range fns {
		fmt.Fprintf(&b, "%d ( 1.0%%)  %s\n", v, k)
	}
	return b.String()
}

func TestParseListing(t *testing.T) {
	text := `--------------------
1,000,000 (100.0%)  PROGRAM TOTALS
--------------------
  900,000 (90.00%)  /go/src/github.com/liustack/quickjs/internal/cengine/engine_amd64.s:JS_CallInternal [/bin/x]
  400,000 (40.00%)  /x/y.go:github.com/liustack/quickjs/internal/cengine.callEngine [/bin/x]
  300,000 (30.00%)  /x/y.go:rec'2 [/bin/x]
  200,000 (20.00%)  /x/y.go:foo (3x) [/bin/x]
   50,000 ( 5.00%)  /x/y.go:JS_CallInternal [/bin/x]
`
	l := ParseListing(text)
	if l.Total != 1e6 {
		t.Errorf("total = %v", l.Total)
	}
	if l.Fn["JS_CallInternal"] != 900000 {
		t.Errorf("the largest entry of a name wins: %v", l.Fn)
	}
	if l.Fn["github.com/liustack/quickjs/internal/cengine.callEngine"] != 400000 {
		t.Errorf("fns = %v", l.Fn)
	}
	if _, ok := l.Fn["rec'2"]; ok || len(l.Fn) != 2 {
		t.Errorf("recursive entries are counted: %v", l.Fn)
	}
}

func TestDiffCancelsLoading(t *testing.T) {
	small := ParseListing(listing(1_000_000, map[string]int{"/a.go:render": 400_000, "/a.go:load": 500_000}))
	big := ParseListing(listing(3_000_000, map[string]int{"/a.go:render": 2_400_000, "/a.go:load": 500_000}))
	d := Diff(10, small, big)
	if d.Total != 200_000 || d.Fn["render"] != 200_000 || d.Fn["load"] != 0 {
		t.Errorf("diff = %+v", d)
	}
	var b bytes.Buffer
	WriteDiff(&b, d, nil)
	if !strings.Contains(b.String(), "render") || strings.Contains(b.String(), "load ") {
		t.Errorf("table:\n%s", b.String())
	}
	e := PerRender{Total: 190_000, Fn: map[string]float64{"render": 190_000}}
	b.Reset()
	WriteDiff(&b, d, &e)
	if !strings.Contains(b.String(), "-5.00%") {
		t.Errorf("pair table lacks the change:\n%s", b.String())
	}
}

func TestCGOptions(t *testing.T) {
	ok := CGOptions{Bins: []string{"a"}, Small: 2, Big: 12, Args: []string{"-test.benchtime={N}x"}, Out: "o", Toggle: "pkg.RenderVM", ValgrindRoot: "/vg"}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	prog, env := ok.ValgrindCmd()
	if prog != filepath.FromSlash("/vg/usr/bin/valgrind") || env[0] != "VALGRIND_LIB="+filepath.FromSlash("/vg/usr/libexec/valgrind") {
		t.Errorf("%s %v", prog, env)
	}
	got := ok.CGArgs("/bin/x", "/o/x.cg", 12)
	want := []string{"--tool=callgrind", "--vgdb=no", "--callgrind-out-file=/o/x.cg", "--toggle-collect=pkg.RenderVM", "/bin/x", "-test.benchtime=12x"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v", got)
	}
	for name, mod := range map[string]func(*CGOptions){
		"no bins":   func(o *CGOptions) { o.Bins = nil },
		"3 bins":    func(o *CGOptions) { o.Bins = []string{"a", "b", "c"} },
		"big":       func(o *CGOptions) { o.Big = 2 },
		"no {N}":    func(o *CGOptions) { o.Args = []string{"x"} },
		"no out":    func(o *CGOptions) { o.Out = "" },
		"neg small": func(o *CGOptions) { o.Small = -1 },
	} {
		o := ok
		mod(&o)
		if o.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	plain := CGOptions{}
	if p, e := plain.ValgrindCmd(); p != "valgrind" || e != nil {
		t.Errorf("%s %v", p, e)
	}
}

func TestCGRunsEachBinaryTwice(t *testing.T) {
	dir := t.TempDir()
	var cmds []string
	run := func(ctx context.Context, c measure.Cmd, _ io.Writer) measure.Result {
		cmds = append(cmds, filepath.Base(c.Name)+" "+strings.Join(c.Args, " "))
		if filepath.Base(c.Name) == "callgrind_annotate" {
			big := strings.Contains(c.Args[len(c.Args)-1], "-12.")
			total, fn := 1_000_000, 400_000
			if big {
				total, fn = 3_000_000, 2_400_000
			}
			return measure.Result{Out: listing(total, map[string]int{"/a.go:render": fn})}
		}
		return measure.Result{}
	}
	var out bytes.Buffer
	err := CG(context.Background(), CGOptions{Bins: []string{"x"}, Small: 2, Big: 12, Args: []string{"-n={N}"}, Out: dir}, run, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 4 || !strings.Contains(cmds[0], "-n=2") || !strings.Contains(cmds[2], "-n=12") {
		t.Errorf("commands: %v", cmds)
	}
	if !strings.Contains(out.String(), "per render") || !strings.Contains(out.String(), "200000") {
		t.Errorf("output:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "0-x-12.incl")); err != nil {
		t.Error(err)
	}
}

const cgFile = `version: 1
creator: callgrind-3.25.1
positions: line
events: Ir Bc Bcm
fn=github.com/liustack/quickjs/internal/cengine.JS_CallInternal_block.abi0
0 100 10 2
0 50 5 1
cfn=github.com/liustack/quickjs/internal/cengine.js_malloc.abi0
calls=3 0
0 400 40 4
fn=github.com/liustack/quickjs/internal/cengine.js_malloc.abi0
0 400 40 4
fn=main.f'2
0 10 1 0
totals: 560 56 7
`

func TestParseProfileSumsSelfCostAndSkipsCalls(t *testing.T) {
	p, err := ParseProfile(strings.NewReader(cgFile))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(p.Events) != "[Ir Bc Bcm]" || fmt.Sprint(p.Total) != "[560 56 7]" {
		t.Errorf("events %v total %v", p.Events, p.Total)
	}
	if fmt.Sprint(p.fnRow("JS_CallInternal_block")) != "[150 15 3]" || fmt.Sprint(p.Fn["main.f"]) != "[10 1 0]" {
		t.Errorf("fns %v", p.Fn)
	}
	q := p.Scale(10)
	if q.Total[0] != 56 || q.fnRow("js_malloc")[1] != 4 {
		t.Errorf("scaled %v", q)
	}
	// The file's totals must match the sum of the functions.
	if _, err := ParseProfile(strings.NewReader(strings.Replace(cgFile, "totals: 560", "totals: 561", 1))); err == nil {
		t.Error("totals that do not add up accepted")
	}
	if _, err := ParseProfile(strings.NewReader("fn=(1) x\n")); err == nil {
		t.Error("compressed names accepted")
	}
	var b strings.Builder
	WriteProfiles(&b, "JS_CallInternal_block", p, &q)
	if !strings.Contains(b.String(), "| JS_CallInternal_block (own code) | Bcm | 3 | 0 | -90.00% |") {
		t.Errorf("table:\n%s", b.String())
	}
}

func TestCGWindowRunsEachBinaryOnce(t *testing.T) {
	out := t.TempDir()
	var cmds []measure.Cmd
	run := func(ctx context.Context, c measure.Cmd, _ io.Writer) measure.Result {
		cmds = append(cmds, c)
		for _, a := range c.Args {
			if f, ok := strings.CutPrefix(a, "--callgrind-out-file="); ok {
				os.WriteFile(f, []byte(cgFile), 0o644)
			}
		}
		return measure.Result{}
	}
	o := CGOptions{Bins: []string{"/a", "/b"}, Big: 10, Toggle: "*cgWindow*", Sim: true, Interp: "JS_CallInternal_block", Out: out, Args: []string{"-test.benchtime={N}x"}}
	var b bytes.Buffer
	if err := CG(context.Background(), o, run, &b); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 || !slices.Contains(cmds[0].Args, "--branch-sim=yes") || !slices.Contains(cmds[0].Args, "--compress-pos=no") || !slices.Contains(cmds[0].Args, "-test.benchtime=10x") {
		t.Errorf("commands %v", cmds)
	}
	if !strings.Contains(b.String(), "| whole program | Ir | 56 | 56 | +0.00% |") {
		t.Errorf("output:\n%s", b.String())
	}
	if _, err := os.Stat(filepath.Join(out, "0-a-10.cg")); err != nil {
		t.Error("the raw file is not kept")
	}
}

func TestCGWindowReadsTheDumpOfTheProgram(t *testing.T) {
	out := t.TempDir()
	run := func(ctx context.Context, c measure.Cmd, _ io.Writer) measure.Result {
		for _, a := range c.Args {
			if f, ok := strings.CutPrefix(a, "--callgrind-out-file="); ok {
				// the final file has the whole run, the dump only the window
				os.WriteFile(f, []byte(strings.Replace(cgFile, "0 10 1 0\ntotals: 560 56 7", "0 1010 1 0\ntotals: 1560 56 7", 1)), 0o644)
				os.WriteFile(f+".1", []byte(cgFile), 0o644)
			}
			if strings.HasPrefix(a, "--toggle-collect") {
				t.Errorf("toggle with dumps: %v", c.Args)
			}
		}
		return measure.Result{}
	}
	o := CGOptions{Bins: []string{"/a"}, Big: 10, Dumps: true, Interp: "JS_CallInternal_block", Out: out, Args: []string{"-test.benchtime={N}x"}}
	var b bytes.Buffer
	if err := CG(context.Background(), o, run, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "| whole program | Ir | 56 |") {
		t.Errorf("output:\n%s", b.String())
	}
}
