// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Phase-0 driver: compile the C test programs with LLVM 22.1.8, lower each
// function to the unified or register-entry convention, re-assemble, and emit
// Go assembly plus the paired Go declarations into the proto package.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gen "github.com/liustack/slowjs/internal/goasm/gen"
)

func main() {
	llvm := flag.String("llvm", "/usr/lib/llvm-22/bin", "LLVM 22.1.8 bin dir")
	data := flag.String("data", "testdata", "C sources")
	out := flag.String("out", "proto", "output package dir")
	bin := flag.String("bin", "", "linked binary to verify against (post-link check)")
	flag.Parse()

	clang := filepath.Join(*llvm, "clang")
	if err := run(clang, "--version"); err != nil {
		die(err)
	}

	// Signatures and kinds of every function the unit references.
	ptr, i64, f64, jsv, void := gen.Ptr, gen.I64, gen.F64, gen.JSV, gen.Void
	specs := []gen.FuncSpec{
		{Name: "mul", Kind: gen.Register, Sig: gen.Sig{Params: []gen.Type{i64, i64}, Ret: i64}},
		{Name: "rec", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "poly", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "bump", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, jsv, i64}, Ret: jsv}},
		{Name: "mix", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, f64, i64}, Ret: f64}},
		{Name: "apply", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64},
			Indirect: []gen.Sig{{Params: []gen.Type{ptr, i64}, Ret: i64}}},
		// Call-overhead benchmark functions.
		{Name: "lf_i", Kind: gen.Register, Sig: gen.Sig{Params: []gen.Type{i64, i64}, Ret: i64}},
		{Name: "lf_d", Kind: gen.Register, Sig: gen.Sig{Params: []gen.Type{f64, f64}, Ret: f64}},
		{Name: "lf_v", Kind: gen.Register, Sig: gen.Sig{Params: []gen.Type{i64, i64}, Ret: jsv}},
		{Name: "uf_i", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}},
		{Name: "uf_d", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, f64, f64}, Ret: f64}},
		{Name: "uf_v", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: jsv}},
		{Name: "b_reg_i", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "b_uni_i", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "b_reg_d", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: f64}},
		{Name: "b_uni_d", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: f64}},
		{Name: "b_reg_v", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "b_uni_v", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "spin", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
	}
	_ = void
	gen.SetCallees(specs)
	// host_poll is a Go host callback, not generated; register its signature
	// so the generator marshals calls to it as a unified call.
	gen.AddCallee("host_poll", gen.Sig{Params: []gen.Type{ptr}, Ret: i64}, gen.Unified)

	// Unified flags: frame pointer kept, red zone off, signed wrap, no tail
	// calls, shrink-wrap off so every function has one canonical prologue.
	uflags := []string{"-O2", "-fno-builtin", "-fno-omit-frame-pointer", "-mno-red-zone",
		"-fwrapv", "-fno-optimize-sibling-calls", "-mllvm", "-enable-shrink-wrap=false",
		"-fcf-protection=none", "-S", "-o", "-"}
	engineAsm := must(capture(clang, append(append([]string{}, uflags...), filepath.Join(*data, "engine.c"))...))

	// Leaf flags: frame pointer omitted so the leaf is frameless.
	lflags := []string{"-O2", "-fno-builtin", "-fomit-frame-pointer", "-mno-red-zone",
		"-fwrapv", "-fcf-protection=none", "-S", "-o", "-"}
	leafAsm := must(capture(clang, append(append([]string{}, lflags...), filepath.Join(*data, "leaves.c"))...))

	var lowered strings.Builder
	lowered.WriteString(".text\n")
	var metas []gen.Meta
	amd := gen.AMD64()
	for _, s := range specs {
		var text string
		var meta gen.Meta
		var err error
		if s.Kind == gen.Register {
			text, meta, err = gen.LowerRegister(leafAsm, s)
		} else {
			text, meta, err = gen.LowerUnified(amd, engineAsm, s)
		}
		if err != nil {
			die(err)
		}
		lowered.WriteString(".globl " + s.Name + "\n.type " + s.Name + ",@function\n" + text +
			".size " + s.Name + ", .-" + s.Name + "\n")
		metas = append(metas, meta)
	}

	os.MkdirAll(*out, 0o755)
	work := *out + "_work"
	os.MkdirAll(work, 0o755)
	loweredPath := filepath.Join(work, "lowered.s")
	writeFile(loweredPath, lowered.String())
	objPath := filepath.Join(work, "lowered.o")
	if err := run(clang, "-c", loweredPath, "-o", objPath); err != nil {
		die(err)
	}

	names := gen.NameMap{}
	for _, s := range specs {
		names[s.Name] = s.Name
	}
	asm, err := gen.Emit(objPath, metas, names, "amd64")
	if err != nil {
		die(err)
	}
	// Function table: the addresses of the unified entries, in spec order.
	// Item 9 verifies these point only at unified entries.
	var unified []string
	for _, m := range metas {
		if m.Kind == gen.Unified {
			unified = append(unified, m.Name)
		}
	}
	var tbl strings.Builder
	tbl.WriteString("\n// Function table: unified entries only.\n")
	fmt.Fprintf(&tbl, "GLOBL ·unifiedTable(SB), RODATA, $%d\n", 8*len(unified))
	for i, n := range unified {
		fmt.Fprintf(&tbl, "DATA ·unifiedTable+%d(SB)/8, $·%s(SB)\n", i*8, n)
	}
	asm += tbl.String()
	writeFile(filepath.Join(*out, "engine_amd64.s"), asm)

	// Paired Go declarations.
	decls := goDecls(specs)
	decls += fmt.Sprintf("\nvar unifiedTable [%d]uintptr\n", len(unified))
	writeFile(filepath.Join(*out, "engine.go"), decls)
	fmt.Fprintln(os.Stderr, "generated", *out)

	// Static check on the engine IR: no variadics, byval, sret or long double.
	engineLL := must(capture(clang, "-O2", "-fno-builtin", "-fno-omit-frame-pointer",
		"-mno-red-zone", "-fwrapv", "-S", "-emit-llvm", "-o", "-", filepath.Join(*data, "engine.c")))
	if err := gen.CheckIR(engineLL); err != nil {
		die(fmt.Errorf("engine IR check: %w", err))
	}

	// The closed allow list: the engine's own functions and the host callbacks.
	allow := map[string]bool{"host_poll": true, "host_memory_grow": true, "host_now": true,
		"host_call": true, "host_release": true, "host_intl": true}
	for _, s := range specs {
		allow[s.Name] = true
	}
	if err := gen.CheckObject(objPath, allow); err != nil {
		die(fmt.Errorf("object check: %w", err))
	}
	fmt.Fprintln(os.Stderr, "static checks passed")

	if *bin != "" {
		reports, err := gen.VerifyLinked(objPath, *bin, "slowjsgen/proto", metas)
		if err != nil {
			die(fmt.Errorf("post-link verify: %w", err))
		}
		for _, r := range reports {
			fmt.Fprintf(os.Stderr, "verified %s: %d body bytes, %d calls, morestack=%v\n",
				r.Func, r.BodyBytes, r.Calls, r.HasMorestack)
		}
	}
}

