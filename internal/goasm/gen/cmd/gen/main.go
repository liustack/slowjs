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
	pass := flag.String("pass", "", "shadow-stack pass plugin (pass/shadow.cpp built as a shared object)")
	pkg := flag.String("pkg", "github.com/liustack/slowjs/internal/goasm/gen/internal/proto", "Go import path of the generated package")
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
		// Shadow stack, reentry, interrupts.
		{Name: "take_addr", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, jsv, f64, ptr}, Ret: i64}},
		{Name: "addr_param", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64, f64}, Ret: i64}},
		{Name: "nop_call", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "vec_spill", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, f64, f64}, Ret: f64}},
		{Name: "reenter", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}},
		{Name: "shrink_frame", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}},
		{Name: "fragment", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64, ptr}, Ret: i64}},
		{Name: "outer_loop", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64}},
		{Name: "deep", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "spin_every", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}},
		{Name: "dyn_alloc", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
	}
	_ = void
	gen.SetCallees(specs)
	// host_poll is a Go host callback, not generated; register its signature
	// so the generator marshals calls to it as a unified call.
	gen.AddCallee("host_poll", gen.Sig{Params: []gen.Type{ptr}, Ret: i64}, gen.Unified)
	gen.AddCallee("host_call", gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}, gen.Unified)
	var leaves []string
	for _, s := range specs {
		if s.Kind == gen.Register {
			leaves = append(leaves, s.Name)
		}
	}

	work := filepath.Join(filepath.Dir(*out), "_"+filepath.Base(*out)+"_work") // ignored by go tooling
	os.MkdirAll(work, 0o755)

	// Unified functions: clang -O2 to IR, the IR checks, the shadow-stack
	// pass, then llc.
	unit, err := gen.CompileUnit(*llvm, *pass, filepath.Join(*data, "engine.c"), work, leaves)
	if err != nil {
		die(err)
	}
	writeFile(filepath.Join(work, "shadow.txt"), unit.Shadow)
	engineObj := unit.Obj
	engineAsm := readFile(unit.Asm)

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
		// Each function starts 16-aligned, as PCALIGN puts it in Go, so the
		// alignment of in-body constants carries over.
		lowered.WriteString(".p2align 4\n.globl " + s.Name + "\n.type " + s.Name + ",@function\n" + text +
			".size " + s.Name + ", .-" + s.Name + "\n")
		metas = append(metas, meta)
	}

	os.MkdirAll(*out, 0o755)
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

	// The closed allow list: the engine's own functions and the host callbacks.
	allow := map[string]bool{"host_poll": true, "host_memory_grow": true, "host_now": true,
		"host_call": true, "host_release": true, "host_intl": true}
	for _, s := range specs {
		allow[s.Name] = true
	}
	// Both the engine object as LLVM wrote it (its data sections and every
	// symbol it references) and the lowered object.
	for _, o := range []string{engineObj, objPath} {
		if err := gen.CheckObject(o, allow); err != nil {
			die(fmt.Errorf("object check %s: %w", filepath.Base(o), err))
		}
	}
	fmt.Fprintln(os.Stderr, "static checks passed")

	if *bin != "" {
		reports, err := gen.VerifyLinked(objPath, *bin, *pkg, metas)
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

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		die(err)
	}
	return string(b)
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
