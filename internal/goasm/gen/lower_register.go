// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Register-entry leaf lowering. A leaf neither grows the stack nor calls a
// split-checked function, so it is emitted NOSPLIT with its arguments left in
// the native argument registers and its result in the native result register.
// Only a direct call from a known caller may target it; the function table and
// indirect calls use the leaf's unified entry instead.
package gen

import (
	"fmt"
	"strings"
)

// LowerRegister transforms a frameless leaf (compiled with -fomit-frame-pointer)
// into a NOSPLIT body. It rejects anything with a frame, a call or a forbidden
// instruction.
func LowerRegister(asm string, fn FuncSpec) (string, Meta, error) {
	lines := strings.Split(asm, "\n")
	start := -1
	for i, l := range lines {
		if clean(l) == fn.Name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return "", Meta{}, fmt.Errorf("%s: not found", fn.Name)
	}
	end := -1
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), ".Lfunc_end") {
			end = i
			break
		}
	}
	var inner []string
	for _, raw := range lines[start+1 : end] {
		l := clean(raw)
		if l == "" || strings.HasPrefix(l, ".cfi_") || strings.HasPrefix(l, ".p2align") ||
			strings.HasPrefix(l, ".size") || strings.HasPrefix(l, ".type") ||
			strings.HasPrefix(l, ".globl") || strings.HasPrefix(l, ".section") ||
			strings.HasPrefix(l, ".addrsig") || strings.HasPrefix(l, ".ident") ||
			strings.HasPrefix(l, ".text") {
			continue
		}
		inner = append(inner, l)
	}
	retLabel := ".Lret_" + fn.Name
	var out []string
	for _, l := range inner {
		n := norm(l)
		if reForbidden.MatchString(n) {
			return "", Meta{}, fmt.Errorf("%s: forbidden instruction in leaf: %q", fn.Name, l)
		}
		if strings.HasPrefix(n, "pushq") || strings.HasPrefix(n, "popq") ||
			reSub.MatchString(n) || reAdd.MatchString(n) || strings.Contains(n, "%rbp") {
			return "", Meta{}, fmt.Errorf("%s: leaf is not frameless: %q", fn.Name, l)
		}
		if reCall.MatchString(n) {
			return "", Meta{}, fmt.Errorf("%s: register-entry leaf must not call: %q", fn.Name, l)
		}
		if n == "retq" || n == "ret" {
			out = append(out, "jmp "+retLabel)
			continue
		}
		out = append(out, l)
	}
	out = append(out, retLabel+":")
	text := fn.Name + ":\n\t" + strings.Join(out, "\n\t") + "\n"
	return text, Meta{Name: fn.Name, Kind: Register, GoName: goIdent(fn.Name), RetType: fn.Sig.Ret}, nil
}
