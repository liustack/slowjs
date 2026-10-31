// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"debug/elf"
	"encoding/binary"
	"strings"
	"testing"
)

func a64BL(at, to uint64) uint32 { return 0x94000000 | uint32((int64(to)-int64(at))/4)&0x3ffffff }
func a64B(at, to uint64) uint32  { return 0x14000000 | uint32((int64(to)-int64(at))/4)&0x3ffffff }

func a64Adrp(at, to uint64, rd uint32) uint32 {
	imm := uint32((int64(to&^0xfff)-int64(at&^0xfff))>>12) & 0x1fffff
	return 0x90000000 | (imm&3)<<29 | (imm>>2)<<5 | rd
}

// a64CheckCase is f(I64) I64 at 0x1000 in a binary, with a stack check and
// a frame, which calls g and takes the address of engine_rodata+16, and the
// object its body comes from.
func a64CheckCase() (*a64Checker, Meta, []uint32) { return a64CheckCaseData(nil) }

// a64CheckCaseData is a64CheckCase with the function's data after its code:
// the words of tail follow the four words of the body, in the object and in
// the binary, and the last four of them are a constant pool of 16 bytes
// aligned to 16 (the label .LCPI0_0.f, the symbol f.const) that the plan holds.
func a64CheckCaseData(tail []uint32) (*a64Checker, Meta, []uint32) {
	return a64CheckCaseNops(tail, 1, nil)
}

// a64CheckCaseNops is a64CheckCaseData with nops words of no-ops between the
// prologue and the body (one pads the prologue of 28 bytes to the 16 of the
// body), and the places of the bodies that -align64 names.
func a64CheckCaseNops(tail []uint32, nops int, align64 map[string]int) (*a64Checker, Meta, []uint32) {
	const base, gAt, rodata, morestack = 0x1000, 0x8000, 0x20000, 0x9000
	sig := Sig{Params: []Type{I64}, Ret: I64}
	m := Meta{Name: "f", Kind: Unified, Frame: 40, ArgBytes: 16,
		Rec: &FuncRecord{Name: "f", Sig: sig, Calls: []CallRecord{{Kind: CallUnified, Target: "g"}}}}
	// The object: mov x0, x20; bl go_g; adrp x8 and add x8 of rodata+16.
	obj := []uint32{0xaa1403e0, 0x94000000, 0x90000008, 0x91000108}
	obj = append(obj, tail...)
	code := make([]byte, 4*len(obj))
	for i, w := range obj {
		binary.LittleEndian.PutUint32(code[4*i:], w)
	}
	o := &a64Object{code: code, addr: map[string][2]int{"f": {0, len(code)}}, relocs: map[int]a64Reloc{
		4:  {elf.R_AARCH64_CALL26, "go_g", 0},
		8:  {elf.R_AARCH64_ADR_PREL_PG_HI21, "__goasm_rodata", 16},
		12: {elf.R_AARCH64_ADD_ABS_LO12_NC, "__goasm_rodata", 16},
	}}
	bin := &binFile{syms: map[string]elf.Symbol{"p.g.abi0": {Value: gAt}}}
	plan := &DataPlan{}
	if tail != nil {
		at := 4 * (len(obj) - 4)
		o.addr["f.const"] = [2]int{at, len(code)}
		o.label = map[string]int{".LCPI0_0.f": at}
		plan.InBody = map[string][]*DataObj{"f": {{Label: ".LCPI0_0", Align: 4}}}
		plan.Rename = map[string]map[string]string{"f": {".LCPI0_0": ".LCPI0_0.f"}}
	}
	k := &a64Checker{c: EngineCheck{Pkg: "p", Names: NameMap{"f": "f", "g": "g"}, Plan: plan, Align64: align64}, o: o, bin: bin, morestack: morestack, rodata: rodata}
	// Go's prologue: the stack check, a frame of 64 (40, the LR, rounded
	// for the frame pointer), the argument from 72(RSP), no-ops to 16.
	w := []uint32{a64StackGuard, 0xeb3063ff, 0, 0xf81c0ffe, a64SaveFP, a64SetFP, 0xf9400000 | 9<<10 | 31<<5 | 20}
	for range nops {
		w = append(w, a64Nop)
	}
	at := func() uint64 { return base + uint64(4*len(w)) }
	w = append(w, 0xaa1403e0, a64BL(at()+4, gAt))
	w = append(w, a64Adrp(at(), rodata+16, 8), 0x91000108|uint32((rodata+16)&0xfff)<<10)
	w = append(w, tail...)
	// The result to 80(RSP), the epilogue, RET, morestack.
	w = append(w, 0xf9000000|10<<10|31<<5|0, a64LoadFP, 0xf84407fe, a64Ret)
	w[2] = 0x54000009 | uint32(len(w)-2)<<5
	w = append(w, a64MovX3LR, a64BL(at()+4, morestack))
	w = append(w, a64B(at(), base), 0)
	return k, m, w
}

