// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

// x86asm gives a 32-bit displacement zero-extended. The check must compare
// the address the processor uses, which sign-extends it: a frame slot at
// -256(%rbp) read as +4294967040 would match no rewrite, or the wrong one.
func TestDecodeSignExtendsDisplacement(t *testing.T) {
	in, err := decode([]byte{0x48, 0x8b, 0x85, 0x00, 0xff, 0xff, 0xff}) // movq -256(%rbp), %rax
	if err != nil {
		t.Fatal(err)
	}
	if m := in.Args[1].(x86asm.Mem); m.Base != x86asm.RBP || m.Disp != -256 {
		t.Fatalf("decoded %+v, want RBP-256", m)
	}
	// XCHG of a 32-bit register with itself clears its upper half: it is
	// an instruction, not padding.
	if in, _ := decode([]byte{0x87, 0xc0}); isPad(in) {
		t.Fatal("xchg of eax with itself taken for padding")
	}
}

// TestPostLinkCheck runs testdata/check/mini.c through cmd/engine, builds a
// Go binary of the package and runs the post-link check on it, which must
// pass in both call modes. It then builds the package from wrong code or
// data, one fault at a time, each a mistake the lowering or the data plan
// could make, and the check must reject every one. It needs LLVM 22.1.8
// (SLOWJS_LLVM), the built pass (SLOWJS_SHADOW_PASS) and the go tool, on
// linux/amd64.
func TestPostLinkCheck(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("builds and reads a linux/amd64 binary")
	}
	work := t.TempDir()
	goEnv := []string{"GOWORK=off", "CGO_ENABLED=0"}
	run := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), goEnv...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	drv := filepath.Join(work, "engine")
	run("", "go", "build", "-o", drv, "./cmd/engine")
	src, err := filepath.Abs(filepath.Join("testdata", "check", "mini.c"))
	if err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(work, "mod")
	pkgDir := filepath.Join(mod, "mini")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"go.mod":       "module checkmini\n\ngo 1.22\n",
		"main.go":      "package main\n\nimport _ \"checkmini/mini\"\n\nfunc main() {}\n",
		"mini/host.go": "package mini\n\nfunc goasmMissing(i int64) { panic(missingNames[i]) }\n",
	} {
		if err := os.WriteFile(filepath.Join(mod, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(work, "bin")
	gen := func(mode string, env []string, extra ...string) (string, error) {
		args := []string{"-src", src, "-llvm", llvm, "-pass", pass, "-mode", mode, "-keep-going",
			"-leaves", "leafmix", "-work", filepath.Join(work, "w-"+mode), "-pkg", "mini"}
		cmd := exec.Command(drv, append(args, extra...)...)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	emit := func(mode string, env []string) {
		t.Helper()
		if out, err := gen(mode, env, "-out", pkgDir); err != nil {
			t.Fatalf("generate (%s): %v\n%s", mode, err, out)
		}
		run(mod, "go", "build", "-tags", "goasmkeep", "-o", bin, ".")
	}
	check := func(mode string, env []string) (string, error) {
		return gen(mode, env, "-verify-bin", bin, "-verify-pkg", "checkmini/mini", "-verify-all")
	}

	for _, mode := range []string{"pn", "body"} {
		emit(mode, nil)
		out, err := check(mode, nil)
		if err != nil || !strings.Contains(out, "post-link check passed") {
			t.Fatalf("%s: the check rejects correct code: %v\n%s", mode, err, out)
		}
		for _, want := range []string{"checked: 1 stubs", "checked: 1 register-entry leaves", "body data objects", "engine_rodata objects"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s: %q missing from the report:\n%s", mode, want, out)
			}
		}
	}

	// Faults in the lowered text, built the same way the lowering's output
	// is (the package stays in body mode from here).
	b, err := os.ReadFile(filepath.Join(work, "w-body", "lowered.s"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	var recs []FuncRecord
	if b, err := os.ReadFile(filepath.Join(work, "w-body", "records.json")); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	rec := func(fn string) FuncRecord {
		for _, r := range recs {
			if r.Name == fn {
				return r
			}
		}
		t.Fatalf("no record of %s", fn)
		return FuncRecord{}
	}
	// find returns the index of the first line of fn's text, at or after
	// from (an index, or -1 for the function's start), that matches re.
	find := func(fn, re string, from int) int {
		t.Helper()
		start, end := -1, len(lines)
		for i, l := range lines {
			if l == fn+":" {
				start = i
			}
			if start >= 0 && strings.HasPrefix(l, ".size "+fn+",") {
				end = i
				break
			}
		}
		if start < 0 {
			t.Fatalf("%s not in the lowered text", fn)
		}
		if from < start {
			from = start
		}
		r := regexp.MustCompile(re)
		for i := from; i < end; i++ {
			if r.MatchString(lines[i]) {
				return i
			}
		}
		t.Fatalf("no line of %s matches %s", fn, re)
		return 0
	}
	replace := func(i int, old, new string) string {
		t.Helper()
		if !strings.Contains(lines[i], old) {
			t.Fatalf("line %d %q has no %q", i+1, lines[i], old)
		}
		return fmt.Sprintf("%d\x1f%s", i+1, strings.Replace(lines[i], old, new, 1))
	}
	drop := func(i int) string { return strconv.Itoa(i + 1) }

	misread := find("add3", `\$1234567,`, -1)
	sp := rec("spill")
	var frameLine int
	var frameDisp string
	for i := find("spill", `call`, -1); ; i++ {
		i = find("spill", `-?\d+\(%rsp\)`, i)
		d, _ := strconv.Atoi(regexp.MustCompile(`(-?\d+)\(%rsp\)`).FindStringSubmatch(lines[i])[1])
		if d >= sp.LocalBase && d < sp.LocalBase+sp.Local {
			frameLine, frameDisp = i, strconv.Itoa(d)+"(%rsp)"
			break
		}
	}
	firstCall := find("spill", `^\s*call\s+(add3|mul3)$`, -1)
	argMove := firstCall - 1
	for !regexp.MustCompile(`movq\s+%rsi, 8\(%rsp\)`).MatchString(lines[argMove]) {
		if argMove--; argMove < firstCall-12 {
			t.Fatal("no move of the second argument before spill's first call")
		}
	}
	other := map[string]string{"add3": "mul3", "mul3": "add3"}
	callee := strings.Fields(lines[firstCall])[1]
	dropped := find("add3", `^\s*(leaq|addq|imulq|subq)\s+[^$]*$`, -1)
	blobRef := find("name", `__goasm_rodata\+\d+\(%rip\)`, -1)
	blobOff := regexp.MustCompile(`__goasm_rodata\+(\d+)`).FindStringSubmatch(lines[blobRef])[1]
	n, _ := strconv.Atoi(blobOff)
	cpRef := find("scale", `\.LCPI\w+\(%rip\)`, -1)
	cpLabel := regexp.MustCompile(`(\.LCPI\w+)\(%rip\)`).FindStringSubmatch(lines[cpRef])[1]
	entry := find("pick", `^\s*\.long\s+\.LBB\w+-\.LJTI\w+$`, -1)
	reEntry := regexp.MustCompile(`\.long\s+(\.LBB\w+)-`)
	target := reEntry.FindStringSubmatch(lines[entry])[1]
	otherEntry := entry + 1
	for reEntry.FindStringSubmatch(lines[otherEntry])[1] == target {
		otherEntry++
	}
	pool := find("scale", `^\s*\.quad\s+\S+$`, find("scale", `^\s*scale\.const:`, -1))

	for _, c := range []struct{ name, edit, want string }{
		{"a misread operand", replace(misread, "1234567", "1234568"), "generated"},
		{"a wrong frame displacement", replace(frameLine, frameDisp, regexp.MustCompile(`^-?\d+`).ReplaceAllStringFunc(frameDisp, func(s string) string {
			d, _ := strconv.Atoi(s)
			return strconv.Itoa(d + 8)
		})), "generated"},
		{"a missing argument move", drop(argMove), "argument word 1"},
		{"a wrong argument move", replace(argMove, "%rsi", "%rdx"), "argument word 1"},
		{"a call to the wrong target", replace(firstCall, callee, other[callee]), "is not a call to " + callee},
		{"a dropped instruction", drop(dropped), "generated"},
		{"a reference moved inside engine_rodata", replace(blobRef, "__goasm_rodata+"+blobOff, "__goasm_rodata+"+strconv.Itoa(n+8)), "should point to"},
		{"a constant read from the wrong place", replace(cpRef, cpLabel+"(%rip)", cpLabel+"+8(%rip)"), "should point to"},
		{"a wrong jump table entry", replace(entry, target, reEntry.FindStringSubmatch(lines[otherEntry])[1]), "entry 0 reaches"},
		{"a wrong constant", fmt.Sprintf("%d\x1f\t.quad 0x1234", pool+1), "in llc's object"},
	} {
		env := []string{"GEN_EDIT_LOWERED=" + c.edit}
		emit("body", env)
		out, err := check("body", env)
		if err == nil || !strings.Contains(out, c.want) {
			t.Errorf("%s: the check did not reject it with %q: %v\n%s", c.name, c.want, err, out)
			continue
		}
		t.Logf("%s: %s", c.name, lastLine(out))
	}

	// Faults in the emitted Go files: the read-only data and a stub. name's
	// LEA of engine_rodata is its table of relative offsets: flip one byte
	// of its first entry.
	emit("body", nil)
	word := n &^ 7
	flip := func(m string) string {
		h := m[len(m)-16:]
		v, _ := strconv.ParseUint(h, 16, 64)
		return m[:len(m)-16] + fmt.Sprintf("%016x", v^1<<(8*(n-word)))
	}
	sub := func(repl string) func(string) string {
		return func(m string) string { return repl }
	}
	for _, c := range []struct {
		name, file, re string
		edit           func(string) string
		want           string
	}{
		{"a wrong data byte", "engine_rodata_amd64.s", `DATA ·engine_rodata\+0\(SB\)/8, \$0x[0-9a-f]{16}`, flip0, "in llc's object"},
		{"a wrong relative entry", "engine_rodata_amd64.s", fmt.Sprintf(`DATA ·engine_rodata\+%d\(SB\)/8, \$0x[0-9a-f]{16}`, word), flip, "the difference is"},
		{"a wrong pointer", "engine_rodata_amd64.s", `\$·engine_rodata(\+\d+)?\(SB\)`, sub("$·engine_rodata+101(SB)"), "the pointer"},
		{"a pointer to the wrong function", "engine_rodata_amd64.s", `\$·add3\(SB\)`, sub("$·mul3(SB)"), "the pointer"},
		{"a stub with the wrong index", "engine_stubs_amd64.s", `MOVQ \$0, 0\(SP\)`, sub("MOVQ $1, 0(SP)"), "index"},
	} {
		path := filepath.Join(pkgDir, c.file)
		orig, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		r := regexp.MustCompile(c.re)
		loc := r.FindIndex(orig)
		if loc == nil {
			t.Fatalf("%s: no %s in %s", c.name, c.re, c.file)
		}
		bad := string(orig[:loc[0]]) + c.edit(string(orig[loc[0]:loc[1]])) + string(orig[loc[1]:])
		if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		run(mod, "go", "build", "-tags", "goasmkeep", "-o", bin, ".")
		out, err := check("body", nil)
		if err == nil || !strings.Contains(out, c.want) {
			t.Errorf("%s: the check did not reject it with %q: %v\n%s", c.name, c.want, err, out)
		} else {
			t.Logf("%s: %s", c.name, lastLine(out))
		}
		if err := os.WriteFile(path, orig, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// flip0 changes the lowest byte of a DATA word.
func flip0(m string) string {
	v, _ := strconv.ParseUint(m[len(m)-16:], 16, 64)
	return m[:len(m)-16] + fmt.Sprintf("%016x", v^0x5a)
}

func lastLine(s string) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	return ls[len(ls)-1]
}
