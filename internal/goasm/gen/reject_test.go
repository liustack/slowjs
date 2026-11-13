// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRejectSamples runs each sample in testdata/reject through the real
// pipeline (clang, IR checks, shadow-stack pass, llc, object checks,
// lowering) and requires the named rule to stop it. It needs LLVM 22.1.8
// (QUICKJS_LLVM, a bin dir) and the built pass (QUICKJS_SHADOW_PASS).
func TestRejectSamples(t *testing.T) {
	llvm, pass := os.Getenv("QUICKJS_LLVM"), os.Getenv("QUICKJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set QUICKJS_LLVM and QUICKJS_SHADOW_PASS")
	}
	cases := map[string]string{
		"writable_bss.c":           "writable data section",
		"writable_data.c":          "writable data section",
		"thread_local.c":           "writable data section",
		"external.c":               "outside the allow list",
		"tail_call.c":              "tail call",
		"variadic.c":               "variadic",
		"byval.c":                  "byval",
		"sret.c":                   "sret",
		"inline_asm.c":             "inline assembly",
		"alloca_aligned_fixed.c":   "local aligned beyond 16",
		"alloca_aligned_dynamic.c": "local aligned beyond 16",
		"heap_aligned.c":           "an access aligned to 16 bytes to memory that is not a local or a global",
		"merged_aligned.c":         "an access aligned to 16 bytes to memory that is not a local or a global",
		"rodata_table.c":           "unsupported read-only reference",
		"format_float.c":           "format string",
		"format_variable.c":        "format string",
	}
	// Files compiled with the flags of quickjs.c, as the engine compiles it:
	// with no SLP vectorizer, llc and not the optimizer merges the reads.
	valueFlags := map[string]bool{"merged_aligned.c": true}
	for file, want := range cases {
		t.Run(file, func(t *testing.T) {
			cflags := EngineCFlags
			if valueFlags[file] {
				cflags = append(append([]string{}, EngineCFlags...), EngineValueFlags...)
			}
			err := pipeline(llvm, pass, filepath.Join("testdata", "reject", file), t.TempDir(), cflags)
			if err == nil {
				t.Fatalf("accepted, want rejection by %q", want)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejected by the wrong rule: %v (want %q)", err, want)
			}
			t.Logf("rejected: %v", err)
		})
	}
}

func pipeline(llvm, pass, cfile, work string, cflags []string) error {
	u, err := CompileUnit(UnitConfig{LLVM: llvm, Pass: pass, Work: work, CFlags: cflags}, cfile)
	if err != nil {
		return err
	}
	allow := map[string]bool{}
	for _, h := range append(append([]string{}, HostCallbacks...), GeneratorCallbacks...) {
		allow[h] = true
	}
	asm, err := os.ReadFile(u.Asm)
	if err != nil {
		return err
	}
	fns := Functions(string(asm))
	var specs []FuncSpec
	for _, f := range fns {
		allow[f] = true
		specs = append(specs, FuncSpec{Name: f, Kind: Unified, Sig: Sig{Params: []Type{Ptr, I64}, Ret: I64}})
	}
	if err := CheckObject(u.Obj, allow); err != nil {
		return err
	}
	SetCallees(specs)
	for _, s := range specs {
		if _, _, err := LowerUnified(AMD64(), string(asm), s); err != nil {
			return err
		}
	}
	return nil
}
