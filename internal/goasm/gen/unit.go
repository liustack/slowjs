// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// EngineCFlags are the clang flags of unified engine code, for the one
// x86-64 target the pass and the lowering know. Frame pointer kept,
// red zone off, signed wrap, no tail calls, and the stack only 8-byte aligned,
// as Go's amd64 stack is: with the pass's no-realign-stack, LLVM then spills
// vectors with unaligned moves and folds no packed operand from the stack.
// +slow-two-mem-ops keeps llc from folding a call target's load into the
// call, so every indirect call goes through a register and the lowering
// can marshal it. They travel in the IR as attributes, so llc applies them
// after the pass.
var EngineCFlags = []string{"--target=x86_64-linux-gnu", "-O2", "-fno-builtin", "-fno-omit-frame-pointer", "-mno-red-zone",
	"-fwrapv", "-fno-optimize-sibling-calls", "-fcf-protection=none", "-mstack-alignment=8",
	"-Xclang", "-target-feature", "-Xclang", "+slow-two-mem-ops"}

// HostCallbacks is the closed list of Go callbacks engine code may call.
var HostCallbacks = []string{"host_poll", "host_memory_grow", "host_now", "host_call", "host_release", "host_intl"}

// ARM64CFlags are the clang flags of arm64 engine code: plain AAPCS with
// x18 and x26 to x28 fixed, x29 reserved as the frame pointer, LR reserved
// for return addresses, no outline atomics and no unwind tables.
var ARM64CFlags = []string{"--target=aarch64-linux-gnu", "-O2", "-fno-builtin", "-fwrapv", "-ffp-contract=off",
	"-ffreestanding", "-nostdinc", "-fno-stack-protector", "-fno-asynchronous-unwind-tables", "-fno-unwind-tables",
	"-mno-outline-atomics", "-fno-optimize-sibling-calls", "-ffixed-x18", "-ffixed-x26", "-ffixed-x27", "-ffixed-x28",
	"-Xclang", "-mframe-pointer=reserved", "-Xclang", "-target-feature", "-Xclang", "+reserve-lr-for-ra"}

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
