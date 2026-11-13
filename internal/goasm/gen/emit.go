// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
//
// Emit stage: read the re-assembled object with debug/elf, disassemble each
// function with x/arch/x86/x86asm, and write it as a Go assembly TEXT. Every
// call becomes a Go-visible CALL (direct ones symbolic through a relocation,
// indirect ones as CALL <reg>), so Go knows the frame, traceback and stack
// bookkeeping; all other instructions pass through as machine-code bytes. This
// is the only place the generated code enters the module, and verify.go checks
// the linker left those bytes unchanged.
package gen

import (
	"debug/elf"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

// NameMap maps a C symbol to the Go identifier the assembly defines and calls.
type NameMap map[string]string

type object struct {
	code  []byte
	funcs map[string][2]uint64 // name -> [value,size]
	calls map[uint64]string    // E8 offset -> target name (direct calls)
	// refs are the other PC-relative references: the offset of the 32-bit
	// displacement -> target and addend. Emit turns each into a symbolic Go
	// instruction or fails.
	refs map[uint64]objRef
}

type objRef struct {
	sym string
	add int64
}

func readObject(path string) (*object, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	text := f.Section(".text")
	code, err := text.Data()
	if err != nil {
		return nil, err
	}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	o := &object{code: code, funcs: map[string][2]uint64{}, calls: map[uint64]string{}, refs: map[uint64]objRef{}}
	for _, s := range syms {
		if s.Name != "" {
			o.funcs[s.Name] = [2]uint64{s.Value, s.Size}
		}
	}
	for _, rs := range f.Sections {
		if rs.Type != elf.SHT_RELA {
			continue
		}
		if int(rs.Info) >= len(f.Sections) || f.Sections[rs.Info] != text {
			continue
		}
		data, err := rs.Data()
		if err != nil {
			return nil, err
		}
		for off := 0; off+24 <= len(data); off += 24 {
			roff := le64(data[off:])
			rinfo := le64(data[off+8:])
			radd := int64(le64(data[off+16:]))
			typ := elf.R_X86_64(rinfo & 0xffffffff)
			sidx := rinfo >> 32
			if typ != elf.R_X86_64_PLT32 && typ != elf.R_X86_64_PC32 {
				return nil, fmt.Errorf("unexpected relocation %v at %#x", typ, roff)
			}
			if roff >= 1 && o.code[roff-1] == 0xe8 && radd == -4 {
				o.calls[roff-1] = syms[sidx-1].Name
				continue
			}
			if typ != elf.R_X86_64_PC32 {
				return nil, fmt.Errorf("relocation %v at %#x is not a CALL rel32 (tail call or data?)", typ, roff)
			}
			o.refs[roff] = objRef{syms[sidx-1].Name, radd}
		}
	}
	return o, nil
}

// funcHeader is the TEXT directive one lowered function starts with, shared
// by both emit entries so a register-entry leaf's saved-register frame is
// always declared. A leaf that saved registers (Meta.Frame > 0) must not be
// written with a zero frame: Go would give it no stack space and its stores
// would overwrite the return address. A leaf without a frame or a call says
// NOFRAME, which changes nothing in its code (Go writes no prologue for a
// NOSPLIT function with neither) and is what go vet -asmdecl asks of a
// function with no Go declaration. With a call, Go saves BP, which NOFRAME
// would stop, so such a leaf, like one with a frame, would need a Go
// declaration for that check (TestEngineAsmDecl).
//
// A NOSPLIT unified function without a frame says NOFRAME as well, which the
// Go assembler gives a frameless leaf by itself: on windows its room check
// calls goasm_room, and the call would otherwise make the assembler save BP
// and move the arguments the body reads. Every unified function is followed
// by its room check (roomCheck).
func funcHeader(goName string, m Meta) string {
	if m.Kind == Register && m.Frame == 0 && len(m.Rec.Calls) == 0 {
		return fmt.Sprintf("TEXT ·%s(SB), NOSPLIT|NOFRAME, $0-0\n", goName)
	}
	if m.Kind == Register {
		return fmt.Sprintf("TEXT ·%s(SB), NOSPLIT, $%d-0\n", goName, m.Frame)
	}
	if m.NoSplit && m.Frame == 0 {
		return fmt.Sprintf("TEXT ·%s(SB), NOSPLIT|NOFRAME, $0-%d\n\tNO_LOCAL_POINTERS\n", goName, m.ArgBytes) + roomCheck(m)
	}
	if m.NoSplit {
		return fmt.Sprintf("TEXT ·%s(SB), NOSPLIT, $%d-%d\n\tNO_LOCAL_POINTERS\n", goName, m.Frame, m.ArgBytes) + roomCheck(m)
	}
	return fmt.Sprintf("TEXT ·%s(SB), $%d-%d\n\tNO_LOCAL_POINTERS\n", goName, m.Frame, m.ArgBytes) + roomCheck(m)
}

// RoomBelow is how many bytes past Go's stack guard every unified function
// on windows has below its frame before its body runs (docs/changes/c-engine.md,
// When the engine fails). Windows writes the state of the CPU at a fault
// below the stack pointer of the faulting thread, which is the goroutine's
// stack, and Go keeps 4 KiB below each stack for it (golang/go#81238). With
// AMX that state takes 11.7 KiB, and with the frames of the exception
// dispatch 12.8 KiB. The guard is 5024 bytes above the bottom of the stack
// on windows/amd64 (800 + 4096 + 128), so a body has at least
// RoomBelow + 5024 bytes below its stack pointer, and a register-entry leaf
// it calls, which takes less than 128 bytes and the return address, at least
// 17176. A NOSPLIT function checks too, since Go calls some directly.
const RoomBelow = 12 << 10

// RoomFrame is the frame of goasm_room. Its own stack check passes only when
// SP - (RoomFrame + 8 - 128) is above the guard, so the room check of the
// function that called it then passes with 8 bytes to spare, and morestack
// grows the stack for that frame.
const RoomFrame = RoomBelow + 128

// RoomFile is the generated file of goasm_room. Its name builds it on
// windows/amd64 only, the one system with the room check.
const RoomFile = "engine_room_windows_amd64.s"

// roomCheck is the check that follows Go's prologue of a unified function, in
// the assembly of windows only: when the stack less RoomBelow is at or below
// the guard in g, it calls goasm_room, whose stack check grows the stack.
// R14 holds g after a split prologue, and a NOSPLIT function loads it first.
// No argument is in a register yet (the body loads them from the Go stack)
// and R12 is the scratch register of Go's own check, so both are free.
// The JHI lands on a NOP, not on the PCALIGN of the body that follows: Go
// before 1.26.11 and 1.27.4 loops forever on a forward branch to a PCALIGN
// once a branch of the function needs its long form (golang/go#81792).
func roomCheck(m Meta) string {
	load := ""
	if m.NoSplit {
		load = "\tMOVQ (TLS), R14\n"
	}
	return "#ifdef GOOS_windows\n" + load +
		fmt.Sprintf("\tLEAQ -%d(SP), R12\n\tCMPQ R12, 16(R14)\n\tJHI 2(PC)\n\tCALL ·goasm_room(SB)\n\tBYTE $0x90\n", RoomBelow) +
		"#endif\n"
}

// roomText is goasm_room: a function with a frame of RoomFrame bytes that
// does nothing, so that its stack check makes the room (see RoomBelow). The
// host's callbacks call it too, before they return into the engine, since the
// stack may have shrunk while they ran. Only windows/amd64 has it: EmitEngine
// writes it into a file whose name says so, as the name of its Go
// declaration does, so that go vet -asmdecl finds the declaration wherever
// the function is built.
func roomText() string {
	return fmt.Sprintf("// goasm_room makes RoomBelow bytes of room below the stack pointer of its\n// caller: its stack check grows the stack for a frame it never touches.\nTEXT ·goasm_room(SB), $%d-0\n\tNO_LOCAL_POINTERS\n\tRET\n", RoomFrame)
}

// roomFunc is goasm_room for the one file Emit writes, which every system
// builds: only windows assembles the function in it.
func roomFunc() string {
	return "#ifdef GOOS_windows\n" + roomText() + "#endif\n"
}

// Emit writes the Go assembly for the functions described by metas.
func Emit(objPath string, metas []Meta, names NameMap, buildTag string) (string, error) {
	o, err := readObject(objPath)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by internal/goasm/gen. DO NOT EDIT.\n")
	fmt.Fprintf(&b, "//go:build %s\n\n", buildTag)
	fmt.Fprintf(&b, "#include \"textflag.h\"\n#include \"funcdata.h\"\n\n")
	for _, m := range metas {
		vs, ok := o.funcs[m.Name]
		if !ok {
			return "", fmt.Errorf("symbol %s absent", m.Name)
		}
		lo, hi := int(vs[0]), int(vs[0]+vs[1])
		b.WriteString(funcHeader(m.GoName, m))
		fmt.Fprintf(&b, "\tPCALIGN $16\n")
		// A function's constants sit after its code: only the code part is
		// disassembled, the constants pass through as bytes.
		code := hi
		if cs, ok := o.funcs[m.Name+".const"]; ok {
			code = int(cs[0])
		}
		fp, err := fpArgsOf(m)
		if err != nil {
			return "", err
		}
		if err := emitFunc(&b, o, lo, code, names, fp); err != nil {
			return "", fmt.Errorf("%s: %w", m.Name, err)
		}
		if err := emitData(&b, o.code[code:hi], code); err != nil {
			return "", fmt.Errorf("%s: %w", m.Name, err)
		}
		fmt.Fprintf(&b, "\tRET\n\n")
	}
	b.WriteString(roomFunc())
	return b.String(), nil
}

// emitBytes writes raw data bytes.
func emitBytes(b *strings.Builder, data []byte) error {
	for len(data) >= 8 {
		fmt.Fprintf(b, "\tQUAD $0x%016x\n", le64(data))
		data = data[8:]
	}
	for _, by := range data {
		fmt.Fprintf(b, "\tBYTE $0x%02x\n", by)
	}
	return nil
}

// emitData writes the data a function carries after its code (constants and
// jump tables) after dataComment, so that a reader of the .s file knows these
// bytes are no instructions. at is where the data starts in the lowered
// object, where the function sits as it will in the binary (FuncPlace).
//
// A PCALIGN $16 goes in front of the data: the Go assembler puts the
// instruction after a PCALIGN on its alignment and raises the alignment of
// the function to it, so a constant pool that an SSE instruction reads with
// an operand that has to be on 16 bytes is on 16 bytes by what the Go
// assembler promises, and not by where the data falls from the start of the
// body. The code reaches the data by displacements already in its bytes, so
// the PCALIGN must add nothing: the data is on 16 bytes in the object
// (llc puts a .p2align 4 in front of it), and a function whose data is not
// is refused here. Should the PCALIGN add bytes all the same, the code
// would read past where the data went: the post-link check refuses that
// (verify_insn.go, the bytes of the body and the targets of its RIP
// references).
func emitData(b *strings.Builder, data []byte, at int) error {
	if len(data) == 0 {
		return nil
	}
	if at%16 != 0 {
		return fmt.Errorf("the data after the code starts %d bytes past a multiple of 16 in the lowered object: a PCALIGN $16 in front of it would move it away from the code that reads it", at%16)
	}
	b.WriteString("\tPCALIGN $16\n" + dataComment + "\n")
	return emitBytes(b, data)
}

// fpArgs is what emitFunc needs to write the moves of a unified function's
// arguments and result by name through FP: the frame its TEXT declares,
// where the lowered text has the Go argument area from SP (Meta.ArgBase) and
// the words of that area by their offset in it.
type fpArgs struct {
	frame, base     int64
	params, results map[int64]GoSlot
}

// fpArgsOf gives the argument area of m, nil for a register-entry leaf,
// which has none.
func fpArgsOf(m Meta) (*fpArgs, error) {
	if m.Kind != Unified {
		return nil, nil
	}
	if m.Rec == nil {
		return nil, fmt.Errorf("%s: no record of the lowering, so no signature", m.Name)
	}
	if m.ArgBase < m.Frame+8 {
		return nil, fmt.Errorf("%s: the argument area at %d(SP) is inside the frame of %d bytes and the return address", m.Name, m.ArgBase, m.Frame)
	}
	fp := &fpArgs{frame: int64(m.Frame), base: int64(m.ArgBase), params: map[int64]GoSlot{}, results: map[int64]GoSlot{}}
	params, results := GoSlots(m.Rec.Sig)
	for _, s := range params {
		fp.params[int64(s.Off)] = s
	}
	for _, s := range results {
		fp.results[int64(s.Off)] = s
	}
	return fp, nil
}

// goMove gives the Go instruction for an instruction of the lowered text
// that reaches above the frame from SP. That must be a load of an argument
// word or a store of a result word, as the lowering writes them, and it is
// written by name through FP: Go promises to find an argument that way, not
// at an offset from SP. The Go assembler encodes it with the same registers
// and, while it lays out the frame as the lowering expects, the same
// displacement, so the bytes stay the same, and the post-link check holds
// them to where Go's prologue puts the area. ok is false for any other
// instruction.
func (fp *fpArgs) goMove(in x86asm.Inst) (line string, ok bool, err error) {
	mi := -1
	for k, a := range in.Args {
		if m, isMem := a.(x86asm.Mem); isMem && m.Base == x86asm.RSP && normMem(m).Disp >= fp.frame {
			mi = k
		}
	}
	if mi < 0 {
		return "", false, nil
	}
	m := normMem(in.Args[mi].(x86asm.Mem))
	if m.Disp < fp.base {
		return "", false, fmt.Errorf("%v reaches %d(SP), Go's saved base pointer or return address", in, m.Disp)
	}
	if mi > 1 {
		return "", false, fmt.Errorf("%v reaches the Go argument area and is no 8-byte move of a register", in)
	}
	sse := in.Op == x86asm.MOVSD_XMM
	reg, isReg := in.Args[1-mi].(x86asm.Reg)
	if (in.Op != x86asm.MOV || in.DataSize != 64) && !sse || !isReg || in.Args[2] != nil || in.MemBytes != 8 ||
		!plain(in) || m != (x86asm.Mem{Base: x86asm.RSP, Disp: m.Disp}) {
		return "", false, fmt.Errorf("%v reaches the Go argument area and is no 8-byte move of a register", in)
	}
	slots, what := fp.params, "argument"
	if mi == 0 {
		slots, what = fp.results, "result"
	}
	s, found := slots[m.Disp-fp.base]
	if !found {
		return "", false, fmt.Errorf("%v: no %s word at %d in the Go argument area", in, what, m.Disp-fp.base)
	}
	if s.Float != sse {
		return "", false, fmt.Errorf("%v moves the %s word %s through the wrong kind of register", in, what, s.Name)
	}
	op := "MOVQ"
	if sse {
		op = "MOVSD"
	}
	r := goReg(strings.ToLower(reg.String()))
	if mi == 0 {
		return fmt.Sprintf("\t%s %s, %s+%d(FP)\n", op, r, s.Name, s.Off), true, nil
	}
	return fmt.Sprintf("\t%s %s+%d(FP), %s\n", op, s.Name, s.Off, r), true, nil
}

// emitFunc disassembles [lo,hi) and writes bytes with CALLs made symbolic,
// and for a unified function (fp not nil) each move of an argument or of the
// result by name through FP.
func emitFunc(b *strings.Builder, o *object, lo, hi int, names NameMap, fp *fpArgs) error {
	i := lo
	pending := []byte{} // raw bytes awaiting a flush
	flush := func() {
		for len(pending) >= 8 {
			var q uint64
			for k := 7; k >= 0; k-- {
				q = q<<8 | uint64(pending[k])
			}
			fmt.Fprintf(b, "\tQUAD $0x%016x\n", q)
			pending = pending[8:]
		}
		for _, by := range pending {
			fmt.Fprintf(b, "\tBYTE $0x%02x\n", by)
		}
		pending = pending[:0]
	}
	for i < hi {
		inst, err := x86asm.Decode(o.code[i:hi], 64)
		if isRDSEED(o.code[i:hi]) {
			return fmt.Errorf("forbidden instruction at %#x: reads the hardware random source", i)
		}
		if err != nil {
			return fmt.Errorf("decode at %#x: %w", i, err)
		}
		if why, bad := systemOps[inst.Op]; bad {
			return fmt.Errorf("forbidden instruction at %#x: %v %s", i, inst, why)
		}
		if fp != nil {
			line, ok, err := fp.goMove(inst)
			if err != nil {
				return fmt.Errorf("at %#x: %w", i, err)
			}
			if ok {
				flush()
				b.WriteString(line)
				i += inst.Len
				continue
			}
		}
		if r, at, ok := o.refIn(i, i+inst.Len); ok {
			// A LEA of read-only data or of a function: the Go assembler
			// writes the same 7-byte RIP-relative LEA with a relocation.
			reg, isReg := inst.Args[0].(x86asm.Reg)
			mem, isMem := inst.Args[1].(x86asm.Mem)
			if inst.Op != x86asm.LEA || !isReg || !isMem || mem.Base != x86asm.RIP || inst.DataSize != 64 {
				return fmt.Errorf("PC-relative reference to %s in an instruction emit cannot write: %v at %#x", r.sym, inst, i)
			}
			off := r.add + int64(i+inst.Len) - int64(at)
			flush()
			sym := names[r.sym]
			if r.sym == RodataSym {
				sym = RodataGo
			}
			if sym == "" {
				return fmt.Errorf("LEA of unknown symbol %s at %#x", r.sym, i)
			}
			if off != 0 {
				fmt.Fprintf(b, "\tLEAQ ·%s+%d(SB), %s\n", sym, off, goReg(strings.ToLower(reg.String())))
			} else {
				fmt.Fprintf(b, "\tLEAQ ·%s(SB), %s\n", sym, goReg(strings.ToLower(reg.String())))
			}
			i += inst.Len
			continue
		}
		if inst.Op == x86asm.CALL {
			flush()
			if name, ok := o.calls[uint64(i)]; ok { // direct E8
				gn := names[name]
				if gn == "" {
					gn = name
				}
				fmt.Fprintf(b, "\tCALL ·%s(SB)\n", gn)
			} else { // indirect
				reg, ok := inst.Args[0].(x86asm.Reg)
				if !ok {
					return fmt.Errorf("indirect call via %v unsupported at %#x", inst.Args[0], i)
				}
				fmt.Fprintf(b, "\tCALL %s\n", goReg(strings.ToLower(reg.String())))
			}
			i += inst.Len
			continue
		}
		if inst.Op == x86asm.JMP {
			// A jmp with a relocation is a tail call: forbidden.
			if _, ok := o.calls[uint64(i)]; ok {
				return fmt.Errorf("tail call (jmp to symbol) at %#x", i)
			}
		}
		pending = append(pending, o.code[i:i+inst.Len]...)
		i += inst.Len
	}
	flush()
	return nil
}

// RodataSym is the symbol the lowering gives engine_rodata in its text, and
// RodataGo the Go symbol emit writes for it.
const (
	RodataSym = "__goasm_rodata"
	RodataGo  = "engine_rodata"
)

// refIn finds a reference whose displacement lies in [lo, hi).
func (o *object) refIn(lo, hi int) (objRef, int, bool) {
	for k := lo; k < hi; k++ {
		if r, ok := o.refs[uint64(k)]; ok {
			return r, k, true
		}
	}
	return objRef{}, 0, false
}

func le64(b []byte) uint64 {
	var v uint64
	for i := 7; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return v
}

// sortedCallOffsets is used by verify.
func (o *object) sortedCallOffsets() []int {
	var offs []int
	for k := range o.calls {
		offs = append(offs, int(k))
	}
	sort.Ints(offs)
	return offs
}

// dataComment is the line before the data a function carries after its code
// (its constants and jump tables), which are bytes but not instructions.
const dataComment = "\t// data"