func a64Bytes(w []uint32) []byte {
	b := make([]byte, 4*len(w))
	for i, x := range w {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return b
}

// TestCheckARM64Function: the post-link check takes a function as the Go
// assembler and linker make it of the emit's output, and refuses each
// change to it.
func TestCheckARM64Function(t *testing.T) {
	k, m, w := a64CheckCase()
	if err := k.function(m, 0x1000, a64Bytes(w)); err != nil {
		t.Fatalf("the function as made: %v", err)
	}
	for name, c := range map[string]struct {
		edit func(w []uint32, m *Meta)
		want string
	}{
		"no stack check":       {func(w []uint32, m *Meta) { w[0] = a64Nop }, "stack check false"},
		"NOSPLIT with a check": {func(w []uint32, m *Meta) { m.NoSplit = true }, "stack check true"},
		"other frame":          {func(w []uint32, m *Meta) { w[3] = 0xf81b0ffe }, "a frame of 80 bytes, want 64"},
		"argument elsewhere":   {func(w []uint32, m *Meta) { w[6]++ }, "argument load"},
		"no padding":           {func(w []uint32, m *Meta) { w[7] = 0xaa1403e0 }, "where only no-ops go"},
		"body word":            {func(w []uint32, m *Meta) { w[8] = 0xaa1503e0 }, "the object has"},
		"call elsewhere":       {func(w []uint32, m *Meta) { w[9]++ }, "does not go to go_g"},
		"address elsewhere":    {func(w []uint32, m *Meta) { w[11] += 1 << 10 }, "is not __goasm_rodata+16"},
		"result elsewhere":     {func(w []uint32, m *Meta) { w[12] += 1 << 10 }, "result store"},
		"frame freed wrong":    {func(w []uint32, m *Meta) { w[14] = 0xf84507fe }, "not freed as made"},
		"no RET":               {func(w []uint32, m *Meta) { w[15] = a64Nop }, "where the RET goes"},
		"no morestack":         {func(w []uint32, m *Meta) { w[17] = a64Nop }, "no call of morestack"},
		"code after the end":   {func(w []uint32, m *Meta) { w[19] = a64Ret }, "after the end"},
		"another call":         {func(w []uint32, m *Meta) { m.Rec.Calls = append(m.Rec.Calls, CallRecord{Kind: CallReg}) }, "indirect calls"},
	} {
		k, m, w := a64CheckCase()
		c.edit(w, &m)
		if err := k.function(m, 0x1000, a64Bytes(w)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// TestARM64StackLocal: the frames the Go assembler made of the generated
// engine (js_malloc_rt $56 with calls, memmove $24 with a call, a leaf with
// no frame), as ARM64Stack counts them.
func TestARM64StackLocal(t *testing.T) {
	for _, c := range []struct {
		frame int
		calls bool
		want  int
	}{{56, true, 80}, {24, true, 48}, {0, false, 0}, {40, false, 64}, {536, true, 560}} {
		if got := ARM64Stack.Local(c.frame, c.calls); got != c.want {
			t.Errorf("$%d (calls %v): %d, want %d", c.frame, c.calls, got, c.want)
		}
	}
}

// TestCheckARM64Rodata: engine_rodata in the binary holds the planned bytes,
// a pointer into itself where the plan has one (a pointer between data,
// which names no symbol) and the address of the function a pointer names.
// The check takes it as made and refuses each change to it.
func TestCheckARM64Rodata(t *testing.T) {
	const rodata, gAt = 0x20000, 0x8000
	plan := &DataPlan{Blob: make([]byte, 24), Relocs: []BlobReloc{
		{Off: 8, Add: 4, Label: "data", LabelAdd: 4},
		{Off: 16, Sym: "g", Func: true, Label: "g"},
	}}
	copy(plan.Blob, "engine!!")
	made := func() []byte {
		b := append([]byte(nil), plan.Blob...)
		binary.LittleEndian.PutUint64(b[8:], rodata+4)
		binary.LittleEndian.PutUint64(b[16:], gAt)
		return b
	}
	check := func(b []byte) error {
		sec := &elf.Section{SectionHeader: elf.SectionHeader{Type: elf.SHT_PROGBITS, Addr: rodata, Size: uint64(len(b))}}
		bin := &binFile{f: &elf.File{Sections: []*elf.Section{sec}}, secs: map[*elf.Section][]byte{sec: b},
			syms: map[string]elf.Symbol{"p.g.abi0": {Value: gAt}, "p." + RodataGo: {Value: rodata, Size: uint64(len(b))}}}
		k := &a64Checker{c: EngineCheck{Pkg: "p", Names: NameMap{"g": "g"}, Plan: plan}, bin: bin, rodata: rodata}
		_, err := k.checkRodata()
		return err
	}
	if err := check(made()); err != nil {
		t.Fatalf("engine_rodata as made: %v", err)
	}
	for name, c := range map[string]struct {
		at   int
		want string
	}{
		"other byte":           {3, "byte 3"},
		"data pointer moved":   {8, "pointer at 8"},
		"function pointer off": {16, "pointer at 16"},
	} {
		b := made()
		b[c.at]++
		if err := check(b); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// a64LeafCase is f(I64) I64 at 0x1000 that keeps a frame of 40 bytes and calls
// nothing, as the Go assembler makes it: a leaf with locals is NOSPLIT, and
// its epilogue restores the frame pointer and the stack pointer with two ADDs
// (cmd/internal/obj/arm64/obj7.go, the leaf case of ARET) and does not load
// the saved registers back.
func a64LeafCase() (*a64Checker, Meta, []uint32) {
	const base = 0x1000
	sig := Sig{Params: []Type{I64}, Ret: I64}
	m := Meta{Name: "f", Kind: Unified, Frame: 40, ArgBytes: 16, NoSplit: true, Rec: &FuncRecord{Name: "f", Sig: sig}}
	obj := []uint32{0xaa1403e0} // mov x0, x20
	code := make([]byte, 4*len(obj))
	binary.LittleEndian.PutUint32(code, obj[0])
	o := &a64Object{code: code, addr: map[string][2]int{"f": {0, len(code)}}, relocs: map[int]a64Reloc{}}
	k := &a64Checker{c: EngineCheck{Pkg: "p", Names: NameMap{"f": "f"}, Plan: &DataPlan{}}, o: o, bin: &binFile{}}
	// A frame of 64 (40, the LR, rounded for the frame pointer), the argument
	// from 72(RSP), the body, the result to 80(RSP), the two ADDs, RET.
	w := []uint32{0xf81c0ffe, a64SaveFP, a64SetFP, 0xf9400000 | 9<<10 | 31<<5 | 20, 0xaa1403e0,
		0xf9000000 | 10<<10 | 31<<5 | 0, 0x910003fd | 56<<10, 0x910003ff | 64<<10, a64Ret}
	return k, m, w
}

// TestCheckARM64LeafFrame: a function with a frame and no call frees it with
// the leaf epilogue of the Go assembler, and with no other.
func TestCheckARM64LeafFrame(t *testing.T) {
	k, m, w := a64LeafCase()
	if err := k.function(m, 0x1000, a64Bytes(w)); err != nil {
		t.Fatalf("the leaf as made: %v", err)
	}
	for name, c := range map[string]struct {
		edit func(w []uint32)
		want string
	}{
		"frame pointer from elsewhere": {func(w []uint32) { w[6] = 0x910003fd | 48<<10 }, "frame pointer not restored"},
		"stack freed by another size":  {func(w []uint32) { w[7] = 0x910003ff | 48<<10 }, "frame not freed as made"},
		"stack pointer not restored":   {func(w []uint32) { w[7] = a64Nop }, "frame not freed as made"},
		"the epilogue of a caller":     {func(w []uint32) { w[6], w[7] = a64LoadFP, 0xf84407fe }, "frame pointer not restored"},
		"shifted amount":               {func(w []uint32) { w[7] = 0x914003ff | 1<<10 }, "frame not freed as made"},
	} {
		k, m, w := a64LeafCase()
		c.edit(w)
		if err := k.function(m, 0x1000, a64Bytes(w)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
	// A caller keeps the epilogue that loads the saved registers back.
	k, m, w = a64CheckCase()
	w[13], w[14] = 0x910003fd|32<<10, 0x910003ff|64<<10
	if err := k.function(m, 0x1000, a64Bytes(w)); err == nil || !strings.Contains(err.Error(), "unknown epilogue") {
		t.Errorf("a leaf epilogue in a function that calls: %v", err)
	}
}

// TestCheckARM64DataAlignment: the constant pool of a function is on the 16
// bytes llc gave it where it sits in the binary. The case has its body on 16
// (the no-ops before it fill the prologue to 32 bytes), so the pool is
// aligned after four words of code, and a pool put two words later, as a
// function whose body started 8 bytes off in the object would have it, is
// refused.
func TestCheckARM64DataAlignment(t *testing.T) {
	pool := []uint32{0x11111111, 0x22222222, 0x33333333, 0x44444444}
	for _, c := range []struct {
		pads int
		want string
	}{{0, ""}, {4, ""}, {2, "is not a multiple of its alignment 16"}, {1, "is not a multiple of its alignment 16"}} {
		tail := make([]uint32, c.pads)
		for i := range tail {
			tail[i] = a64Nop
		}
		k, m, w := a64CheckCaseData(append(tail, pool...))
		err := k.function(m, 0x1000, a64Bytes(w))
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%d no-ops before the pool: %v", c.pads, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%d no-ops before the pool: %v, want %q", c.pads, err, c.want)
		}
	}
}

// TestCheckARM64BodyPlace: the body of a function put on 64 bytes with
// -align64 starts after the PCALIGN $64 that follows the prologue and the
// offset of no-ops, wherever the prologue ends in its line. A prologue of 28
// bytes at 0x1000 and an offset of 40 put the body at 0x1068, not at 0x1028,
// the first address in front of it with the offset in its line.
func TestCheckARM64BodyPlace(t *testing.T) {
	a64 := map[string]int{"f": 40}
	for _, c := range []struct {
		nops int
		want string
	}{
		{19, ""}, // 0x101c up to 0x1068
		{3, "before the body, where only no-ops"}, // the body at 0x1028
		{1, "before the body, where only no-ops"}, // the body at 0x1020
		{20, "the object has"},                    // the body at 0x106c
	} {
		k, m, w := a64CheckCaseNops(nil, c.nops, a64)
		err := k.function(m, 0x1000, a64Bytes(w))
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%d no-ops: %v", c.nops, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%d no-ops: %v, want %q", c.nops, err, c.want)
		}
	}
	// Without -align64 the body starts on the next 16 bytes.
	k, m, w := a64CheckCaseNops(nil, 1, nil)
	if err := k.function(m, 0x1000, a64Bytes(w)); err != nil {
		t.Errorf("a plain body: %v", err)
	}
}

// a64CallCase is f(I64) I64 at 0x1000 that calls the generated g(I64) I64
// and returns what g returns: it stores the argument, calls, and loads the
// result back from 16(RSP) when load is set. The check is told g is a
// lowered unified function, which it holds to its return sequence.
func a64CallCase(regResult, load bool) (*a64Checker, Meta, []uint32) {
	const base, gAt, morestack = 0x1000, 0x8000, 0x9000
	sig := Sig{Params: []Type{I64}, Ret: I64}
	m := Meta{Name: "f", Kind: Unified, Frame: 40, ArgBytes: 16,
		Rec: &FuncRecord{Name: "f", Sig: sig, Calls: []CallRecord{{Kind: CallUnified, Target: "g", Sig: sig, RegResult: regResult}}}}
	g := Meta{Name: "g", Kind: Unified, Rec: &FuncRecord{Name: "g", Sig: sig}}
	// The object: str x20, [sp, #8]; bl go_g; ldr x0, [sp, #16].
	obj := []uint32{0xf9000000 | 1<<10 | 31<<5 | 20, 0x94000000}
	if load {
		obj = append(obj, 0xf9400000|2<<10|31<<5|0)
	}
	code := make([]byte, 4*len(obj))
	for i, w := range obj {
		binary.LittleEndian.PutUint32(code[4*i:], w)
	}
	o := &a64Object{code: code, addr: map[string][2]int{"f": {0, len(code)}}, relocs: map[int]a64Reloc{
		4: {elf.R_AARCH64_CALL26, "go_g", 0},
	}}
	bin := &binFile{syms: map[string]elf.Symbol{"p.g.abi0": {Value: gAt}}}
	k := &a64Checker{c: EngineCheck{Pkg: "p", Names: NameMap{"f": "f", "g": "g"}, Plan: &DataPlan{}, Metas: []Meta{m, g}},
		o: o, bin: bin, morestack: morestack}
	w := []uint32{a64StackGuard, 0xeb3063ff, 0, 0xf81c0ffe, a64SaveFP, a64SetFP, 0xf9400000 | 9<<10 | 31<<5 | 20, a64Nop}
	at := func() uint64 { return base + uint64(4*len(w)) }
	w = append(w, obj[0], a64BL(at()+4, gAt))
	w = append(w, obj[2:]...)
	w = append(w, 0xf9000000|10<<10|31<<5|0, a64LoadFP, 0xf84407fe, a64Ret)
	w[2] = 0x54000009 | uint32(len(w)-2)<<5
	w = append(w, a64MovX3LR, a64BL(at()+4, morestack))
	w = append(w, a64B(at(), base))
	return k, m, w
}

// TestCheckARM64RegResult: a call may leave out the load of its result only
// where the record says so and the callee is a lowered unified function the
// check holds to its return sequence, and a callee's return sequence takes
// nothing between the store of the result and the RET but Go's epilogue.
func TestCheckARM64RegResult(t *testing.T) {
	for name, c := range map[string]struct {
		regResult, load bool
		edit            func(k *a64Checker, m *Meta, w []uint32) []uint32
		want            string
	}{
		"load":                   {false, true, nil, ""},
		"from registers":         {true, false, nil, ""},
		"no load, none declared": {false, false, nil, "no load of the result"},
		"from registers of a stub": {true, false, func(k *a64Checker, m *Meta, w []uint32) []uint32 {
			k.c.Stubs, k.c.Metas = k.c.Metas[1:], k.c.Metas[:1]
			return w
		}, "no lowered unified function"},
		"from registers of Go code": {true, false, func(k *a64Checker, m *Meta, w []uint32) []uint32 { k.c.Metas = k.c.Metas[:1]; return w }, "no lowered unified function"},
		"from registers of a leaf":  {true, false, func(k *a64Checker, m *Meta, w []uint32) []uint32 { k.c.Metas[1].Kind = Register; return w }, "no lowered unified function"},
		"through a pointer":         {true, false, func(k *a64Checker, m *Meta, w []uint32) []uint32 { m.Rec.Calls[0].Kind = CallReg; return w }, "through a register"},
		// mov x0, xzr after the store of the result.
		"a word after the result store": {false, true, func(k *a64Checker, m *Meta, w []uint32) []uint32 {
			return append(w[:12:12], append([]uint32{0xaa1f03e0}, w[12:]...)...)
		}, "epilogue"},
	} {
		k, m, w := a64CallCase(c.regResult, c.load)
		if c.edit != nil {
			w = c.edit(k, &m, w)
		}
		err := k.function(m, 0x1000, a64Bytes(w))
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}
