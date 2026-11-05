// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// postLinkSpecialize is PostLinkOptArgs with function specialization forced,
// so that the small fixture gets the clone the engine gets from the optimizer
// on its own (mark_children.specialized.N).
var postLinkSpecialize = strings.Join(append(append([]string{}, PostLinkOptArgs...), "-force-specialization"), " ")

// TestPostLinkOptFor: the optimizer over the linked module is on for amd64
// and arm64, off for any other name, and the arguments are the ones the
// documents name.
func TestPostLinkOptFor(t *testing.T) {
	if !PostLinkOptFor("amd64") || !PostLinkOptFor("arm64") || PostLinkOptFor("riscv64") {
		t.Errorf("PostLinkOptFor: amd64 %v, arm64 %v, riscv64 %v, want true, true and false", PostLinkOptFor("amd64"), PostLinkOptFor("arm64"), PostLinkOptFor("riscv64"))
	}
	want := []string{"-passes=default<O2>", "-inline-threshold=400", "-slp-threshold=1000000"}
	if strings.Join(PostLinkOptArgs, " ") != strings.Join(want, " ") {
		t.Errorf("PostLinkOptArgs = %v, want %v", PostLinkOptArgs, want)
	}
}

// TestPostLinkOptKeepsValueFlags: the second run of the optimizer must not
// undo what quickjs.c asks of clang. The SLP vectorizer is a clang option, not
// an attribute of the functions, so a run with it on joins the two 8-byte
// moves of a JSValue into one 16-byte move again (EngineValueFlags);
// PostLinkOptArgs turn it off. +slow-unaligned-mem-16 is an attribute and
// stays with its function. Needs LLVM (QUICKJS_LLVM).
func TestPostLinkOptKeepsValueFlags(t *testing.T) {
	llvm := os.Getenv("QUICKJS_LLVM")
	if llvm == "" {
		t.Skip("set QUICKJS_LLVM")
	}
	work := t.TempDir()
	ir := filepath.Join(work, "copy.ll")
	args := append(append([]string{}, EngineCFlags...), ValueFlagsFor("amd64", "quickjs")...)
	if out, err := exec.Command(filepath.Join(llvm, "clang"), append(args, "-S", "-emit-llvm", "-o", ir,
		filepath.Join("testdata", "valueflags", "copy.c"))...).CombinedOutput(); err != nil {
		t.Fatalf("clang: %v\n%s", err, out)
	}
	vector := regexp.MustCompile(`<2 x (i64|double)>`)
	opt := func(name string, optArgs []string) string {
		out := filepath.Join(work, name+".ll")
		if err := PostLinkOpt(llvm, optArgs, ir, out); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// Without the threshold the optimizer would vectorize the copies (if it no
	// longer does, the next check shows nothing).
	if !vector.MatchString(opt("slp", []string{"-passes=default<O2>"})) {
		t.Skip("the optimizer no longer joins the moves of a JSValue by itself, so this test shows nothing")
	}
	got := opt("noslp", PostLinkOptArgs)
	if m := vector.FindString(got); m != "" {
		t.Errorf("the optimizer over the linked module made a vector %q of the copy of a JSValue, e.g.\n%s", m, firstLines(got, vector, 4))
	}
	// The target attributes of the file are still on its functions.
	if !strings.Contains(got, "+slow-unaligned-mem-16") {
		t.Errorf("+slow-unaligned-mem-16 is gone from the functions of the file:\n%s", got)
	}
}

// firstLines gives the first n lines of text that re matches.
func firstLines(text string, re *regexp.Regexp, n int) string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if re.MatchString(l) && len(out) < n {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return strings.Join(out, "\n")
}

// TestPostLinkOpt runs the driver over testdata/run/postlink.c and
// postlink_lib.c, two files, with the optimizer over the linked module off,
// on, and on with function specialization forced, and holds every place that
// goes by the name of a function to what the optimizer does:
//   - off: no engine.opt.ll, pl_add is called, walk is a function of its own.
//   - on: pl_add is inlined into run (no call to it), and is still defined,
//     declared to Go and in the keep list. pl_sum, the leaf, is still called
//     and is still a register-entry leaf with no Go declaration. engine.opt.ll
//     exists and the pass read it.
//   - specialized: walk is gone, walk.specialized.1 and .2 are unified
//     functions with Go names (walk_specialized_1), declared and listed to
//     keep, called by pl_inc_sum and pl_dbl_sum, and their code is in the file
//     of walk's group, not in the file for names no file defined.
//
// It does this for amd64 and for arm64 (mode pn), and runs the post-link check
// over the on and spec builds of each, with a binary cross-built for linux. It
// needs LLVM 22.1.8 (QUICKJS_LLVM), the built pass (QUICKJS_SHADOW_PASS) and the
// go tool.
func TestPostLinkOpt(t *testing.T) {
	llvm, pass := os.Getenv("QUICKJS_LLVM"), os.Getenv("QUICKJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set QUICKJS_LLVM and QUICKJS_SHADOW_PASS")
	}
	work := t.TempDir()
	run := driverRunner(t, work, llvm, pass)
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) { postLinkOptArch(t, work, run, llvm, arch) })
	}
}

