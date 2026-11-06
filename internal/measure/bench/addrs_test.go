// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
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
}

func TestOpcodeNames(t *testing.T) {
	h := "FMT(none)\nDEF(invalid, 1, 0, 0, none)\nDEF(       push_i32, 5, 0, 1, i32)\n#if SHORT_OPCODES\nDEF(get_loc0, 1, 0, 1, none_loc)\ndef(temp, 1, 0, 0, none)\n"
	names, err := OpcodeNames(strings.NewReader(h))
	if err != nil || fmt.Sprint(names) != "[invalid push_i32 get_loc0]" {
		t.Errorf("%v %v", names, err)
	}
}

// formalLayout and moved are the formal binary of all2.tsv and a binary at p
// that landed d bytes later, with every offset in the function front later.
var formalLayout = Layout{Func: 0x6f7e00, Body: 0x6f7e68, Size: 9000, Handlers: map[string]uint64{"get_loc0": 0x6f804d, "call": 0x6f8129}}

func moved(d uint64, front int) Layout {
	l := Layout{Func: formalLayout.Func + d, Body: formalLayout.Body + d + uint64(front), Size: formalLayout.Size + uint64(front), Handlers: map[string]uint64{}}
	for op, a := range formalLayout.Handlers {
		l.Handlers[op] = a + d + uint64(front)
	}
	return l
}

func TestCheckLayoutTakesWhatTheBinariesShowed(t *testing.T) {
	// The moves of all2.tsv: a shift lands on the next 64 bytes.
	for _, c := range []struct {
		p Position
		d uint64
	}{{Position{0, 0, 0}, 0}, {Position{0, 8, 0}, 0}, {Position{32, 0, 0}, 64}, {Position{1664, 8, 0}, 1664}, {Position{3296, 16, 0}, 3328}, {Position{1120, 48, 0}, 1152}, {Position{1120, 48, 0}, 1088}, {Position{64, 0, 0}, 64}} {
		if err := CheckLayout(formalLayout, moved(c.d, c.p.Front), c.p); err != nil {
			t.Errorf("%s: %v", c.p, err)
		}
	}
}

func TestCheckLayoutFindsAWrongLanding(t *testing.T) {
	for name, c := range map[string]struct {
		l Layout
		p Position
	}{
		"not moved":           {moved(0, 8), Position{1664, 8, 0}},
		"moved without shift": {moved(64, 8), Position{0, 8, 0}},
		"moved too far":       {moved(1728, 8), Position{1664, 8, 0}},
		"moved by 32":         {moved(1696, 8), Position{1664, 8, 0}},
		"taken up, 1 shift":   {moved(0, 0), Position{32, 0, 0}},
		"too far from shift":  {moved(1728, 8), Position{1664, 8, 0}},
		"short of shift":      {moved(1600, 8), Position{1664, 8, 0}},
		"front not there":     {moved(1664, 0), Position{1664, 8, 0}},
		"handler moved": {func() Layout {
			l := moved(1664, 8)
			l.Handlers["call"] += 64
			return l
		}(), Position{1664, 8, 0}},
		"handler missing": {func() Layout {
			l := moved(1664, 8)
			delete(l.Handlers, "call")
			return l
		}(), Position{1664, 8, 0}},
	} {
		if err := CheckLayout(formalLayout, c.l, c.p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestWriteLayoutReplacesTheRowOfABinary(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLayout(dir, "a@0.8", "h1", moved(0, 8)); err != nil {
		t.Fatal(err)
	}
	if err := WriteLayout(dir, "a@32.0", "h2", moved(64, 0)); err != nil {
		t.Fatal(err)
	}
	if err := WriteLayout(dir, "a@0.8", "h3", moved(0, 8)); err != nil {
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
	if err := WriteLayout(dir, "a@0.8", "h3", moved(0, 8)); err == nil {
		t.Error("wrote to a file of another header")
	}
}

func TestCheckLayoutSaysWhenTheAlignmentTookUpTheShift(t *testing.T) {
	// The first function aligned to 64 after the shift takes up all of a
	// shift of 32 when the formal binary has 32 bytes or more of padding in
	// front of it, and nothing after it moves.
	err := CheckLayout(formalLayout, moved(0, 0), Position{32, 0, 0})
	if err == nil || !strings.Contains(err.Error(), "taken up by the alignment") {
		t.Errorf("err = %v", err)
	}
	// Half a shift lost to the padding is what an odd multiple of 32 does in
	// such a binary, and is no fault of the build.
	for _, c := range []struct {
		p Position
		d uint64
	}{{Position{1120, 0, 0}, 1088}, {Position{1120, 0, 0}, 1152}, {Position{2208, 8, 0}, 2176}} {
		if err := CheckLayout(formalLayout, moved(c.d, c.p.Front), c.p); err != nil {
			t.Errorf("%s moved by %d: %v", c.p, c.d, err)
		}
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
