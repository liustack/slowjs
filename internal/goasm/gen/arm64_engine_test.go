// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"
)

// a64TestEnv is a unit with a function f(I64, I64) I64 that calls callee(I64)
// I64, a register-entry leaf memcpy, a variadic printf, data at offset 16 of
// engine_rodata and a constant pool of f, with llc's facts for the calls the
// cases below make.
func a64TestEnv() (FuncSpec, *ARM64Env) {
	sig := Sig{Params: []Type{I64, I64}, Ret: I64}
	SetCallees([]FuncSpec{
		{Name: "f", Sig: sig, Kind: Unified},
		{Name: "callee", Sig: Sig{Params: []Type{I64}, Ret: I64}, Kind: Unified},
		{Name: "memcpy", Sig: Sig{Params: []Type{Ptr, Ptr, I64}, Ret: Ptr}, Kind: Register},
		{Name: "printf", Sig: Sig{Params: []Type{Ptr}, Ret: I64}, Kind: Unified},
	})
	plan := &DataPlan{BlobOff: map[string]int{"data": 16}, Rename: map[string]map[string]string{"f": {".LCPI0_0": ".LCPI0_0"}}}
	env := &ARM64Env{Plan: plan, Funcs: map[string]bool{"f": true, "callee": true, "memcpy": true, "printf": true},
		Variadic: map[string]bool{"printf": true}}
	return FuncSpec{Name: "f", Sig: sig, Kind: Unified}, env
}

// a64Good is a function every check takes: a frame of 16 bytes with LR in
// it, the address of data, a constant from its pool, a call and a return.
var a64Good = []string{
	"str x30, [sp, #-16]!",
	"adrp x8, data+8",
	"add x8, x8, :lo12:data+12",
	"ldr x9, [x8, #4]",
	"adrp x10, .LCPI0_0",
	"ldr d0, [x10, :lo12:.LCPI0_0]",
	"mov x20, x21",
	"bl callee",
	"ldr x30, [sp], #16",
	"ret",
}

