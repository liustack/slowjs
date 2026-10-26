// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"reflect"
	"strings"
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

func TestParseRegFacts(t *testing.T) {
	f, err := ParseRegFacts(strings.Join([]string{
		"fn run args rdi,rsi,edx entry bh,bl,rbx,rdi,rsi,r12wh",
		"call run 0 *2214592512 args rdi,esi defs eax after ah,al,ax,eax,hax,rbx",
		"call run 1 host_narrow args r12,r13d,r14d,r15d,rdi defs rax after -",
		"ret run 0 uses rax",
		"fn goasm.probe.half args r12,r13d entry - ",
		"ret goasm.probe.half 0 uses eax",
		"",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := RegFacts{
		"run": {Args: []string{"rdi", "rsi", "edx"}, Entry: []string{"bh", "bl", "rbx", "rdi", "rsi", "r12wh"},
			Calls: []CallFacts{
				{Target: "*2214592512", Args: []string{"rdi", "esi"}, Defs: []string{"eax"}, After: []string{"ah", "al", "ax", "eax", "hax", "rbx"}},
				{Target: "host_narrow", Args: []string{"r12", "r13d", "r14d", "r15d", "rdi"}, Defs: []string{"rax"}},
			},
			Rets: [][]string{{"rax"}}},
		"goasm.probe.half": {Args: []string{"r12", "r13d"}, Rets: [][]string{{"eax"}}},
	}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v\nwant %+v", f, want)
	}
	for _, bad := range []string{
		"fn f args rdi",                                         // no entry
		"call f 0 g args - defs - after -",                      // a call before its function
		"fn f args - entry -\ncall f 1 g args - defs - after -", // a call out of order
		"fn f args - entry -\nfn f args - entry -",              // a function twice
		"fn f args - entry -\nret f 0 uses",                     // no registers
		"op f",
	} {
		if _, err := ParseRegFacts(bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}

func TestX86Regs(t *testing.T) {
	got, err := x86Regs(strings.Split("ah,al,ax,eax,hax,rax,r12wh,r12b,r15d,sil,dih,xmm3,ymm3,zmm15", ","))
	if err != nil {
		t.Fatal(err)
	}
	want := []x86asm.Reg{x86asm.RAX, x86asm.R12, x86asm.R15, x86asm.RSI, x86asm.RDI, x86asm.X3, x86asm.X15}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := x86Regs([]string{"eflags"}); err == nil {
		t.Fatal("eflags taken for a register the check knows")
	}
}
