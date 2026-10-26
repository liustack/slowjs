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
)

// The clang flags of engine code come in two parts. The portable freestanding
// ones (no system headers, no fused multiply-add, wrapping signed overflow,
// unsigned char, 32-bit BigInt limbs, and on arm64 no outline atomics) live
// in one place, engine/test/flags.sh, which the build and check scripts of
// the C tree read as well: init runs it in sh and puts what it gives in
// EngineCFlags and ARM64CFlags, so there is no second copy to keep in step.
// What follows is what only this generator needs.
//
// engineOnly are the x86-64 flags of unified engine code, for the one target
// the pass and the lowering know. Frame pointer kept, red zone off, no tail
// calls, no unwind tables, and the stack only 8-byte aligned, as Go's amd64
// stack is: with the pass's no-realign-stack, LLVM then spills vectors with
// unaligned moves and folds no packed operand from the stack.
// +slow-two-mem-ops keeps llc from folding a call target's load into the
// call, so every indirect call goes through a register and the lowering
// can marshal it. They travel in the IR as attributes, so llc applies them
// after the pass.
var engineOnly = []string{"--target=x86_64-linux-gnu", "-O2", "-fno-builtin", "-fno-omit-frame-pointer", "-mno-red-zone",
	"-fno-optimize-sibling-calls", "-fcf-protection=none", "-mstack-alignment=8",
	"-fno-asynchronous-unwind-tables", "-fno-unwind-tables",
	"-Xclang", "-target-feature", "-Xclang", "+slow-two-mem-ops"}

// arm64Only are the arm64 flags of engine code: plain AAPCS with x18 and x26
// to x28 fixed, x29 reserved as the frame pointer, LR reserved for return
// addresses, no tail calls and no unwind tables.
var arm64Only = []string{"--target=aarch64-linux-gnu", "-O2", "-fno-builtin", "-fno-asynchronous-unwind-tables", "-fno-unwind-tables",
	"-fno-optimize-sibling-calls", "-ffixed-x18", "-ffixed-x26", "-ffixed-x27", "-ffixed-x28",
	"-Xclang", "-mframe-pointer=reserved", "-Xclang", "-target-feature", "-Xclang", "+reserve-lr-for-ra"}

// EngineCFlags and ARM64CFlags are the clang flags of engine code for the two
// targets: the generator's own (engineOnly, arm64Only) and the portable ones
// of engine/test/flags.sh. If sh could not read that file they hold the
// generator's part only and FlagsErr says why: CompileUnit and the drivers
// stop on it.
var (
	EngineCFlags []string
	ARM64CFlags  []string
	FlagsErr     error
)

func init() {
	file := engineFlagsFile()
	amd, errA := SharedCFlags(file, "x86_64-linux-gnu")
	arm, errB := SharedCFlags(file, "aarch64-linux-gnu")
	if FlagsErr = errA; FlagsErr == nil {
		FlagsErr = errB
	}
	EngineCFlags = append(append([]string{}, engineOnly...), amd...)
	ARM64CFlags = append(append([]string{}, arm64Only...), arm...)
}

// engineFlagsFile is engine/test/flags.sh of this repository, found from the
// place of this source file. A build with -trimpath does not know it.
func engineFlagsFile() string {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(here), "..", "..", "..", "engine", "test", "flags.sh")
}

// SharedCFlags runs a flags.sh in sh, as the scripts of the C tree do, and
// returns its engine_flags followed by engine_target_flags for a clang target
// triple.
func SharedCFlags(flagsFile, triple string) ([]string, error) {
	if flagsFile == "" {
		return nil, fmt.Errorf("cannot find engine/test/flags.sh from the source of the generator")
	}
	cmd := exec.Command("sh", "-c", `. "$1" && printf '%s\n' "$engine_flags" && engine_target_flags "$2"`, "sh", flagsFile, triple)
	var errb strings.Builder
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("sh flags.sh (%s): %v: %s", flagsFile, err, strings.TrimSpace(errb.String()))
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("%s sets no engine_flags", flagsFile)
	}
	return strings.Fields(strings.Join(lines, " ")), nil
}

// HostCallbacks is the closed list of Go callbacks engine C may call (see
// engine/host.h, and host_memory_grow of engine/heap.h).
var HostCallbacks = []string{"host_poll", "host_memory_grow", "host_now", "host_call", "host_release"}