func TestLowerARM64EngineFunc(t *testing.T) {
	fn, env := a64TestEnv()
	text, meta, err := LowerARM64EngineFunc(a64Good, fn, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"str x30, [sp, #32]", // the LLVM frame at L=16+16: the call's area is 16 bytes
		"adrp x8, __goasm_rodata+24\n\tadd x8, x8, :lo12:__goasm_rodata+24", // the whole address of data+8
		"add x8, x8, #4",   // :lo12:data+12 is 4 past it
		"ldr d0, .LCPI0_0", // the function's own data, PC-relative
		"str x20, [sp, #8]\n\tbl go_callee\n\tldr x0, [sp, #16]", // the ABI0 call
		"ldr x30, [sp, #32]\n\tb .Lgoret_f",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	if meta.Frame != 32+16-8 || meta.ArgBytes != 24 {
		t.Errorf("frame %d, arguments %d", meta.Frame, meta.ArgBytes)
	}
}

// TestLowerARM64EngineRejects: each case breaks one rule of the lowering and
// must be refused with its reason.
func TestLowerARM64EngineRejects(t *testing.T) {
	replace := func(old string, new ...string) []string {
		var out []string
		found := false
		for _, l := range a64Good {
			if l == old {
				out = append(out, new...)
				found = true
				continue
			}
			out = append(out, l)
		}
		if !found {
			t.Fatalf("no %q", old)
		}
		return out
	}
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"system call":          {replace("mov x20, x21", "svc #0"), "forbidden instruction"},
		"system register":      {replace("mov x20, x21", "mrs x0, TPIDR_EL0"), "forbidden instruction"},
		"g":                    {replace("mov x20, x21", "mov x20, x28"), "reserved register"},
		"platform register":    {replace("mov x20, x21", "mov x18, x0"), "reserved register"},
		"frame pointer":        {replace("mov x20, x21", "mov x29, x0"), "reserved register"},
		"page read as a value": {replace("add x8, x8, :lo12:data+12", "mov x0, x8", "add x8, x8, :lo12:data+12"), "holds the page"},
		"page in another block": {replace("add x8, x8, :lo12:data+12", ".LBB0_1:", "add x8, x8, :lo12:data+12"),
			"not in the block of an adrp"},
		"page of other data":     {replace("add x8, x8, :lo12:data+12", "add x8, x8, :lo12:other"), "not in the block of an adrp of other"},
		"unplaced data":          {replace("adrp x8, data+8", "adrp x8, other"), "unplaced data"},
		"GOT":                    {replace("adrp x8, data+8", "adrp x8, :got:data"), "unreviewed relocation form"},
		"stack past the frame":   {replace("mov x20, x21", "ldr x0, [sp, #16]"), "outside the LLVM frame"},
		"stack pair past it":     {replace("mov x20, x21", "ldp x0, x1, [sp, #8]"), "outside the LLVM frame"},
		"stack address":          {replace("mov x20, x21", "mov x0, sp"), "stack address escapes"},
		"tuple address read":     {replace("mov x20, x21", "add x0, sp, #0", "mov x1, x0"), "stack address is read"},
		"tail call":              {replace("bl callee", "b callee"), "tail jump"},
		"conditional tail call":  {replace("mov x20, x21", "cbz x0, callee"), "conditional tail jump"},
		"unknown callee":         {replace("bl callee", "bl nowhere"), "unknown nowhere"},
		"variadic call":          {replace("bl callee", "bl printf"), "variadic call"},
		"indirect without label": {replace("bl callee", "blr x8"), "without LLVM's signature label"},
		"return with frame":      {replace("ldr x30, [sp], #16"), "ret with 16 bytes"},
		"frame grows later":      {replace("mov x20, x21", ".LBB0_1:", "sub sp, sp, #16"), "frame grows past its prologue"},
		"adr of data":            {replace("mov x20, x21", "adr x0, data"), "unreviewed adr"},
		"LR as a value":          {replace("mov x20, x21", "mov x0, x30"), "LR used outside a stack save"},
		"fused multiply-add":     {replace("mov x20, x21", "fmadd d0, d1, d2, d0"), "fused multiply-add"},
		"fused vector multiply":  {replace("mov x20, x21", "fmla v0.2d, v1.2d, v2.2d"), "fused multiply-add"},
	} {
		fn, env := a64TestEnv()
		if _, _, err := LowerARM64EngineFunc(c.lines, fn, env); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// TestLowerARM64EngineFacts: the lowering moves each argument and result
// where llc's facts say llc reads and writes it, and refuses a difference: a
// narrow integer llc passes in another register, a result it reads from
// another, an entry that reads an argument elsewhere.
func TestLowerARM64EngineFacts(t *testing.T) {
	good := func() (RegFacts, RegFacts) {
		facts := RegFacts{"f": {Args: []string{"x20", "x21"},
			Calls: []CallFacts{{Target: "callee", Args: []string{"w20"}, Defs: []string{"lr", "x0"}}}}}
		probes := RegFacts{ProbePrefix + "f": {Args: []string{"x20", "w21"}}}
		return facts, probes
	}
	fn, env := a64TestEnv()
	env.Facts, env.Probes = good()
	if _, _, err := LowerARM64EngineFunc(a64Good, fn, env); err != nil {
		t.Fatalf("llc's own registers: %v", err)
	}
	for name, c := range map[string]struct {
		edit func(facts, probes RegFacts)
		want string
	}{
		"narrow argument elsewhere": {func(f, p RegFacts) { f["f"].Calls[0].Args = []string{"w0"} }, "call registers"},
		"result elsewhere":          {func(f, p RegFacts) { f["f"].Calls[0].Defs = []string{"lr", "x20"} }, "result registers"},
		"entry elsewhere":           {func(f, p RegFacts) { p[ProbePrefix+"f"].Args = []string{"x20", "x22"} }, "entry registers"},
		"other call":                {func(f, p RegFacts) { f["f"].Calls[0].Target = "memcpy" }, "llc's facts say memcpy"},
		"a call more":               {func(f, p RegFacts) { f["f"].Calls = append(f["f"].Calls, f["f"].Calls[0]) }, "llc's facts have 2"},
		"no facts":                  {func(f, p RegFacts) { delete(f, "f") }, "no register facts"},
		"no probe":                  {func(f, p RegFacts) { delete(p, ProbePrefix+"f") }, "no probe facts"},
	} {
		fn, env := a64TestEnv()
		env.Facts, env.Probes = good()
		c.edit(env.Facts, env.Probes)
		if _, _, err := LowerARM64EngineFunc(a64Good, fn, env); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// TestLowerARM64EngineLeaf: a register-entry leaf may keep a frame and call
// another leaf, never a unified entry or through a pointer.
func TestLowerARM64EngineLeaf(t *testing.T) {
	fn, env := a64TestEnv()
	fn = FuncSpec{Name: "memmove", Sig: Sig{Params: []Type{Ptr, Ptr, I64}, Ret: Ptr}, Kind: Register}
	leaf := []string{"str x30, [sp, #-16]!", "bl memcpy", "ldr x30, [sp], #16", "ret"}
	text, meta, err := LowerARM64EngineFunc(leaf, fn, env)
	if err != nil || !strings.Contains(text, "bl go_memcpy\n") || strings.Contains(text, "[sp, #8]") || meta.ArgBytes != 0 || meta.Frame != 16+16-8 {
		t.Fatalf("%v %+v\n%s", err, meta, text)
	}
	if _, _, err := LowerARM64EngineFunc([]string{"str x30, [sp, #-16]!", "bl callee", "ldr x30, [sp], #16", "ret"}, fn, env); err == nil ||
		!strings.Contains(err.Error(), "which is no leaf") {
		t.Errorf("a leaf calling a unified entry: %v", err)
	}
}
