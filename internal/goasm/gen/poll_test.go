// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCheckPollOffset: the counter must be a whole word inside the region.
func TestCheckPollOffset(t *testing.T) {
	for _, c := range []struct {
		off, log2 int
		ok        bool
	}{
		{PollCounterOffset, 25, true},
		{0, 16, true},
		{1<<16 - 8, 16, true},
		{1 << 16, 16, false},
		{1<<16 - 4, 16, false},
		{-8, 25, false},
		{388, 25, false},
	} {
		if err := CheckPollOffset(c.off, c.log2); (err == nil) != c.ok {
			t.Errorf("offset %d in a region of 2^%d: %v", c.off, c.log2, err)
		}
	}
}

// TestCheckDispatchCopies: the indirect jumps of llc's text are counted, a
// count below the least fails and names the llc options, and a function with
// a table but no least number fails.
func TestCheckDispatchCopies(t *testing.T) {
	old := DispatchCopies
	defer func() { DispatchCopies = old }()
	DispatchCopies = map[string]int{"interp": 3}
	text := func(n int) []string {
		ls := []string{"\tmovq\t(%rdi), %rax", "\tjmp\t.LBB0_4", "\tcallq\t*%rax"}
		for i := 0; i < n; i++ {
			ls = append(ls, "\tjmpq\t*%rcx", "\taddq\t$8, %rsp")
		}
		return ls
	}
	if n, err := CheckDispatchCopies("interp", text(3)); err != nil || n != 3 {
		t.Fatalf("3 copies: %d, %v", n, err)
	}
	if n, err := CheckDispatchCopies("interp", text(2)); err == nil || n != 2 || !strings.Contains(err.Error(), "-tail-dup-pred-size") {
		t.Fatalf("2 copies: %d, %v", n, err)
	}
	if _, err := CheckDispatchCopies("other", text(5)); err == nil || !strings.Contains(err.Error(), "DispatchCopies") {
		t.Fatalf("a function with no least number: %v", err)
	}
}

// pollIR has one function for each kind of cycle the pass treats apart, and for
// each place a loop keeps the count. nest has a loop around one that runs in
// chunks. interp is an interpreter: its dispatch is a computed goto through
// ops, and one of its handlers calls a function that ticks.
const pollIR = `declare void @llvm.memcpy.p0.p0.i64(ptr, ptr, i64, i1)

@ops = internal constant [2 x ptr] [ptr blockaddress(@interp, %inc), ptr blockaddress(@interp, %done)]

define void @ext(i64 %x) {
  ret void
}

define i64 @sum(ptr %a, i64 %n) {
entry:
  %z = icmp eq i64 %n, 0
  br i1 %z, label %exit, label %loop
loop:
  %i = phi i64 [ 0, %entry ], [ %i1, %loop ]
  %s = phi i64 [ 0, %entry ], [ %s1, %loop ]
  %p = getelementptr i64, ptr %a, i64 %i
  %v = load i64, ptr %p
  %s1 = add i64 %s, %v
  %i1 = add i64 %i, 1
  %c = icmp ult i64 %i1, %n
  br i1 %c, label %loop, label %exit
exit:
  %r = phi i64 [ 0, %entry ], [ %s1, %loop ]
  ret i64 %r
}

define i64 @short(ptr %a) {
entry:
  br label %loop
loop:
  %i = phi i64 [ 0, %entry ], [ %i1, %loop ]
  %s = phi i64 [ 0, %entry ], [ %s1, %loop ]
  %p = getelementptr i64, ptr %a, i64 %i
  %v = load i64, ptr %p
  %s1 = add i64 %s, %v
  %i1 = add i64 %i, 1
  %c = icmp ult i64 %i1, 8
  br i1 %c, label %loop, label %exit
exit:
  ret i64 %s1
}

define void @calls(i64 %n) {
entry:
  br label %loop
loop:
  %i = phi i64 [ 0, %entry ], [ %i1, %loop ]
  call void @ext(i64 %i)
  %i1 = add i64 %i, 1
  %c = icmp ult i64 %i1, %n
  br i1 %c, label %loop, label %exit
exit:
  ret void
}

define i64 @nest(ptr %a, i64 %n, i64 %m) {
entry:
  br label %outer
outer:
  %j = phi i64 [ 0, %entry ], [ %j1, %olatch ]
  %t = phi i64 [ 0, %entry ], [ %s1, %olatch ]
  br label %inner
inner:
  %i = phi i64 [ 0, %outer ], [ %i1, %inner ]
  %s = phi i64 [ %t, %outer ], [ %s1, %inner ]
  %p = getelementptr i64, ptr %a, i64 %i
  %v = load i64, ptr %p
  %s1 = add i64 %s, %v
  %i1 = add i64 %i, 1
  %c = icmp ult i64 %i1, %m
  br i1 %c, label %inner, label %olatch
olatch:
  %j1 = add i64 %j, 1
  %d = icmp ult i64 %j1, %n
  br i1 %d, label %outer, label %exit
exit:
  ret i64 %s1
}

define void @forever() {
entry:
  br label %loop
loop:
  br label %loop
}

define void @copy(ptr %d, ptr %s, i64 %n) {
  call void @llvm.memcpy.p0.p0.i64(ptr %d, ptr %s, i64 %n, i1 false)
  ret void
}

define void @copysmall(ptr %d, ptr %s) {
  call void @llvm.memcpy.p0.p0.i64(ptr %d, ptr %s, i64 4096, i1 false)
  ret void
}

define i64 @interp(ptr %pc) {
entry:
  br label %dispatch
dispatch:
  %p = phi ptr [ %pc, %entry ], [ %p1, %inc ]
  %n = phi i64 [ 0, %entry ], [ %n1, %inc ]
  %op = load i8, ptr %p
  %p1 = getelementptr i8, ptr %p, i64 1
  %i = zext i8 %op to i64
  %slot = getelementptr inbounds ptr, ptr @ops, i64 %i
  %to = load ptr, ptr %slot
  indirectbr ptr %to, [label %inc, label %done]
inc:
  %n1 = add i64 %n, 1
  call void @calls(i64 %n1)
  br label %dispatch
done:
  ret i64 %n
}
`

