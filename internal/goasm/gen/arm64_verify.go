// Copyright (c) 2026 The liustack/quickjs Authors.
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
	a64Ret         = 0xd65f03c0
	a64Nop         = 0xd503201f
	a64StackGuard  = 0xf9400b90 // ldr x16, [x28, #16]: g.stackguard0
	a64CmpSPGuard  = 0xeb3063ff // cmp sp, x16: the check of a frame of at most StackSmall
	a64CmpTmpGuard = 0xeb10023f // cmp x17, x16: the check of a bigger frame
	a64SubSPTmp    = 0xd10003f1 // sub x17, sp, #0: with #(frame - StackSmall) in bits 10 to 21
	a64BLRX9       = 0xd63f0120
	a64MovX3LR     = 0xaa1e03e3 // mov x3, x30, before the call of morestack
	a64SaveFP      = 0xf81f83fd // stur x29, [sp, #-8]
	a64SetFP       = 0xd10023fd // sub x29, sp, #8
	a64LoadFP      = 0xf85f83fd // ldur x29, [sp, #-8]
	a64BigSave     = 0xa93ffa9d // stp x29, x30, [x20, #-8]
	a64BigSetSP    = 0x9100029f // mov sp, x20
	a64BigLoad     = 0xa97ffbfd // ldp x29, x30, [sp, #-8]
	a64AddFP       = 0x910003fd // add x29, sp, #0: the leaf epilogue, with #frame-8 in bits 10 to 21
	a64AddSP       = 0x910003ff // add sp, sp, #0: with #frame in bits 10 to 21
)

// CheckEngineARM64 runs the check on c.Bin, a linked linux/arm64 binary of
// the package c.Pkg, against c.Lowered.
func CheckEngineARM64(c EngineCheck) (*CheckReport, error) {
	rep := &CheckReport{Checked: map[string]int{}, Skipped: map[string][]string{}}
	o, err := readA64Object(c.Lowered)
	if err != nil {
		return nil, err
	}
	bin, err := openBin(c.Bin, "arm64")
	if err != nil {
		return nil, err
	}
	defer bin.Close()
	rep.Binary = bin.describe()
	ms, ok := bin.syms["runtime.morestack_noctxt.abi0"]
	if !ok {
		return nil, fmt.Errorf("no runtime.morestack_noctxt in the binary")
	}
	rodata := uint64(0)
	if s, ok := bin.syms[c.Pkg+"."+RodataGo]; ok {
		rodata = s.Value
	}
	k := &a64Checker{c: c, o: o, bin: bin, morestack: ms.Value, rodata: rodata}
	if c.HostPoll {
		if err := k.checkHostPoll(); err != nil {
			return rep, err
		}
		rep.Checked["host_poll"]++
	}
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
	slid, err := bin.loaderWrites()
	if err != nil {
		return rep, err
	}
	if bin.pie {
		rep.Checked["pointers the loader slides"] = slid
	}
	return rep, nil
}

type a64Checker struct {
	c         EngineCheck
	o         *a64Object
	bin       *binFile
	morestack uint64
	rodata    uint64
	// ptrOK holds the addresses a function pointer may hold (pointerTarget).
	ptrOK map[uint64]bool
}

