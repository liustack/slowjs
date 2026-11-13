// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBodyStartSkipsTheFrameSetUpAndTheNOPs(t *testing.T) {
	// 0x1c bytes of frame set-up, PCALIGN padding to 64, five 8-byte NOPs, then
	// 8 bytes of NOP from xform and the body.
	code := make([]byte, 0, 200)
	code = append(code, make([]byte, 0x1c)...)
	for i := range code {
		code[i] = 0x48
	}
	for len(code) < 64 {
		code = append(code, 0x90)
	}
	for range 6 {
		code = append(code, nops[8]...)
	}
	code = append(code, 0x48, 0x89, 0xe5)
	if got := bodyStart(0x6f7e00, code); got != 64+48 {
		t.Errorf("body at %d, want %d", got, 64+48)
	}
	if got := bodyStart(0x1000, append(append([]byte{}, nops[3]...), 0x55)); got != 3 {
		t.Errorf("body at %d, want 3", got)
	}
	// the register loop: Go's prologue, its SUBQ of SP, then the NOPs of a
	// PCALIGN $16 and of a front of 8
	prologue := []byte{0x64, 0x4c, 0x8b, 0x34, 0x25, 0xf8, 0xff, 0xff, 0xff, 0x4c, 0x8d, 0x64, 0x24, 0x80, 0x4d, 0x3b, 0x66, 0x10,
		0x0f, 0x86, 0, 0, 0, 0, 0x55, 0x48, 0x89, 0xe5, 0x48, 0x81, 0xec, 0, 1, 0, 0}
	reg := append(append(append(append(append([]byte{}, prologue...), nop9...), nops[1]...), nops[8]...), 0x4c, 0x8b)
	if got, want := bodyStart(0x1020, reg), uint64(len(prologue)+18); got != want {
		t.Errorf("body of the register loop at %d, want %d", got, want)
	}
}

// TestOpcodeNames reads the instructions of the register loop in the order of
// their enum, from a small engine folder and from engine/, which has each of
// HotOps.
func TestOpcodeNames(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"regvm-areas.h": "/* #include \"regvm-ops-x.h\" */\n#include \"regvm-ops-b.h\"\n#include \"regvm-ops-a.h\"\n",
		"regvm-ops-b.h": "#if RV_PART == RV_PART_LIST\nRV_OP(MOV)     /* d s */\nRV_OP(GOTO) RV_OP(JT)\n#elif RV_PART == RV_PART_RUN\n RV_OP(NOT)\nRV_OP(NOTE)\n#endif\n",
		"regvm-ops-a.h": "#if RV_PART == RV_PART_INFO\nRV_OP(INFO)\n#elif RV_PART == RV_PART_LIST\nRV_OP(CALL)\n#endif\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := OpcodeNames(dir)
	if err != nil || fmt.Sprint(names) != "[MOV GOTO JT CALL]" {
		t.Errorf("%v %v", names, err)
	}
	names, err = OpcodeNames(filepath.Join("..", "..", "..", "engine"))
	if err != nil {
		t.Fatal(err)
	}
	if names[0] != "MOV" || len(names) > 256 {
		t.Errorf("%d instructions, the first %s", len(names), names[0])
	}
	for _, op := range HotOps {
		if !slices.Contains(names, op) {
			t.Errorf("%s of HotOps is no instruction of the register loop", op)
		}
	}
}

// formalLayout is the formal binary of all2.tsv, with the regexp matcher, the
// engine functions between it and the interpreter, and one on each side.
var formalLayout = Layout{
	Func: 0x6f7e00, Body: 0x6f7e68, Size: 9000, Handlers: map[string]uint64{"MOV": 0x6f804d, "CALL": 0x6f8129},
	Matcher: 0x6f0000,
	Funcs: map[string]uint64{
		"first": 0x600000, "heap_alloc": 0x640020, "re_parse_error": 0x6ea0c0, "lre_exec_backtrack": 0x6f0000,
		"js_malloc_rt": 0x6f4b20, "js_free_rt": 0x6f4e60, "JS_NewObjectFromShape": 0x6f6c80,
		"JS_RegvmRun_block": 0x6f7e00, "JS_SetPropertyInternal_block": 0x6f9a00,
	},
}

