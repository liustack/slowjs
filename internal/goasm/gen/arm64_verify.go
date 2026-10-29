// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The post-link check of the arm64 engine. It reads a linked linux/arm64
// binary of the generated package and holds every generated function to the
// object llvm-mc made of the lowered text, which the emit wrote as WORDs:
//
//   - Go's prologue: a stack check exactly when the function is not on the
//     NOSPLIT list, the frame the TEXT asks for (with the saved LR and frame
//     pointer as cmd/internal/obj/arm64 rounds it, ARM64Stack.Local), and
//     nothing else,
//   - the loads of the arguments from the Go argument area into the
//     registers of preserve_none, then no-ops up to where the body starts
//     (16 bytes, or the line of 64 of -align64),
//   - the body word for word: each BL the emit wrote goes to the function
//     the object's call names, each ADRP and ADD to engine_rodata plus the
//     object's offset or to the function it names, an indirect call is the
//     same BLR, every other word and the function's own data are as in the
//     object,
//   - the stores of the result, Go's epilogue and one RET, and after it only
//     the call of morestack of the stack check,
//
// and engine_rodata byte for byte with each pointer at the function or
// data it names. It does not take llc's object: that the lowering kept the
// meaning of llc's code is what the lowering's own rules and llc's register
// facts hold (LowerARM64EngineFunc).
package gen

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	a64Ret        = 0xd65f03c0
	a64Nop        = 0xd503201f
	a64StackGuard = 0xf9400b90 // ldr x16, [x28, #16]: g.stackguard0
	a64BLRX9      = 0xd63f0120
	a64MovX3LR    = 0xaa1e03e3 // mov x3, x30, before the call of morestack
	a64SaveFP     = 0xf81f83fd // stur x29, [sp, #-8]
	a64SetFP      = 0xd10023fd // sub x29, sp, #8
	a64LoadFP     = 0xf85f83fd // ldur x29, [sp, #-8]
	a64BigSave    = 0xa93ffa9d // stp x29, x30, [x20, #-8]
	a64BigSetSP   = 0x9100029f // mov sp, x20
	a64BigLoad    = 0xa97ffbfd // ldp x29, x30, [sp, #-8]
	a64AddFP      = 0x910003fd // add x29, sp, #0: the leaf epilogue, with #frame-8 in bits 10 to 21
	a64AddSP      = 0x910003ff // add sp, sp, #0: with #frame in bits 10 to 21
)

// CheckEngineARM64 runs the check on c.Bin, a linked linux/arm64 binary of
// the package c.Pkg, against c.Lowered.
func CheckEngineARM64(c EngineCheck) (*CheckReport, error) {
	rep := &CheckReport{Checked: map[string]int{}, Skipped: map[string][]string{}}
	o, err := readA64Object(c.Lowered)
	if err != nil {
		return nil, err
	}
	bin, err := openBinMachine(c.Bin, elf.EM_AARCH64)
	if err != nil {
		return nil, err
	}
	defer bin.f.Close()
	ms, ok := bin.syms["runtime.morestack_noctxt.abi0"]
	if !ok {
		return nil, fmt.Errorf("no runtime.morestack_noctxt in the binary")
	}
	rodata := uint64(0)
	if s, ok := bin.syms[c.Pkg+"."+RodataGo]; ok {
		rodata = s.Value
	}
	k := &a64Checker{c: c, o: o, bin: bin, morestack: ms.Value, rodata: rodata}
	for _, m := range c.Metas {
		gn := c.Names[m.Name]
		s, ok := bin.abi0(c.Pkg, gn)
		if !ok {
			if c.All {
				return rep, fmt.Errorf("%s: not in the binary", m.Name)
			}
			rep.Skipped["not in the binary"] = append(rep.Skipped["not in the binary"], m.Name)
			continue
		}
		words, err := bin.read(s.Value, int(s.Size))
		if err != nil {
			return rep, fmt.Errorf("%s: %w", m.Name, err)
		}
		if err := k.function(m, s.Value, words); err != nil {
			return rep, fmt.Errorf("%s: %w", m.Name, err)
		}
		rep.Checked["functions"]++
	}
	n, err := k.checkRodata()
	if err != nil {
		return rep, fmt.Errorf("engine_rodata: %w", err)
	}
	rep.Checked["engine_rodata pointers"] = n
	return rep, nil
}

type a64Checker struct {
	c         EngineCheck
	o         *a64Object
	bin       *binFile
	morestack uint64
	rodata    uint64
}

