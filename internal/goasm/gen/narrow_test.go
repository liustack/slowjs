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
	"runtime"
	"strings"
	"testing"
)

// TestNarrowArgs runs testdata/narrow/narrow.c natively and through the
// whole generator (cmd/engine) in body and pn mode, plainly and as a poison
// build, and requires the same results: f(void*, void*, uint8_t, uint16_t,
// _Bool, int, long) called directly and through a pointer, g6 with each
// narrow kind after a pointer, the DynBuf-like writers that take one narrow
// value, a host callback with narrow arguments, narrow results, and g6
// called from Go with garbage above each narrow value. The probe then calls
// put16 from Go assembly three times with the same value: plainly, with the
// goroutine's stack guard set so put16's stack check preempts it, and set so
// it grows (and moves) the stack. All three must write the same bytes:
// morestack loads the goroutine into DI, which preserve_none would read a
// narrow argument from. The post-link check must pass on each build but the
// poison ones, and reject the negative build that leaves narrow integers as
// the generator once took them (GEN_KEEP_NARROW): preserve_none passes them
// in other registers than the lowering moves. That build's probe must show
// the fault: right when called plainly, wrong once morestack ran. It needs LLVM 22.1.8
// (SLOWJS_LLVM), the pass (SLOWJS_SHADOW_PASS) and ld.lld (SLOWJS_LLD), on
// linux/amd64.
func TestNarrowArgs(t *testing.T) {
	llvm, pass, lld := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS"), os.Getenv("SLOWJS_LLD")
	if llvm == "" || pass == "" || lld == "" {
		t.Skip("set SLOWJS_LLVM, SLOWJS_SHADOW_PASS and SLOWJS_LLD")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("runs linux/amd64 code")
	}
	src := filepath.Join("testdata", "narrow")
	work := t.TempDir()
	goEnv := append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	try := func(dir string, env []string, name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, append(append([]string{}, goEnv...), env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	tool := func(t *testing.T, dir, name string, args ...string) string {
		t.Helper()
		out, err := try(dir, nil, name, args...)
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", filepath.Base(name), args, err, out)
		}
		return out
	}
	at := func(n string) string { return filepath.Join(work, n) }
	clang := filepath.Join(llvm, "clang")
	plain := []string{"--target=x86_64-linux-gnu", "-O2", "-ffreestanding", "-nostdinc", "-fno-builtin", "-fno-stack-protector",
		"-fno-asynchronous-unwind-tables"}
	tool(t, "", clang, append(append([]string{}, plain...), "-c", "-o", at("narrow.o"), filepath.Join(src, "narrow.c"))...)
	tool(t, "", clang, append(append([]string{}, plain...), "-c", "-o", at("main.o"), filepath.Join(src, "narrow_main.c"))...)
	tool(t, "", lld, "-static", "-e", "_start", "-o", at("native"), at("main.o"), at("narrow.o"))
	native := strings.Split(strings.TrimSpace(tool(t, "", at("native"))), "\n")
	if len(native) != 20 {
		t.Fatalf("the native run printed %d results:\n%s", len(native), strings.Join(native, "\n"))
	}
	// The sample has no undefined behavior: the same run under UBSan, which
	// traps in place of a runtime library, gives the same lines and does not
	// trap (the signed sums it once had overflowed on the third case).
	ubsan := []string{"-fsanitize=undefined", "-fsanitize-trap=undefined"}
	tool(t, "", clang, append(append(append([]string{}, plain...), ubsan...), "-c", "-o", at("narrow-ub.o"), filepath.Join(src, "narrow.c"))...)
	tool(t, "", lld, "-static", "-e", "_start", "-o", at("native-ub"), at("main.o"), at("narrow-ub.o"))
	if ub := strings.Split(strings.TrimSpace(tool(t, "", at("native-ub"))), "\n"); strings.Join(ub, "\n") != strings.Join(native, "\n") {
		t.Fatalf("the run under UBSan printed\n%s\nthe plain native run\n%s", strings.Join(ub, "\n"), strings.Join(native, "\n"))
	}
	// Go calls g6 with each case too, as run does.
	want := strings.Join(append(append([]string{}, native...), native[2], native[5], native[8], native[11]), "\n") + "\n"

	drv := at("engine")
	tool(t, "", "go", "build", "-o", drv, "./cmd/engine")
	c, err := filepath.Abs(filepath.Join(src, "narrow.c"))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []struct {
		mode   string
		extra  []string
		env    []string
		reject string // the post-link check must reject the build with this; only its probe runs
	}{
		{"body", nil, nil, ""},
		{"pn", nil, nil, ""},
		{"body", []string{"-poison"}, nil, ""},
		{"pn", []string{"-poison"}, nil, ""},
		{"pn", nil, []string{"GEN_KEEP_NARROW=1"}, "where llc reads it"},
	} {
		name := b.mode + strings.Join(b.extra, "")
		if len(b.env) > 0 {
			name += "-keep-narrow"
		}
		t.Run(name, func(t *testing.T) {
			mod := at(name)
			args := append([]string{"-src", c, "-llvm", llvm, "-pass", pass, "-mode", b.mode, "-allow", "host_cb,host_narrow",
				"-leaves", "", "-work", at("w-" + name), "-pkg", "nw"}, b.extra...)
			gen := func(t *testing.T, args ...string) (string, error) {
				t.Helper()
				return try("", b.env, drv, args...)
			}
			if out, err := gen(t, append(args, "-out", filepath.Join(mod, "nw"))...); err != nil {
				t.Fatalf("generate: %v\n%s", err, out)
			}
			for name, text := range map[string]string{
				"go.mod":       "module narrow\n\ngo 1.22\n",
				"main.go":      narrowGoMain,
				"nw/host.go":   narrowGoHost,
				"nw/probe.go":  narrowGoProbe,
				"nw/probe_s.s": narrowGoProbeAsm,
			} {
				if err := os.WriteFile(filepath.Join(mod, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if b.reject != "" {
				// The probe shows the fault the check rejects: put16 reads
				// its value from DI, which holds it only until morestack
				// writes the goroutine there. The package of a negative build
				// does not compile without the tag goasmnegative.
				if out, err := try(mod, nil, "go", "build", "-o", "run", "."); err == nil || !strings.Contains(out, "negative_build_of_the_generator") {
					t.Fatalf("the negative build compiled without its tag: %v\n%s", err, out)
				}
				tool(t, mod, "go", "build", "-tags", "goasmnegative", "-o", "run", ".")
				probe := strings.Fields(tool(t, mod, filepath.Join(mod, "run"), "probe"))
				if len(probe) != 3 || probe[0] != "plain:3412" || probe[1] == "preempted:3412" && probe[2] == "moved:3412" {
					t.Fatalf("put16(0x1234) of the negative build wrote %v, want 3412 only when not preempted or moved", probe)
				}
				t.Logf("probe of the negative build: %v", probe)
				tool(t, mod, "go", "build", "-tags", "goasmkeep,goasmnegative", "-o", "keep", ".")
				out, err := gen(t, append(args, "-verify-bin", filepath.Join(mod, "keep"), "-verify-pkg", "narrow/nw", "-verify-all")...)
				if err == nil || !strings.Contains(out, b.reject) {
					t.Fatalf("the post-link check did not reject it with %q: %v\n%s", b.reject, err, out)
				}
				t.Logf("rejected: %s", lastLine(out))
				return
			}
			tool(t, mod, "go", "build", "-o", "run", ".")
			if have := tool(t, mod, filepath.Join(mod, "run")); have != want {
				t.Fatalf("Go runs the generated code to\n%s\nnative code gives\n%s", have, want)
			}
			probe := strings.Fields(tool(t, mod, filepath.Join(mod, "run"), "probe"))
			if len(probe) != 3 || probe[0] != "plain:3412" || probe[1] != "preempted:3412" || probe[2] != "moved:3412" {
				t.Fatalf("put16(0x1234) wrote %v, want plain:3412 preempted:3412 moved:3412", probe)
			}
			if len(b.extra) > 0 {
				return // a poison build is for running only
			}
			tool(t, mod, "go", "build", "-tags", "goasmkeep", "-o", "keep", ".")
			out, err := gen(t, append(args, "-verify-bin", filepath.Join(mod, "keep"), "-verify-pkg", "narrow/nw", "-verify-all")...)
			if err != nil || !strings.Contains(out, "post-link check passed") {
				t.Fatalf("the post-link check did not pass: %v\n%s", err, out)
			}
		})
	}
}

// TestNarrowArgsARM64 compiles testdata/narrow/narrow.c for arm64 in pn
// mode and reads the registers llc gives g6's probe (pass/facts.cpp): g6
// takes a pointer and four narrow integers after the shadow pointer, and
// with the pass's widening they arrive in X20 to X25 in order, the
// registers the arm64 lowering moves argument words in. The negative build
// (GEN_KEEP_NARROW) shows the mistake the widening removes: preserve_none
// leaves the narrow ones to the C rules, which start at W0. It needs LLVM
// 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestNarrowArgsARM64(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	drv := filepath.Join(work, "engine")
	if out, err := exec.Command("go", "build", "-o", drv, "./cmd/engine").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	c, err := filepath.Abs(filepath.Join("testdata", "narrow", "narrow.c"))
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []bool{false, true} {
		w := filepath.Join(work, fmt.Sprint("w", keep))
		cmd := exec.Command(drv, "-arch", "arm64", "-mode", "pn", "-src", c, "-llvm", llvm, "-pass", pass,
			"-allow", "host_cb,host_narrow", "-leaves", "", "-work", w)
		cmd.Env = os.Environ()
		if keep {
			cmd.Env = append(cmd.Env, "GEN_KEEP_NARROW=1")
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generate (keep narrow %v): %v\n%s", keep, err, out)
		}
		b, err := os.ReadFile(filepath.Join(w, "probes.facts"))
		if err != nil {
			t.Fatal(err)
		}
		facts, err := ParseRegFacts(string(b))
		if err != nil {
			t.Fatal(err)
		}
		g6, ok := facts[ProbePrefix+"g6"]
		if !ok {
			t.Fatalf("no probe of g6 in:\n%s", b)
		}
		var regs []string
		for _, r := range g6.Args {
			if strings.HasPrefix(r, "w") {
				r = "x" + r[1:]
			}
			regs = append(regs, r)
		}
		got := strings.Join(regs, ",")
		const want = "x20,x21,x22,x23,x24,x25"
		switch {
		case !keep && got != want:
			t.Fatalf("llc passes g6's arguments in %s, want %s", got, want)
		case keep && !strings.Contains(got, "x0"):
			t.Fatalf("the negative build passes g6's arguments in %s, which shows no narrow one in W0", got)
		}
		t.Logf("keep narrow %v: g6's arguments in %s", keep, strings.Join(g6.Args, ","))
	}
}

// The Go side of TestNarrowArgs: main prints what nw.Run returns, or with
// the argument probe what nw.Probe does.
const narrowGoMain = `package main

import (
	"fmt"
	"os"

	"narrow/nw"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		for _, l := range nw.Probe() {
			fmt.Println(l)
		}
		return
	}
	for _, v := range nw.Run() {
		fmt.Println(v)
	}
}
`

// narrowGoHost runs narrow.c's run on a shadow region and a result buffer
// outside the Go heap, then calls g6 for each case with garbage above every
// narrow value, which g6 must not read. host_cb grows the goroutine stack,
// which copies it, generated frames and all.
const narrowGoHost = `package nw

import (
	"syscall"
	"unsafe"
)

func goasmMissing(i int64) { panic("missing: " + missingNames[i]) }

//go:noinline
func deepen(d int, x int64) int64 {
	var junk [512]int64
	for i := range junk {
		junk[i] = x ^ (0x5a5a5a5a5a5a5a5a + int64(i))
	}
	if d == 0 {
		return junk[3] & 0xff
	}
	return deepen(d-1, x+junk[d&63]) & 0xffff
}

func host_cb(sp, x uintptr) uintptr {
	deepen(100, int64(x))
	return x
}

// host_narrow gets each narrow value in the low bits of its word.
func host_narrow(sp, c, d, e, tag uintptr) uintptr {
	if e&0xff > 1 {
		panic("a bool that is neither 0 nor 1")
	}
	return uintptr(int64(uint8(c))*1000003 + int64(int16(d))*1009 + int64(e&1)*17 + int64(tag))
}

var cases = []struct{ a, b, c, d, e, g, h int64 }{
	{1, 2, 3, 4, 1, 5, 6},
	{0x1000, -1, 0xff, 0xffff, 0, -1, -1},
	{-7, 0x7fffffffffffffff, 0x80, 0x8000, 1, -2147483648, -0x8000000000000000},
	{0, 0, 0x7f, 0x7fff, 0, 2147483647, 0x123456789abcdef},
}

const noise = 0x5a5a5a5a5a5a5a5a

func region() (sp, r uintptr) {
	m, err := syscall.Mmap(-1, 0, 4<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	base := (uintptr(unsafe.Pointer(&m[0])) + 1<<20 - 1) &^ (1<<20 - 1)
	return base + 4096, base + 2<<20
}

func Run() []int64 {
	sp, r := region()
	n := int(run(sp, r, 1))
	out := make([]int64, n)
	for i := range out {
		out[i] = *(*int64)(unsafe.Pointer(r + uintptr(8*i)))
	}
	for _, c := range cases {
		v := g6(sp, uintptr(c.a), uintptr(c.c)|noise&^0xff, uintptr(c.d)|noise&^0xffff, uintptr(c.e)|noise&^0xff,
			uintptr(uint32(c.g))|noise&^0xffffffff)
		out = append(out, int64(v))
	}
	return out
}
`

// narrowGoProbe calls put16(0x1234) through probeCall three times. Each
// call must have gone through morestack where it was forced to.
const narrowGoProbe = `package nw

import (
	"fmt"
	"unsafe"
)

// probeCall calls put16(sp, b, v) with v also in DI. force 1 sets the
// goroutine's stack guard to stackPreempt and 2 to the current SP first,
// so put16's stack check preempts or grows the stack. It returns put16's
// result, how far the stack moved, and the stack guard after the call.
func probeCall(sp, b, v, force uintptr) (ret, moved, guard uintptr)

const stackPreempt = ^uintptr(1313) // runtime's stackPreempt, -1314

func Probe() []string {
	sp, r := region()
	buf, bytes := r+1024, r+2048
	var out []string
	for force, name := range []string{"plain", "preempted", "moved"} {
		*(*[3]uintptr)(unsafe.Pointer(buf)) = [3]uintptr{bytes, 0, 16}
		*(*uint16)(unsafe.Pointer(bytes)) = 0
		ret, moved, guard := probeCall(sp, buf, 0x1234, uintptr(force))
		if uint32(ret) != 0 {
			panic("put16 failed")
		}
		if force == 1 && guard == stackPreempt {
			panic("put16 did not go through morestack")
		}
		if force == 2 && moved == 0 {
			panic("put16 did not move the stack")
		}
		out = append(out, fmt.Sprintf("%s:%02x%02x", name, *(*uint8)(unsafe.Pointer(bytes)), *(*uint8)(unsafe.Pointer(bytes + 1))))
	}
	return out
}
`

// narrowGoProbeAsm is probeCall. The stack guard is the third word of the
// runtime's g, which (TLS) holds on linux/amd64.
const narrowGoProbeAsm = `#include "textflag.h"
#include "funcdata.h"

TEXT ·probeCall(SB), $40-56
	NO_LOCAL_POINTERS
	MOVQ sp+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ b+8(FP), AX
	MOVQ AX, 8(SP)
	MOVQ v+16(FP), DI
	MOVQ DI, 16(SP)
	MOVQ SP, 32(SP)
	MOVQ force+24(FP), AX
	TESTQ AX, AX
	JEQ ready
	MOVQ (TLS), CX
	CMPQ AX, $2
	JEQ grow
	MOVQ $-1314, 16(CX)
	JMP ready
grow:
	MOVQ SP, 16(CX)
ready:
	CALL ·put16(SB)
	MOVQ 24(SP), AX
	MOVQ AX, ret+32(FP)
	MOVQ 32(SP), AX
	SUBQ SP, AX
	MOVQ AX, moved+40(FP)
	MOVQ (TLS), CX
	MOVQ 16(CX), AX
	MOVQ AX, guard+48(FP)
	RET
`