// moved is the binary at p when the matcher moved by dm bytes and the
// interpreter by d, with every offset in the interpreter front later. The
// functions between the two moved by dm plus the second shift of p, and the
// ones between the first engine function and the matcher by the shift of p.
func moved(p Position, dm, d uint64) Layout {
	l := Layout{Func: formalLayout.Func + d, Body: formalLayout.Body + d + uint64(p.Front), Size: formalLayout.Size + uint64(p.Front),
		Handlers: map[string]uint64{}, Matcher: formalLayout.Matcher + dm, Funcs: map[string]uint64{}}
	for op, a := range formalLayout.Handlers {
		l.Handlers[op] = a + d + uint64(p.Front)
	}
	for name, a := range formalLayout.Funcs {
		switch {
		case a > formalLayout.Matcher && a < formalLayout.Func:
			a += dm + uint64(p.Shift2)
		case a >= formalLayout.Func:
			a += d + uint64(p.Front)
		case a > formalLayout.Funcs["first"] && a < formalLayout.Matcher:
			a += uint64(p.Shift)
		case a == formalLayout.Matcher:
			a += dm
		}
		l.Funcs[name] = a
	}
	return l
}

func TestCheckLayoutTakesWhatTheBinariesShowed(t *testing.T) {
	// The moves of all2.tsv: a shift lands on the next 64 bytes, and the
	// interpreter moves with the matcher.
	for _, c := range []struct {
		p     Position
		dm, d uint64
	}{
		{Position{0, 0, 0}, 0, 0}, {Position{0, 8, 0}, 0, 0}, {Position{32, 0, 0}, 64, 64}, {Position{1664, 8, 0}, 1664, 1664},
		{Position{3296, 16, 0}, 3328, 3328}, {Position{1120, 48, 0}, 1152, 1152}, {Position{1120, 48, 0}, 1088, 1088}, {Position{64, 0, 0}, 64, 64},
		// a second shift moves the functions between the two by exactly its
		// size, and the interpreter to the multiple of 64 within 32 of it
		{Position{0, 0, 2464}, 0, 2496}, {Position{0, 0, 2464}, 0, 2432}, {Position{0, 8, 2432}, 0, 2432},
		{Position{576, 24, 3328}, 576, 3904}, {Position{64, 0, 2208}, 64, 2240}, {Position{64, 0, 2208}, 64, 2304},
		// 32 more can be taken up by the alignment of the interpreter, and the
		// functions before it have still moved
		{Position{0, 0, 32}, 0, 0}, {Position{0, 0, 32}, 0, 64},
		// an odd multiple of 32 as the first shift: the matcher goes down or up to
		// a multiple of 64, and the functions before it move by the shift itself
		{Position{1376, 8, 1120}, 1344, 2432}, {Position{1376, 8, 1120}, 1408, 2496}, {Position{96, 0, 32}, 64, 64}, {Position{96, 0, 32}, 128, 128},
	} {
		if err := CheckLayout(formalLayout, moved(c.p, c.dm, c.d), c.p, "first"); err != nil {
			t.Errorf("%s (matcher %d, interpreter %d): %v", c.p, c.dm, c.d, err)
		}
	}
}

// TestCheckLayoutInterpreterOn32: the register loop has no PCALIGN $64, so the
// linker puts it on 32 bytes, and it moves with the functions before it by the
// shifts exactly, an odd multiple of 32 too.
func TestCheckLayoutInterpreterOn32(t *testing.T) {
	formal := formalLayout
	formal.Funcs = maps.Clone(formalLayout.Funcs)
	off := uint64(32)
	formal.Func += off
	formal.Body += off
	formal.Handlers = map[string]uint64{}
	for op, a := range formalLayout.Handlers {
		formal.Handlers[op] = a + off
	}
	for _, f := range []string{"JS_RegvmRun_block"} {
		if _, ok := formal.Funcs[f]; ok {
			formal.Funcs[f] += off
		}
	}
	shifted := func(p Position, dm, d uint64) Layout {
		l := moved(p, dm, d)
		l.Func += off
		l.Body += off
		for op := range l.Handlers {
			l.Handlers[op] += off
		}
		return l
	}
	p := Position{1376, 8, 1120}
	if err := CheckLayout(formal, shifted(p, 1344, 2464), p, "first"); err != nil {
		t.Errorf("moved with the functions before it: %v", err)
	}
	if err := CheckLayout(formal, shifted(p, 1344, 2496), p, "first"); err == nil {
		t.Error("an interpreter on 32 bytes that moved to the next 64 is taken")
	}
	q := Position{1376, 8, 0}
	if err := CheckLayout(formal, shifted(q, 1344, 1344), q, "first"); err != nil {
		t.Errorf("no second shift: %v", err)
	}
	// on 32 bytes but on a multiple of 64 in the formal binary, by chance
	if err := CheckLayout(formalLayout, moved(p, 1344, 2464), p, "first"); err != nil {
		t.Errorf("on 32 bytes, formal on 64: %v", err)
	}
}