// GeneratorCallbacks are the Go callbacks the generator itself puts into the
// engine, which C never calls: host_trap stands for a trap instruction (see
// pass/shadow.cpp). The C tree still may not name them, since
// engine/libc/symbols.txt allows only HostCallbacks.
var GeneratorCallbacks = []string{"host_trap"}

// Unit is one C file compiled through the shadow-stack pass.
type Unit struct {
	IR, ShadowIR, Asm, Obj string // paths in the work dir
	Shadow                 string // the pass's report: one line per function
}

// UnitConfig says how to compile a unit.
type UnitConfig struct {
	LLVM, Pass, Work string   // LLVM bin dir, built pass, work dir
	CFlags           []string // clang flags (EngineCFlags or ARM64CFlags)
	PassArgs         []string // extra shadow-stack pass options
	Leaves           []string // register-entry leaves: no shadow pointer
}

// CompileUnit compiles cfile with clang, runs the IR checks and the
// shadow-stack pass, and writes assembly and an object with llc.
func CompileUnit(cfg UnitConfig, cfile string) (*Unit, error) {
	if FlagsErr != nil {
		return nil, FlagsErr
	}
	base := filepath.Join(cfg.Work, strings.TrimSuffix(filepath.Base(cfile), ".c"))
	u := &Unit{IR: base + ".ll", ShadowIR: base + ".shadow.ll", Asm: base + ".s", Obj: base + ".o"}
	if _, err := runTool(filepath.Join(cfg.LLVM, "clang"), append(append([]string{}, cfg.CFlags...), "-S", "-emit-llvm", "-o", u.IR, cfile)...); err != nil {
		return nil, err
	}
	ll, err := os.ReadFile(u.IR)
	if err != nil {
		return nil, err
	}
	if err := CheckIR(string(ll)); err != nil {
		return nil, fmt.Errorf("IR check: %w", err)
	}
	// The formats are read in the IR as written, before the optimizer
	// changes the calls that carry them.
	plain := base + ".plain.ll"
	if _, err := runTool(filepath.Join(cfg.LLVM, "clang"), append(append([]string{}, cfg.CFlags...), "-O0", "-S", "-emit-llvm", "-o", plain, cfile)...); err != nil {
		return nil, err
	}
	pll, err := os.ReadFile(plain)
	if err != nil {
		return nil, err
	}
	if vs := CheckFormats(string(pll), EngineNonFormatVariadic); len(vs) > 0 {
		return nil, fmt.Errorf("IR check: %s: %s: %s", vs[0].Func, vs[0].Rule, vs[0].Line)
	}
	args := []string{"-load-pass-plugin", cfg.Pass, "-passes=goasm-shadow-stack",
		"-shadow-leaf=" + strings.Join(cfg.Leaves, ",")}
	args = append(append(args, cfg.PassArgs...), "-S", "-o", u.ShadowIR, u.IR)
	u.Shadow, err = runTool(filepath.Join(cfg.LLVM, "opt"), args...)
	if err != nil {
		return nil, fmt.Errorf("shadow-stack pass: %w", err)
	}
	// A failing llc can leave a partial file: nothing from an earlier run
	// may stand in for its output.
	os.Remove(u.Asm)
	os.Remove(u.Obj)
	llc := filepath.Join(cfg.LLVM, "llc")
	if _, err := runTool(llc, "-O2", "-enable-shrink-wrap=false", "-o", u.Asm, u.ShadowIR); err != nil {
		return nil, err
	}
	if _, err := runTool(llc, "-O2", "-enable-shrink-wrap=false", "-filetype=obj", "-o", u.Obj, u.ShadowIR); err != nil {
		return nil, err
	}
	return u, nil
}

var reGlobl = regexp.MustCompile(`(?m)^\s*\.globl\s+(\w+)`)

// Functions lists the global functions an assembly file defines.
func Functions(asm string) []string {
	var out []string
	for _, m := range reGlobl.FindAllStringSubmatch(asm, -1) {
		out = append(out, m[1])
	}
	return out
}

// runTool runs a command and returns its stderr; a failure carries it.
func runTool(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(name), err, strings.TrimSpace(errb.String()))
	}
	return errb.String(), nil
}
