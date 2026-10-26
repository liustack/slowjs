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
	arch := flag.String("arch", "amd64", "target: amd64 or arm64")
	mode := flag.String("mode", "body", "how a unified call keeps registers: body (the ones the caller's body uses), all, pn (preserve_none), stub (arm64: all, in a stub)")
	flag.Parse()
	noSaves := os.Getenv("GEN_NO_SAVES") != "" // negative build: must fail the behavior tests

	clang := filepath.Join(*llvm, "clang")
	if err := run(clang, "--version"); err != nil {
		die(err)
	}

	// Signatures and kinds of every function the unit references.
	ptr, i64, f64, jsv, void := gen.Ptr, gen.I64, gen.F64, gen.JSV, gen.Void
	sig5 := gen.Sig{Params: []gen.Type{ptr, i64, i64, i64, i64}, Ret: i64}
	sig6 := gen.Sig{Params: []gen.Type{ptr, i64, i64, i64, i64, i64}, Ret: i64}
	sig7 := gen.Sig{Params: []gen.Type{ptr, i64, i64, i64, i64, i64, i64}, Ret: i64}
	sig7v := gen.Sig{Params: []gen.Type{ptr, jsv, f64, i64, jsv, i64}, Ret: i64}
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
		{Name: "b_reg_p", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
		{Name: "b_uni_p", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
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
		// Argument registers, live values and frame sizes.
		{Name: "s5", Kind: gen.Unified, Sig: sig5},
		{Name: "s6", Kind: gen.Unified, Sig: sig6},
		{Name: "s7", Kind: gen.Unified, Sig: sig7},
		{Name: "s7v", Kind: gen.Unified, Sig: sig7v},
		{Name: "slots_ind", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64},
			Indirect: []gen.Sig{sig5, sig6, sig7, sig7v}},
		{Name: "live_mix", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64, f64}, Ret: i64}},
		{Name: "big_f0", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64}},
		{Name: "big_4k", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64}},
		{Name: "big_caller", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64}},
		// Faults on purpose.
		{Name: "peek", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr}, Ret: i64}},
		{Name: "poke", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, ptr, i64}, Ret: i64}},
		{Name: "hit_end", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr}, Ret: i64}},
		{Name: "trap_now", Kind: gen.Unified, Sig: gen.Sig{Params: []gen.Type{ptr, i64}, Ret: i64}},
	}
	_ = void
	// The integer argument registers a unified entry may use: SysV has six,
	// AAPCS eight, preserve_none ten on amd64 (R12 to R9: R11 is kept for
	// calls through memory) and six on arm64 (x20 to x25: the next ones are
	// x26 to x28, which the engine reserves). The pass
	// moves the arguments of a function over the limit into a block, and the
	// function becomes f(sp, blk).
	pn := *mode == "pn"
	maxInt := map[string]int{"amd64": 6, "arm64": 8}[*arch]
	if pn {
		maxInt = map[string]int{"amd64": 10, "arm64": 6}[*arch]
	}
	if maxInt == 0 {
		die(fmt.Errorf("unknown -arch %q", *arch))
	}
	if os.Getenv("GEN_NO_PACK") != "" {
		maxInt = 0 // negative build: the 7th argument must reach a reserved register and fail the scan
	}
	packed := func(s gen.Sig) gen.Sig {
		if maxInt > 0 && intSlots(s) > maxInt {
			return gen.Sig{Params: []gen.Type{ptr, ptr}, Ret: s.Ret}
		}
		return s
	}
	for i := range specs {
		if specs[i].Kind == gen.Register {
			continue
		}
		specs[i].Sig = packed(specs[i].Sig)
		for j := range specs[i].Indirect {
			specs[i].Indirect[j] = packed(specs[i].Indirect[j])
		}
	}
	gen.SetCallees(specs)
	// host_poll is a Go host callback, not generated; register its signature
	// so the generator marshals calls to it as a unified call.
	gen.AddCallee("host_poll", gen.Sig{Params: []gen.Type{ptr}, Ret: i64}, gen.Unified)
	gen.AddCallee("host_call", gen.Sig{Params: []gen.Type{ptr, i64, i64}, Ret: i64}, gen.Unified)
	gen.AddCallee("host_trap", gen.Sig{Params: []gen.Type{ptr}}, gen.Unified)
	var leaves []string
	for _, s := range specs {
		if s.Kind == gen.Register {
			leaves = append(leaves, s.Name)
		}
	}

	work := filepath.Join(filepath.Dir(*out), "_"+filepath.Base(*out)+"_work") // ignored by go tooling
	os.MkdirAll(work, 0o755)

	cflags := gen.EngineCFlags
	if *arch == "arm64" {
		cflags = gen.ARM64CFlags
	}
	passArgs := []string{fmt.Sprintf("-shadow-max-int-args=%d", maxInt)}
	if pn {
		passArgs = append(passArgs, "-shadow-cc=preserve_none")
	}
	// Unified functions: clang -O2 to IR, the IR checks, the shadow-stack
	// pass, then llc.
	unit, err := gen.CompileUnit(gen.UnitConfig{LLVM: *llvm, Pass: *pass, Work: work, CFlags: cflags,
		PassArgs: passArgs, Leaves: leaves}, filepath.Join(*data, "engine.c"))
	if err != nil {
		die(err)
	}
	writeFile(filepath.Join(work, "shadow.txt"), unit.Shadow)
	engineObj := unit.Obj
	engineAsm := readFile(unit.Asm)
	allow := map[string]bool{"host_poll": true, "host_memory_grow": true, "host_now": true,
		"host_call": true, "host_release": true, "host_trap": true}
	for _, s := range specs {
		allow[s.Name] = true
	}
	if *arch == "arm64" {
		genARM64(clang, *data, *out, work, engineObj, engineAsm, specs, *mode, noSaves, allow)
		return
	}
	amd := gen.AMD64()
	switch *mode {
	case "body":
	case "all":
		amd.SaveAll = true
	case "pn":
		amd.PreserveNone = true
	default:
		die(fmt.Errorf("unknown amd64 -mode %q", *mode))
	}
	if noSaves && *mode != "body" {
		die(fmt.Errorf("GEN_NO_SAVES goes with -mode body"))
	}

	// Leaf flags: frame pointer omitted so the leaf is frameless.
	lflags := []string{"-O2", "-fno-builtin", "-fomit-frame-pointer", "-mno-red-zone",
		"-fwrapv", "-fcf-protection=none", "-S", "-o", "-"}
	leafAsm := must(capture(clang, append(append([]string{}, lflags...), filepath.Join(*data, "leaves.c"))...))

	var lowered strings.Builder
	lowered.WriteString(".text\n")
	var metas []gen.Meta
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
	writeFile(filepath.Join(*out, "engine.go"), goDecls(specs, unified, "amd64", *mode))
	fmt.Fprintln(os.Stderr, "generated", *out)
	reportSizes(objPath)

	// The closed allow list: the engine's own functions and the host callbacks.
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

