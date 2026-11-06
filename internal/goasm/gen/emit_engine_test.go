// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEmitLeafFrame: a leaf that saves a callee-saved register must be
// emitted with a frame for that save, through either emit entry, or its
// store would overwrite the return address. Needs LLVM 22.1.8 (QUICKJS_LLVM).
func TestEmitLeafFrame(t *testing.T) {
	llvm := os.Getenv("QUICKJS_LLVM")
	if llvm == "" {
		t.Skip("set QUICKJS_LLVM")
	}
	fn := FuncSpec{Name: "savingleaf", Sig: Sig{Params: []Type{I64}, Ret: I64}, Kind: Register}
	SetCallees([]FuncSpec{fn})
	asm := "savingleaf:\n\tpushq\t%rbx\n\tmovq\t%rdi, %rbx\n\tmovq\t%rbx, %rax\n\tpopq\t%rbx\n\tretq\n.Lfunc_end0:\n"
	text, m, err := LowerRegister(asm, fn)
	if err != nil {
		t.Fatal(err)
	}
	if m.Frame != 8 {
		t.Fatalf("frame %d, want 8", m.Frame)
	}
	work := t.TempDir()
	srcPath := filepath.Join(work, "l.s")
	if err := os.WriteFile(srcPath, []byte(".text\n.p2align 4\n.globl savingleaf\n.type savingleaf,@function\n"+
		text+".size savingleaf, .-savingleaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(work, "l.o")
	if out, err := exec.Command(filepath.Join(llvm, "clang"), "--target=x86_64-linux-gnu", "-c", srcPath, "-o", obj).CombinedOutput(); err != nil {
		t.Fatalf("assemble: %v\n%s", err, out)
	}
	asmGo, err := Emit(obj, []Meta{m}, NameMap{"savingleaf": m.GoName}, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asmGo, "TEXT ·savingleaf(SB), NOSPLIT, $8-0") {
		i := strings.Index(asmGo, "TEXT ·savingleaf")
		t.Fatalf("leaf frame not declared:\n%s", asmGo[i:i+60])
	}
}

// TestEmitNoStubs: a release build never emits a stub. EmitEngine stops on
// one before it reads the object (the path here does not exist), naming the
// functions, and without NoStubs the same input gets past that check.
func TestEmitNoStubs(t *testing.T) {
	e := EngineEmit{Stubs: []Meta{{Name: "js_atomics_pause"}, {Name: "other"}}, Arch: "amd64"}
	_, err := EmitEngine("no-such-object.o", e)
	if err == nil || strings.Contains(err.Error(), "release") {
		t.Fatalf("without NoStubs: %v", err)
	}
	e.NoStubs = true
	_, err = EmitEngine("no-such-object.o", e)
	if err == nil || !strings.Contains(err.Error(), "no stub") || !strings.Contains(err.Error(), "js_atomics_pause other") {
		t.Fatalf("with NoStubs: %v", err)
	}
}

// TestBuildExpr: the build line of a generated .s file is the architecture,
// and with a tag set it also requires the tag, so that a package whose files
// all need the tag has no file in a build without it.
func TestBuildExpr(t *testing.T) {
	for _, c := range []struct{ tag, arch, want string }{
		{"", "amd64", "amd64"},
		{"", "arm64", "arm64"},
		{"quickjs_cengine", "amd64", "quickjs_cengine && amd64"},
		{"quickjs_cengine", "arm64", "quickjs_cengine && arm64"},
	} {
		if got := BuildExpr(c.tag, c.arch); got != c.want {
			t.Errorf("BuildExpr(%q, %q) = %q, want %q", c.tag, c.arch, got, c.want)
		}
	}
}

// TestFuncPlace: a function starts in the lowered object where its body will
// sit in the binary, on 16 bytes or at its -align64 place in a line of 64, so
// that the alignment of its constant pool holds where the code runs.
func TestFuncPlace(t *testing.T) {
	a64 := map[string]int{"interp": 40, "matcher": 16, "zero": 0}
	for name, want := range map[string]string{
		"plain":   ".p2align 4\n",
		"interp":  ".p2align 6\n.skip 40\n",
		"matcher": ".p2align 6\n.skip 16\n",
		"zero":    ".p2align 6\n",
	} {
		if got := FuncPlace(a64, name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if got := FuncPlace(nil, "interp"); got != ".p2align 4\n" {
		t.Errorf("no -align64: %q", got)
	}
}

// TestCheckBodyDataAlign: the generator stops at data in the body of a
// function that asks for more alignment than FuncPlace keeps (16 bytes, 64 for
// a body put on 64 bytes), also when the post-link check would find it on its
// alignment by luck.
func TestCheckBodyDataAlign(t *testing.T) {
	plan := &DataPlan{InBody: map[string][]*DataObj{
		"plain":   {{Label: ".LCPI0_0", Align: 4}, {Label: ".LJTI0_0", Align: 3}},
		"wide":    {{Label: ".LCPI1_0", Align: 5}},
		"placed":  {{Label: ".LCPI2_0", Align: 6}},
		"toowide": {{Label: ".LCPI3_0", Align: 7}},
		"other":   {{Label: ".LCPI4_0", Align: 6}}, // not lowered: not looked at
	}}
	a64 := map[string]int{"placed": 40, "toowide": 8}
	metas := func(names ...string) []Meta {
		var ms []Meta
		for _, n := range names {
			ms = append(ms, Meta{Name: n})
		}
		return ms
	}
	if err := CheckBodyDataAlign(plan, metas("plain", "placed"), a64); err != nil {
		t.Errorf("data within what is kept: %v", err)
	}
	for name, want := range map[string]string{
		"wide":    "wide: .LCPI1_0 in its body asks for 32 bytes of alignment, and the body keeps 16",
		"toowide": "toowide: .LCPI3_0 in its body asks for 128 bytes of alignment, and the body keeps 64",
	} {
		if err := CheckBodyDataAlign(plan, metas("plain", name), a64); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", name, err, want)
		}
	}
	// A body put on 64 bytes keeps 64, a plain one 16.
	if got := [2]int{BodyDataAlign(a64, "placed"), BodyDataAlign(a64, "plain")}; got != [2]int{6, 4} {
		t.Errorf("kept alignments %v, want [6 4]", got)
	}
}

// TestRoomCheck: every unified function has the room check for a fault on
// windows after its TEXT, inside #ifdef GOOS_windows, so that the code of
// the other systems is what it was. A function on the NOSPLIT list loads g
// first, and one without a frame says NOFRAME, so that the call of
// goasm_room does not make the assembler save BP. A register-entry leaf has
// none: its callers have it. Room of internal/faulttest, which the tests that
// fault in Go code call, has the frame of goasm_room.
func TestRoomCheck(t *testing.T) {
	check := fmt.Sprintf("#ifdef GOOS_windows\n\tLEAQ -%d(SP), R12\n\tCMPQ R12, 16(R14)\n\tJHI 2(PC)\n\tCALL ·goasm_room(SB)\n\tBYTE $0x90\n#endif\n", RoomBelow)
	leafCheck := strings.Replace(check, "#ifdef GOOS_windows\n", "#ifdef GOOS_windows\n\tMOVQ (TLS), R14\n", 1)
	for _, c := range []struct {
		m    Meta
		want string
	}{
		{Meta{Kind: Unified, Frame: 48, ArgBytes: 24}, "TEXT ·f(SB), $48-24\n\tNO_LOCAL_POINTERS\n" + check},
		{Meta{Kind: Unified, NoSplit: true, ArgBytes: 24}, "TEXT ·f(SB), NOSPLIT|NOFRAME, $0-24\n\tNO_LOCAL_POINTERS\n" + leafCheck},
		{Meta{Kind: Unified, NoSplit: true, Frame: 16, ArgBytes: 24}, "TEXT ·f(SB), NOSPLIT, $16-24\n\tNO_LOCAL_POINTERS\n" + leafCheck},
		{Meta{Kind: Register, NoSplit: true, Frame: 8}, "TEXT ·f(SB), NOSPLIT, $8-0\n"},
	} {
		if got := funcHeader("f", c.m); got != c.want {
			t.Errorf("funcHeader of %+v:\n%s\nwant\n%s", c.m, got, c.want)
		}
	}
	if RoomBelow+5024-136 < 16<<10 {
		t.Fatalf("a register-entry leaf has %d bytes of room, less than 16 KiB", RoomBelow+5024-136)
	}
	if !strings.Contains(roomFunc(), fmt.Sprintf("TEXT ·goasm_room(SB), $%d-0\n", RoomFrame)) {
		t.Fatalf("goasm_room:\n%s", roomFunc())
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "faulttest", "room_windows_amd64.s"))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("TEXT ·Room(SB), $%d-0\n", RoomFrame); !strings.Contains(string(b), want) {
		t.Fatalf("internal/faulttest/room_windows_amd64.s has no %q", want)
	}
}

// TestRoomCheckAssembles: the assembler of windows/amd64 takes a function
// with the room check before the PCALIGN of a body long enough that Go's
// own stack check needs the long form of its branch. Go before 1.26.11 and
// 1.27.4 loops forever when the branch of the room check lands on the
// PCALIGN (golang/go#81792).
func TestRoomCheckAssembles(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("#include \"textflag.h\"\n#include \"funcdata.h\"\n\n")
	b.WriteString(funcHeader("f", Meta{Kind: Unified, Frame: 64, ArgBytes: 32}))
	b.WriteString("\tPCALIGN $16\n")
	for range 64 {
		b.WriteString("\tQUAD $0\n")
	}
	b.WriteString("\tRET\n")
	b.WriteString(roomFunc())
	src := filepath.Join(dir, "f.s")
	if err := os.WriteFile(src, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	env, err := exec.Command("go", "env", "GOROOT", "GOTOOLDIR").Output()
	if err != nil {
		t.Fatal(err)
	}
	dirs := strings.Fields(string(env))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// The asm binary itself, since go tool asm leaves it running when killed.
	cmd := exec.CommandContext(ctx, filepath.Join(dirs[1], "asm"), "-p", "x", "-D", "GOOS_windows", "-D", "GOARCH_amd64",
		"-I", filepath.Join(dirs[0], "pkg", "include"),
		"-o", filepath.Join(dir, "f.o"), src)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("the assembler of windows/amd64 did not finish in a minute")
	}
	if err != nil {
		t.Fatalf("assemble: %v\n%s", err, out)
	}
}
