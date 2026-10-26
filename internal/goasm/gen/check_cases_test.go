// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The post-link check's cases from hand-written llc code: each case lowers
// a few functions written the way llc writes them, through the same
// lowering, re-assembly, emit and Go link as the whole engine, and runs the
// check on the binary. A case with a fault changes one thing on the way (the
// text the lowering reads, the lowered text, a record, what the check is
// told) and the check must reject it for the expected reason.

// ckFn wraps a function body the way llc writes a function.
func ckFn(name string, n int, body string) string {
	return fmt.Sprintf("\t.globl\t%s\n\t.p2align\t4\n\t.type\t%s,@function\n%s:\n%s.Lfunc_end%d:\n\t.size\t%s, .Lfunc_end%d-%s\n",
		name, name, name, body, n, name, n, name)
}

// ckMarker is the indirect-call marker the pass leaves in llc's text.
func ckMarker(code string) string { return "\t#APP\n\t# goasm.icall " + code + "\n\t#NO_APP\n" }

// ckBody is llc's text of the body-mode functions: two leaves that save
// callee-saved registers, a frameless function, one that keeps values
// across calls in a register and a local, one with constants and 16-byte
// frame and heap accesses, two register calls of different signatures in a
// row, a call reached by markers from both sides of a branch, and a call
// through memory.
var ckBody = "\t.text\n\t.file\t\"ck.c\"\n" +
	ckFn("lf", 0, "\tpushq\t%rbx\n\tmovq\t%rdi, %rbx\n\tleaq\t(%rbx,%rsi), %rax\n\tpopq\t%rbx\n\tretq\n") +
	ckFn("lf2", 1, "\tpushq\t%r12\n\tpushq\t%rbx\n\tmovq\t%rdi, %rbx\n\tmovq\t%rsi, %r12\n\tleaq\t(%rbx,%r12), %rax\n\tpopq\t%rbx\n\tpopq\t%r12\n\tretq\n") +
	ckFn("h", 2, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tleaq\t(%rdi,%rsi), %rax\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("g", 3, `	pushq	%rbp
	movq	%rsp, %rbp
	pushq	%rbx
	subq	$24, %rsp
	movq	%rsi, %rbx
	movq	%rdi, -24(%rbp)
	testq	%rdi, %rdi
	je	.LBB3_2
	callq	h
	addq	%rax, %rbx
.LBB3_2:
	movq	-24(%rbp), %rdi
	movq	%rbx, %rsi
	callq	lf
	addq	%rbx, %rax
	addq	$24, %rsp
	popq	%rbx
	popq	%rbp
	retq
`) +
	"\t.section\t.rodata.cst8,\"aM\",@progbits,8\n\t.p2align\t3, 0x0\n.LCPI4_0:\n\t.quad\t0x3ff8000000000000\n" +
	".LCPI4_1:\n\t.quad\t0x3fd0000000000000\n\t.text\n" +
	ckFn("fp", 4, `	pushq	%rbp
	movq	%rsp, %rbp
	subq	$32, %rsp
	mulsd	.LCPI4_0(%rip), %xmm0
	addsd	.LCPI4_1(%rip), %xmm0
	movaps	%xmm0, -32(%rbp)
	movaps	(%rdi), %xmm1
	addsd	-32(%rbp), %xmm1
	movapd	%xmm1, %xmm0
	addq	$32, %rsp
	popq	%rbp
	retq
`) +
	ckFn("ic", 5, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tpushq\t%r14\n\tpushq\t%rbx\n\tmovq\t%rsi, %rbx\n\tmovq\t%rdx, %r14\n"+
		"\tmovq\t%rdi, %rax\n\tmovq\t%rdx, %rdi\n\tmovq\t%rdx, %rsi\n"+ckMarker("ii>i")+"\tcallq\t*%rax\n"+
		"\tmovq\t%rax, %rdi\n"+ckMarker("i>i")+"\tcallq\t*%rbx\n\taddq\t%r14, %rax\n\tpopq\t%rbx\n\tpopq\t%r14\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("jn", 6, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tmovq\t%rdi, %rax\n\ttestq\t%rsi, %rsi\n\tje\t.LBB6_2\n"+
		"\tmovq\t%rdx, %rdi\n"+ckMarker("i>i")+"\tjmp\t.LBB6_3\n.LBB6_2:\n\tleaq\t1(%rdx), %rdi\n"+ckMarker("i>i")+
		".LBB6_3:\n\tcallq\t*%rax\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("mc", 7, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tmovq\t%rdi, %rax\n\tmovq\t%rsi, %rdi\n"+ckMarker("i>i")+
		"\tcallq\t*8(%rax)\n\tpopq\t%rbp\n\tretq\n")

var ckBodySigs = map[string]string{"lf": "ii>i", "lf2": "ii>i", "h": "ii>i", "g": "ii>i", "fp": "id>d",
	"ic": "iii>i", "jn": "iii>i", "mc": "ii>i"}

// ckPN is llc's text of preserve_none functions: arguments in R12, R13 and
// on, nothing callee-saved.
var ckPN = "\t.text\n\t.file\t\"ck.c\"\n" +
	ckFn("ph", 0, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tleaq\t(%r12,%r13), %rax\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("pg", 1, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tsubq\t$16, %rsp\n\tmovq\t%r13, -8(%rbp)\n\tcallq\tph\n"+
		"\taddq\t-8(%rbp), %rax\n\taddq\t$16, %rsp\n\tpopq\t%rbp\n\tretq\n")

var ckPNSigs = map[string]string{"ph": "ii>i", "pg": "ii>i"}

type ckCase struct {
	name   string
	pn     bool
	input  func(string) string         // the text the lowering reads
	as     map[string]string           // lower a function with this signature code
	callee map[string]string           // the callee signatures the lowering sees
	lower  func(string, []Meta) string // the lowered text
	metas  func([]Meta)                // the records and metas, before emit
	edit   func(*EngineCheck)          // what the check is told
	calls  func(*testing.T, []Meta)    // what the lowering recorded
	want   string                      // the reason the check rejects it; "" passes
}

func TestCheckCases(t *testing.T) {
	llvm := os.Getenv("SLOWJS_LLVM")
	if llvm == "" {
		t.Skip("set SLOWJS_LLVM")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("builds and reads a linux/amd64 binary")
	}
	// The helpers below edit fixed text: a miss is a mistake of the case,
	// reported by a panic.
	meta := func(ms []Meta, name string) *Meta {
		for i := range ms {
			if ms[i].Name == name {
				return &ms[i]
			}
		}
		panic("no meta of " + name)
	}
	// part applies f to the text from the line "fn:" to the line starting
	// with end.
	part := func(s, fn, end string, f func(string) string) string {
		a := strings.Index(s, "\n"+fn+":\n")
		b := -1
		if a >= 0 {
			b = strings.Index(s[a:], "\n"+end)
		}
		if b < 0 {
			panic(fn + " not in the text")
		}
		return s[:a] + f(s[a:a+b]) + s[a+b:]
	}
	// sub replaces the first match of re in fn's lowered text.
	sub := func(fn, re, repl string) func(string, []Meta) string {
		return func(s string, _ []Meta) string {
			return part(s, fn, ".size "+fn+",", func(b string) string {
				r := regexp.MustCompile(re)
				loc := r.FindStringSubmatchIndex(b)
				if loc == nil {
					panic(fmt.Sprintf("no %s in %s:\n%s", re, fn, b))
				}
				return b[:loc[0]] + string(r.ExpandString(nil, repl, b, loc)) + b[loc[1]:]
			})
		}
	}
	// llcSub replaces old by new in fn's text of llc, n times.
	llcSub := func(fn, old, new string, n int) func(string) string {
		return func(s string) string {
			return part(s, fn, ".Lfunc_end", func(b string) string {
				if strings.Count(b, old) < n {
					panic(fmt.Sprintf("fewer than %d %q in %s", n, old, fn))
				}
				return strings.Replace(b, old, new, n)
			})
		}
	}
	wantCalls := func(fn string, codes ...string) func(*testing.T, []Meta) {
		return func(t *testing.T, ms []Meta) {
			var got []string
			for _, c := range meta(ms, fn).Rec.Calls {
				got = append(got, sigCodeOf(c.Sig))
			}
			if strings.Join(got, " ") != strings.Join(codes, " ") {
				t.Fatalf("%s's calls were marshalled as %v, want %v", fn, got, codes)
			}
		}
	}
	for _, c := range []ckCase{
		{name: "body mode"},
		{name: "preserve_none", pn: true},

		// Code that would not run as decoded, and the templates.
		{name: "a LOCK NOP inserted", lower: sub("lf", `\nlf:\n`, "\nlf:\n\t.byte 0xf0, 0x90\n"), want: "invalid prefix"},
		{name: "a LOCK on an argument move", lower: sub("g", `movq\s+%rdi, 0\(%rsp\)`, ".byte 0xf0, 0x48, 0x89, 0x3c, 0x24"),
			want: "invalid prefix"},
		{name: "a REP prefix on a register save", lower: sub("g", `movq\s+%rbx, (\d+)\(%rsp\)`, ".byte 0xf3\n\tmovq %rbx, ${1}(%rsp)"),
			want: "is not the save of rbx"},
		{name: "a no-op that is no padding encoding", lower: sub("g", `\n\ttestq`, "\n\t.byte 0x48, 0x90\n\ttestq"),
			want: "none of the padding encodings"},
		{name: "an ignored segment prefix", lower: sub("g", `\n\ttestq`, "\n\t.byte 0x2e\n\ttestq"), want: "ignored prefix"},

		// A record and its output wrong together.
		{name: "a kept register no call saves",
			lower: func(s string, ms []Meta) string {
				r := meta(ms, "g").Rec
				return part(s, "g", ".size g,", func(b string) string {
					return regexp.MustCompile(fmt.Sprintf(`\n\tmovq\s+(%%rbx, %d\(%%rsp\)|%d\(%%rsp\), %%rbx)`, r.SaveBase, r.SaveBase)).ReplaceAllString(b, "")
				})
			},
			metas: func(ms []Meta) { meta(ms, "g").Rec.Saved = nil },
			want:  "the calls do not save it"},
		{name: "locals moved over the saves",
			lower: func(s string, ms []Meta) string {
				// g's one local, -24(%rbp), stored and loaded.
				r := meta(ms, "g").Rec
				d := r.LocalBase + 8*len(r.Pushed) + r.Local - 24
				move := sub("g", fmt.Sprintf(`([ ,])%d\(%%rsp\)`, d), fmt.Sprintf("${1}%d(%%rsp)", d-8))
				return move(move(s, ms), ms)
			},
			metas: func(ms []Meta) { meta(ms, "g").Rec.LocalBase -= 8 },
			want:  "frame parts overlap"},
		{name: "a direct call marshalled with the wrong signature", callee: map[string]string{"h": "i>i"},
			want: "marshalled as"},
		{name: "references to two constants swapped",
			lower: func(s string, ms []Meta) string {
				return part(s, "fp", ".size fp,", func(b string) string {
					return regexp.MustCompile(`(sd\s+)(\S+)\(%rip\)`).ReplaceAllStringFunc(b, func(x string) string {
						return strings.NewReplacer("LCPI4_0", "LCPI4_1", "LCPI4_1", "LCPI4_0").Replace(x)
					})
				})
			},
			metas: func(ms []Meta) {
				r := meta(ms, "fp").Rec
				r.Refs[0].Label, r.Refs[1].Label = r.Refs[1].Label, r.Refs[0].Label
			},
			want: "as declared"},

		// Instructions inserted, swapped, repeated, branches redirected.
		{name: "an instruction inserted", lower: sub("g", `\n\ttestq`, "\n\taddq $$1, %rax\n\ttestq"), want: "generated"},
		{name: "two instructions swapped", lower: sub("g", `(\n\tmovq\s+%rsi, %rbx)(\n\tmovq\s+%rdi, \d+\(%rsp\))`, "$2$1"),
			want: "generated"},
		{name: "an instruction repeated", lower: sub("g", `(\n\taddq\s+%rax, %rbx)`, "$1$1"), want: "generated"},
		{name: "a branch to another block", lower: sub("g", `je\s+\.LBB3_2`, "je .Lret_g"), want: "goes to"},
		{name: "a branch into a call's marshalling",
			lower: func(s string, ms []Meta) string {
				s = sub("g", `(movq\s+%rdi, 0\(%rsp\))`, "$1\n.Lmid:")(s, ms)
				return sub("g", `je\s+\.LBB3_2`, "je .Lmid")(s, ms)
			},
			want: "goes to"},
		{name: "an aligned heap load made unaligned", lower: sub("fp", `movaps\s+\(%rdi\)`, "movups (%rdi)"), want: "generated"},
		{name: "a frame store left aligned", lower: sub("fp", `movups\s+%xmm0`, "movaps %xmm0"), want: "generated"},

		// preserve_none.
		{name: "preserve_none: an argument from the wrong register", pn: true,
			lower: sub("pg", `movq\s+%r13, 8\(%rsp\)`, "movq %r12, 8(%rsp)"), want: "argument word 1"},

		// What the check is told is generated.
		{name: "a function left out", edit: func(c *EngineCheck) {
			var ms []Meta
			for _, m := range c.Metas {
				if m.Name != "h" {
					ms = append(ms, m)
				}
			}
			c.Metas = ms
		}, want: "neither lowered nor stubbed"},
		{name: "a constant pool left out of the plan", edit: func(c *EngineCheck) {
			p := *c.Plan
			p.InBody = map[string][]*DataObj{}
			for fn, objs := range c.Plan.InBody {
				if fn == "fp" {
					objs = objs[1:]
				}
				p.InBody[fn] = objs
			}
			c.Plan = &p
		}, want: "belongs to none of its data objects"},

		// Indirect calls. The record and the output must agree call by
		// call; the markers themselves are the pass's, read by the
		// lowering: a marker swapped in llc's text gives code the check
		// passes (README: trusted input).
		{name: "neighbouring calls' records swapped", metas: func(ms []Meta) {
			r := meta(ms, "ic").Rec
			r.Calls[0].Sig, r.Calls[1].Sig = r.Calls[1].Sig, r.Calls[0].Sig
		}, want: "is not a call through"},
		{name: "a call marshalled with its neighbour's signature", lower: sub("ic", `\n\tmovq\s+%rsi, 8\(%rsp\)`, ""),
			want: "argument word 1"},
		{name: "markers as read", calls: wantCalls("ic", "ii>i", "i>i")},
		{name: "neighbouring markers swapped (trusted)", calls: wantCalls("ic", "i>i", "ii>i"),
			input: func(s string) string {
				s = llcSub("ic", "icall ii>i", "icall @", 1)(s)
				s = llcSub("ic", "icall i>i", "icall ii>i", 1)(s)
				return llcSub("ic", "icall @", "icall i>i", 1)(s)
			}},
		{name: "markers at a join changed together (trusted)", input: llcSub("jn", "icall i>i", "icall ii>i", 2),
			calls: wantCalls("jn", "ii>i")},
		{name: "a call through memory with another marker (trusted)", input: llcSub("mc", "icall i>i", "icall ii>i", 1),
			calls: wantCalls("mc", "ii>i")},
	} {
		t.Run(c.name, func(t *testing.T) {
			ms, err := ckBuild(t, llvm, c)
			if c.calls != nil {
				c.calls(t, ms)
			}
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("the check rejects it: %v", err)
			case c.want != "" && err == nil:
				t.Fatalf("the check passes it, want a rejection with %q", c.want)
			case c.want != "" && !strings.Contains(err.Error(), c.want):
				t.Fatalf("the check rejects it for another reason: %v (want %q)", err, c.want)
			case err != nil:
				t.Logf("rejected: %v", err)
			}
		})
	}
	// A join whose sides bring different markers is the lowering's to
	// refuse: the call has no one signature.
	_, err := ckBuild(t, llvm, ckCase{input: func(s string) string {
		return strings.Replace(s, "icall i>i\n\t#NO_APP\n.LBB6_3", "icall ii>i\n\t#NO_APP\n.LBB6_3", 1)
	}})
	if err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("a join with two markers: %v", err)
	}
}