// intSlots counts the integer argument registers of a signature.
func intSlots(s gen.Sig) int {
	n := 0
	for _, p := range s.Params {
		switch p {
		case gen.I64, gen.Ptr:
			n++
		case gen.JSV:
			n += 2
		}
	}
	return n
}

// reportSizes prints the code size of the call-overhead loops and of the
// whole lowered unit.
func reportSizes(obj string) {
	sizes, err := gen.FuncSizes(obj)
	if err != nil {
		die(err)
	}
	total := 0
	for _, n := range sizes {
		total += n
	}
	fmt.Fprintf(os.Stderr, "CODE-SIZE b_uni_i=%d b_uni_d=%d b_uni_v=%d b_uni_p=%d b_reg_i=%d b_reg_p=%d live_mix=%d slots_ind=%d big_caller=%d total=%d\n",
		sizes["b_uni_i"], sizes["b_uni_d"], sizes["b_uni_v"], sizes["b_uni_p"], sizes["b_reg_i"], sizes["b_reg_p"],
		sizes["live_mix"], sizes["slots_ind"], sizes["big_caller"], total)
}

func goDecls(specs []gen.FuncSpec, unified []string, arch, mode string) string {
	var b strings.Builder
	b.WriteString("// Code generated by internal/goasm/gen. DO NOT EDIT.\npackage proto\n\n")
	fmt.Fprintf(&b, "// genArch and genMode say how this package was generated.\nconst genArch, genMode = %q, %q\n\n", arch, mode)
	fmt.Fprintf(&b, "var unifiedTable [%d]uintptr\n\n// unifiedIndex gives each unified entry's index in unifiedTable.\nvar unifiedIndex = map[string]int{", len(unified))
	for i, n := range unified {
		fmt.Fprintf(&b, "%q: %d, ", n, i)
	}
	b.WriteString("}\n\n")
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

// genARM64 lowers, assembles and emits the arm64 package.
func genARM64(clang, data, out, work, engineObj, engineAsm string, specs []gen.FuncSpec, mode string, noSaves bool, allow map[string]bool) {
	m, err := gen.ParseARM64Mode(mode)
	if err != nil {
		die(err)
	}
	leafAsm := must(capture(clang, append(append([]string{}, gen.ARM64CFlags...), "-S", "-o", "-", filepath.Join(data, "leaves.c"))...))
	u, err := gen.LowerARM64(engineAsm, leafAsm, specs, m, noSaves)
	if err != nil {
		die(err)
	}
	loweredPath := filepath.Join(work, "lowered_arm64.s")
	writeFile(loweredPath, u.Lowered)
	objPath := filepath.Join(work, "lowered_arm64.o")
	os.Remove(objPath)
	if err := run(filepath.Join(filepath.Dir(clang), "llvm-mc"), "-triple=aarch64-linux-gnu", "-filetype=obj", "-o", objPath, loweredPath); err != nil {
		die(err)
	}
	asm, err := gen.EmitARM64(objPath, u, "arm64")
	if err != nil {
		die(err)
	}
	os.MkdirAll(out, 0o755)
	writeFile(filepath.Join(out, "engine_arm64.s"), asm)
	var unified []string
	for _, s := range specs {
		if s.Kind == gen.Unified {
			unified = append(unified, s.Name)
		}
	}
	writeFile(filepath.Join(out, "engine.go"), goDecls(specs, unified, "arm64", mode))
	if err := gen.CheckObject(engineObj, allow); err != nil {
		die(fmt.Errorf("object check engine: %w", err))
	}
	fmt.Fprintf(os.Stderr, "generated %s (arm64, mode %s): %d unified call sites, %d register saves at them\n",
		out, mode, u.CallSites, u.SavedRegs)
	reportSizes(objPath)
}