// TestPassPolls runs the pass with -shadow-poll on pollIR. A loop scalar
// evolution cannot bound runs in chunks and ticks at the end of each, a loop of
// 8 runs has no tick, a loop that calls a function and one that never ends
// tick, a memcpy of a length that may be long goes through the chunked helper,
// whose tick counts a whole chunk, one of 4 KiB stays, and the dispatch of the
// interpreter turns to its poll label when the count runs out. A loop that runs
// in chunks counts in memory, a loop around one keeps the count in a register
// for both, and so does a loop that calls a function that does not tick. The
// interpreter keeps the count in a register and stores and loads it around its
// call. The pass checks its own output, so it must also succeed, and it must
// stop on the faults -shadow-poll-fault puts in: ticks whose test never holds,
// a dispatch whose index the count does not change, and a count in a register
// that is not stored before a call that may tick, not loaded again after it, or
// not stored where its loop is left. An offset that is not a word inside the
// region stops it too. It needs LLVM 22.1.8 (SLOWJS_LLVM) and the built pass
// (SLOWJS_SHADOW_PASS).
func TestPassPolls(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	dir := t.TempDir()
	ir, outIR := filepath.Join(dir, "f.ll"), filepath.Join(dir, "out.ll")
	if err := os.WriteFile(ir, []byte(pollIR), 0o644); err != nil {
		t.Fatal(err)
	}
	opt := func(offset int, more ...string) (string, error) {
		args := append([]string{"-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp",
			"-shadow-size-log2=25", "-shadow-poll", fmt.Sprintf("-shadow-poll-offset=%d", offset), "-S", "-o", outIR, ir}, more...)
		out, err := exec.Command(filepath.Join(llvm, "opt"), args...).CombinedOutput()
		return string(out), err
	}
	if out, err := opt(PollCounterOffset); err != nil {
		t.Fatalf("pass: %v\n%s", err, out)
	}
	text, err := os.ReadFile(outIR)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]string{}
	for _, f := range strings.Split(string(text), "\ndefine ")[1:] {
		name := f[strings.Index(f, "@")+1 : strings.Index(f, "(")]
		funcs[name] = f[:strings.Index(f, "\n}")]
	}
	polls := func(name string) bool {
		f, ok := funcs[name]
		if !ok {
			t.Fatalf("no function %s in the output", name)
		}
		return strings.Contains(f, "call void @host_poll(")
	}
	for name, want := range map[string]bool{"ext": false, "sum": true, "short": false, "calls": true, "forever": true,
		"nest": true, "copy": false, "copysmall": false, "goasm_chunked_memcpy": true, "interp": true} {
		if got := polls(name); got != want {
			t.Errorf("%s calls host_poll: %v, want %v", name, got, want)
		}
	}
	if !strings.Contains(funcs["sum"], "poll.chunk.end") {
		t.Errorf("sum does not run in chunks:\n%s", funcs["sum"])
	}
	if !strings.Contains(funcs["copy"], "@goasm_chunked_memcpy(") || strings.Contains(funcs["copysmall"], "@goasm_chunked_memcpy(") {
		t.Errorf("only copy goes through the chunked helper:\n%s\n%s", funcs["copy"], funcs["copysmall"])
	}
	if !strings.Contains(funcs["goasm_chunked_memcpy"], fmt.Sprintf(", %d", 65536)) {
		t.Errorf("the tick of the chunked helper does not count a whole chunk:\n%s", funcs["goasm_chunked_memcpy"])
	}
	if !strings.Contains(funcs["interp"], "select i1") {
		t.Errorf("the dispatch does not pick its table entry by the count:\n%s", funcs["interp"])
	}
	for name, want := range map[string][2]int{"sum": {0, 1}, "calls": {1, 0}, "nest": {2, 0}} {
		if reg, mem := tickPlaces(funcs[name]); reg != want[0] || mem != want[1] {
			t.Errorf("%s ticks %d times in a register and %d in memory, want %d and %d:\n%s", name, reg, mem, want[0], want[1], funcs[name])
		}
	}
	if f := funcs["interp"]; !syncedAround(f, "@calls(") {
		t.Errorf("interp does not store the count before its call and load it after:\n%s", f)
	}
	if out, err := opt(PollCounterOffset + 4); err == nil || !strings.Contains(out, "is not a word inside the region") {
		t.Errorf("an offset that is not a word: %v\n%s", err, out)
	}
	for fault, want := range map[string]string{
		"cond":      "poll check: a tick whose branch does not poll when the count runs out",
		"dispatch":  "poll check: a dispatch whose index does not turn to the poll label when the count runs out",
		"callstore": "poll check: a call that may tick while the count is not in memory",
		"callload":  "of a value that is not the count as it stands",
		"exit":      "poll check: a return while the count is not in memory",
	} {
		if out, err := opt(PollCounterOffset, "-shadow-poll-fault="+fault); err == nil || !strings.Contains(out, want) {
			t.Errorf("-shadow-poll-fault=%s: %v\n%s", fault, err, out)
		}
	}
}

// tickPlaces counts the ticks of a function of the pass's output whose count
// is a load of the counter (in memory) and the others (in a register).
func tickPlaces(f string) (reg, mem int) {
	loads := map[string]bool{}
	for _, m := range regexp.MustCompile(`(%[\w.]+) = load i64, ptr %poll\.counter`).FindAllStringSubmatch(f, -1) {
		loads[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`= sub i64 (%[\w.]+), \d+, !goasm\.tick`).FindAllStringSubmatch(f, -1) {
		if loads[m[1]] {
			mem++
		} else {
			reg++
		}
	}
	return reg, mem
}

// syncedAround tells whether the line of f that calls call has a store to the
// counter on the line before it and a load of the counter within the five
// lines after it (the address of the counter comes first).
func syncedAround(f, call string) bool {
	ls := strings.Split(f, "\n")
	for i, l := range ls {
		if !strings.Contains(l, call) || i == 0 || !regexp.MustCompile(`store i64 %[\w.]+, ptr %poll\.counter`).MatchString(ls[i-1]) {
			continue
		}
		for _, n := range ls[i+1 : min(i+6, len(ls))] {
			if strings.Contains(n, "load i64, ptr %poll.counter") {
				return true
			}
		}
	}
	return false
}
