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
	"regexp"
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
	// A leaf may save callee-saved registers it uses (musl's memcpy uses
	// RBX): the pushes at its start become stores into a NOSPLIT Go frame
	// and each pop before a return a load from it. RBP counts as one of
	// them: Go saves and restores the caller's BP around the frame.
	var pushed []string
	for len(pushed) < len(inner) {
		m := reLeafPush.FindStringSubmatch(norm(inner[len(pushed)]))
		if m == nil {
			break
		}
		pushed = append(pushed, m[1])
	}
	slot := map[string]int{}
	var out []string
	for k, r := range pushed {
		slot[r] = 8 * (len(pushed) - 1 - k)
		out = append(out, fmt.Sprintf("movq %%%s, %d(%%rsp)", r, slot[r]))
	}
	for _, l := range inner[len(pushed):] {
		n := norm(l)
		if reForbidden.MatchString(n) {
			return "", Meta{}, fmt.Errorf("%s: forbidden instruction in leaf: %q", fn.Name, l)
		}
		if m := reLeafPop.FindStringSubmatch(n); m != nil {
			off, ok := slot[m[1]]
			if !ok {
				return "", Meta{}, fmt.Errorf("%s: leaf pops a register it did not save: %q", fn.Name, l)
			}
			out = append(out, fmt.Sprintf("movq %d(%%rsp), %%%s", off, m[1]))
			continue
		}
		if strings.HasPrefix(n, "pushq") || strings.HasPrefix(n, "popq") ||
			reSub.MatchString(n) || reAdd.MatchString(n) || strings.Contains(n, "%rsp") {
			return "", Meta{}, fmt.Errorf("%s: leaf is not frameless: %q", fn.Name, l)
		}
		if m := reCall.FindStringSubmatch(n); m != nil && calleeKinds[m[1]] == Register {
			// Another register-entry leaf: arguments stay in registers.
			out = append(out, "call "+m[1])
			continue
		}
		if strings.HasPrefix(n, "call") || isTailJump(n) {
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
	return text, Meta{Name: fn.Name, Kind: Register, GoName: goIdent(fn.Name), RetType: fn.Sig.Ret, Frame: 8 * len(pushed)}, nil
}

var (
	reLeafPush = regexp.MustCompile(`^pushq\s+%(rbx|rbp|r12|r13|r14|r15)$`)
	reLeafPop  = regexp.MustCompile(`^popq\s+%(rbx|rbp|r12|r13|r14|r15)$`)
)