func TestCheckLayoutFindsAWrongLanding(t *testing.T) {
	p := Position{1664, 8, 0}
	q := Position{64, 8, 2208}
	for name, c := range map[string]struct {
		l Layout
		p Position
	}{
		"not moved":           {moved(p, 0, 0), p},
		"moved without shift": {moved(Position{0, 8, 0}, 64, 64), Position{0, 8, 0}},
		"moved too far":       {moved(p, 1728, 1728), p},
		"moved by 32":         {moved(p, 1696, 1696), p},
		"short of shift":      {moved(p, 1600, 1600), p},
		"interpreter lags":    {moved(p, 1664, 1600), p},
		"front not there":     {moved(Position{1664, 0, 0}, 1664, 1664), p},
		"handler moved":       {withHandler(moved(p, 1664, 1664), "CALL", 64), p},
		"handler missing":     {withoutHandler(moved(p, 1664, 1664), "CALL"), p},
		"before the matcher lagged": {func() Layout {
			l := moved(Position{1376, 8, 0}, 1344, 1344)
			l.Funcs["heap_alloc"] = formalLayout.Funcs["heap_alloc"] + 1344
			return l
		}(), Position{1376, 8, 0}},
		"before the matcher missing": {func() Layout {
			l := moved(p, 1664, 1664)
			delete(l.Funcs, "re_parse_error")
			return l
		}(), p},
		"first function moved": {func() Layout {
			l := moved(p, 1664, 1664)
			l.Funcs["first"] += 1664
			return l
		}(), p},
		"second shift ignored": {moved(Position{64, 8, 0}, 64, 64), q},
		"second shift only in the interpreter": {func() Layout {
			l := moved(Position{64, 8, 0}, 64, 2240)
			return l
		}(), q},
		"second shift too big":           {moved(Position{64, 8, 4416}, 64, 4480), q},
		"second shift moved the matcher": {moved(q, 2272, 2304), q},
		"interpreter did not follow":     {moved(q, 64, 64), q},
		"a function between lagged": {func() Layout {
			l := moved(q, 64, 2240)
			l.Funcs["js_free_rt"] -= 32
			return l
		}(), q},
		"a function between missing": {func() Layout {
			l := moved(q, 64, 2240)
			delete(l.Funcs, "js_malloc_rt")
			return l
		}(), q},
	} {
		if err := CheckLayout(formalLayout, c.l, c.p, "first"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Nothing between the matcher and the interpreter to check is an error too.
	empty := formalLayout
	empty.Funcs = map[string]uint64{"first": formalLayout.Funcs["first"], "lre_exec_backtrack": formalLayout.Matcher, "JS_RegvmRun_block": formalLayout.Func}
	if err := CheckLayout(empty, empty, Position{}, ""); err == nil || !strings.Contains(err.Error(), "between") {
		t.Errorf("no function between: %v", err)
	}
	noMatcher := formalLayout
	noMatcher.Matcher = 0
	if err := CheckLayout(noMatcher, moved(Position{}, 0, 0), Position{}, ""); err == nil {
		t.Error("no matcher accepted")
	}
}

func withHandler(l Layout, op string, d uint64) Layout { l.Handlers[op] += d; return l }
func withoutHandler(l Layout, op string) Layout        { delete(l.Handlers, op); return l }

func TestWriteLayoutReplacesTheRowOfABinary(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLayout(dir, "a@0.8", "h1", moved(Position{0, 8, 0}, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := WriteLayout(dir, "a@32.0", "h2", moved(Position{32, 0, 0}, 64, 64)); err != nil {
		t.Fatal(err)
	}
	if err := WriteLayout(dir, "a@0.8", "h3", moved(Position{0, 8, 0}, 0, 0)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "layouts.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || lines[0] != LayoutHeader() || !strings.HasPrefix(lines[1], "a@32.0\th2\t") || !strings.HasPrefix(lines[2], "a@0.8\th3\t") {
		t.Errorf("layouts.tsv:\n%s", b)
	}
	// A file of another format is not added to.
	os.WriteFile(filepath.Join(dir, "layouts.tsv"), []byte("binary\tfunc\n"), 0o644)
	if err := WriteLayout(dir, "a@0.8", "h3", moved(Position{0, 8, 0}, 0, 0)); err == nil {
		t.Error("wrote to a file of another header")
	}
}

func TestCheckLayoutSaysWhenTheAlignmentTookUpTheShift(t *testing.T) {
	// The regexp matcher takes up all of a shift of 32 when the formal binary
	// has 32 bytes or more of padding in front of it, and nothing after it
	// moves.
	p := Position{32, 0, 0}
	err := CheckLayout(formalLayout, moved(p, 0, 0), p, "first")
	if err == nil || !strings.Contains(err.Error(), "taken up by the alignment") {
		t.Errorf("err = %v", err)
	}
	// Half a shift lost to the padding is what an odd multiple of 32 does in
	// such a binary, and is no fault of the build.
	for _, c := range []struct {
		p     Position
		dm, d uint64
	}{{Position{1120, 0, 0}, 1088, 1088}, {Position{1120, 0, 0}, 1152, 1152}, {Position{2208, 8, 0}, 2176, 2176}} {
		if err := CheckLayout(formalLayout, moved(c.p, c.dm, c.d), c.p, "first"); err != nil {
			t.Errorf("%s moved by %d: %v", c.p, c.dm, err)
		}
	}
	// A second shift of 32 or 96 and the like is taken up by the alignment of
	// the interpreter in the same way. The functions before it moved, so the
	// layout is not the formal one for them.
	for _, c := range []struct {
		p Position
		d uint64
	}{{Position{0, 0, 96}, 64}, {Position{0, 0, 96}, 128}, {Position{0, 8, 2336}, 2304}, {Position{0, 8, 2336}, 2368}} {
		if err := CheckLayout(formalLayout, moved(c.p, 0, c.d), c.p, "first"); err != nil {
			t.Errorf("%s, interpreter moved by %d: %v", c.p, c.d, err)
		}
	}
	// But a second shift that moved no function between the two is a layout
	// that is the formal one for them, and fails.
	p = Position{0, 0, 96}
	l := moved(p, 0, 64)
	for name, a := range l.Funcs {
		if a > formalLayout.Matcher && a < l.Func {
			l.Funcs[name] = formalLayout.Funcs[name]
		}
	}
	if err := CheckLayout(formalLayout, l, p, "first"); err == nil || !strings.Contains(err.Error(), "between") {
		t.Errorf("err = %v", err)
	}
}

// leaRIP is lea disp32(%rip) into register reg (0 to 15) at address at, loading
// target.
func leaRIP(reg int, at, target uint64) []byte {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x04 // REX.R
	}
	b := []byte{rex, 0x8d, byte(0x05 | (reg&7)<<3), 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[3:], uint32(int32(int64(target)-int64(at+7))))
	return b
}

func TestJumpTableInAnyRegister(t *testing.T) {
	const fn, size = 0x10000, 0x2000
	const table, decoy = 0x20000, 0x21000
	// the table: 256 offsets, relative to it, of handlers in the function
	mem := map[uint64][]byte{table: make([]byte, 1024), decoy: make([]byte, 1024)}
	for i := range 256 {
		binary.LittleEndian.PutUint32(mem[table][i*4:], uint32(int32(int64(fn+0x100+16*i)-table)))
		// the decoy is read more often, but its offsets point outside the function
		binary.LittleEndian.PutUint32(mem[decoy][i*4:], uint32(int32(0x30000+i-decoy)))
	}
	read := func(a uint64) []byte { return mem[a] }
	// r11 is where the jump table was until the engine of exp/array-iter-fast
	// put it in r10. rax, r10 and r11 cover REX.R both ways.
	for _, reg := range []int{11, 10, 0, 14} {
		t.Run(fmt.Sprint("r", reg), func(t *testing.T) {
			code := make([]byte, size)
			at := 0
			put := func(r int, target uint64) {
				copy(code[at:], leaRIP(r, fn+uint64(at), target))
				at += 7 + 5
			}
			for range 5 {
				put(reg, table)
			}
			decoyReg := 3
			if reg == 3 {
				decoyReg = 4
			}
			for range 9 {
				put(decoyReg, decoy)
			}
			got, ok := jumpTable(fn, code, read)
			if !ok || got != table {
				t.Fatalf("jump table at %#x (found %v), want %#x", got, ok, uint64(table))
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		code := make([]byte, size)
		copy(code, leaRIP(10, fn, decoy))
		if got, ok := jumpTable(fn, code, read); ok {
			t.Errorf("found a jump table at %#x in code that has none", got)
		}
	})
}

func TestEngineFuncsLeavesOutWhatIsNotAnAssemblyFunctionOfTheEngine(t *testing.T) {
	const pkg = "github.com/liustack/quickjs/internal/cengine."
	fn := func(name string, v uint64) elf.Symbol {
		return elf.Symbol{Name: name, Value: v, Info: byte(elf.STT_FUNC)}
	}
	got := engineFuncs([]elf.Symbol{
		fn(pkg+"lre_exec_backtrack.abi0", 0x6f0000),
		fn(pkg+"js_malloc_rt.abi0", 0x6f4b20),
		fn(pkg+"New", 0x5dbae0),              // Go, not assembly
		fn("runtime.memmove.abi0", 0x401000), // another package
		fn(pkg+"dup.abi0", 0x6f1000),         // named twice: left out
		fn(pkg+"dup.abi0", 0x6f2000),
		{Name: pkg + "tbl.abi0", Value: 0x700000, Info: byte(elf.STT_OBJECT)},
	})
	want := map[string]uint64{"lre_exec_backtrack": 0x6f0000, "js_malloc_rt": 0x6f4b20}
	if !maps.Equal(got, want) {
		t.Errorf("engineFuncs = %v, want %v", got, want)
	}
}

func TestCheckLayoutGoesByTheFunctionTheShiftWentAfter(t *testing.T) {
	p := Position{1376, 8, 0}
	// The shift went after heap_alloc, not after the lowest function: the
	// functions up to it stay, the ones after it and before the matcher move.
	l := moved(p, 1344, 1344)
	l.Funcs["heap_alloc"] = formalLayout.Funcs["heap_alloc"]
	if err := CheckLayout(formalLayout, l, p, "heap_alloc"); err != nil {
		t.Errorf("pad heap_alloc: %v", err)
	}
	if err := CheckLayout(formalLayout, l, p, "first"); err == nil {
		t.Error("heap_alloc stayed, with the shift after first: accepted")
	}
	// the function the shift went after moved
	if err := CheckLayout(formalLayout, moved(p, 1344, 1344), p, "heap_alloc"); err == nil {
		t.Error("the pad function moved: accepted")
	}
	// A pad whose name is used twice is not in Funcs: say so, and do not take
	// another function in its place.
	dup := formalLayout
	dup.Funcs = maps.Clone(formalLayout.Funcs)
	delete(dup.Funcs, "heap_alloc")
	ld := moved(p, 1344, 1344)
	delete(ld.Funcs, "heap_alloc")
	err := CheckLayout(dup, ld, p, "heap_alloc")
	if err == nil || !strings.Contains(err.Error(), "heap_alloc") || !strings.Contains(err.Error(), "twice") {
		t.Errorf("pad named twice: %v", err)
	}
	// A shift needs the function it went after.
	if err := CheckLayout(formalLayout, moved(p, 1344, 1344), p, ""); err == nil {
		t.Error("a shift with no pad function: accepted")
	}
	// Without a shift nothing before the matcher moves, whatever the pad.
	if err := CheckLayout(formalLayout, moved(Position{}, 0, 0), Position{}, ""); err != nil {
		t.Errorf("no shift, no pad: %v", err)
	}
}