func goDecls(specs []gen.FuncSpec) string {
	var b strings.Builder
	b.WriteString("// Code generated by internal/goasm/gen. DO NOT EDIT.\npackage proto\n\n")
	b.WriteString("// JSValue mirrors the 16-byte engine value.\ntype JSValue struct{ Tag, Val int64 }\n\n")
	for _, s := range specs {
		if s.Kind == gen.Register {
			continue // register-entry leaf: called only from assembly
		}
		b.WriteString("//go:noescape\nfunc " + s.Name + "(")
		var ps []string
		for i, p := range s.Sig.Params {
			ps = append(ps, fmt.Sprintf("a%d %s", i, goType(p)))
		}
		b.WriteString(strings.Join(ps, ", ") + ")")
		if s.Sig.Ret != gen.Void {
			b.WriteString(" " + goType(s.Sig.Ret))
		}
		b.WriteString("\n\n")
	}
	return b.String()
}

func goType(t gen.Type) string {
	switch t {
	case gen.I64:
		return "int64"
	case gen.Ptr:
		return "uintptr"
	case gen.F64:
		return "float64"
	case gen.JSV:
		return "JSValue"
	}
	return ""
}

func capture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, errb.String())
	}
	return out.String(), nil
}
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}
func must(s string, err error) string {
	if err != nil {
		die(err)
	}
	return s
}
func writeFile(p, s string) {
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		die(err)
	}
}
func die(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