// sigCodeOf writes a signature as a pass signature code.
func sigCodeOf(s Sig) string {
	letter := map[Type]string{I64: "i", Ptr: "i", F64: "d", JSV: "j", Void: "v"}
	var b strings.Builder
	for _, p := range s.Params {
		b.WriteString(letter[p])
	}
	return b.String() + ">" + letter[s.Ret]
}

// ckBuild lowers the case's functions, builds a Go binary of them and runs
// the check. A lowering error is returned as the result, so a case can
// expect one.
func ckBuild(t *testing.T, llvm string, c ckCase) ([]Meta, error) {
	t.Helper()
	dir := t.TempDir()
	text, codes := ckBody, ckBodySigs
	if c.pn {
		text, codes = ckPN, ckPNSigs
	}
	mc := func(src, obj, text string) {
		t.Helper()
		if err := os.WriteFile(src, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(filepath.Join(llvm, "llvm-mc"), "-filetype=obj", "-triple=x86_64-linux-gnu",
			"--save-temp-labels", "-o", obj, src).CombinedOutput(); err != nil {
			t.Fatalf("llvm-mc: %v\n%s", err, out)
		}
	}
	llc := filepath.Join(dir, "llc.o")
	mc(filepath.Join(dir, "llc.s"), llc, text)
	in := text
	if c.input != nil {
		in = c.input(text)
	}
	unit, err := SplitAsm(in, "#")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]bool{}
	for _, n := range unit.FuncOrder {
		funcs[n] = true
	}
	plan := PlanData(unit, funcs)
	for fn, errs := range plan.Errors {
		t.Fatalf("data plan of %s: %v", fn, errs)
	}
	sig := func(code string) Sig {
		s, err := SigFromCode(code)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	sigs, leaves := map[string]Sig{}, map[string]bool{}
	var specs []FuncSpec
	for _, n := range unit.FuncOrder {
		sigs[n] = sig(codes[n])
		leaves[n] = strings.HasPrefix(n, "lf")
		kind := Unified
		if leaves[n] {
			kind = Register
		}
		code := codes[n]
		if x, ok := c.callee[n]; ok {
			code = x
		}
		specs = append(specs, FuncSpec{Name: n, Sig: sig(code), Kind: kind})
	}
	SetCallees(specs)
	amd := AMD64()
	amd.PreserveNone = c.pn
	env := &LowerEnv{Plan: plan, Funcs: funcs}
	var metas []Meta
	lowered := ".text\n"
	for _, n := range unit.FuncOrder {
		code := codes[n]
		if x, ok := c.as[n]; ok {
			code = x
		}
		spec := FuncSpec{Name: n, Sig: sig(code), Kind: Unified}
		var s string
		var m Meta
		if leaves[n] {
			spec.Kind = Register
			s, m, err = LowerRegister(in, spec)
		} else {
			s, m, err = LowerUnifiedFunc(amd, unit.Funcs[n].Lines, spec, env, "")
		}
		if err != nil {
			return metas, err
		}
		lowered += ".p2align 4\n.globl " + n + "\n.type " + n + ",@function\n" + s + ".size " + n + ", .-" + n + "\n"
		metas = append(metas, m)
	}
	if c.lower != nil {
		lowered = c.lower(lowered, metas)
	}
	low := filepath.Join(dir, "low.o")
	mc(filepath.Join(dir, "low.s"), low, lowered)
	if c.metas != nil {
		c.metas(metas)
	}
	names, err := GoNames(unit.FuncOrder)
	if err != nil {
		t.Fatal(err)
	}
	files, err := EmitEngine(low, EngineEmit{Metas: metas, Names: names, MaxBytes: 1 << 20, Plan: plan, Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	var decls, keep strings.Builder
	goType := map[Type]string{I64: "uintptr", Ptr: "uintptr", F64: "float64"}
	for _, n := range unit.FuncOrder {
		var ps []string
		if !leaves[n] {
			for w, p := range sigs[n].Params {
				if p == JSV {
					ps = append(ps, fmt.Sprintf("a%d, b%d uintptr", w, w))
				} else {
					ps = append(ps, fmt.Sprintf("a%d %s", w, goType[p]))
				}
			}
		}
		ret := ""
		if !leaves[n] {
			switch r := sigs[n].Ret; r {
			case JSV:
				ret = " (uintptr, uintptr)"
			case Void:
			default:
				ret = " " + goType[r]
			}
		}
		fmt.Fprintf(&decls, "func %s(%s)%s\n", names[n], strings.Join(ps, ", "), ret)
		fmt.Fprintf(&keep, "%s, ", names[n])
	}
	files["main.go"] = "package main\n\n" + decls.String() + "\nvar keep = []any{" + keep.String() +
		"}\n\nfunc main() {\n\tfor _, f := range keep {\n\t\tprintln(f != nil)\n\t}\n}\n"
	files["go.mod"] = "module ck\n\ngo 1.22\n"
	mod := filepath.Join(dir, "mod")
	if err := os.Mkdir(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, s := range files {
		if err := os.WriteFile(filepath.Join(mod, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(dir, "bin")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	ec := EngineCheck{LLC: llc, Lowered: low, Bin: bin, Pkg: "main", Names: names, Unit: unit, Plan: plan,
		Metas: metas, Sigs: sigs, Leaves: leaves, All: true}
	if c.edit != nil {
		c.edit(&ec)
	}
	_, err = CheckEngine(ec)
	return metas, err
}