// target is the address the binary must have for a symbol of the object: a
// generated or Go function (go_name) or engine_rodata.
func (k *a64Checker) target(sym string, add int64) (uint64, error) {
	if sym == "__goasm_rodata" {
		if k.rodata == 0 {
			return 0, fmt.Errorf("no engine_rodata in the binary")
		}
		return uint64(int64(k.rodata) + add), nil
	}
	t, ok := strings.CutPrefix(sym, "go_")
	gn := k.c.Names[t]
	if !ok || gn == "" {
		return 0, fmt.Errorf("relocation to %s, which is no generated function", sym)
	}
	s, ok := k.bin.abi0(k.c.Pkg, gn)
	if !ok {
		return 0, fmt.Errorf("%s not in the binary", t)
	}
	return uint64(int64(s.Value) + add), nil
}

func a64Word(b []byte, i int) uint32 { return binary.LittleEndian.Uint32(b[4*i:]) }

// a64Imm9 is the signed 9-bit offset of a load or store with writeback.
func a64Imm9(w uint32) int { return int(int32(w<<11) >> 23) }

func (k *a64Checker) function(m Meta, base uint64, b []byte) error {
	n := len(b) / 4
	i := 0
	next := func() (uint32, error) {
		if i >= n {
			return 0, fmt.Errorf("ends at word %d", i)
		}
		w := a64Word(b, i)
		i++
		return w, nil
	}
	// Go's stack check: ldr x16, [x28, #16], a compare, b.ls to the call of
	// morestack after the RET.
	checked := n > 0 && a64Word(b, 0) == a64StackGuard
	if checked == m.NoSplit {
		return fmt.Errorf("stack check %v, but NOSPLIT %v", checked, m.NoSplit)
	}
	morestackAt := -1
	if checked {
		i = 1
		for ; i < n && i < 5; i++ {
			w := a64Word(b, i)
			if w&0xff00001f == 0x54000009 { // b.ls
				morestackAt = i + int(int32(w<<8)>>13)
				i++
				break
			}
		}
		if morestackAt < 0 {
			return fmt.Errorf("a stack check with no branch to morestack")
		}
	}
	// The frame.
	calls := len(m.Rec.Calls) > 0
	frame := 0
	if m.Frame > 0 || calls {
		frame = ARM64Stack.Local(m.Frame, calls)
		w, err := next()
		if err != nil {
			return err
		}
		switch {
		case w&0xffe00fff == 0xf8000ffe: // str x30, [sp, #-N]!
			if -a64Imm9(w) != frame {
				return fmt.Errorf("a frame of %d bytes, want %d", -a64Imm9(w), frame)
			}
			if w, _ := next(); w != a64SaveFP {
				return fmt.Errorf("frame pointer not saved: %#08x", w)
			}
		case w&0xff8003ff == 0xd10003f4: // sub x20, sp, #N
			got := int(w>>10) & 0xfff
			if w&(1<<22) != 0 {
				got <<= 12
			}
			if got != frame {
				return fmt.Errorf("a frame of %d bytes, want %d", got, frame)
			}
			if w, _ := next(); w != a64BigSave {
				return fmt.Errorf("frame pointer and LR not saved: %#08x", w)
			}
			if w, _ := next(); w != a64BigSetSP {
				return fmt.Errorf("SP not set: %#08x", w)
			}
		default:
			return fmt.Errorf("unknown frame setup %#08x", w)
		}
		if w, _ := next(); w != a64SetFP {
			return fmt.Errorf("frame pointer not set: %#08x", w)
		}
	}
	// The arguments from the Go argument area, above the frame and the LR.
	if m.Kind != Register {
		regs, err := a64ArgRegs(m.Rec.Sig, true)
		if err != nil {
			return err
		}
		argOff, _, _ := layout(m.Rec.Sig)
		want := a64FPWords(regs, m.Rec.Sig.Params, argOff, frame+8, true)
		for _, wv := range want {
			if w, err := next(); err != nil || w != wv {
				return fmt.Errorf("argument load %#08x, want %#08x", w, wv)
			}
		}
	}
	// No-ops up to the body: it starts after the PCALIGN $16 that follows the
	// prologue, or after the PCALIGN $64 that does and its offset of no-ops
	// (the emitter's rule, as bodyAt of the amd64 check has it).
	bodyAt := alignUp64(base+uint64(4*i), 16)
	if off, ok := k.c.Align64[m.Name]; ok {
		bodyAt = alignUp64(base+uint64(4*i), 64) + uint64(off)
	}
	for base+uint64(4*i) < bodyAt {
		if w, err := next(); err != nil || w != a64Nop {
			return fmt.Errorf("%#08x before the body, where only no-ops go", w)
		}
	}
	// The body and its data.
	r, ok := k.o.addr[m.Name]
	if !ok {
		return fmt.Errorf("not in the lowered object")
	}
	dataAt := r[1]
	if c, ok := k.o.addr[m.Name+".const"]; ok {
		dataAt = c[0]
	}
	// The function's data is aligned as llc aligned it, where it sits in the
	// binary and not only from the start of the body, which Go puts at its own
	// place (Align64 puts it at an offset past a multiple of 64).
	for _, o := range k.c.Plan.InBody[m.Name] {
		label := k.c.Plan.Rename[m.Name][o.Label]
		off, ok := k.o.label[label]
		if !ok {
			return fmt.Errorf("body data %s absent from the re-assembled object", label)
		}
		if at, a := base+uint64(4*i)+uint64(off-r[0]), uint64(1)<<o.Align; at%a != 0 {
			return fmt.Errorf("body data %s sits at %#x, which is not a multiple of its alignment %d", o.Label, at, a)
		}
	}
	icalls := 0
	for _, c := range m.Rec.Calls {
		if c.Kind == CallReg {
			icalls++
		}
	}
	for pc := r[0]; pc < r[1]; pc += 4 {
		want := binary.LittleEndian.Uint32(k.o.code[pc:])
		at := base + uint64(4*i)
		got, err := next()
		if err != nil {
			return err
		}
		rl, isReloc := k.o.relocs[pc]
		switch {
		case pc >= dataAt || !isReloc:
			if got != want {
				return fmt.Errorf("word %#x of the body is %#08x, the object has %#08x", pc-r[0], got, want)
			}
			if pc < dataAt && want == a64BLRX9 {
				icalls--
			}
		case rl.typ == elf.R_AARCH64_CALL26:
			t, err := k.target(rl.sym, rl.add)
			if err != nil {
				return err
			}
			if got&0xfc000000 != 0x94000000 || at+uint64(int64(int32(got<<6)>>4)) != t {
				return fmt.Errorf("the call at %#x does not go to %s", pc-r[0], rl.sym)
			}
		case rl.typ == elf.R_AARCH64_ADR_PREL_PG_HI21:
			t, err := k.target(rl.sym, rl.add)
			if err != nil {
				return err
			}
			add, err := next()
			if err != nil {
				return err
			}
			rd := got & 0x1f
			if got&0x9f000000 != 0x90000000 || add&0xffc003ff != 0x91000000|rd<<5|rd {
				return fmt.Errorf("the address at %#x is not an adrp and an add", pc-r[0])
			}
			page := int64(int32((got>>5&0x7ffff)<<2|got>>29&3)<<11>>11) << 12
			if uint64(int64(at&^0xfff)+page)+uint64(add>>10&0xfff) != t {
				return fmt.Errorf("the address at %#x is not %s%+d", pc-r[0], rl.sym, rl.add)
			}
			pc += 4
		default:
			return fmt.Errorf("relocation %v at %#x", rl.typ, pc-r[0])
		}
	}
	if icalls != 0 {
		return fmt.Errorf("%d indirect calls the record has not found in the body", icalls)
	}
	// The result into the Go argument area, the epilogue and RET.
	if m.Kind != Register {
		_, retOff, _ := layout(m.Rec.Sig)
		rets := a64RetRegs(m.Rec.Sig.Ret)
		for k, wv := range a64FPWords(rets, nil, nil, frame+8+retOff, false) {
			if w, err := next(); err != nil || w != wv {
				return fmt.Errorf("result store %d is %#08x, want %#08x", k, w, wv)
			}
		}
	}
	if m.Frame > 0 && !calls {
		// A function with a frame and no call is a leaf to the Go
		// assembler, which restores the frame pointer and the stack pointer
		// with two ADDs and leaves the saved registers in the frame
		// (cmd/internal/obj/arm64/obj7.go, ARET of a leaf). A frame past
		// 4095 bytes needs a longer sequence, which nothing has reviewed.
		if frame > 4095 {
			return fmt.Errorf("a leaf frame of %d bytes: its epilogue is not reviewed", frame)
		}
		if w, err := next(); err != nil || w != a64AddFP|uint32(frame-8)<<10 {
			return fmt.Errorf("frame pointer not restored: %#08x", w)
		}
		if w, err := next(); err != nil || w != a64AddSP|uint32(frame)<<10 {
			return fmt.Errorf("frame not freed as made: %#08x", w)
		}
	} else if m.Frame > 0 || calls {
		w, err := next()
		if err != nil {
			return err
		}
		switch w {
		case a64LoadFP: // ldur x29, [sp, #-8]; ldr x30, [sp], #N
			if w, _ := next(); w&0xffe00fff != 0xf84007fe || a64Imm9(w) != frame {
				return fmt.Errorf("frame not freed as made: %#08x", w)
			}
		case a64BigLoad: // ldp x29, x30, [sp, #-8]; add sp, sp, #N
			w, _ := next()
			got := int(w>>10) & 0xfff
			if w&(1<<22) != 0 {
				got <<= 12
			}
			if w&0xff8003ff != 0x910003ff || got != frame {
				return fmt.Errorf("frame not freed as made: %#08x", w)
			}
		default:
			return fmt.Errorf("unknown epilogue %#08x", w)
		}
	}
	if w, err := next(); err != nil || w != a64Ret {
		return fmt.Errorf("%#08x where the RET goes", w)
	}
	// After the RET: the call of morestack and the jump back, then padding.
	if checked {
		if i != morestackAt {
			return fmt.Errorf("the stack check goes to word %d, the call of morestack is at %d", morestackAt, i)
		}
		if w, _ := next(); w != a64MovX3LR {
			return fmt.Errorf("%#08x before the call of morestack", w)
		}
		at := base + uint64(4*i)
		if w, _ := next(); w&0xfc000000 != 0x94000000 || at+uint64(int64(int32(w<<6)>>4)) != k.morestack {
			return fmt.Errorf("no call of morestack after the RET")
		}
		at = base + uint64(4*i)
		if w, _ := next(); w&0xfc000000 != 0x14000000 || at+uint64(int64(int32(w<<6)>>4)) != base {
			return fmt.Errorf("no jump back to the entry after morestack")
		}
	}
	for ; i < n; i++ {
		if w := a64Word(b, i); w != 0 {
			return fmt.Errorf("%#08x after the end of the function", w)
		}
	}
	return nil
}

