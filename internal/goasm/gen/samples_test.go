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
//   - vecpool.c: constants read with instructions that need them on 16 bytes,
//     in functions whose bodies start 8 bytes past a multiple of 16 (-align64):
//     the constants must still sit on 16 bytes where they run.
//   - nosplit.c: the shapes the NOSPLIT choice (PickNoSplit) tells apart run
//     as native code does. The post-link check holds each function's stack
//     check to the choice, and nosplit_fault below holds the choice itself,
//     faults at the bottom of its chain and forces it past what the linker
//     takes.
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
		{"nosplit.c", nil, 4, false},
		{"vecpool.c", []string{"-align64", "mixa:40,mixb:8,mixc:16"}, 9, false},
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

	// The NOSPLIT choice on nosplit.c: the bottom of the chain and the
	// callers of what recurses or goes through a pointer are chosen, what
	// recurses, has its address taken or calls through a pointer is not, and
	// one function of the chain is left for the room. A wild pointer read at
	// the bottom of the chain must be a panic that recovers whatever the depth
	// of the goroutine's stack above it, and forcing a recursion or the whole
	// chain onto the list must fail the build at the linker.
	t.Run("nosplit_fault", func(t *testing.T) {
		c, err := filepath.Abs(filepath.Join(src, "nosplit.c"))
		if err != nil {
			t.Fatal(err)
		}
		gen := func(name, main string, env ...string) (string, string) {
			mod, w := at("nosplit-"+name), at("w-nosplit-"+name)
			cmd := exec.Command(drv, "-src", c, "-llvm", llvm, "-pass", pass, "-mode", "pn", "-leaves", "",
				"-work", w, "-pkg", "nw", "-out", filepath.Join(mod, "nw"))
			cmd.Env = append(append([]string{}, goEnv...), env...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("the driver: %v\n%s", err, out)
			}
			for f, text := range map[string]string{
				"go.mod":      "module samples\n\ngo 1.22\n",
				"main.go":     main,
				"nw/host.go":  sampleGoHost,
				"nw/sweep.go": sampleGoSweep,
			} {
				if err := os.WriteFile(filepath.Join(mod, f), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return mod, w
		}
		mod, w := gen("ok", sampleGoMainSweep)
		b, err := os.ReadFile(filepath.Join(w, "nosplit.txt"))
		if err != nil {
			t.Fatal(err)
		}
		on := map[string]string{}
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if f := strings.Fields(l); len(f) > 1 {
				on[f[0]] = f[1]
			}
		}
		for n, kind := range map[string]string{"down23": "auto", "down22": "chosen", "poke": "chosen", "run": "chosen",
			"callsrec": "chosen", "callsptr": "chosen", "sq": "auto"} {
			if on[n] != kind {
				t.Errorf("%s is %q on the list, want %s", n, on[n], kind)
			}
		}
		for _, n := range []string{"self", "ping", "pong", "viaptr", "icaller"} {
			if on[n] != "" {
				t.Errorf("%s is on the list", n)
			}
		}
		var cut []string
		for i := 0; i <= 22; i++ {
			if n := fmt.Sprintf("down%d", i); on[n] == "" {
				cut = append(cut, n)
			}
		}
		if len(cut) == 0 || len(cut) > 2 {
			t.Fatalf("the chain is cut at %v, want one or two functions:\n%s", cut, b)
		}

		// The goroutine's stack starts at its smallest, so that the sweep
		// reaches every place of its bottom.
		tool(t, mod, "go", "build", "-o", "run", ".")
		out, err := runIn(mod, append(append([]string{}, goEnv...), "GODEBUG=adaptivestackstart=0"), filepath.Join(mod, "run"))
		if err != nil {
			t.Fatalf("the sweep: %v\n%s", err, out)
		}
		var runs, moved, sigpanic, lo, hi int
		if _, err := fmt.Sscanf(out, "runs %d moved %d sigpanic %d depths %d to %d", &runs, &moved, &sigpanic, &lo, &hi); err != nil {
			t.Fatalf("the sweep printed %q: %v", out, err)
		}
		// Every place of the first two stacks, 2 and 4 KiB, and faults that
		// only sigpanic's morestack made room for.
		if hi-lo < 6<<10 || sigpanic == 0 {
			t.Fatalf("the sweep did not reach the bottom of the stack: %s", out)
		}
		t.Logf("cut at %v, %s", cut, strings.TrimSpace(out))

		for _, force := range []string{"self", "ping,pong", strings.Join(cut, ",")} {
			t.Run("force_"+force, func(t *testing.T) {
				// The linker checks what the program reaches: run calls
				// every function.
				mod, _ := gen("force", sampleGoMain, "GEN_FORCE_NOSPLIT="+force)
				if o, err := runIn(mod, goEnv, "go", "build", "-o", "run", "."); err == nil ||
					!strings.Contains(o, "negative_build_of_the_generator_is_not_an_engine") {
					t.Fatalf("a forced build compiled without the tag: %v\n%s", err, o)
				}
				if o, err := runIn(mod, goEnv, "go", "build", "-tags", "goasmnegative", "-o", "run", "."); err == nil ||
					!strings.Contains(o, "nosplit stack over 792 byte limit") {
					t.Fatalf("the linker took %s on the NOSPLIT list: %v\n%s", force, err, o)
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

// sampleGoMainSweep prints what nw.Sweep finds.
const sampleGoMainSweep = `package main

import (
	"fmt"

	"samples/nw"
)

func main() { fmt.Println(nw.Sweep()) }
`

// sampleGoSweep calls poke of nosplit.c with a pointer to nothing, which its
// chain of calls reads at the bottom, from fresh goroutines whose stacks are
// used to every depth in steps of 8 bytes: dive takes a frame for each level,
// a pad a frame of 1 to 16 words. Each call must panic with a runtime.Error
// at the address, which the goroutine recovers, wherever the bottom of the
// stack is. A fault the runtime cannot take ends the process instead
// ("unexpected fault address", or "split stack overflow" when sigpanic finds
// no room for morestack). The byte at the top of the goroutine stays on its
// stack, and the pointer to it goes down as an argument, so that a stack that
// grows moves both and the depth is right after it (a probe that held the
// pointer would take the byte to the heap with what it recovers). moved
// counts the calls that grew the stack. Each place is also run with a pointer
// that reads, in a goroutine of its own, which starts with the same stack:
// sigpanic counts the places where only the fault grew the stack, where the
// chain went below the guard and the fault came there, so that only the
// room the chain left let sigpanic call morestack.
var sampleGoSweep = `package nw

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"
)

const wild = 0xdead0000

type probe struct {
	sp, k, addr, depth uintptr
	moved              bool
	got                any
}

//go:noinline
func dive(n int, top *byte, p *probe) {
	if n > 0 {
		dive(n-1, top, p)
		return
	}
	switch p.k {
` + sweepCases() + `	}
}

//go:noinline
func fire(top *byte, p *probe) {
	var here byte
	p.depth = uintptr(unsafe.Pointer(top)) - uintptr(unsafe.Pointer(&here))
	defer func() { p.got = recover() }()
	poke(p.sp, p.addr, 1)
}

` + sweepPads() + `

func Sweep() string {
	m, err := syscall.Mmap(-1, 0, 4<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	sp := (uintptr(unsafe.Pointer(&m[0]))+1<<20-1)&^(1<<20-1) + 4096
	run := func(n int, k, addr uintptr) probe {
		res := make(chan probe, 1)
		go func() {
			debug.SetPanicOnFault(true)
			var top byte
			p := probe{sp: sp, k: k, addr: addr}
			dive(n, &top, &p)
			res <- p
		}()
		return <-res
	}
	depths := map[uintptr]bool{}
	runs, moved, sigpanic := 0, 0, 0
	for n := 0; n < 160; n++ {
		for k := uintptr(0); k < 16; k++ {
			ok := run(n, k, sp)
			if ok.got != nil {
				panic(fmt.Sprintf("at depth %d poke of a good pointer panicked with %v", ok.depth, ok.got))
			}
			p := run(n, k, wild)
			a, at := p.got.(interface{ Addr() uintptr })
			if _, re := p.got.(runtime.Error); !re || !at || a.Addr() != wild || p.depth != ok.depth {
				panic(fmt.Sprintf("at depth %d (%d) poke panicked with %T %v", p.depth, ok.depth, p.got, p.got))
			}
			depths[p.depth] = true
			runs++
			if p.moved {
				moved++
				if !ok.moved {
					sigpanic++
				}
			}
		}
	}
	lo := ^uintptr(0)
	for d := range depths {
		lo = min(lo, d)
	}
	hi := lo
	for depths[hi+8] {
		hi += 8
	}
	return fmt.Sprintf("runs %d moved %d sigpanic %d depths %d to %d", runs, moved, sigpanic, lo, hi)
}
`

// sweepCases and sweepPads write the pads of sampleGoSweep, which dive calls
// directly, so that the probe stays on the stack: pad k takes k+1 words of
// its frame before it fires, and says whether the stack moved meanwhile.
func sweepCases() string {
	var b strings.Builder
	for k := range 16 {
		fmt.Fprintf(&b, "\tcase %d:\n\t\tpad%d(top, p)\n", k, k)
	}
	return b.String()
}

func sweepPads() string {
	var b strings.Builder
	for k := range 16 {
		fmt.Fprintf(&b, `
//go:noinline
func pad%d(top *byte, p *probe) {
	var a [%d]uintptr
	before := uintptr(unsafe.Pointer(&a[0]))
	fire(top, p)
	p.moved = uintptr(unsafe.Pointer(&a[0])) != before
}
`, k, k+1)
	}
	return b.String()
}

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
