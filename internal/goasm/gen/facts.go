// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// What llc itself decided about registers, as the plugin in pass/facts.cpp
// writes it (-goasm-facts). The post-link check holds the lowering to it
// instead of to the generator's own model of the calling convention, so a
// mistake in the model (a type it moves in the wrong register) shows as a
// difference instead of being checked against itself.
package gen

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

// RegFacts are the facts of one llc run, by function.
type RegFacts map[string]*FuncFacts

// FuncFacts is what llc decided for one function: the live-in registers of
// its arguments in order (only the ones its code reads, which for a probe
// of -shadow-probes is all of them), the registers its code reads before
// writing them, each call and each return in code order. Registers are
// llc's names, every name of a register that is live (eax and al beside
// rax).
type FuncFacts struct {
	Args, Entry []string
	Calls       []CallFacts
	Rets        [][]string
}

// CallFacts is one call: its target (a symbol, or "*" and the kcfi type id
// of an indirect call), its argument registers in order, the registers it
// defines (its results) and the ones live right after it.
type CallFacts struct {
	Target            string
	Args, Defs, After []string
}

// ProbePrefix starts the name of the probe of a unified definition.
const ProbePrefix = "goasm.probe."

// ParseRegFacts reads the plugin's output.
func ParseRegFacts(text string) (RegFacts, error) {
	facts := RegFacts{}
	fn := func(name string) *FuncFacts {
		f := facts[name]
		if f == nil {
			f = &FuncFacts{}
			facts[name] = f
		}
		return f
	}
	regs := func(s string) []string {
		if s == "-" {
			return nil
		}
		return strings.Split(s, ",")
	}
	// index checks that the n-th call or return of a function comes as
	// the n-th.
	index := func(s string, have int) error {
		n, err := strconv.Atoi(s)
		if err != nil || n != have {
			return fmt.Errorf("number %q out of order (want %d)", s, have)
		}
		return nil
	}
	for i, l := range strings.Split(text, "\n") {
		if l == "" {
			continue
		}
		f := strings.Fields(l)
		bad := func(err error) (RegFacts, error) {
			return nil, fmt.Errorf("register facts line %d %q: %w", i+1, l, err)
		}
		switch {
		case len(f) == 6 && f[0] == "fn" && f[2] == "args" && f[4] == "entry":
			if _, dup := facts[f[1]]; dup {
				return bad(fmt.Errorf("a second function %s", f[1]))
			}
			x := fn(f[1])
			x.Args, x.Entry = regs(f[3]), regs(f[5])
		case len(f) == 10 && f[0] == "call" && f[4] == "args" && f[6] == "defs" && f[8] == "after":
			x, ok := facts[f[1]]
			if !ok {
				return bad(fmt.Errorf("a call before its function"))
			}
			if err := index(f[2], len(x.Calls)); err != nil {
				return bad(err)
			}
			x.Calls = append(x.Calls, CallFacts{Target: f[3], Args: regs(f[5]), Defs: regs(f[7]), After: regs(f[9])})
		case len(f) == 5 && f[0] == "ret" && f[3] == "uses":
			x, ok := facts[f[1]]
			if !ok {
				return bad(fmt.Errorf("a return before its function"))
			}
			if err := index(f[2], len(x.Rets)); err != nil {
				return bad(err)
			}
			x.Rets = append(x.Rets, regs(f[4]))
		default:
			return bad(fmt.Errorf("not a fact"))
		}
	}
	return facts, nil
}

// x86Family maps every name llc gives a part of a register to the x86asm
// name of the whole: eax, ax, al, ah and hax (the upper half of eax) to
// RAX, r12d, r12w, r12b, r12bh and r12wh to R12, ymm3 and zmm3 to X3.
var x86Family = func() map[string]x86asm.Reg {
	m := map[string]x86asm.Reg{}
	for r, names := range map[x86asm.Reg]string{
		x86asm.RAX: "rax eax ax al ah hax", x86asm.RBX: "rbx ebx bx bl bh hbx",
		x86asm.RCX: "rcx ecx cx cl ch hcx", x86asm.RDX: "rdx edx dx dl dh hdx",
		x86asm.RSI: "rsi esi si sil sih hsi", x86asm.RDI: "rdi edi di dil dih hdi",
		x86asm.RBP: "rbp ebp bp bpl bph hbp", x86asm.RSP: "rsp esp sp spl sph hsp",
	} {
		for _, n := range strings.Fields(names) {
			m[n] = r
		}
	}
	for i := 8; i <= 15; i++ {
		r := x86asm.R8 + x86asm.Reg(i-8)
		for _, suf := range []string{"", "d", "w", "b", "bh", "wh"} {
			m[fmt.Sprintf("r%d%s", i, suf)] = r
		}
	}
	for i := 0; i <= 15; i++ {
		for _, p := range []string{"xmm", "ymm", "zmm"} {
			m[fmt.Sprintf("%s%d", p, i)] = x86asm.X0 + x86asm.Reg(i)
		}
	}
	return m
}()

// x86Regs maps llc's names to whole registers, keeping the order of their
// first appearance and dropping the other names of one.
func x86Regs(names []string) ([]x86asm.Reg, error) {
	var out []x86asm.Reg
	seen := map[x86asm.Reg]bool{}
	for _, n := range names {
		r, ok := x86Family[n]
		if !ok {
			return nil, fmt.Errorf("llc's register %s is none the check knows", n)
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

func isXMM(r x86asm.Reg) bool { return r >= x86asm.X0 && r <= x86asm.X15 }