// a64FPWords encodes the loads (or stores) of the registers regs from (to)
// the Go argument area at base bytes above SP: each register's word at the
// layout offset of its argument (params and offs; nil for the result, whose
// words follow each other from base).
func a64FPWords(regs []string, params []Type, offs []int, base int, load bool) []uint32 {
	var out []uint32
	add := func(reg string, off int) {
		var w uint32
		n := uint32(a64RegNumber(reg))
		switch {
		case reg[0] == 'd' && load:
			w = 0xfd400000
		case reg[0] == 'd':
			w = 0xfd000000
		case load:
			w = 0xf9400000
		default:
			w = 0xf9000000
		}
		out = append(out, w|uint32(off/8)<<10|31<<5|n)
	}
	if params == nil {
		for k, r := range regs {
			add(r, base+8*k)
		}
		return out
	}
	k := 0
	for i, p := range params {
		for w := range wordTypes([]Type{p}) {
			add(regs[k], base+offs[i]+8*w)
			k++
		}
	}
	return out
}

// checkRodata compares engine_rodata in the binary with the plan: its
// bytes, and each pointer the address of what it names.
func (k *a64Checker) checkRodata() (int, error) {
	p := k.c.Plan
	if len(p.Blob) == 0 {
		return 0, nil
	}
	s, ok := k.bin.syms[k.c.Pkg+"."+RodataGo]
	if !ok {
		return 0, fmt.Errorf("not in the binary")
	}
	if int(s.Size) < len(p.Blob) {
		return 0, fmt.Errorf("%d bytes in the binary, %d planned", s.Size, len(p.Blob))
	}
	blob, err := k.bin.read(s.Value, len(p.Blob))
	if err != nil {
		return 0, err
	}
	relocs := map[int]BlobReloc{}
	for _, r := range p.Relocs {
		relocs[r.Off] = r
	}
	for off := 0; off < len(blob); {
		if r, ok := relocs[off]; ok {
			want := uint64(int64(k.rodata) + int64(r.Add))
			if r.Func {
				gn := k.c.Names[r.Sym]
				f, ok := k.bin.abi0(k.c.Pkg, gn)
				if !ok {
					return 0, fmt.Errorf("pointer at %d to %s, which is not in the binary", off, r.Sym)
				}
				want = uint64(int64(f.Value) + r.Add)
			} else if r.Sym != "" {
				// PlanData gives a pointer between data no symbol: its
				// addend is the offset in engine_rodata.
				return 0, fmt.Errorf("pointer at %d to %s, which is not engine_rodata", off, r.Sym)
			}
			if got := binary.LittleEndian.Uint64(blob[off:]); got != want {
				return 0, fmt.Errorf("pointer at %d is %#x, want %#x (%s%+d)", off, got, want, r.Sym, r.Add)
			}
			off += 8
			continue
		}
		if blob[off] != p.Blob[off] {
			return 0, fmt.Errorf("byte %d is %#02x, planned %#02x", off, blob[off], p.Blob[off])
		}
		off++
	}
	return len(relocs), nil
}
