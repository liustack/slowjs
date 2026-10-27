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
	"strconv"
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

// ckLabel is the symbol llc writes at an indirect call: the n-th of fn,
// with the type id of the signature code LLVM bound to it.
func ckLabel(fn string, n int, code string) string {
	id, ok := ICallID(code)
	if !ok {
		panic("no type id for " + code)
	}
	return fmt.Sprintf("%s%s.%d.%d:\n", ICallPrefix, fn, n, id)
}

// ckBody is llc's text of the body-mode functions: two leaves that save
// callee-saved registers, a frameless function, one that keeps values
// across calls in a register and a local, one with constants and 16-byte
// frame and heap accesses, two register calls of different signatures in a
// row, a call after a join of two branches, and a call through a pointer
// loaded from memory.
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
	movups	%xmm0, -32(%rbp)
	movaps	(%rdi), %xmm1
	addsd	-32(%rbp), %xmm1
	movapd	%xmm1, %xmm0
	addq	$32, %rsp
	popq	%rbp
	retq
`) +
	ckFn("ic", 5, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tpushq\t%r14\n\tpushq\t%rbx\n\tmovq\t%rsi, %rbx\n\tmovq\t%rdx, %r14\n"+
		"\tmovq\t%rdi, %rax\n\tmovq\t%rdx, %rdi\n\tmovq\t%rdx, %rsi\n"+ckLabel("ic", 0, "ii>i")+"\tcallq\t*%rax\n"+
		"\tmovq\t%rax, %rdi\n"+ckLabel("ic", 1, "i>i")+"\tcallq\t*%rbx\n\taddq\t%r14, %rax\n\tpopq\t%rbx\n\tpopq\t%r14\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("jn", 6, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tmovq\t%rdi, %rax\n\ttestq\t%rsi, %rsi\n\tje\t.LBB6_2\n"+
		"\tmovq\t%rdx, %rdi\n\tjmp\t.LBB6_3\n.LBB6_2:\n\tleaq\t1(%rdx), %rdi\n"+
		".LBB6_3:\n"+ckLabel("jn", 0, "i>i")+"\tcallq\t*%rax\n\tpopq\t%rbp\n\tretq\n") +
	ckFn("mc", 7, "\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tmovq\t%rdi, %rax\n\tmovq\t%rsi, %rdi\n\tmovq\t8(%rax), %rax\n"+
		ckLabel("mc", 0, "i>i")+"\tcallq\t*%rax\n\tpopq\t%rbp\n\tretq\n")

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
	input  func(string) string          // the text the lowering reads
	llc    func(string) string          // the text llc's object is assembled from
	as     map[string]string            // lower a function with this signature code
	callee map[string]string            // the callee signatures the lowering sees
	lower  func(string, []Meta) string  // the lowered text
	metas  func([]Meta)                 // the records and metas, before emit
	files  func(map[string]string)      // the emitted Go files, before they are built
	poll   string                       // Go source of host_poll in the built program; "" for none
	edit   func(*EngineCheck)           // what the check is told
	facts  func(probes, facts RegFacts) // what llc's facts say
	calls  func(*testing.T, []Meta)     // what the lowering recorded
	want   string                       // the reason the check rejects it; "" passes
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
	cases := []ckCase{
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

		// A leaf's saves must fit its Go frame.
		{name: "a leaf with no Go frame for its save",
			metas: func(ms []Meta) { m := meta(ms, "lf"); m.Frame, m.Rec.Frame = 0, 0 },
			want:  "cannot hold its 1 saved registers"},
		{name: "a leaf frame one slot short",
			metas: func(ms []Meta) { m := meta(ms, "lf2"); m.Frame, m.Rec.Frame = 8, 8 },
			want:  "cannot hold its 2 saved registers"},

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
		{name: "an entry lowered with the wrong signature", as: map[string]string{"h": "i>i"},
			want: "lowered with the signature"},
		{name: "a record of another function",
			metas: func(ms []Meta) { meta(ms, "h").Rec.Name = "g" },
			want:  "the record is of g"},

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
		{name: "a frame store made aligned", lower: sub("fp", `movups\s+%xmm0`, "movaps %xmm0"), want: "generated"},
		{name: "an aligned frame store from llc", llc: llcSub("fp", "movups\t%xmm0", "movaps\t%xmm0", 1),
			want: "may need alignment the Go frame lacks"},

		// preserve_none.
		{name: "preserve_none: an argument from the wrong register", pn: true,
			lower: sub("pg", `movq\s+%r13, 8\(%rsp\)`, "movq %r12, 8(%rsp)"), want: "argument word 1"},
		{name: "preserve_none: the check told the other convention", pn: true,
			edit: func(c *EngineCheck) { c.PreserveNone = false }, want: "preserve_none"},

		// The registers are llc's, not the generator's model: each case
		// says llc decided otherwise than the lowering took it.
		{name: "llc reads an argument word from another register",
			facts: func(p, _ RegFacts) { p[ProbePrefix+"h"].Args = []string{"rdi", "r15d"} },
			want:  "is not the load of argument word 1 into R15"},
		{name: "preserve_none: llc reads an argument word from another register", pn: true,
			facts: func(p, _ RegFacts) { p[ProbePrefix+"ph"].Args = []string{"r12", "edi"} },
			want:  "is not the load of argument word 1 into RDI"},
		{name: "llc's probe with a word too few",
			facts: func(p, _ RegFacts) { p[ProbePrefix+"h"].Args = []string{"rdi"} },
			want:  "has 2 and 1 words"},
		{name: "llc's code reads a register at the entry no load sets",
			facts: func(_, f RegFacts) { f["h"].Entry = append(f["h"].Entry, "r9b") },
			want:  "reads R9 at the entry"},
		{name: "llc returns the result in another register",
			facts: func(p, _ RegFacts) { p[ProbePrefix+"h"].Rets = [][]string{{"xmm0"}} },
			want:  "is not the store of X0"},
		{name: "llc's return gives a register the stores do not take",
			facts: func(_, f RegFacts) { f["h"].Rets[0] = []string{"rax", "rdx"} },
			want:  "which the result stores do not take"},
		{name: "llc passes an indirect call's argument word in another register",
			facts: func(_, f RegFacts) { f["ic"].Calls[0].Args = []string{"rdi", "rcx"} },
			want:  "is not the move of argument word 1 from RCX"},
		{name: "llc passes a call's arguments elsewhere than its callee reads them",
			facts: func(_, f RegFacts) { f["g"].Calls[0].Args = []string{"rsi", "rdi"} },
			want:  "h reads them from [RDI RSI]"},
		{name: "llc takes a call's result from another register",
			facts: func(_, f RegFacts) { f["g"].Calls[0].Defs = []string{"rdx"} },
			want:  "is not the load of the result into RDX"},
		{name: "llc reads a register after a call that nothing sets",
			facts: func(_, f RegFacts) { f["g"].Calls[0].After = append(f["g"].Calls[0].After, "ecx") },
			want:  "reads RCX after the call"},
		{name: "preserve_none: llc reads a register after a call that nothing sets", pn: true,
			facts: func(_, f RegFacts) { f["pg"].Calls[0].After = append(f["pg"].Calls[0].After, "r13") },
			want:  "reads R13 after the call"},
		{name: "llc's facts of a call to another function",
			facts: func(_, f RegFacts) { f["g"].Calls[0].Target = "lf" },
			want:  "llc's facts give this call to lf"},
		{name: "llc's facts of an indirect call with another type id",
			facts: func(_, f RegFacts) { f["ic"].Calls[1].Target = f["ic"].Calls[0].Target },
			want:  "its signature symbol has type id"},
		{name: "llc's facts missing a call",
			facts: func(_, f RegFacts) { f["g"].Calls = f["g"].Calls[:1] },
			want:  "a call llc's facts do not list"},
		{name: "llc's facts with a return too many",
			facts: func(_, f RegFacts) { f["h"].Rets = append(f["h"].Rets, f["h"].Rets[0]) },
			want:  "llc's facts list 0 calls and 2 returns"},
		{name: "no probe of a function", facts: func(p, _ RegFacts) { delete(p, ProbePrefix+"h") },
			want: "no registers"},
		{name: "llc's facts of a leaf with a call it does not make",
			facts: func(_, f RegFacts) { f["lf"].Calls = append(f["lf"].Calls, CallFacts{Target: "h"}) },
			want:  "llc's facts list 1 calls and 1 returns, its object has 0 and 1"},
		{name: "no facts of a leaf", facts: func(_, f RegFacts) { delete(f, "lf") },
			want: "llc wrote no register facts of it"},

		// The Go function the polls call must begin with a stack check.
		{name: "host_poll with a stack check", poll: pollSplit, edit: needPoll},
		{name: "host_poll that is not in the binary", edit: needPoll, want: "is not in the binary"},
		{name: "host_poll marked nosplit", poll: pollWork + "//go:nosplit\n" + pollCall, edit: needPoll, want: "host_poll has no stack check"},
		{name: "host_poll a small leaf the compiler marks nosplit", poll: pollLeaf, edit: needPoll, want: "host_poll has no stack check"},

		// Go's own frame and stack check around a body, and the return.
		{name: "a Go frame bigger than declared", files: tweakText("g", `, \$(\d+)-`, func(n int) int { return n + 16 }),
			want: "Go frame is"},
		{name: "a Go frame smaller than declared", files: tweakText("g", `, \$(\d+)-`, func(n int) int { return n - 8 }),
			want: "Go frame is"},
		// The NOSPLIT list (ChooseNoSplit): g is on it, ic, which calls
		// through a pointer, is not.
		{name: "a function off the NOSPLIT list with no stack check", files: addFlag("ic", "NOSPLIT"),
			want: "not on the NOSPLIT list, but Go does not check the stack"},
		{name: "a function on the NOSPLIT list with a stack check", files: dropFlag("g", "NOSPLIT"),
			want: "on the NOSPLIT list, but Go checks the stack"},
		{name: "a record that says NOSPLIT when the function does not", metas: func(ms []Meta) {
			for i := range ms {
				if ms[i].Name == "ic" {
					ms[i].Rec.NoSplit = true
				}
			}
		}, want: "NOSPLIT true), the function ic"},
		{name: "a return without its result store", lower: sub("h", `\n\tmovq\s+%rax, 24\(%rsp\)`, ""),
			want: "is not the store of"},
		{name: "a result stored in the wrong slot", lower: sub("h", `movq\s+%rax, 24\(%rsp\)`, "movq %rax, 32(%rsp)"),
			want: "is not the store of"},
		{name: "a return that is a plain RET", lower: sub("h", `jmp\s+\.Lret_h`, "retq"),
			want: "is not a jump to the epilogue"},
		{name: "a return that jumps into the body", lower: sub("g", `jmp\s+\.Lret_g`, "jmp .LBB3_2"),
			want: "not to the Go epilogue"},
		{name: "a frameless body with no Go epilogue", files: dropRET("h"), want: "Go epilogue"},
		{name: "a framed body with no Go epilogue", files: dropRET("g"), want: "Go epilogue"},

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
		{name: "a leaf the pass did not take for one", edit: func(c *EngineCheck) { c.Leaves["lf"] = false },
			want: "took it for a leaf"},
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
		// call, and with the signature LLVM bound to each call in llc's
		// object, whatever marker the lowering read.
		{name: "neighbouring calls' records swapped", metas: func(ms []Meta) {
			r := meta(ms, "ic").Rec
			r.Calls[0].Sig, r.Calls[1].Sig = r.Calls[1].Sig, r.Calls[0].Sig
		}, want: "LLVM bound the signature"},
		{name: "a call marshalled with its neighbour's signature", lower: sub("ic", `\n\tmovq\s+%rsi, 8\(%rsp\)`, ""),
			want: "argument word 1"},
		{name: "labels as read", calls: wantCalls("ic", "ii>i", "i>i")},
		{name: "neighbouring labels swapped as the lowering read them", calls: wantCalls("ic", "i>i", "ii>i"),
			input: func(s string) string {
				s = llcSub("ic", ckLabel("ic", 0, "ii>i"), ckLabel("ic", 0, "i>i"), 1)(s)
				return llcSub("ic", ckLabel("ic", 1, "i>i"), ckLabel("ic", 1, "ii>i"), 1)(s)
			}, want: "LLVM bound the signature"},
		{name: "a label at a join changed as the lowering read it", input: llcSub("jn", ckLabel("jn", 0, "i>i"), ckLabel("jn", 0, "ii>i"), 1),
			calls: wantCalls("jn", "ii>i"), want: "LLVM bound the signature"},
		{name: "an indirect call with no signature symbol", llc: llcSub("ic", ckLabel("ic", 1, "i>i"), "", 1),
			want: "no signature symbol from LLVM"},
		{name: "a signature symbol of another function", llc: llcSub("ic", ckLabel("ic", 1, "i>i"), ckLabel("jn", 1, "i>i"), 1),
			want: "names another function"},
		{name: "a signature symbol on a direct call", llc: llcSub("g", "\tcallq\th\n", ckLabel("g", 0, "ii>i")+"\tcallq\th\n", 1),
			want: "to a direct call"},
		{name: "a signature symbol off any call", llc: llcSub("ic", "\taddq\t%r14, %rax\n", ckLabel("ic", 2, "i>i")+"\taddq\t%r14, %rax\n", 1),
			want: "signature symbols in the function"},
		{name: "a call through memory", llc: llcSub("mc", "\tmovq\t8(%rax), %rax\n"+ckLabel("mc", 0, "i>i")+"\tcallq\t*%rax",
			ckLabel("mc", 0, "i>i")+"\tcallq\t*8(%rax)", 1), want: "llc calls through memory"},
	}
	cases = append(cases, forbiddenCases(sub, llcSub)...)
	for _, c := range cases {
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
	if c.llc != nil {
		mc(filepath.Join(dir, "llc.s"), llc, c.llc(text))
	} else {
		mc(filepath.Join(dir, "llc.s"), llc, text)
	}
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
	callbacks := map[string]bool{}
	for _, h := range append(append([]string{}, HostCallbacks...), GeneratorCallbacks...) {
		callbacks[h] = true
	}
	if _, _, err := ChooseNoSplit(metas, nil, nil, plan, unit, callbacks, nil); err != nil {
		t.Fatal(err)
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
		return metas, err
	}
	if c.files != nil {
		c.files(files)
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
	if c.poll != "" {
		keep.WriteString("host_poll, ")
	}
	files["main.go"] = "package main\n\n" + decls.String() + "\n" + c.poll + "\nvar keep = []any{" + keep.String() +
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
	llcText := text
	if c.llc != nil {
		llcText = c.llc(text)
	}
	probes, facts := ckFacts(t, llcText, codes, leaves, c.pn)
	if c.facts != nil {
		c.facts(probes, facts)
	}
	ec := EngineCheck{LLC: llc, Lowered: low, Bin: bin, Pkg: "main", Names: names, Unit: unit, Plan: plan,
		Metas: metas, Sigs: sigs, Leaves: leaves, PreserveNone: c.pn, Probes: probes, Facts: facts, All: true}
	if c.edit != nil {
		c.edit(&ec)
	}
	_, err = CheckEngine(ec)
	return metas, err
}

// ckFacts writes what llc reports of the registers of the case's text
// (pass/facts.cpp): for each unified function a probe with the registers of
// its argument and result words, as the C convention or preserve_none puts
// them for its signature (code), and its calls, returns and live registers
// as its text has them. A case with a fault in them edits these.
func ckFacts(t *testing.T, text string, codes map[string]string, leaves map[string]bool, pn bool) (probes, facts RegFacts) {
	t.Helper()
	regs := func(code string) (args, res []string) {
		s, err := SigFromCode(code)
		if err != nil {
			t.Fatal(err)
		}
		ints := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		if pn {
			ints = []string{"r12", "r13", "r14", "r15", "rdi", "rsi", "rdx", "rcx", "r8", "r9", "r11", "rax"}
		}
		ni, nx := 0, 0
		for _, p := range s.Params {
			for w := 0; w < p.words(); w++ {
				if p == F64 {
					args = append(args, fmt.Sprintf("xmm%d", nx))
					nx++
				} else {
					args = append(args, ints[ni])
					ni++
				}
			}
		}
		switch s.Ret {
		case F64:
			res = []string{"xmm0"}
		case JSV:
			res = []string{"rax", "rdx"}
		case Void:
		default:
			res = []string{"rax"}
		}
		return args, res
	}
	unit, err := SplitAsm(text, "#")
	if err != nil {
		t.Fatal(err)
	}
	probes, facts = RegFacts{}, RegFacts{}
	for _, name := range unit.FuncOrder {
		args, res := regs(codes[name])
		if !leaves[name] {
			probes[ProbePrefix+name] = &FuncFacts{Args: args, Entry: args, Rets: [][]string{res}}
		}
		f := &FuncFacts{Entry: append([]string{}, args...)}
		var pushed []string
		label := ""
		for i, raw := range unit.Funcs[name].Lines {
			l := strings.Join(strings.Fields(clean(raw)), " ")
			switch {
			case i >= 2 && strings.HasPrefix(l, "pushq %") && len(f.Calls) == 0 && strings.Contains("rbx r12 r13 r14 r15", l[7:]):
				pushed = append(pushed, l[7:])
			case strings.HasPrefix(l, ICallPrefix):
				_, _, id, ok := ParseICallSymbol(strings.TrimSuffix(l, ":"))
				if !ok {
					t.Fatalf("bad label %s", l)
				}
				label = fmt.Sprintf("*%d", id)
			case strings.HasPrefix(l, "callq "):
				target := strings.TrimPrefix(l, "callq ")
				code := codes[target]
				if strings.HasPrefix(target, "*") {
					target, label = label, ""
					if target == "" {
						target = "*?"
					} else {
						id, _ := strconv.ParseUint(target[1:], 10, 32)
						if code, err = ICallCode(uint32(id)); err != nil {
							t.Fatal(err)
						}
					}
				}
				label = ""
				var a, r []string
				if code != "" {
					a, r = regs(code)
				}
				f.Calls = append(f.Calls, CallFacts{Target: target, Args: a, Defs: r, After: append(append([]string{}, r...), pushed...)})
			case l == "retq":
				f.Rets = append(f.Rets, res)
			}
		}
		f.Entry = append(f.Entry, pushed...)
		facts[name] = f
	}
	return probes, facts
}

// forbiddenCases put each forbidden instruction into function g three ways,
// and each is refused as the instruction it is, not only as a difference
// from the other side: as bytes in the lowered text (emit refuses them), as
// the first bytes of g's emitted body (the Go binary then holds them, and the
// check refuses them where it decodes the binary), and as text in llc's code
// (the check refuses them where it decodes llc's object).
func forbiddenCases(sub func(fn, re, repl string) func(string, []Meta) string,
	llcSub func(fn, old, new string, n int) func(string) string) []ckCase {
	var out []ckCase
	for _, f := range []struct{ name, bytes, text string }{
		{"syscall", "0f 05", "syscall"},
		{"sysenter", "0f 34", "sysenter"},
		{"int 0x80", "cd 80", "int\t$128"},
		{"rdtsc", "0f 31", "rdtsc"},
		{"rdtscp", "0f 01 f9", "rdtscp"},
		{"rdrand", "48 0f c7 f0", "rdrandq\t%rax"},
		{"rdseed", "48 0f c7 f8", ".byte 0x48, 0x0f, 0xc7, 0xf8"},
		{"cpuid", "0f a2", "cpuid"},
		{"ud2", "0f 0b", "ud2"},
		{"int3", "cc", "int3"},
		{"hlt", "f4", "hlt"},
	} {
		var lit, quad []string
		for _, h := range strings.Fields(f.bytes) {
			lit = append(lit, "0x"+h)
		}
		for len(lit) < 8 {
			lit = append(lit, "0x90")
		}
		for i := 7; i >= 0; i-- {
			quad = append(quad, strings.TrimPrefix(lit[i], "0x"))
		}
		first := regexp.MustCompile(`(?s)(TEXT ·g\(SB\).*?QUAD \$0x)[0-9a-f]{16}`)
		out = append(out,
			ckCase{name: f.name + " in the lowered text", lower: sub("g", `\n\ttestq`, "\n\t.byte "+strings.Join(lit[:len(strings.Fields(f.bytes))], ", ")+"\n\ttestq"),
				want: "forbidden instruction"},
			ckCase{name: f.name + " in the linked code", want: "forbidden instruction",
				files: func(files map[string]string) {
					for name, text := range files {
						if first.MatchString(text) {
							files[name] = first.ReplaceAllString(text, "${1}"+strings.Join(quad, ""))
							return
						}
					}
					panic("no QUAD in g's emitted text")
				}},
			ckCase{name: f.name + " in llc's code", llc: llcSub("g", "\ttestq\t%rdi, %rdi\n", "\t"+f.text+"\n\ttestq\t%rdi, %rdi\n", 1),
				want: "forbidden instruction"})
	}
	return out
}

// needPoll tells the check the engine calls host_poll.
func needPoll(c *EngineCheck) { c.HostPoll = true }

// host_poll for the built program: pollCall calls a function the compiler
// does not inline, so it gets a stack check, and pollLeaf is the small leaf
// the compiler marks NOSPLIT.
const (
	pollWork  = "//go:noinline\nfunc pollWork(x int64) int64 { return x*3 + 1 }\n\n"
	pollCall  = "func host_poll(sp uintptr) int64 { return pollWork(int64(sp)) }\n"
	pollSplit = pollWork + pollCall
	pollLeaf  = "func host_poll(sp uintptr) int64 { return int64(sp) + 1 }\n"
)

// emitted finds the file of the emitted Go assembly that holds fn's TEXT.
func emitted(files map[string]string, fn string) (name string, at int) {
	for n, text := range files {
		if i := strings.Index(text, "TEXT ·"+fn+"(SB)"); i >= 0 {
			return n, i
		}
	}
	panic("no TEXT of " + fn + " in the emitted files")
}

// tweakText changes the number re captures in the TEXT line of fn.
func tweakText(fn, re string, f func(int) int) func(map[string]string) {
	return func(files map[string]string) {
		name, at := emitted(files, fn)
		text := files[name]
		eol := at + strings.Index(text[at:], "\n")
		line := regexp.MustCompile(re).ReplaceAllStringFunc(text[at:eol], func(m string) string {
			n, _ := strconv.Atoi(regexp.MustCompile(`\d+`).FindString(m))
			return strings.Replace(m, strconv.Itoa(n), strconv.Itoa(f(n)), 1)
		})
		files[name] = text[:at] + line + text[eol:]
	}
}

// addFlag puts a TEXT flag on fn: "TEXT ·fn(SB), $N-M" becomes "TEXT ·fn(SB), FLAG, $N-M".
func addFlag(fn, flag string) func(map[string]string) {
	return func(files map[string]string) {
		name, at := emitted(files, fn)
		text := files[name]
		pre := "TEXT ·" + fn + "(SB), "
		files[name] = text[:at] + pre + flag + ", " + text[at+len(pre):]
	}
}

// dropFlag takes flag out of the TEXT line of fn.
func dropFlag(fn, flag string) func(map[string]string) {
	return func(files map[string]string) {
		name, at := emitted(files, fn)
		text := files[name]
		pre := "TEXT ·" + fn + "(SB), " + flag + ", "
		if !strings.HasPrefix(text[at:], pre) {
			panic("the TEXT of " + fn + " does not say " + flag)
		}
		files[name] = text[:at] + "TEXT ·" + fn + "(SB), " + text[at+len(pre):]
	}
}

// dropRET takes the RET that ends fn's emitted text, which is where the Go
// assembler writes its epilogue, and puts a no-op in its place.
func dropRET(fn string) func(map[string]string) {
	return func(files map[string]string) {
		name, at := emitted(files, fn)
		text := files[name]
		end := len(text)
		if i := strings.Index(text[at+1:], "\nTEXT "); i >= 0 {
			end = at + 1 + i
		}
		body := text[at:end]
		i := strings.LastIndex(body, "\tRET")
		if i < 0 {
			panic("no RET in " + fn)
		}
		files[name] = text[:at] + body[:i] + "\tBYTE $0x90" + body[i+len("\tRET"):] + text[end:]
	}
}
