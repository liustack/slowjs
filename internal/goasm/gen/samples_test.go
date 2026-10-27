// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSampleRuns runs the C samples of testdata/run natively and through the
// whole generator (cmd/engine), in body and pn mode and as a pn poison build,
// and requires the same lines from each. A sample defines
// `long run(long *r, int one)`, which writes its results to r, returns how
// many, and is called with one = 1 (so the optimizer folds nothing that
// depends on it). main.c prints them natively, a Go host does for the
// generated code. The native build also runs under UBSan, which traps in
// place of a runtime library: a sample has no undefined behavior on any of
// its inputs. The post-link check must pass on the generated code.
//
// Each sample makes one claim:
//   - alias.c: narrow arguments and results reach the callee whole through a
//     global alias in a table of function pointers, and the generator takes
//     the calls the optimizer folds into direct ones.
//   - alias_weak.c: a call of a weak alias, which the optimizer cannot fold,
//     is refused.
//   - narrow_va.c (with -varargs-block): narrow fixed parameters and results
//     of variadic functions, called directly, through a pointer and from a
//     function that forwards its va_list. The pass moves the variable
//     arguments into a block first and widens the narrow ones after, and the
//     two meet in these calls.
//   - trap.c: a __builtin_trap the run does not reach is a call of host_trap in
//     the generated code and no trap instruction, which the post-link check
//     refuses. A second run reaches it and must land in host_trap.
//   - leaf_trap.c: a register-entry leaf that traps is refused.
//   - icalls.c: for each shape of signature, a function called directly and
//     through a table gives the same result, and each result is what native C
//     gives. TestICallSymbols holds each call's id to the type in its source
//     line, but only the run shows that the arguments moved the way that id
//     says: a call that carries another call's signature moves them wrongly.
//
// It needs LLVM 22.1.8 (SLOWJS_LLVM), the pass (SLOWJS_SHADOW_PASS) and ld.lld
// (SLOWJS_LLD), on linux/amd64.
func TestSampleRuns(t *testing.T) {
	llvm, pass, lld := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS"), os.Getenv("SLOWJS_LLD")
	if llvm == "" || pass == "" || lld == "" {
		t.Skip("set SLOWJS_LLVM, SLOWJS_SHADOW_PASS and SLOWJS_LLD")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("runs linux/amd64 code")
	}
	src := filepath.Join("testdata", "run")
	work := t.TempDir()
	goEnv := append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	tool := func(t *testing.T, dir, name string, args ...string) string {
		t.Helper()
		out, err := runIn(dir, goEnv, name, args...)
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", filepath.Base(name), args, err, out)
		}
		return out
	}
	at := func(n string) string { return filepath.Join(work, n) }
	clang := filepath.Join(llvm, "clang")
	plain := []string{"--target=x86_64-linux-gnu", "-O2", "-ffreestanding", "-nostdinc", "-fno-builtin", "-fno-stack-protector",
		"-fno-asynchronous-unwind-tables"}
	ubsan := []string{"-fsanitize=undefined", "-fsanitize-trap=undefined"}
	tool(t, "", clang, append(append([]string{}, plain...), "-c", "-o", at("main.o"), filepath.Join(src, "main.c"))...)
	drv := at("engine")
	tool(t, "", "go", "build", "-o", drv, "./cmd/engine")

	for _, s := range []struct {
		file  string
		extra []string // driver arguments
		lines int      // results of run
		pairs bool     // the results come in pairs that must be equal
	}{
		{"alias.c", nil, 24, false},
		{"icalls.c", nil, 56, true},
		{"trap.c", nil, 2, false},
		{"narrow_va.c", []string{"-varargs-block", "-non-format-variadic", "tagged,tagged2,small,oddsum,forward,forward2"}, 12, false},
	} {
		name := strings.TrimSuffix(s.file, ".c")
		t.Run(name, func(t *testing.T) {
			c, err := filepath.Abs(filepath.Join(src, s.file))
			if err != nil {
				t.Fatal(err)
			}
			for _, u := range []struct {
				suffix string
				flags  []string
			}{{"", nil}, {"-ub", ubsan}} {
				obj, exe := at(name+u.suffix+".o"), at(name+u.suffix)
				tool(t, "", clang, append(append(append([]string{}, plain...), u.flags...), "-c", "-o", obj, c)...)
				tool(t, "", lld, "-static", "-e", "_start", "-o", exe, at("main.o"), obj)
			}
			want := tool(t, "", at(name))
			if ub := tool(t, "", at(name+"-ub")); ub != want {
				t.Fatalf("the run under UBSan printed\n%s\nthe plain native run\n%s", ub, want)
			}
			native := strings.Split(strings.TrimSpace(want), "\n")
			if len(native) != s.lines {
				t.Fatalf("the native run printed %d results, want %d:\n%s", len(native), s.lines, want)
			}
			if s.pairs {
				for i := 0; i+1 < len(native); i += 2 {
					if native[i] != native[i+1] {
						t.Fatalf("results %d and %d of the native run differ: %s and %s", i, i+1, native[i], native[i+1])
					}
				}
			}
			for _, b := range []struct {
				mode  string
				extra []string
			}{{"body", nil}, {"pn", nil}, {"pn", []string{"-poison"}}} {
				mode := b.mode + strings.Join(b.extra, "")
				t.Run(mode, func(t *testing.T) {
					mod := at(name + "-" + mode)
					args := append(append([]string{"-src", c, "-llvm", llvm, "-pass", pass, "-mode", b.mode, "-leaves", "",
						"-work", at("w-" + name + "-" + mode), "-pkg", "nw"}, s.extra...), b.extra...)
					tool(t, "", drv, append(append([]string{}, args...), "-out", filepath.Join(mod, "nw"))...)
					for f, text := range map[string]string{
						"go.mod":     "module samples\n\ngo 1.22\n",
						"main.go":    sampleGoMain,
						"nw/host.go": sampleGoHost,
					} {
						if err := os.WriteFile(filepath.Join(mod, f), []byte(text), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					tool(t, mod, "go", "build", "-o", "run", ".")
					if have := tool(t, mod, filepath.Join(mod, "run")); have != want {
						t.Fatalf("Go runs the generated code to\n%s\nnative code gives\n%s", have, want)
					}
					if len(b.extra) > 0 {
						return // a poison build is for running only
					}
					tool(t, mod, "go", "build", "-tags", "goasmkeep", "-o", "keep", ".")
					out := tool(t, "", drv, append(append([]string{}, args...), "-verify-bin", filepath.Join(mod, "keep"),
						"-verify-pkg", "samples/nw", "-verify-all")...)
					if !strings.Contains(out, "post-link check passed") {
						t.Fatalf("the post-link check did not pass:\n%s", out)
					}
				})
			}
		})
	}

	// The trap is taken: the generated code calls host_trap, which panics,
	// where a ud2 would have been SIGILL and ended the process.
	t.Run("trap_reaches_host", func(t *testing.T) {
		c, err := filepath.Abs(filepath.Join(src, "trap.c"))
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"body", "pn"} {
			t.Run(mode, func(t *testing.T) {
				mod := at("trap-" + mode)
				tool(t, "", drv, "-src", c, "-llvm", llvm, "-pass", pass, "-mode", mode, "-leaves", "",
					"-work", at("w-trap-"+mode), "-pkg", "nw", "-out", filepath.Join(mod, "nw"))
				for f, text := range map[string]string{
					"go.mod":     "module samples\n\ngo 1.22\n",
					"main.go":    sampleGoMain,
					"nw/host.go": sampleGoHostTrap,
				} {
					if err := os.WriteFile(filepath.Join(mod, f), []byte(text), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				tool(t, mod, "go", "build", "-o", "run", ".")
				if have := tool(t, mod, filepath.Join(mod, "run")); have != "1\n" {
					t.Fatalf("the run with the trap taken printed %q, want the panic of host_trap (1)", have)
				}
			})
		}
	})

	// A leaf has no shadow pointer to call host_trap with, so one that traps
	// is refused.
	t.Run("leaf_trap", func(t *testing.T) {
		c, err := filepath.Abs(filepath.Join(src, "leaf_trap.c"))
		if err != nil {
			t.Fatal(err)
		}
		out, err := runIn("", goEnv, drv, "-src", c, "-llvm", llvm, "-pass", pass, "-mode", "pn", "-leaves", "lf",
			"-work", at("w-leaf-trap"), "-out", at("out-leaf-trap"), "-pkg", "nw")
		if err == nil || !strings.Contains(out, "register-entry leaf with a trap") {
			t.Fatalf("a leaf that traps was taken: %v\n%s", err, out)
		}
	})

	// The call of a weak alias is refused, whatever mode: the call names a
	// symbol that is neither a function of the engine nor a host callback.
	t.Run("alias_weak", func(t *testing.T) {
		c, err := filepath.Abs(filepath.Join(src, "alias_weak.c"))
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"body", "pn"} {
			out, err := runIn("", goEnv, drv, "-src", c, "-llvm", llvm, "-pass", pass, "-mode", mode, "-leaves", "",
				"-work", at("w-weak-"+mode), "-out", at("out-weak-"+mode), "-pkg", "nw")
			if err == nil || !strings.Contains(out, `"mix16_weak"`) {
				t.Fatalf("%s: a call of a weak alias was taken: %v\n%s", mode, err, out)
			}
			if _, err := os.Stat(at("out-weak-" + mode)); err == nil {
				t.Fatalf("%s: a refused run wrote a package", mode)
			}
		}
	})
}

// runIn runs a command in dir (the current directory when empty) with env
// and returns its combined output.
func runIn(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// sampleGoMain prints what nw.Run returns, one result per line.
const sampleGoMain = `package main

import (
	"fmt"

	"samples/nw"
)

func main() {
	for _, v := range nw.Run() {
		fmt.Println(v)
	}
}
`

// sampleGoHost runs a sample's run on a shadow region and a result buffer
// outside the Go heap. The samples call no host function, but host_trap,
// which the pass puts where a trap would be, and host_poll, which the
// generator calls when the count of cycles runs out. host_poll sets the count
// again, as the host of the engine does, and calls a function so that it
// keeps its stack check, which the post-link check holds it to.
const sampleGoHost = `package nw

import (
	"syscall"
	"unsafe"
)

func goasmMissing(i int64) { panic("missing: " + missingNames[i]) }

func host_trap(sp uintptr) { panic("host_trap") }
` + samplePoll + `

func Run() []int64 {
	m, err := syscall.Mmap(-1, 0, 4<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	base := (uintptr(unsafe.Pointer(&m[0])) + 1<<20 - 1) &^ (1<<20 - 1)
	sp, r := base+4096, base+2<<20
	n := int(run(sp, r, 1))
	out := make([]int64, n)
	for i := range out {
		out[i] = *(*int64)(unsafe.Pointer(r + uintptr(8*i)))
	}
	return out
}
`

const samplePoll = `
func host_poll(sp uintptr) { pollAgain(sp) }

//go:noinline
func pollAgain(sp uintptr) {
	*(*int64)(unsafe.Pointer(sp&^(1<<shadowSizeLog2-1) + pollCounterOffset)) = 1 << 10
}
`

// sampleGoHostTrap is the host of trap.c run with the trap taken: Run prints
// 1 when run lands in host_trap, and fails otherwise.
const sampleGoHostTrap = `package nw

import "syscall"
import "unsafe"

func goasmMissing(i int64) { panic("missing: " + missingNames[i]) }

func host_trap(sp uintptr) { panic("host_trap") }
` + samplePoll + `

func Run() (out []int64) {
	m, err := syscall.Mmap(-1, 0, 4<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	base := (uintptr(unsafe.Pointer(&m[0])) + 1<<20 - 1) &^ (1<<20 - 1)
	sp, r := base+4096, base+2<<20
	defer func() {
		if got := recover(); got != "host_trap" {
			panic(got)
		}
		out = []int64{1}
	}()
	run(sp, r, 2)
	return []int64{0}
}
`