// pointerTarget is checker.pointerTarget on arm64: a function pointer, the
// code's adrp and add of sym (go_name) or a pointer of engine_rodata to the
// C function name, may hold only the start of a lowered unified function or
// of a stub (CheckPointerTargets).
func (k *a64Checker) pointerTarget(name string, add int64) error {
	if k.ptrOK == nil {
		k.ptrOK = map[uint64]bool{}
		for _, m := range append(append([]Meta{}, k.c.Metas...), k.c.Stubs...) {
			if m.Kind != Unified {
				continue
			}
			if s, ok := k.bin.abi0(k.c.Pkg, k.c.Names[m.Name]); ok {
				k.ptrOK[s.Value] = true
			}
		}
	}
	if add != 0 {
		return fmt.Errorf("a pointer to %s%+d, inside a function", name, add)
	}
	gn := k.c.Names[name]
	if gn == "" {
		return fmt.Errorf("a pointer to %s, which is no generated function", name)
	}
	s, ok := k.bin.abi0(k.c.Pkg, gn)
	if !ok {
		return fmt.Errorf("a pointer to %s, which is not in the binary", name)
	}
	if !k.ptrOK[s.Value] {
		return fmt.Errorf("a pointer to %s, which is no lowered unified function: a call through it would not get its result", name)
	}
	return nil
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

// checkHostPoll holds the Go function host_poll to the one thing the poll is
// for: a safe point. As on amd64 (checker.checkHostPoll), it must begin with
// Go's stack check, the load of the guard, a compare of the stack (or the
// stack less the frame) with it and a branch on lower or same to a call of
// runtime.morestack_noctxt and a jump back to its start. The compiler makes
// host_poll, so its frame is not known here, and either form of the compare
// is taken. A small function that calls nothing is marked NOSPLIT by the
// compiler and has none of this.
func (k *a64Checker) checkHostPoll() error {
	sym, ok := k.bin.syms[k.c.Pkg+".host_poll"]
	if !ok {
		return fmt.Errorf("host_poll is called but is not in the binary")
	}
	fail := func(f string, a ...any) error {
		return fmt.Errorf("host_poll has no stack check, so the Go runtime cannot stop C code at it: "+f, a...)
	}
	b, err := k.bin.read(sym.Value, int(sym.Size)&^3)
	if err != nil {
		return err
	}
	n := len(b) / 4
	if n == 0 || a64Word(b, 0) != a64StackGuard {
		first := uint32(0)
		if n > 0 {
			first = a64Word(b, 0)
		}
		return fail("it starts with %#08x, not the load of the guard", first)
	}
	_, more, err := a64StackCheck(b, -1)
	if err != nil {
		return fail("%v", err)
	}
	if more < 0 || more >= n {
		return fail("its stack check branches to word %d, outside the function", more)
	}
	// The block the b.ls goes to is one straight run of words: the spill of
	// the arguments, mov x3, x30, bl morestack, the reload and a b to the
	// start. Any other branch in it (a b.cond or a cbz that jumps over the call,
	// a br, a blr, a ret) lets the stack check be skipped while the words of
	// the call are still there, so none is allowed.
	called := false
	for t := more; t < n && t < more+24; t++ {
		w := a64Word(b, t)
		at := sym.Value + uint64(4*t)
		target := at + uint64(int64(int32(w<<6)>>4))
		switch {
		case w&0xfc000000 == 0x94000000 && target == k.morestack: // bl morestack
			if t == more || a64Word(b, t-1) != a64MovX3LR {
				return fail("the call of morestack does not follow mov x3, x30")
			}
			called = true
		case w&0xfc000000 == 0x14000000: // b
			if !called || target != sym.Value {
				return fail("its stack check does not call morestack and jump back to the start")
			}
			return nil
		case w&0xfc000000 == 0x94000000, // bl elsewhere
			w&0xff000010 == 0x54000000,                             // b.cond
			w&0x7e000000 == 0x34000000,                             // cbz, cbnz
			w&0x7e000000 == 0x36000000,                             // tbz, tbnz
			w&0xff9ffc1f == 0xd61f0000, w&0xfffffc1f == 0xd65f0000: // br, blr, ret
			return fail("its stack check has a branch of its own (%#08x) that can jump over the call of morestack", w)
		}
	}
	return fail("its stack check does not call morestack and jump back to the start")
}

func a64Word(b []byte, i int) uint32 { return binary.LittleEndian.Uint32(b[4*i:]) }

// a64Imm9 is the signed 9-bit offset of a load or store with writeback.
func a64Imm9(w uint32) int { return int(int32(w<<11) >> 23) }

// a64StackCheck reads the stack check at the start of b (word 0 is the load
// of the guard) as Go's assembler writes it for a function whose frame is
// frame bytes (cmd/internal/obj/arm64/obj7.go, stacksplit): for a frame of at
// most abi.StackSmall (128) the compare of SP with the guard, for a bigger one
// up to abi.StackBig (4096) the compare with it of SP less (frame - 128),
// kept in x17. Then b.ls. It returns the index of the first word after the
// check and the index the branch goes to. A frame past StackBig has a longer
// form that tests for underflow, which no function of the engine has and
// nothing has reviewed. With frame below 0 (the frame is not known, as for
// host_poll, which the compiler makes) either form is taken.
func a64StackCheck(b []byte, frame int) (next, morestackAt int, err error) {
	word := func(i int) uint32 {
		if 4*i+4 > len(b) {
			return 0
		}
		return a64Word(b, i)
	}
	i := 1
	switch {
	case frame > 4096:
		return 0, 0, fmt.Errorf("a stack check for a frame of %d bytes, past the 4096 whose form is reviewed", frame)
	case frame >= 0 && frame <= 128, frame < 0 && word(1) == a64CmpSPGuard:
		if word(i) != a64CmpSPGuard {
			return 0, 0, fmt.Errorf("the stack check has %#08x, want cmp sp, x16 (%#08x) for a frame of %d bytes", word(i), uint32(a64CmpSPGuard), frame)
		}
		i++
	default:
		sub := word(i)
		if frame >= 0 {
			if want := a64SubSPTmp | uint32(frame-128)<<10; sub != want {
				return 0, 0, fmt.Errorf("the stack check has %#08x, want sub x17, sp, #%d (%#08x) for a frame of %d bytes", sub, frame-128, want, frame)
			}
		} else if sub&0xffc003ff != a64SubSPTmp {
			return 0, 0, fmt.Errorf("the stack check starts with %#08x, which is no compare of the stack with the guard", sub)
		}
		if cmp := word(i + 1); cmp != a64CmpTmpGuard {
			return 0, 0, fmt.Errorf("the stack check has %#08x, want cmp x17, x16 (%#08x)", cmp, uint32(a64CmpTmpGuard))
		}
		i += 2
	}
	w := word(i)
	if w&0xff00001f != 0x54000009 { // b.ls
		return 0, 0, fmt.Errorf("a stack check with no branch to morestack: %#08x", w)
	}
	return i + 1, i + int(int32(w<<8)>>13), nil
}

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
	// The frame the TEXT asks for, which the stack check is made for.
	calls := len(m.Rec.Calls) > 0
	frame := 0
	if m.Frame > 0 || calls {
		frame = ARM64Stack.Local(m.Frame, calls)
	}
	// Go's stack check: ldr x16, [x28, #16], the compare of the stack (less
	// what the frame needs past StackSmall) with it, b.ls to the call of
	// morestack after the RET.
	checked := n > 0 && a64Word(b, 0) == a64StackGuard
	if checked == m.NoSplit {
		return fmt.Errorf("stack check %v, but NOSPLIT %v", checked, m.NoSplit)
	}
	morestackAt := -1
	if checked {
		var err error
		if i, morestackAt, err = a64StackCheck(b, frame); err != nil {
			return err
		}
	}
	// The frame.
	if m.Frame > 0 || calls {
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
	// Each call of the body is the next one of the record, and it loads its
	// result back from the Go stack unless the record says it takes it from
	// the result registers (CallRecord.RegResult), which only a lowered
	// unified function leaves the result in: this check holds each of them
	// to storing it from x0, x1 or d0 and then running only Go's epilogue.
	unified := map[string]bool{}
	for _, f := range k.c.Metas {
		if f.Kind == Unified {
			unified[f.Name] = true
		}
	}
	ci := 0
	callAt := func(pc int, reg bool, target string) error {
		if ci >= len(m.Rec.Calls) {
			return fmt.Errorf("the call at %#x is none of the record's", pc-r[0])
		}
		c := m.Rec.Calls[ci]
		ci++
		switch {
		case reg && c.Kind != CallReg:
			return fmt.Errorf("the call at %#x is through a register, the record has one to %s", pc-r[0], c.Target)
		case !reg && c.Kind == CallReg:
			return fmt.Errorf("the call at %#x goes to %s, the record has one through a register", pc-r[0], target)
		case !reg && c.Target != target:
			return fmt.Errorf("the call at %#x goes to %s, the record has one to %s", pc-r[0], target, c.Target)
		}
		// A direct call is a leaf call only if the pass has its target as a
		// leaf, and a unified call has the signature the pass gives its
		// target (a leaf call is kept as llc wrote it and records none): the
		// record alone would let a call of a unified function returning a
		// result skip the load as a leaf call or as one returning nothing.
		if !reg {
			if leaf := k.c.Leaves[target]; leaf != (c.Kind == CallLeaf) {
				return fmt.Errorf("the call at %#x to %s: the record has it as a leaf call %v, the pass has %s as a leaf %v", pc-r[0], target, c.Kind == CallLeaf, target, leaf)
			}
			if sig, ok := k.c.Sigs[target]; c.Kind != CallLeaf && (!ok || !sameSig(sig, c.Sig)) {
				return fmt.Errorf("the call at %#x to %s is recorded as %v, its signature is %v", pc-r[0], target, c.Sig, sig)
			}
		}
		if c.Kind == CallLeaf || c.Sig.Ret == Void {
			if c.RegResult {
				return fmt.Errorf("the call at %#x to %s takes from registers a result it has not", pc-r[0], c.Target)
			}
			return nil
		}
		if c.RegResult {
			// A call through a pointer reaches only a lowered unified
			// function (pointerTarget holds every pointer to one), and each
			// of them stores its result from the registers of its type.
			if c.Kind == CallReg {
				return nil
			}
			if c.Kind != CallUnified || !unified[c.Target] {
				return fmt.Errorf("the call at %#x takes the result of %s from its registers, which is no lowered unified function", pc-r[0], c.Target)
			}
			return nil
		}
		_, retOff, _ := layout(c.Sig)
		for n, wv := range a64FPWords(a64RetRegs(c.Sig.Ret), nil, nil, 8+retOff, true) {
			if at := pc + 4 + 4*n; at+4 > dataAt || binary.LittleEndian.Uint32(k.o.code[at:]) != wv {
				return fmt.Errorf("the call at %#x to %s: no load of the result after it", pc-r[0], c.Target)
			}
		}
		return nil
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
				if err := callAt(pc, true, ""); err != nil {
					return err
				}
			}
		case rl.typ == elf.R_AARCH64_CALL26:
			t, err := k.target(rl.sym, rl.add)
			if err != nil {
				return err
			}
			if got&0xfc000000 != 0x94000000 || at+uint64(int64(int32(got<<6)>>4)) != t {
				return fmt.Errorf("the call at %#x does not go to %s", pc-r[0], rl.sym)
			}
			if err := callAt(pc, false, strings.TrimPrefix(rl.sym, "go_")); err != nil {
				return err
			}
		case rl.typ == elf.R_AARCH64_ADR_PREL_PG_HI21:
			t, err := k.target(rl.sym, rl.add)
			if err != nil {
				return err
			}
			if f, ok := strings.CutPrefix(rl.sym, "go_"); ok {
				if err := k.pointerTarget(f, rl.add); err != nil {
					return fmt.Errorf("the address at %#x: %w", pc-r[0], err)
				}
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
	if ci != len(m.Rec.Calls) {
		return fmt.Errorf("the record has %d calls, the body %d", len(m.Rec.Calls), ci)
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
	s, ok, err := k.bin.dataSym(k.c.Pkg+"."+RodataGo, uint64(RodataSize(p)))
	if err != nil {
		return 0, err
	}
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
				if err := k.pointerTarget(r.Sym, r.Add); err != nil {
					return 0, fmt.Errorf("pointer at %d: %w", off, err)
				}
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
			if err := k.bin.pointer(s.Value+uint64(off), want); err != nil {
				return 0, fmt.Errorf("pointer at %d: %w (%s%+d)", off, err, r.Sym, r.Add)
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