// postLinkOptArch is TestPostLinkOpt for one architecture. amd64 is the
// driver's default (mode body), arm64 is generated in mode pn. Both are
// generated here and not run: running the sample is TestSampleRuns, on the
// machine of its architecture, but the post-link check reads the binary of
// the module built for arch, which a machine of another architecture can
// make, so it runs on both (the module is built with GOARCH=arch).
func postLinkOptArch(t *testing.T, work string, run func(name, src string, env []string, extra ...string) (string, error), llvm, arch string) {
	at := func(n string) string { return filepath.Join(work, n) }
	src := fixture(t, "run/postlink.c") + "," + fixture(t, "run/postlink_lib.c")
	archArgs := []string{}
	if arch == "arm64" {
		archArgs = []string{"-arch", "arm64", "-mode", "pn"}
	}
	args := map[string][]string{} // the driver arguments of each build, for its post-link check
	build := func(name string, extra ...string) {
		t.Helper()
		args[name] = append(append(append([]string{}, archArgs...), "-leaves", "pl_sum"), extra...)
		if out, err := run(arch+"-"+name, src, nil, args[name]...); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
	// verify builds the generated package of a build into a module for
	// linux/arch with the tag goasmkeep and gives that binary to the post-link
	// check, which must pass over every function, clones included.
	verify := func(name string) {
		t.Helper()
		mod := at("mod-" + arch + "-" + name)
		gen := at("out-" + arch + "-" + name)
		if err := os.MkdirAll(filepath.Join(mod, "nw"), 0o755); err != nil {
			t.Fatal(err)
		}
		files, err := os.ReadDir(gen)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			b, err := os.ReadFile(filepath.Join(gen, f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(mod, "nw", f.Name()), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for f, text := range map[string]string{"go.mod": "module samples\n\ngo 1.22\n", "main.go": sampleGoMain, "nw/host.go": sampleGoHost} {
			if err := os.WriteFile(filepath.Join(mod, f), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		b := exec.Command("go", "build", "-tags", "goasmkeep", "-o", "keep", ".")
		b.Dir = mod
		b.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
		if out, err := b.CombinedOutput(); err != nil {
			t.Fatalf("%s: go build: %v\n%s", name, err, out)
		}
		out, err := run(arch+"-"+name, src, nil, append(append([]string{}, args[name]...),
			"-verify-bin", filepath.Join(mod, "keep"), "-verify-pkg", "samples/nw", "-verify-all")...)
		if err != nil || !strings.Contains(out, "post-link check passed") {
			t.Fatalf("%s: the post-link check did not pass: %v\n%s", name, err, out)
		}
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "checked: ") && strings.HasSuffix(l, " functions") || strings.HasPrefix(l, "functions: ") {
				t.Logf("%s %s: %s", arch, name, l)
			}
		}
	}
	read := func(name, file string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(work, name, file))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	calls := func(name, fn string) []string {
		t.Helper()
		var recs []FuncRecord
		if err := json.Unmarshal([]byte(read("w-"+arch+"-"+name, "records.json")), &recs); err != nil {
			t.Fatal(err)
		}
		for _, r := range recs {
			if r.Name == fn {
				var ts []string
				for _, c := range r.Calls {
					ts = append(ts, c.Target)
				}
				return ts
			}
		}
		t.Fatalf("%s: no record of %s", name, fn)
		return nil
	}
	has := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}
	funcOf := func(name, fn string) bool { // declared to Go, and kept for the post-link check
		decl := read("out-"+arch+"-"+name, "engine_decl_"+arch+".go")
		keep := read("out-"+arch+"-"+name, "engine_keep_"+arch+".go")
		d := regexp.MustCompile(`(?m)^func ` + fn + `\(`).MatchString(decl)
		k := regexp.MustCompile(`(?m)^\t` + fn + `,$`).MatchString(keep)
		if d != k {
			t.Errorf("%s: %s is declared to Go: %v, in the keep list: %v", name, fn, d, k)
		}
		return d
	}
	sig := func(name, fn string) string { // "def unified", "def leaf", "" for no such function
		for _, l := range strings.Split(read("w-"+arch+"-"+name, "pass.txt"), "\n") {
			if f := strings.Fields(l); len(f) >= 5 && f[0] == "SIG" && f[1] == fn {
				return f[3] + " " + f[4]
			}
		}
		return ""
	}
	textIn := func(name, sym string) []string { // the .s files that define the symbol
		files, _ := filepath.Glob(filepath.Join(work, "out-"+arch+"-"+name, "*_"+arch+".s"))
		var in []string
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "TEXT ·"+sym+"(SB)") {
				in = append(in, filepath.Base(f))
			}
		}
		return in
	}

	build("off", "-postlink-opt=off")
	if _, err := os.Stat(at("w-" + arch + "-off/engine.opt.ll")); err == nil {
		t.Error("off: the driver ran the optimizer: engine.opt.ll exists")
	}
	if !has(calls("off", "run"), "pl_add") {
		t.Errorf("off: run does not call pl_add: %v", calls("off", "run"))
	}
	if !funcOf("off", "walk") || !funcOf("off", "pl_add") {
		t.Error("off: walk or pl_add is not a Go function")
	}

	build("on")
	if _, err := os.Stat(at("w-" + arch + "-on/engine.opt.ll")); err != nil {
		t.Errorf("on: no engine.opt.ll: %v", err)
	}
	if has(calls("on", "run"), "pl_add") {
		t.Errorf("on: run still calls pl_add, which the optimizer inlines: %v", calls("on", "run"))
	}
	if !has(calls("on", "run"), "pl_sum") {
		t.Errorf("on: run does not call the leaf pl_sum: %v", calls("on", "run"))
	}
	if !funcOf("on", "pl_add") {
		t.Error("on: pl_add, external and inlined, is no longer a Go function: a caller outside the module could not find it")
	}
	if got := sig("on", "pl_sum"); got != "def leaf" {
		t.Errorf("on: pl_sum is %q, want def leaf", got)
	}
	if funcOf("on", "pl_sum") {
		t.Error("on: the register-entry leaf pl_sum has a Go declaration")
	}
	if !funcOf("on", "walk") {
		t.Error("on: walk is not a Go function (without the forced option the optimizer leaves it)")
	}

	build("spec", "-postlink-opt-args", postLinkSpecialize)
	if funcOf("spec", "walk") || sig("spec", "walk") != "" {
		t.Error("spec: walk is still a function after the optimizer cloned it for both of its calls")
	}
	for _, clone := range []string{"walk.specialized.1", "walk.specialized.2"} {
		if got := sig("spec", clone); got != "def unified" {
			t.Errorf("spec: %s is %q, want def unified", clone, got)
		}
		g := strings.ReplaceAll(clone, ".", "_")
		if !funcOf("spec", g) {
			t.Errorf("spec: %s has no Go function %s", clone, g)
		}
		// The code goes with walk's file (postlink), not with the names no file defined.
		if in := textIn("spec", g); len(in) != 1 || in[0] != "postlink_"+arch+".s" {
			t.Errorf("spec: %s is defined in %v, want only the file of postlink.c", g, in)
		}
	}
	if !has(calls("spec", "pl_inc_sum"), "walk.specialized.1") && !has(calls("spec", "pl_inc_sum"), "walk.specialized.2") {
		t.Errorf("spec: pl_inc_sum calls %v, want a clone of walk", calls("spec", "pl_inc_sum"))
	}
	if got := sig("spec", "pl_sum"); got != "def leaf" {
		t.Errorf("spec: pl_sum is %q, want def leaf", got)
	}
	if !has(calls("spec", "run"), "pl_sum") {
		t.Errorf("spec: run does not call the leaf pl_sum: %v", calls("spec", "run"))
	}

	// The lowering, the relocations and the post-link check over the module
	// the optimizer made, with the inlined function, the leaf and the clones.
	verify("on")
	verify("spec")
}
