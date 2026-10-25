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

// EngineCFlags are the clang flags of unified engine code. Frame pointer kept,
// red zone off, signed wrap, no tail calls, and the stack only 8-byte aligned,
// as Go's amd64 stack is: with the pass's no-realign-stack, LLVM then spills
// vectors with unaligned moves and folds no packed operand from the stack.
// They travel in the IR as attributes, so llc applies them after the pass.
var EngineCFlags = []string{"-O2", "-fno-builtin", "-fno-omit-frame-pointer", "-mno-red-zone",
	"-fwrapv", "-fno-optimize-sibling-calls", "-fcf-protection=none", "-mstack-alignment=8"}

// HostCallbacks is the closed list of Go callbacks engine code may call.
var HostCallbacks = []string{"host_poll", "host_memory_grow", "host_now", "host_call", "host_release", "host_intl"}

// Unit is one C file compiled through the shadow-stack pass.
type Unit struct {
	IR, ShadowIR, Asm, Obj string // paths in the work dir
	Shadow                 string // the pass's report: one line per function
}

// CompileUnit compiles cfile with LLVM in llvmBin, runs the IR checks and the
// shadow-stack pass, and writes assembly and an object with llc.
func CompileUnit(llvmBin, pass, cfile, work string, leaves []string) (*Unit, error) {
	base := filepath.Join(work, strings.TrimSuffix(filepath.Base(cfile), ".c"))
	u := &Unit{IR: base + ".ll", ShadowIR: base + ".shadow.ll", Asm: base + ".s", Obj: base + ".o"}
	if _, err := runTool(filepath.Join(llvmBin, "clang"), append(append([]string{}, EngineCFlags...), "-S", "-emit-llvm", "-o", u.IR, cfile)...); err != nil {
		return nil, err
	}
	ll, err := os.ReadFile(u.IR)
	if err != nil {
		return nil, err
	}
	if err := CheckIR(string(ll)); err != nil {
		return nil, fmt.Errorf("IR check: %w", err)
	}
	u.Shadow, err = runTool(filepath.Join(llvmBin, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
		"-shadow-leaf="+strings.Join(leaves, ","), "-S", "-o", u.ShadowIR, u.IR)
	if err != nil {
		return nil, fmt.Errorf("shadow-stack pass: %w", err)
	}
	llc := filepath.Join(llvmBin, "llc")
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
