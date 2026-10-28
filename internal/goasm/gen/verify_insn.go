// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Instruction-level post-link check of the whole engine. The lowering reads
// llc's text with a hand-written parser and rewrites it; a parser mistake
// that still assembles (a misread operand, a dropped line, a wrong frame
// offset or argument move) would reach the binary without any byte check
// noticing, since the generator's own re-assembly is then the reference. This
// check takes llc's object file instead, which llc writes from its own
// instructions without any text, disassembles it and the generated functions
// in the linked Go binary with x/arch, and matches them instruction by
// instruction. A difference is accepted only where the lowering declared a
// rewrite (FuncRecord) and the check confirms the rewrite's form itself:
// the prologue and epilogue it replaced, the frame mapping applied to each
// displacement, each call's saves, argument moves, call target and result,
// and each data reference resolving to the same llc label plus offset. Every
// other llc instruction must appear in order with the same meaning. The read-
// only data is checked the same way: every byte and every relocated pointer
// of engine_rodata in the binary, and every constant pool and jump table a
// function carries, against llc's object. The signatures are the pass's:
// EngineCheck.Sigs for each function and direct call. For an indirect call
// LLVM itself binds the signature to the call (a kcfi type id the pass sets,
// which llc keeps on the machine call and the plugin in pass/icall.cpp names
// in a symbol at it), and the check reads it from that symbol in llc's
// object. A signature gives only the number of argument and result words:
// which register holds each word is what llc decided (EngineCheck.Probes and
// Facts), and the check holds every entry load, argument move, result move
// and the registers llc's code reads at an entry and after a call to it.
package gen

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

// EngineCheck is what the post-link check reads.
type EngineCheck struct {
	LLC     string // llc's object of the engine, with temporary labels kept
	Lowered string // the re-assembled lowered text, with temporary labels kept
	Bin     string // a linked Go binary of the generated package
	Pkg     string // the package's link symbol prefix
	Names   NameMap
	Unit    *AsmUnit
	Plan    *DataPlan
	Metas   []Meta // lowered functions (unified and register-entry)
	Stubs   []Meta // in missingNames order
	// Sigs and Leaves are the pass's signature of every function and the
	// register-entry leaves it was given, and PreserveNone the convention it
	// gave every unified entry: the check holds each function's record to
	// them.
	Sigs         map[string]Sig
	Leaves       map[string]bool
	PreserveNone bool
	// Probes and Facts are what llc itself decided about registers
	// (facts.go): Probes from llc's run over the probe module of
	// -shadow-probes, which gives the argument and result registers of each
	// unified definition, Facts from the run that wrote LLC, which gives
	// each function's calls, returns and live registers.
	Probes, Facts RegFacts
	// All requires every function to be in the binary and checked.
	All bool
	// Align64 is EngineEmit.Align64: the body of each of these functions
	// starts that many bytes past a multiple of 64, after the PCALIGN $64
	// that replaces the usual PCALIGN $16 and the no-ops that follow it.
	Align64 map[string]int
	// HostPoll says the engine calls host_poll even though llc's object does
	// not name it (it is always required when the object does). The poll is
	// the only place the Go runtime can stop C code to collect garbage, so the
	// Go function must begin with a stack check: a function without one would
	// let a loop of C run on without a safe point.
	HostPoll bool
}

// CheckReport counts what was checked and lists what was not, by reason.
type CheckReport struct {
	Checked map[string]int
	Skipped map[string][]string
}

// CheckEngine runs the instruction-level and data checks.
func CheckEngine(c EngineCheck) (*CheckReport, error) {
	k := &checker{c: c, rep: &CheckReport{Checked: map[string]int{}, Skipped: map[string][]string{}}}
	var err error
	if k.llc, err = openObj(c.LLC); err != nil {
		return nil, err
	}
	if k.low, err = openObj(c.Lowered); err != nil {
		return nil, err
	}
	if k.bin, err = openBin(c.Bin); err != nil {
		return nil, err
	}
	if err := k.checkRodata(); err != nil {
		return k.rep, err
	}
	if hp, ok := k.llc.byName["host_poll"]; c.HostPoll || (ok && hp.Section == elf.SHN_UNDEF) {
		if err := k.checkHostPoll(); err != nil {
			return k.rep, err
		}
		k.rep.Checked["host_poll"]++
	}
	// Every function of llc's object is generated: lowered or a stub.
	generated := map[string]bool{}
	for _, m := range append(append([]Meta{}, c.Metas...), c.Stubs...) {
		generated[m.Name] = true
	}
	at := map[uint64]bool{}
	for name := range generated {
		if s, ok := k.llc.byName[name]; ok && int(s.Section) == k.llc.text {
			at[s.Value] = true
		}
	}
	for _, name := range SortedKeys(k.llc.byName) {
		s := k.llc.byName[name]
		// A second name at a generated function's start is an alias of it.
		if int(s.Section) == k.llc.text && elf.ST_TYPE(s.Info) == elf.STT_FUNC && !generated[name] && !at[s.Value] {
			return k.rep, fmt.Errorf("llc's function %s was neither lowered nor stubbed", name)
		}
	}
	seen := map[string]bool{}
	for _, m := range append(append([]Meta{}, c.Metas...), c.Stubs...) {
		if seen[m.Name] {
			return k.rep, fmt.Errorf("%s is generated twice", m.Name)
		}
		seen[m.Name] = true
	}
	for _, m := range c.Metas {
		if err := k.checkMeta(m); err != nil {
			return k.rep, fmt.Errorf("%s: %w", m.Name, err)
		}
		if _, ok := k.bin.abi0(c.Pkg, c.Names[m.Name]); !ok {
			k.skip("not in the binary (the linker left it out)", m.Name)
			continue
		}
		var err error
		if m.Kind == Register {
			err = k.checkLeaf(m)
			k.rep.Checked["register-entry leaves"]++
		} else {
			err = k.checkUnified(m)
			k.rep.Checked["unified functions"]++
		}
		if err != nil {
			return k.rep, fmt.Errorf("%s: %w", m.Name, err)
		}
	}
	for i, m := range c.Stubs {
		if _, ok := k.bin.abi0(c.Pkg, c.Names[m.Name]); !ok {
			k.skip("not in the binary (the linker left it out)", m.Name)
			continue
		}
		if err := k.checkStub(m, i); err != nil {
			return k.rep, fmt.Errorf("stub %s: %w", m.Name, err)
		}
		k.rep.Checked["stubs"]++
	}
	if c.All {
		for why, fns := range k.rep.Skipped {
			return k.rep, fmt.Errorf("%d functions not checked (%s), e.g. %s", len(fns), why, fns[0])
		}
	}
	return k.rep, nil
}

// checkMeta holds a function's record to what the pass says of it: its
// name and kind, its signature (the number and class of its parameters and
// its result, which give how many words the entry loads and the returns
// store, in the registers llc chose) and its calling convention. The record's own frame is the one the
// emitted function declares.
func (k *checker) checkMeta(m Meta) error {
	r := m.Rec
	if r == nil {
		return fmt.Errorf("no rewrite record")
	}
	if r.Name != m.Name || r.Kind != m.Kind || r.Frame != m.Frame || r.NoSplit != m.NoSplit {
		return fmt.Errorf("the record is of %s (kind %v, frame %d, NOSPLIT %v), the function %s (kind %v, frame %d, NOSPLIT %v)",
			r.Name, r.Kind, r.Frame, r.NoSplit, m.Name, m.Kind, m.Frame, m.NoSplit)
	}
	if m.Kind == Register && !m.NoSplit {
		return fmt.Errorf("a register-entry leaf off the NOSPLIT list")
	}
	sig, ok := k.c.Sigs[m.Name]
	if !ok {
		return fmt.Errorf("the pass gave no signature")
	}
	if !sameSig(sig, r.Sig) || m.RetType != sig.Ret {
		return fmt.Errorf("lowered with the signature %v, the pass's is %v", r.Sig, sig)
	}
	if leaf := k.c.Leaves[m.Name]; leaf != (m.Kind == Register) {
		return fmt.Errorf("lowered as kind %v, but the pass took it for a leaf: %v", m.Kind, leaf)
	}
	if want := k.c.PreserveNone && m.Kind == Unified; r.PreserveNone != want {
		return fmt.Errorf("lowered with preserve_none %v, the pass's convention for it is %v", r.PreserveNone, want)
	}
	return nil
}

type checker struct {
	c        EngineCheck
	llc, low *objFile
	bin      *binFile
	rep      *CheckReport
}

func (k *checker) skip(why, fn string) { k.rep.Skipped[why] = append(k.rep.Skipped[why], fn) }

// ---- object files ----

type objReloc struct {
	typ elf.R_X86_64
	sym elf.Symbol
	add int64
}

type objFile struct {
	f      *elf.File
	byName map[string]elf.Symbol
	data   map[int][]byte
	rel    map[int]map[uint64]objReloc
	text   int
	starts map[int][]uint64 // section -> sorted values of its named symbols
	icall  map[uint64]icallSym
}

// icallSym is the symbol the plugin names at an indirect call.
type icallSym struct {
	name, fn string
	id       uint32
}

func openObj(path string) (*objFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	o := &objFile{f: f, byName: map[string]elf.Symbol{}, data: map[int][]byte{}, rel: map[int]map[uint64]objReloc{},
		text: -1, starts: map[int][]uint64{}, icall: map[uint64]icallSym{}}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	for i, s := range f.Sections {
		if s.Name == ".text" {
			o.text = i
		}
		if s.Type == elf.SHT_PROGBITS && s.Flags&elf.SHF_ALLOC != 0 {
			if o.data[i], err = s.Data(); err != nil {
				return nil, err
			}
		}
	}
	if o.text < 0 {
		return nil, fmt.Errorf("%s: no .text", path)
	}
	for _, s := range syms {
		t := elf.ST_TYPE(s.Info)
		if s.Name == "" || t == elf.STT_SECTION || t == elf.STT_FILE {
			continue
		}
		if _, dup := o.byName[s.Name]; dup {
			return nil, fmt.Errorf("%s: two symbols named %s", path, s.Name)
		}
		o.byName[s.Name] = s
		if strings.HasPrefix(s.Name, ICallPrefix) {
			fn, _, id, ok := ParseICallSymbol(s.Name)
			if !ok || int(s.Section) != o.text {
				return nil, fmt.Errorf("%s: %s is no indirect-call symbol of the text", path, s.Name)
			}
			if prev, dup := o.icall[s.Value]; dup {
				return nil, fmt.Errorf("%s: %s and %s at one place", path, prev.name, s.Name)
			}
			o.icall[s.Value] = icallSym{s.Name, fn, id}
		}
		if s.Section != elf.SHN_UNDEF && int(s.Section) < len(f.Sections) {
			o.starts[int(s.Section)] = append(o.starts[int(s.Section)], s.Value)
		}
	}
	for i := range o.starts {
		sort.Slice(o.starts[i], func(a, b int) bool { return o.starts[i][a] < o.starts[i][b] })
	}
	for _, rs := range f.Sections {
		if rs.Type != elf.SHT_RELA {
			continue
		}
		d, err := rs.Data()
		if err != nil {
			return nil, err
		}
		m := map[uint64]objReloc{}
		for off := 0; off+24 <= len(d); off += 24 {
			info := binary.LittleEndian.Uint64(d[off+8:])
			si := int(info >> 32)
			if si < 1 || si > len(syms) {
				return nil, fmt.Errorf("%s: relocation with symbol index %d", path, si)
			}
			m[binary.LittleEndian.Uint64(d[off:])] = objReloc{elf.R_X86_64(info & 0xffffffff), syms[si-1],
				int64(binary.LittleEndian.Uint64(d[off+16:]))}
		}
		o.rel[int(rs.Info)] = m
	}
	return o, nil
}

// loc is a place in an object: a section and an offset in it.
type loc struct {
	sec int
	off int64
}

// label finds a defined symbol by name.
func (o *objFile) label(name string) (elf.Symbol, error) {
	s, ok := o.byName[name]
	if !ok || s.Section == elf.SHN_UNDEF || int(s.Section) >= len(o.f.Sections) {
		return elf.Symbol{}, fmt.Errorf("label %s not defined in llc's object", name)
	}
	return s, nil
}

// extent is the size of the data object at s: its st_size when llc wrote a
// .size, the entry size of a mergeable constant section, or for a table of
// relative entries the run of relocated 4-byte slots up to the next label,
// with nothing but zero padding after it. Anything else is unknown.
func (o *objFile) extent(s elf.Symbol) (int, error) {
	if s.Size > 0 {
		return int(s.Size), nil
	}
	si := int(s.Section)
	sec := o.f.Sections[si]
	if sec.Flags&elf.SHF_MERGE != 0 && sec.Flags&elf.SHF_STRINGS == 0 && sec.Entsize > 0 {
		return int(sec.Entsize), nil
	}
	end := uint64(len(o.data[si]))
	for _, v := range o.starts[si] {
		if v > s.Value {
			end = v
			break
		}
	}
	n := uint64(0)
	for s.Value+n+4 <= end {
		r, ok := o.rel[si][s.Value+n]
		if !ok || r.typ != elf.R_X86_64_PC32 {
			break
		}
		n += 4
	}
	if n == 0 {
		return 0, fmt.Errorf("size of the data at %s is unknown", s.Name)
	}
	for b := s.Value + n; b < end; b++ {
		if _, rel := o.rel[si][b]; rel || o.data[si][b] != 0 {
			return 0, fmt.Errorf("size of the table at %s is unknown: data follows its relocated entries", s.Name)
		}
	}
	return int(n), nil
}

// ---- the linked binary ----

type binFile struct {
	f    *elf.File
	syms map[string]elf.Symbol
	secs map[*elf.Section][]byte
}

func openBin(path string) (*binFile, error) { return openBinMachine(path, elf.EM_X86_64) }

// openBinMachine opens a linked executable of the machine m.
func openBinMachine(path string, m elf.Machine) (*binFile, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	if f.Type != elf.ET_EXEC || f.Machine != m {
		return nil, fmt.Errorf("%s: not a linked %v executable (PIE pointers are not in the file)", path, m)
	}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	b := &binFile{f: f, syms: map[string]elf.Symbol{}, secs: map[*elf.Section][]byte{}}
	for _, s := range syms {
		b.syms[s.Name] = s
	}
	return b, nil
}

// upTo reads at most n bytes at addr, fewer at the end of a section.
func (b *binFile) upTo(addr uint64, n int) ([]byte, error) {
	for _, s := range b.f.Sections {
		if s.Type == elf.SHT_PROGBITS && addr >= s.Addr && addr < s.Addr+s.Size {
			if end := s.Addr + s.Size; addr+uint64(n) > end {
				n = int(end - addr)
			}
			return b.read(addr, n)
		}
	}
	return nil, fmt.Errorf("address %#x not in the binary", addr)
}

func (b *binFile) read(addr uint64, n int) ([]byte, error) {
	for _, s := range b.f.Sections {
		if s.Type != elf.SHT_PROGBITS || addr < s.Addr || addr+uint64(n) > s.Addr+s.Size {
			continue
		}
		d, ok := b.secs[s]
		if !ok {
			var err error
			if d, err = s.Data(); err != nil {
				return nil, err
			}
			b.secs[s] = d
		}
		return d[addr-s.Addr : addr-s.Addr+uint64(n)], nil
	}
	return nil, fmt.Errorf("address %#x+%d not in the binary", addr, n)
}

// abi0 gives the ABI0 entry of a Go-visible function: the assembly body
// (pkg.f, or pkg.f.abi0 when Go also has an ABIInternal wrapper) or the ABI0
// wrapper of a Go function. Every CALL, LEAQ and DATA of the generated
// assembly names this symbol.
func (b *binFile) abi0(pkg, goName string) (elf.Symbol, bool) {
	if s, ok := b.syms[pkg+"."+goName+".abi0"]; ok {
		return s, true
	}
	s, ok := b.syms[pkg+"."+goName]
	return s, ok
}

func (k *checker) funcAddr(cname string) (uint64, error) {
	gn := k.c.Names[cname]
	if gn == "" {
		gn = cname
	}
	s, ok := k.bin.abi0(k.c.Pkg, gn)
	if !ok {
		return 0, fmt.Errorf("function %s not in the binary", cname)
	}
	return s.Value, nil
}

// ---- instruction streams ----

type insn struct {
	at uint64 // section offset (llc) or address (binary)
	in x86asm.Inst
}

// stream is a run of decoded code without its padding: the no-ops are left
// out, and at maps the start of every decoded instruction (a no-op too) to
// the index of the first real one at or after it, so a branch to padding
// lands where execution would.
type stream struct {
	ins []insn
	at  map[uint64]int
}

// isPad reports alignment padding: decode returns NOP only for the padding
// encodings below. XCHG of a 32-bit register with itself clears the upper
// half, so it is a real instruction.
func isPad(in x86asm.Inst) bool { return in.Op == x86asm.NOP }

// padNops are the no-ops the assemblers pad with, and the only ones the
// check skips: the Go assembler's table (1 to 9 bytes), and LLVM's (the
// same, then a 10-byte NOPW with a CS override and up to five more 0x66 in
// front of it for 11 to 15 bytes).
var padNops = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range []string{
		"\x90",
		"\x66\x90",
		"\x0f\x1f\x00",
		"\x0f\x1f\x40\x00",
		"\x0f\x1f\x44\x00\x00",
		"\x66\x0f\x1f\x44\x00\x00",
		"\x0f\x1f\x80\x00\x00\x00\x00",
		"\x0f\x1f\x84\x00\x00\x00\x00\x00",
		"\x66\x0f\x1f\x84\x00\x00\x00\x00\x00",
	} {
		m[s] = true
	}
	for k := 0; k <= 5; k++ {
		m[strings.Repeat("\x66", k+1)+"\x2e\x0f\x1f\x84\x00\x00\x00\x00\x00"] = true
	}
	return m
}()

// decode decodes one instruction and puts its operands in the form the
// check compares: x86asm returns a 32-bit displacement zero-extended, while
// the processor sign-extends it in 64-bit mode. It refuses what would not
// run as decoded, so neither side can hold it: an opcode x86asm does not
// know or an instruction cut short (for both it may return a lone prefix
// and no error), a prefix it marks invalid (LOCK on an instruction that
// takes none faults) or ignored (a repeated prefix, a segment override other
// than FS and GS, REP on an instruction that does not repeat: reserved or
// without effect, so not what llc or the Go assembler write), and a no-op
// other than the padding encodings. It also refuses the instructions that
// reach the system or read machine state (forbidden.go), so no code of either
// side holds one.
func decode(b []byte) (x86asm.Inst, error) {
	in, err := x86asm.Decode(b, 64)
	if isRDSEED(b) {
		return in, fmt.Errorf("forbidden instruction % x: reads the hardware random source", b[:min(len(b), 15)])
	}
	if err != nil || in.Op == 0 || in.Len <= 0 || in.Len > len(b) {
		return in, fmt.Errorf("unknown or incomplete instruction % x", b[:min(len(b), 15)])
	}
	if why, bad := systemOps[in.Op]; bad {
		return in, fmt.Errorf("forbidden instruction % x: %s", b[:in.Len], why)
	}
	for _, p := range in.Prefix {
		if p != 0 && p&x86asm.PrefixInvalid != 0 {
			return in, fmt.Errorf("invalid prefix %#02x in % x", byte(p), b[:in.Len])
		}
	}
	if in.Op == x86asm.NOP {
		if !padNops[string(b[:in.Len])] {
			return in, fmt.Errorf("no-op % x is none of the padding encodings", b[:in.Len])
		}
		return in, nil
	}
	for _, p := range in.Prefix {
		if p != 0 && p&x86asm.PrefixIgnored != 0 && p&x86asm.PrefixImplicit == 0 {
			return in, fmt.Errorf("ignored prefix %#02x in % x", byte(p), b[:in.Len])
		}
	}
	for i, a := range in.Args {
		if m, ok := a.(x86asm.Mem); ok {
			m.Disp = int64(int32(m.Disp))
			if m.Index == 0 {
				m.Scale = 0
			}
			in.Args[i] = m
		}
	}
	return in, nil
}

func decodeStream(code []byte, base uint64) (stream, error) {
	s := stream{at: map[uint64]int{}}
	var pads []uint64
	for off := 0; off < len(code); {
		in, err := decode(code[off:])
		if err != nil {
			return s, fmt.Errorf("cannot decode at %#x: %v", base+uint64(off), err)
		}
		a := base + uint64(off)
		if isPad(in) {
			pads = append(pads, a)
		} else {
			for _, p := range pads {
				s.at[p] = len(s.ins)
			}
			pads = pads[:0]
			s.at[a] = len(s.ins)
			s.ins = append(s.ins, insn{a, in})
		}
		off += in.Len
	}
	for _, p := range pads {
		s.at[p] = len(s.ins)
	}
	s.at[base+uint64(len(code))] = len(s.ins)
	return s, nil
}

// ---- registers and the ABI, written out again here so the check does not
// share the lowering's tables ----

var gpRegs = map[string]x86asm.Reg{"rax": x86asm.RAX, "rbx": x86asm.RBX, "rcx": x86asm.RCX, "rdx": x86asm.RDX,
	"rsi": x86asm.RSI, "rdi": x86asm.RDI, "r8": x86asm.R8, "r9": x86asm.R9, "r10": x86asm.R10, "r11": x86asm.R11,
	"r12": x86asm.R12, "r13": x86asm.R13, "r14": x86asm.R14, "r15": x86asm.R15}

var calleeSavedRegs = map[x86asm.Reg]bool{x86asm.RBX: true, x86asm.R12: true, x86asm.R13: true, x86asm.R14: true, x86asm.R15: true}

// frameRegs are every name of RSP and RBP.
var frameRegs = map[x86asm.Reg]bool{x86asm.RSP: true, x86asm.ESP: true, x86asm.SP: true, x86asm.SPB: true,
	x86asm.RBP: true, x86asm.EBP: true, x86asm.BP: true, x86asm.BPB: true}

// sigWords is the number of argument words of a signature.
func sigWords(sig Sig) int {
	n := 0
	for _, p := range sig.Params {
		n += p.words()
	}
	return n
}

func memOf(in x86asm.Inst) (x86asm.Mem, int, bool) {
	for i, a := range in.Args {
		if m, ok := a.(x86asm.Mem); ok {
			return m, i, true
		}
	}
	return x86asm.Mem{}, -1, false
}

func normMem(m x86asm.Mem) x86asm.Mem {
	if m.Index == 0 {
		m.Scale = 0
	}
	m.Disp = int64(int32(m.Disp))
	return m
}

// plain reports an instruction without a lock, repeat or branch-hint
// prefix: the form of every instruction the check expects in a rewrite (the
// moves, calls and jumps it adds, and Go's prologue and epilogue). A segment
// override shows in the memory operand, which the templates compare.
func plain(in x86asm.Inst) bool {
	for _, p := range semPrefixes(in) {
		if p != x86asm.PrefixFS && p != x86asm.PrefixGS {
			return false
		}
	}
	return true
}

// isStackMove reports a 64-bit (or SSE scalar) move between reg and
// disp(RSP), a store when store is set.
func isStackMove(in x86asm.Inst, store bool, reg x86asm.Reg, disp int64, sse bool) bool {
	op := x86asm.MOV
	if sse {
		op = x86asm.MOVSD_XMM
	}
	mi, ri := 1, 0
	if store {
		mi, ri = 0, 1
	}
	m, ok1 := in.Args[mi].(x86asm.Mem)
	r, ok2 := in.Args[ri].(x86asm.Reg)
	if in.Op != op || !plain(in) || !ok1 || !ok2 || in.Args[2] != nil || in.MemBytes != 8 || (!sse && in.DataSize != 64) {
		return false
	}
	m = normMem(m)
	return r == reg && m == (x86asm.Mem{Base: x86asm.RSP, Disp: disp})
}

// semPrefixes lists the prefixes that change what an instruction does
// (LOCK, REP, segment and branch hints), leaving out REX and the size
// prefixes, which DataSize and AddrSize already carry, and the ones the
// decoder marks implied or ignored.
func semPrefixes(in x86asm.Inst) []x86asm.Prefix {
	var out []x86asm.Prefix
	for _, p := range in.Prefix {
		if p == 0 {
			break
		}
		if p&(x86asm.PrefixImplicit|x86asm.PrefixIgnored) != 0 || p.IsREX() || p.IsVEX() {
			continue
		}
		switch b := p & 0xff; b {
		case 0x66, 0x67:
		default:
			out = append(out, b)
		}
	}
	return out
}

// sameMeaning compares two instructions with the memory operand of the
// first replaced by want (when want is set) and with any relative branch
// argument left out (branches are compared through the instruction map).
func sameMeaning(a, b x86asm.Inst, want *x86asm.Mem) bool {
	if a.Op != b.Op || a.DataSize != b.DataSize || a.AddrSize != b.AddrSize || a.MemBytes != b.MemBytes {
		return false
	}
	pa, pb := semPrefixes(a), semPrefixes(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return false
		}
	}
	for i := range a.Args {
		x, y := a.Args[i], b.Args[i]
		if mx, ok := x.(x86asm.Mem); ok {
			my, ok := y.(x86asm.Mem)
			if !ok {
				return false
			}
			if want != nil {
				mx = *want
			}
			if normMem(mx) != normMem(my) {
				return false
			}
			continue
		}
		if _, ok := x.(x86asm.Rel); ok {
			if _, ok := y.(x86asm.Rel); !ok {
				return false
			}
			continue
		}
		if x != y {
			return false
		}
	}
	return true
}

// ---- the Go wrapper around each body ----

type goFrame struct {
	split     bool   // Go checks the stack and grows it (not NOSPLIT)
	bp        bool   // Go pushed BP
	frame     int64  // Go's SP adjustment below BP
	more      uint64 // the stack check's branch to morestack
	afterLead uint64 // the first byte after Go's prologue
}

func (k *checker) dec(addr uint64) (x86asm.Inst, error) {
	b, err := k.bin.upTo(addr, 16)
	if err != nil {
		return x86asm.Inst{}, err
	}
	in, err := decode(b)
	if err != nil {
		return in, fmt.Errorf("cannot decode the binary at %#x: %v", addr, err)
	}
	if !plain(in) {
		return in, fmt.Errorf("a prefixed instruction in Go's code at %#x: %v", addr, in)
	}
	return in, nil
}

func isReg(a x86asm.Arg, r x86asm.Reg) bool { x, ok := a.(x86asm.Reg); return ok && x == r }

func spAdjust(in x86asm.Inst) (int64, bool) {
	if !isReg(in.Args[0], x86asm.RSP) || in.DataSize != 64 {
		return 0, false
	}
	imm, ok := in.Args[1].(x86asm.Imm)
	if !ok {
		return 0, false
	}
	v := int64(int32(imm))
	switch in.Op {
	case x86asm.SUB:
		return v, true
	case x86asm.ADD:
		return -v, true
	}
	return 0, false
}

// goPrologue reads the prologue the Go assembler put before a body: the g
// load and stack check of a split function, then BP and the frame.
func (k *checker) goPrologue(start uint64) (goFrame, error) {
	var g goFrame
	a := start
	in, err := k.dec(a)
	if err != nil {
		return g, err
	}
	if in.Op == x86asm.MOV && isReg(in.Args[0], x86asm.R14) {
		m, ok := in.Args[1].(x86asm.Mem)
		if !ok || m.Segment != x86asm.FS || m.Base != 0 || m.Disp != -8 {
			return g, fmt.Errorf("unknown Go prologue at %#x: %v", a, in)
		}
		g.split = true
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		check := x86asm.RSP
		if in.Op == x86asm.LEA && isReg(in.Args[0], x86asm.R12) {
			if m, ok := in.Args[1].(x86asm.Mem); !ok || m.Base != x86asm.RSP || m.Index != 0 {
				return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
			}
			check = x86asm.R12
			a += uint64(in.Len)
			if in, err = k.dec(a); err != nil {
				return g, err
			}
		}
		guard, ok := in.Args[1].(x86asm.Mem)
		if in.Op != x86asm.CMP || !isReg(in.Args[0], check) || !ok || guard.Base != x86asm.R14 || guard.Disp != 16 {
			return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
		}
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		rel, ok := in.Args[0].(x86asm.Rel)
		if in.Op != x86asm.JBE || !ok {
			return g, fmt.Errorf("unknown Go stack check at %#x: %v", a, in)
		}
		g.more = uint64(int64(a) + int64(in.Len) + int64(rel))
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
	}
	if in.Op == x86asm.PUSH && isReg(in.Args[0], x86asm.RBP) {
		g.bp = true
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		if in.Op != x86asm.MOV || !isReg(in.Args[0], x86asm.RBP) || !isReg(in.Args[1], x86asm.RSP) {
			return g, fmt.Errorf("unknown Go prologue at %#x: %v", a, in)
		}
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return g, err
		}
		if d, ok := spAdjust(in); ok {
			g.frame = d
			a += uint64(in.Len)
		}
	}
	g.afterLead = a
	return g, nil
}

// goEpilogue checks what follows a body at e: Go's expansion of RET, then
// for a split function the morestack call and the jump back to the start.
func (k *checker) goEpilogue(g goFrame, e uint64, sym elf.Symbol) error {
	a := e
	next := func() (x86asm.Inst, error) {
		in, err := k.dec(a)
		if err == nil {
			a += uint64(in.Len)
		}
		return in, err
	}
	in, err := next()
	if err != nil {
		return err
	}
	if g.bp {
		if g.frame != 0 {
			if d, ok := spAdjust(in); !ok || d != -g.frame {
				return fmt.Errorf("Go epilogue at %#x does not free the %d-byte frame: %v", e, g.frame, in)
			}
			if in, err = next(); err != nil {
				return err
			}
		}
		if in.Op != x86asm.POP || !isReg(in.Args[0], x86asm.RBP) {
			return fmt.Errorf("Go epilogue at %#x does not restore BP: %v", e, in)
		}
		if in, err = next(); err != nil {
			return err
		}
	}
	if in.Op != x86asm.RET || in.Args[0] != nil {
		return fmt.Errorf("Go epilogue at %#x does not return: %v", e, in)
	}
	if g.split {
		if a != g.more {
			return fmt.Errorf("the stack check branches to %#x, not to the morestack call at %#x", g.more, a)
		}
		ms, ok := k.bin.syms["runtime.morestack_noctxt.abi0"]
		if !ok {
			return fmt.Errorf("no runtime.morestack_noctxt.abi0 in the binary")
		}
		at := a
		if in, err = next(); err != nil {
			return err
		}
		if rel, ok := in.Args[0].(x86asm.Rel); in.Op != x86asm.CALL || !ok || uint64(int64(a)+int64(rel)) != ms.Value {
			return fmt.Errorf("no morestack call at %#x: %v", at, in)
		}
		at = a
		if in, err = next(); err != nil {
			return err
		}
		if rel, ok := in.Args[0].(x86asm.Rel); in.Op != x86asm.JMP || !ok || uint64(int64(a)+int64(rel)) != sym.Value {
			return fmt.Errorf("no jump back to the start after morestack at %#x: %v", at, in)
		}
	}
	end := sym.Value + sym.Size
	if a > end {
		return fmt.Errorf("Go epilogue runs past the symbol's end")
	}
	pad, err := k.bin.read(a, int(end-a))
	if err != nil {
		return err
	}
	for _, b := range pad {
		if b != 0xcc && b != 0x90 && b != 0 {
			return fmt.Errorf("unexplained bytes after the Go epilogue at %#x", a)
		}
	}
	return nil
}

// bodyAt is where the emitter puts the body of a function whose Go prologue
// ends at lead: after a PCALIGN $64 and the bytes of EngineCheck.Align64 for
// one of those, else after a PCALIGN $16. The padding runs, so the check
// holds it to no-ops (allPad).
func (k *checker) bodyAt(name string, lead uint64) uint64 {
	if off, ok := k.c.Align64[name]; ok {
		return alignUp64(lead, 64) + uint64(off)
	}
	return alignUp64(lead, 16)
}

// checkHostPoll holds the Go function host_poll to the one thing the poll
// is for: a safe point. It must begin with Go's stack check, a compare of the
// stack (or the stack less the frame) with the guard in g, a branch on below
// or equal to a call of runtime.morestack_noctxt, and a jump back to its start.
// A small function that calls nothing is marked NOSPLIT by the compiler and
// has none of this.
func (k *checker) checkHostPoll() error {
	sym, ok := k.bin.syms[k.c.Pkg+".host_poll"]
	if !ok {
		return fmt.Errorf("host_poll is called but is not in the binary")
	}
	fail := func(f string, a ...any) error {
		return fmt.Errorf("host_poll has no stack check, so the Go runtime cannot stop C code at it: "+f, a...)
	}
	a := sym.Value
	in, err := k.dec(a)
	if err != nil {
		return err
	}
	check := x86asm.RSP
	if m, ok := in.Args[1].(x86asm.Mem); in.Op == x86asm.LEA && isReg(in.Args[0], x86asm.R12) && ok && m.Base == x86asm.RSP && m.Index == 0 {
		check = x86asm.R12
		a += uint64(in.Len)
		if in, err = k.dec(a); err != nil {
			return err
		}
	}
	guard, ok := in.Args[1].(x86asm.Mem)
	if in.Op != x86asm.CMP || !isReg(in.Args[0], check) || !ok || guard.Base != x86asm.R14 || guard.Index != 0 || guard.Disp != 16 {
		return fail("it starts with %v", in)
	}
	a += uint64(in.Len)
	if in, err = k.dec(a); err != nil {
		return err
	}
	rel, ok := in.Args[0].(x86asm.Rel)
	if in.Op != x86asm.JBE || !ok {
		return fail("the compare is followed by %v, not a branch to morestack", in)
	}
	t := uint64(int64(a) + int64(in.Len) + int64(rel))
	if t < sym.Value || t >= sym.Value+sym.Size {
		return fail("its stack check branches to %#x, outside the function", t)
	}
	ms, ok := k.bin.syms["runtime.morestack_noctxt.abi0"]
	if !ok {
		return fmt.Errorf("no runtime.morestack_noctxt.abi0 in the binary")
	}
	called := false
	for n := 0; n < 24; n++ {
		if in, err = k.dec(t); err != nil {
			return err
		}
		rel, isRel := in.Args[0].(x86asm.Rel)
		switch {
		case in.Op == x86asm.CALL && isRel && uint64(int64(t)+int64(in.Len)+int64(rel)) == ms.Value:
			called = true
		case in.Op == x86asm.JMP && isRel:
			if !called || uint64(int64(t)+int64(in.Len)+int64(rel)) != sym.Value {
				return fail("its stack check does not call morestack and jump back to the start")
			}
			return nil
		case in.Op == x86asm.RET || in.Op == x86asm.CALL:
			return fail("its stack check does not call morestack and jump back to the start")
		}
		t += uint64(in.Len)
	}
	return fail("its stack check does not call morestack and jump back to the start")
}

// ---- one unified function ----

type branch struct {
	llcTarget, binTarget uint64
	at                   uint64
}

type matcher struct {
	k        *checker
	r        *FuncRecord
	L, B     stream
	j        int
	bStart   []int
	branches []branch
	rets     []uint64 // binary targets of the jumps that replace returns
	calls    int
	icalls   int // indirect calls, each with LLVM's signature symbol
	nrets    int
	refs     int
	// What llc decided for the function (its facts) and for its probe:
	// the registers of its argument words and of its result.
	facts     *FuncFacts
	args, res []x86asm.Reg
	argBase   int64
	body      uint64 // address of the body in the binary
	lowSym    elf.Symbol
}

func (m *matcher) next(what string) (insn, error) {
	if m.j >= len(m.B.ins) {
		return insn{}, fmt.Errorf("the generated body ends where %s should be", what)
	}
	m.j++
	return m.B.ins[m.j-1], nil
}

// bodyOf locates the lowered function in the re-assembled object: its start
// and size, and where its code ends (its constants follow).
func (k *checker) bodyOf(name string) (elf.Symbol, uint64, error) {
	s, ok := k.low.byName[name]
	if !ok {
		return s, 0, fmt.Errorf("absent from the re-assembled object")
	}
	code := s.Size
	if c, ok := k.low.byName[name+".const"]; ok {
		code = c.Value - s.Value
	}
	return s, code, nil
}

func (k *checker) llcFunc(name string) (stream, elf.Symbol, error) {
	s, err := k.llc.label(name)
	if err != nil {
		return stream{}, s, err
	}
	if int(s.Section) != k.llc.text || elf.ST_TYPE(s.Info) != elf.STT_FUNC {
		return stream{}, s, fmt.Errorf("%s is not a function in llc's .text", name)
	}
	d := k.llc.data[k.llc.text]
	st, err := decodeStream(d[s.Value:s.Value+s.Size], s.Value)
	return st, s, err
}

func (k *checker) checkUnified(meta Meta) error {
	r := meta.Rec
	L, _, err := k.llcFunc(r.Name)
	if err != nil {
		return err
	}
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[r.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	// The generator's NOSPLIT list says which functions have no stack
	// check (PickNoSplit): a function on it must have none, and every other
	// one must have it, which also catches a leaf the Go assembler made
	// NOSPLIT by itself without the generator knowing. A function that calls
	// keeps BP either way; a frameless one that does not is a NOSPLIT leaf
	// with its arguments right above the return address. argBase below
	// follows what Go wrote.
	if r.NoSplit && g.split {
		return fmt.Errorf("on the NOSPLIT list, but Go checks the stack at its entry")
	}
	if !r.NoSplit && !g.split {
		return fmt.Errorf("not on the NOSPLIT list, but Go does not check the stack at its entry")
	}
	if len(r.Calls) > 0 && !g.bp {
		return fmt.Errorf("a calling function without Go's frame: split %v, BP %v", g.split, g.bp)
	}
	if g.frame != int64(r.Frame) {
		return fmt.Errorf("Go frame is %d bytes, the lowering declared %d", g.frame, r.Frame)
	}
	body := k.bodyAt(r.Name, g.afterLead)
	if err := k.allPad(g.afterLead, body); err != nil {
		return err
	}
	lowSym, codeLen, err := k.bodyOf(r.Name)
	if err != nil {
		return err
	}
	code, err := k.bin.read(body, int(codeLen))
	if err != nil {
		return err
	}
	B, err := decodeStream(code, body)
	if err != nil {
		return err
	}
	e := body + lowSym.Size
	if err := k.goEpilogue(g, e, bsym); err != nil {
		return err
	}
	argBase := 8 + g.frame
	if g.bp {
		argBase += 8
	}
	m := &matcher{k: k, r: r, L: L, B: B, bStart: make([]int, len(L.ins)), argBase: argBase, body: body, lowSym: lowSym}
	if err := m.llcRegs(); err != nil {
		return err
	}
	for i := range m.bStart {
		m.bStart[i] = -1
	}
	if err := m.frameLayout(); err != nil {
		return err
	}
	if err := m.run(); err != nil {
		return err
	}
	// The code ends with the result jump target: the Go epilogue, or a jump
	// over the constants to it.
	retAt := map[uint64]bool{e: true}
	if m.j == len(B.ins)-1 {
		last := B.ins[m.j].in
		if rel, ok := last.Args[0].(x86asm.Rel); last.Op == x86asm.JMP && ok && plain(last) &&
			uint64(int64(B.ins[m.j].at)+int64(last.Len)+int64(rel)) == e {
			retAt[B.ins[m.j].at] = true
			m.j++
		}
	}
	if m.j != len(B.ins) {
		return fmt.Errorf("generated instruction at %#x explained by no llc instruction: %v", B.ins[m.j].at, B.ins[m.j].in)
	}
	for _, t := range m.rets {
		if i, ok := B.at[t]; ok && i < len(B.ins) {
			t = B.ins[i].at
		}
		if !retAt[t] {
			return fmt.Errorf("a return jumps to %#x, not to the Go epilogue", t)
		}
	}
	for _, br := range m.branches {
		li, ok := L.at[br.llcTarget]
		if !ok || li >= len(L.ins) || m.bStart[li] < 0 {
			return fmt.Errorf("branch at %#x: llc's target %#x is no instruction of the body", br.at, br.llcTarget)
		}
		bi, ok := B.at[br.binTarget]
		if !ok || bi != m.bStart[li] {
			return fmt.Errorf("branch at %#x goes to %#x, not to where llc's target %#x went", br.at, br.binTarget, br.llcTarget)
		}
	}
	if m.calls != len(r.Calls) || m.refs != len(r.Refs) {
		return fmt.Errorf("the lowering declared %d calls and %d references, llc's object has %d and %d",
			len(r.Calls), len(r.Refs), m.calls, m.refs)
	}
	if m.calls != len(m.facts.Calls) || m.nrets != len(m.facts.Rets) {
		return fmt.Errorf("llc's facts list %d calls and %d returns, its object has %d and %d",
			len(m.facts.Calls), len(m.facts.Rets), m.calls, m.nrets)
	}
	// Every signature symbol in the function sits on one of its indirect
	// calls.
	lsym, _ := k.llc.label(r.Name)
	syms := 0
	for at := range k.llc.icall {
		if at >= lsym.Value && at < lsym.Value+lsym.Size {
			syms++
		}
	}
	if syms != m.icalls {
		return fmt.Errorf("llc's object has %d signature symbols in the function, on %d indirect calls", syms, m.icalls)
	}
	return m.bodyData()
}

// llcRegs reads what llc decided for the function: the registers of its
// argument words and of its result from its probe, which reads every
// argument (the function's own live-in list leaves out the ones it does not
// read), and its calls, returns and live registers. The signature gives
// only how many words there are.
func (m *matcher) llcRegs() error {
	name := m.r.Name
	p, ok := m.k.c.Probes[ProbePrefix+name]
	if !ok {
		return fmt.Errorf("llc gave its probe (-shadow-probes) no registers")
	}
	if m.facts, ok = m.k.c.Facts[name]; !ok {
		return fmt.Errorf("llc wrote no register facts of it")
	}
	if len(p.Rets) != 1 {
		return fmt.Errorf("its probe has %d returns", len(p.Rets))
	}
	var err error
	if m.args, err = x86Regs(p.Args); err != nil {
		return err
	}
	if m.res, err = x86Regs(p.Rets[0]); err != nil {
		return err
	}
	if len(m.args) != sigWords(m.r.Sig) || len(m.res) != m.r.Sig.Ret.words() {
		return fmt.Errorf("llc passes its arguments in %v and returns its result in %v, its signature %v has %d and %d words",
			m.args, m.res, m.r.Sig, sigWords(m.r.Sig), m.r.Sig.Ret.words())
	}
	return nil
}

func alignUp64(x, a uint64) uint64 { return (x + a - 1) / a * a }

func (k *checker) allPad(from, to uint64) error {
	if to <= from {
		return nil
	}
	b, err := k.bin.read(from, int(to-from))
	if err != nil {
		return err
	}
	s, err := decodeStream(b, from)
	if err != nil || len(s.ins) != 0 {
		return fmt.Errorf("unexplained code between the Go prologue and the body at %#x", from)
	}
	return nil
}

// frameLayout checks the declared Go frame holds its parts apart: the
// outgoing arguments below the saved registers, those below the locals, all
// inside the frame, and the saved set covering every callee-saved register
// llc's code keeps a value in across a call.
func (m *matcher) frameLayout() error {
	r := m.r
	if r.SaveBase < 0 || r.SaveBase+8*len(r.Saved) > r.LocalBase || r.LocalBase+r.Local > r.Frame {
		return fmt.Errorf("frame parts overlap: saves at %d (%d), locals at %d (%d), frame %d",
			r.SaveBase, len(r.Saved), r.LocalBase, r.Local, r.Frame)
	}
	saved := map[string]bool{}
	for _, s := range r.Saved {
		if !calleeSavedRegs[gpRegs[s]] {
			return fmt.Errorf("saves %s, which is not callee-saved", s)
		}
		saved[s] = true
	}
	if r.PreserveNone {
		if len(r.Pushed) > 0 {
			return fmt.Errorf("preserve_none code pushed %v", r.Pushed)
		}
		return nil
	}
	for _, p := range r.Pushed {
		if !saved[p] {
			return fmt.Errorf("llc keeps %s across calls, but the calls do not save it", p)
		}
	}
	return nil
}

func (m *matcher) run() error {
	L := m.L.ins
	i := 0
	want := func(ok bool, what string) error {
		if !ok {
			at := uint64(0)
			if i < len(L) {
				at = L[i].at
			}
			return fmt.Errorf("llc's prologue at %#x: no %s", at, what)
		}
		return nil
	}
	if err := want(len(L) >= 2 && L[0].in.Op == x86asm.PUSH && isReg(L[0].in.Args[0], x86asm.RBP), "push of RBP"); err != nil {
		return err
	}
	if err := want(L[1].in.Op == x86asm.MOV && isReg(L[1].in.Args[0], x86asm.RBP) && isReg(L[1].in.Args[1], x86asm.RSP), "mov RSP to RBP"); err != nil {
		return err
	}
	i = 2
	var pushed []string
	for i < len(L) && L[i].in.Op == x86asm.PUSH {
		r, ok := L[i].in.Args[0].(x86asm.Reg)
		if !ok || !calleeSavedRegs[r] {
			break
		}
		pushed = append(pushed, strings.ToLower(r.String()))
		i++
	}
	local := 0
	for i < len(L) {
		in := L[i].in
		if in.Op == x86asm.PUSH && (isReg(in.Args[0], x86asm.RAX) || isReg(in.Args[0], x86asm.RCX)) {
			local += 8
			i++
			continue
		}
		if d, ok := spAdjust(in); ok && d > 0 {
			local += int(d)
			i++
			continue
		}
		break
	}
	if strings.Join(pushed, ",") != strings.Join(m.r.Pushed, ",") || local != m.r.Local {
		return fmt.Errorf("llc's prologue pushes %v and reserves %d, the lowering declared %v and %d", pushed, local, m.r.Pushed, m.r.Local)
	}
	// The entry loads each argument word from the Go argument area into
	// the register llc reads it from, and llc's code reads no other
	// register before writing it: its prologue's pushes, which the lowering
	// drops, are the only other readers at the entry.
	loaded := map[x86asm.Reg]bool{}
	for w, reg := range m.args {
		b, err := m.next("an argument load")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, reg, m.argBase+int64(8*w), isXMM(reg)) {
			return fmt.Errorf("entry at %#x: %v is not the load of argument word %d into %v, where llc reads it", b.at, b.in, w, reg)
		}
		loaded[reg] = true
	}
	for _, p := range m.r.Pushed {
		loaded[gpRegs[p]] = true
	}
	entry, err := x86Regs(m.facts.Entry)
	if err != nil {
		return err
	}
	for _, reg := range entry {
		if !loaded[reg] {
			return fmt.Errorf("llc's code reads %v at the entry before writing it, and no argument load sets it", reg)
		}
	}
	for i < len(L) {
		m.bStart[i] = m.j
		x := L[i]
		if d, ok := spAdjust(x.in); ok && d < 0 || x.in.Op == x86asm.POP {
			n, err := m.epilogue(i, -d)
			if err != nil {
				return err
			}
			for k := i; k < i+n; k++ {
				m.bStart[k] = m.j
			}
			i += n
			continue
		}
		if err := m.one(x); err != nil {
			return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
		}
		i++
	}
	return nil
}

// epilogue checks that the frame restore at L[i] is a whole canonical
// epilogue (free the locals, pop the pushed registers in reverse, pop RBP)
// right before a return, and returns how many instructions it drops.
func (m *matcher) epilogue(i int, freed int64) (int, error) {
	L := m.L.ins
	k := i
	if L[k].in.Op != x86asm.POP {
		if freed != int64(m.r.Local) {
			return 0, fmt.Errorf("llc frees %d bytes at %#x, its prologue reserved %d", freed, L[k].at, m.r.Local)
		}
		k++
	} else if m.r.Local != 0 {
		return 0, fmt.Errorf("llc pops at %#x without freeing its %d-byte locals", L[k].at, m.r.Local)
	}
	// The pushed registers in reverse, then RBP.
	var pops []x86asm.Reg
	for p := len(m.r.Pushed) - 1; p >= 0; p-- {
		pops = append(pops, gpRegs[m.r.Pushed[p]])
	}
	pops = append(pops, x86asm.RBP)
	for _, r := range pops {
		if k >= len(L) || r == 0 || L[k].in.Op != x86asm.POP || !isReg(L[k].in.Args[0], r) {
			return 0, fmt.Errorf("llc's epilogue at %#x does not pop %v", L[i].at, r)
		}
		k++
	}
	if k >= len(L) || L[k].in.Op != x86asm.RET {
		return 0, fmt.Errorf("llc's epilogue at %#x is not followed by a return", L[i].at)
	}
	return k - i, nil
}

// forbiddenOps are instructions the llc side of the body may not hold: the
// system ones (forbidden.go) and the ones that move SP or BP implicitly
// outside the prologue and epilogue the check accounts for.
var forbiddenOps = func() map[x86asm.Op]string {
	m := map[x86asm.Op]string{
		x86asm.PUSH: "pushes in the body", x86asm.PUSHF: "pushes in the body", x86asm.PUSHFD: "pushes in the body",
		x86asm.PUSHFQ: "pushes in the body", x86asm.POPF: "pops in the body", x86asm.POPFD: "pops in the body",
		x86asm.POPFQ: "pops in the body",
		x86asm.ENTER: "builds a frame in the body", x86asm.LEAVE: "leaves a frame in the body",
	}
	for op, why := range systemOps {
		m[op] = why
	}
	return m
}()

// unalignedOK are the 16-byte moves that need no alignment: the Go frame is
// only 8-byte aligned, so any other access wider than 8 bytes may fault.
var unalignedOK = map[x86asm.Op]bool{x86asm.MOVUPS: true, x86asm.MOVUPD: true, x86asm.MOVDQU: true, x86asm.LDDQU: true}

// one matches a single llc instruction of the body.
func (m *matcher) one(x insn) error {
	in := x.in
	r := m.k.llc.relIn(m.k.llc.text, x)
	if why, bad := forbiddenOps[in.Op]; bad {
		return fmt.Errorf("%s", why)
	}
	switch in.Op {
	case x86asm.RET:
		if in.Args[0] != nil {
			return fmt.Errorf("a return that pops arguments")
		}
		return m.ret()
	case x86asm.CALL:
		return m.call(x, r)
	}
	for _, a := range in.Args {
		if reg, ok := a.(x86asm.Reg); ok && frameRegs[reg] {
			return fmt.Errorf("uses the stack or frame register outside a memory operand")
		}
	}
	mem, _, hasMem := memOf(in)
	if hasMem && frameRegs[mem.Index] {
		return fmt.Errorf("indexes by the stack or frame register")
	}
	if hasMem && mem.Base == 0 && mem.Index == 0 {
		return fmt.Errorf("an absolute memory operand")
	}
	if hasMem && (mem.Segment == x86asm.FS || mem.Segment == x86asm.GS) {
		return fmt.Errorf("a thread-local memory operand")
	}
	if rel, ok := in.Args[0].(x86asm.Rel); ok {
		if r != nil {
			return fmt.Errorf("a branch with a relocation (a tail call)")
		}
		b, err := m.next("a branch")
		if err != nil {
			return err
		}
		brel, ok := b.in.Args[0].(x86asm.Rel)
		if !ok || !sameMeaning(in, b.in, nil) {
			return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
		}
		m.branches = append(m.branches, branch{uint64(int64(x.at) + int64(in.Len) + int64(rel)),
			uint64(int64(b.at) + int64(b.in.Len) + int64(brel)), b.at})
		return nil
	}
	if in.Op == x86asm.JMP {
		// Through a register: kept as it is (its targets are the jump
		// table's entries, checked with the function's data).
		if _, ok := in.Args[0].(x86asm.Reg); !ok {
			return fmt.Errorf("an indirect jump through memory")
		}
	}
	var want *x86asm.Mem
	var target uint64
	switch {
	case hasMem && mem.Base == x86asm.RIP:
		t, err := m.dataRef(x, r, mem)
		if err != nil {
			return err
		}
		target = t
	case r != nil:
		return fmt.Errorf("a relocation outside a RIP-relative operand")
	case hasMem && (mem.Base == x86asm.RBP || mem.Base == x86asm.RSP):
		if in.Op == x86asm.LEA {
			return fmt.Errorf("takes the address of a stack slot, which Go may move")
		}
		if mem.Index != 0 {
			return fmt.Errorf("an indexed frame access, whose range the check cannot bound")
		}
		d, err := m.frameDisp(mem, in.MemBytes)
		if err != nil {
			return err
		}
		mm := mem
		mm.Base, mm.Disp = x86asm.RSP, d
		want = &mm
		if in.MemBytes > 8 && !unalignedOK[in.Op] {
			return fmt.Errorf("a %d-byte frame access that may need alignment the Go frame lacks", in.MemBytes)
		}
	case hasMem && frameRegs[mem.Base]:
		return fmt.Errorf("addresses memory from a narrow frame register")
	}
	b, err := m.next("an instruction")
	if err != nil {
		return err
	}
	if hasMem && mem.Base == x86asm.RIP {
		bm, _, ok := memOf(b.in)
		if !ok || bm.Base != x86asm.RIP {
			return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
		}
		if got := uint64(int64(b.at) + int64(b.in.Len) + bm.Disp); got != target {
			return fmt.Errorf("generated %v at %#x points to %#x, the reference should point to %#x", b.in, b.at, got, target)
		}
		w := mem
		w.Disp = bm.Disp
		want = &w
	}
	if !sameMeaning(in, b.in, want) {
		return fmt.Errorf("generated %v at %#x instead", b.in, b.at)
	}
	return nil
}

// relIn returns the relocation inside instruction x of section sec, with its
// offset from the instruction's start.
func (o *objFile) relIn(sec int, x insn) *relAt {
	for k := 0; k < x.in.Len; k++ {
		if r, ok := o.rel[sec][x.at+uint64(k)]; ok {
			return &relAt{r, k}
		}
	}
	return nil
}

type relAt struct {
	objReloc
	off int
}

// relTarget is where a PC-relative relocation inside x points: the symbol
// plus the addend, corrected from the relocated field to the end of x.
func (o *objFile) relTarget(x insn, r *relAt) (loc, string) {
	return loc{int(r.sym.Section), int64(r.sym.Value) + r.add + int64(x.in.Len-r.off)}, r.sym.Name
}

// frameDisp maps a frame access of llc's code to the Go frame and checks the
// access lies inside the locals: RBP-relative below the saved registers, or
// RSP-relative inside the reserved area.
func (m *matcher) frameDisp(mem x86asm.Mem, width int) (int64, error) {
	r := m.r
	saved := int64(8 * len(r.Pushed))
	local := int64(r.Local)
	w := int64(width)
	if w <= 0 {
		return 0, fmt.Errorf("frame access of unknown width")
	}
	if mem.Base == x86asm.RBP {
		d := mem.Disp
		if d >= -saved || d < -(saved+local) || d+w > -saved {
			return 0, fmt.Errorf("frame access %d(RBP) of %d bytes outside the %d-byte locals", d, w, local)
		}
		return int64(r.LocalBase) + saved + local + d, nil
	}
	d := mem.Disp
	if d < 0 || d+w > local {
		return 0, fmt.Errorf("stack access %d(RSP) of %d bytes outside the %d-byte locals", d, w, local)
	}
	return int64(r.LocalBase) + d, nil
}

// llcIs checks that a place llc's object reaches (through a relocation
// against sym, or resolved inside the object when sym is nil) is label plus
// add: the same section and offset, or for a symbol llc's object leaves
// undefined (a host function) the same name and addend.
func (o *objFile) llcIs(at loc, sym *elf.Symbol, label string, add int64) error {
	if sym != nil && sym.Section == elf.SHN_UNDEF {
		if sym.Name != label || at.off != add {
			return fmt.Errorf("llc's operand is %s%+d, not %s%+d as declared", sym.Name, at.off, label, add)
		}
		return nil
	}
	lab, err := o.label(label)
	if err != nil {
		return err
	}
	if want := (loc{int(lab.Section), int64(lab.Value) + add}); at != want {
		return fmt.Errorf("llc's operand reaches %v, not %s%+d (%v) as declared", at, label, add, want)
	}
	return nil
}

// dataRef resolves one RIP-relative operand: in llc's object through its
// relocation, in the binary through where the lowering declared it points.
// It returns the absolute address the generated instruction must reach.
func (m *matcher) dataRef(x insn, r *relAt, mem x86asm.Mem) (uint64, error) {
	k := m.k
	if m.refs >= len(m.r.Refs) {
		return 0, fmt.Errorf("a RIP-relative reference the lowering did not declare")
	}
	ref := m.r.Refs[m.refs]
	m.refs++
	var at loc
	var sym *elf.Symbol
	if r != nil {
		if r.typ != elf.R_X86_64_PC32 {
			return 0, fmt.Errorf("a %v relocation in a RIP-relative operand", r.typ)
		}
		at, _ = k.llc.relTarget(x, r)
		sym = &r.sym
	} else {
		at = loc{k.llc.text, int64(x.at) + int64(x.in.Len) + mem.Disp}
	}
	if err := k.llc.llcIs(at, sym, ref.Label, ref.Add); err != nil {
		return 0, err
	}
	switch {
	case ref.Func:
		if x.in.Op != x86asm.LEA {
			return 0, fmt.Errorf("reads a function's code")
		}
		if sym == nil || sym.Section != elf.SHN_UNDEF {
			if lab, _ := k.llc.label(ref.Label); int(lab.Section) != k.llc.text || elf.ST_TYPE(lab.Info) != elf.STT_FUNC {
				return 0, fmt.Errorf("%s is not a function of llc's object", ref.Label)
			}
		}
		a, err := k.funcAddr(ref.Label)
		if err != nil {
			return 0, err
		}
		return uint64(int64(a) + ref.Add), nil
	case ref.Blob:
		if x.in.Op != x86asm.LEA {
			return 0, fmt.Errorf("reads engine_rodata without LEA")
		}
		off, ok := k.c.Plan.BlobOff[ref.Label]
		if !ok {
			return 0, fmt.Errorf("%s is not in engine_rodata", ref.Label)
		}
		rs, ok := k.bin.syms[k.c.Pkg+"."+RodataGo]
		if !ok {
			return 0, fmt.Errorf("no engine_rodata in the binary")
		}
		return uint64(int64(rs.Value) + int64(off) + ref.Add), nil
	}
	label := k.c.Plan.Rename[m.r.Name][ref.Label]
	ls, ok := k.low.byName[label]
	if label == "" || !ok {
		return 0, fmt.Errorf("%s has no copy in the body", ref.Label)
	}
	if x.in.Op != x86asm.LEA {
		lab, err := k.llc.label(ref.Label)
		if err != nil {
			return 0, err
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return 0, err
		}
		if ref.Add < 0 || ref.Add+int64(x.in.MemBytes) > int64(n) {
			return 0, fmt.Errorf("reads %d bytes at %s%+d, outside its %d bytes", x.in.MemBytes, ref.Label, ref.Add, n)
		}
	}
	return uint64(int64(m.body) + int64(ls.Value-m.lowSym.Value) + ref.Add), nil
}

// ret matches a return: the result stored into the Go result slot, then a
// jump to the Go epilogue (checked once the body is done).
func (m *matcher) ret() error {
	if m.nrets >= len(m.facts.Rets) {
		return fmt.Errorf("a return llc's facts do not list")
	}
	uses, err := x86Regs(m.facts.Rets[m.nrets])
	if err != nil {
		return err
	}
	m.nrets++
	stored := map[x86asm.Reg]bool{}
	off := m.argBase + int64(8*len(m.args))
	for w, reg := range m.res {
		b, err := m.next("a result store")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, reg, off+int64(8*w), isXMM(reg)) {
			return fmt.Errorf("return: %v at %#x is not the store of %v to the result slot", b.in, b.at, reg)
		}
		stored[reg] = true
	}
	for _, reg := range uses {
		if !stored[reg] {
			return fmt.Errorf("llc's return gives %v, which the result stores do not take", reg)
		}
	}
	b, err := m.next("the jump to the epilogue")
	if err != nil {
		return err
	}
	rel, ok := b.in.Args[0].(x86asm.Rel)
	if b.in.Op != x86asm.JMP || !ok || !plain(b.in) {
		return fmt.Errorf("return: %v at %#x is not a jump to the epilogue", b.in, b.at)
	}
	m.rets = append(m.rets, uint64(int64(b.at)+int64(b.in.Len)+int64(rel)))
	return nil
}

// call matches one call against the next declared call.
func (m *matcher) call(x insn, r *relAt) error {
	k := m.k
	if m.calls >= len(m.r.Calls) {
		return fmt.Errorf("a call the lowering did not declare")
	}
	c := m.r.Calls[m.calls]
	if m.calls >= len(m.facts.Calls) {
		return fmt.Errorf("a call llc's facts do not list")
	}
	cf := m.facts.Calls[m.calls]
	m.calls++
	switch a := x.in.Args[0].(type) {
	case x86asm.Rel:
		if s, ok := k.llc.icall[x.at]; ok {
			return fmt.Errorf("LLVM bound the signature symbol %s to a direct call", s.name)
		}
		if c.Kind != CallUnified && c.Kind != CallLeaf {
			return fmt.Errorf("llc calls directly, the lowering declared %+v", c)
		}
		var at loc
		var sym *elf.Symbol
		if r != nil {
			if r.typ != elf.R_X86_64_PLT32 && r.typ != elf.R_X86_64_PC32 {
				return fmt.Errorf("a call with a %v relocation", r.typ)
			}
			at, _ = k.llc.relTarget(x, r)
			sym = &r.sym
		} else {
			at = loc{k.llc.text, int64(x.at) + int64(x.in.Len) + int64(a)}
		}
		// The target is compared by place, so a call through an alias
		// (musl's weak names) matches the function it names.
		if err := k.llc.llcIs(at, sym, c.Target, 0); err != nil {
			return fmt.Errorf("llc's call is not to %s as declared: %w", c.Target, err)
		}
		if sym == nil || sym.Section != elf.SHN_UNDEF {
			if lab, _ := k.llc.label(c.Target); int(lab.Section) != k.llc.text || elf.ST_TYPE(lab.Info) != elf.STT_FUNC {
				return fmt.Errorf("the call target %s is not a function of llc's object", c.Target)
			}
		}
		name := c.Target
		if cf.Target != name {
			return fmt.Errorf("llc's facts give this call to %s, the lowering %s", cf.Target, name)
		}
		addr, err := k.funcAddr(name)
		if err != nil {
			return err
		}
		direct := func(b insn) error {
			rel, ok := b.in.Args[0].(x86asm.Rel)
			if b.in.Op != x86asm.CALL || !ok || !plain(b.in) || uint64(int64(b.at)+int64(b.in.Len)+int64(rel)) != addr {
				return fmt.Errorf("generated %v at %#x is not a call to %s", b.in, b.at, name)
			}
			return nil
		}
		if c.Kind == CallLeaf {
			if !k.c.Leaves[name] {
				return fmt.Errorf("a direct call to %s kept unmarshalled, which is not a register-entry leaf", name)
			}
			b, err := m.next("a leaf call")
			if err != nil {
				return err
			}
			return direct(b)
		}
		if k.c.Leaves[name] {
			return fmt.Errorf("a call to the leaf %s marshalled as a unified one", name)
		}
		sig, ok := k.c.Sigs[name]
		if !ok || !sameSig(sig, c.Sig) {
			return fmt.Errorf("call to %s marshalled as %v, its signature is %v", name, c.Sig, sig)
		}
		// A call of a unified definition passes its words in the registers
		// the definition reads them from: llc assigns both from the type
		// of each, and a call whose type is not the callee's would differ.
		if p, ok := k.c.Probes[ProbePrefix+name]; ok {
			pa, err := x86Regs(p.Args)
			if err != nil {
				return err
			}
			ca, err := x86Regs(cf.Args)
			if err != nil {
				return err
			}
			if fmt.Sprint(pa) != fmt.Sprint(ca) {
				return fmt.Errorf("llc passes the call's arguments in %v, %s reads them from %v", ca, name, pa)
			}
		}
		return m.marshal(c.Sig, cf, direct)
	case x86asm.Reg:
		if c.Kind != CallReg || gpRegs[c.Target] != a {
			return fmt.Errorf("llc calls through %v, the lowering declared %+v", a, c)
		}
		bound, err := m.boundSig(x)
		if err != nil {
			return err
		}
		if !sameSig(bound, c.Sig) {
			return fmt.Errorf("LLVM bound the signature %v to llc's call through %v, the lowering marshalled it as %v", bound, a, c.Sig)
		}
		if want := fmt.Sprintf("*%d", k.llc.icall[x.at].id); cf.Target != want {
			return fmt.Errorf("llc's facts give this call to %s, its signature symbol has type id %s", cf.Target, want)
		}
		m.icalls++
		return m.marshal(c.Sig, cf, func(b insn) error {
			if b.in.Op != x86asm.CALL || !isReg(b.in.Args[0], a) || !plain(b.in) {
				return fmt.Errorf("generated %v at %#x is not a call through %v", b.in, b.at, a)
			}
			return nil
		})
	case x86asm.Mem:
		// EngineCFlags set +slow-two-mem-ops, so llc loads every call
		// target into a register first.
		return fmt.Errorf("llc calls through memory, which the build rules out")
	}
	return fmt.Errorf("a call form the check does not know")
}

// boundSig reads the signature LLVM bound to the indirect call x: the
// symbol the plugin put at its address in llc's object.
func (m *matcher) boundSig(x insn) (Sig, error) {
	s, ok := m.k.llc.icall[x.at]
	if !ok {
		return Sig{}, fmt.Errorf("llc's indirect call has no signature symbol from LLVM")
	}
	if s.fn != m.r.Name {
		return Sig{}, fmt.Errorf("the signature symbol %s at llc's call names another function", s.name)
	}
	code, err := ICallCode(s.id)
	if err != nil {
		return Sig{}, fmt.Errorf("%s: %w", s.name, err)
	}
	return SigFromCode(code)
}

func sameSig(a, b Sig) bool {
	if a.Ret != b.Ret || len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	return true
}

// marshal matches the glue around a unified call: the saves, a store of each
// argument word into the outgoing area from the register llc put it in, the
// call, the load of each result word into the register llc takes it from,
// and the restores. llc's code reads nothing after the call before writing
// it but the result and the saved registers: the callee, Go code or a
// unified function, keeps no register.
func (m *matcher) marshal(sig Sig, cf CallFacts, call func(insn) error) error {
	r := m.r
	for k, s := range r.Saved {
		b, err := m.next("a register save")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, gpRegs[s], int64(r.SaveBase+8*k), false) {
			return fmt.Errorf("call: %v at %#x is not the save of %s", b.in, b.at, s)
		}
	}
	args, err := x86Regs(cf.Args)
	if err != nil {
		return err
	}
	defs, err := x86Regs(cf.Defs)
	if err != nil {
		return err
	}
	after, err := x86Regs(cf.After)
	if err != nil {
		return err
	}
	if len(args) != sigWords(sig) || len(defs) != sig.Ret.words() {
		return fmt.Errorf("call: llc passes the arguments in %v and takes the result from %v, the signature %v has %d and %d words",
			args, defs, sig, sigWords(sig), sig.Ret.words())
	}
	if 8*(len(args)+len(defs)) > r.SaveBase {
		return fmt.Errorf("call: the %d-word argument area reaches the saved registers at %d", len(args)+len(defs), r.SaveBase)
	}
	for w, reg := range args {
		b, err := m.next("an argument move")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, reg, int64(8*w), isXMM(reg)) {
			return fmt.Errorf("call: %v at %#x is not the move of argument word %d from %v", b.in, b.at, w, reg)
		}
	}
	b, err := m.next("the call")
	if err != nil {
		return err
	}
	if err := call(b); err != nil {
		return err
	}
	set := map[x86asm.Reg]bool{}
	off := int64(8 * len(args))
	for w, reg := range defs {
		b, err := m.next("a result load")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, reg, off+int64(8*w), isXMM(reg)) {
			return fmt.Errorf("call: %v at %#x is not the load of the result into %v", b.in, b.at, reg)
		}
		set[reg] = true
	}
	for _, s := range r.Saved {
		set[gpRegs[s]] = true
	}
	for _, reg := range after {
		if !set[reg] {
			return fmt.Errorf("call: llc's code reads %v after the call before writing it, which neither a result load nor a restore sets", reg)
		}
	}
	for k, s := range r.Saved {
		b, err := m.next("a register restore")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, false, gpRegs[s], int64(r.SaveBase+8*k), false) {
			return fmt.Errorf("call: %v at %#x is not the restore of %s", b.in, b.at, s)
		}
	}
	return nil
}

// sameLabels checks that every other name of a data object (an alias) is,
// in llc's object, the same place as its label, and that the plan sends it
// where it sends the label (placed reports that).
func (k *checker) sameLabels(o *DataObj, lab elf.Symbol, placed func(string) bool) error {
	for _, a := range o.Aliases {
		al, err := k.llc.label(a)
		if err != nil {
			return err
		}
		if al.Section != lab.Section || al.Value != lab.Value {
			return fmt.Errorf("%s is not at %s in llc's object", a, o.Label)
		}
		if !placed(a) {
			return fmt.Errorf("the plan places %s apart from %s", a, o.Label)
		}
	}
	return nil
}

// bodyData checks the data the function carries after its code (constant
// pools, jump tables, copies of data it reads) against llc's object. A jump
// table entry is a difference from the table's start; it must reach, in the
// binary, the instruction llc's entry reaches.
func (m *matcher) bodyData() error {
	k := m.k
	rename := k.c.Plan.Rename[m.r.Name]
	owned := map[string]bool{}
	_, codeLen, _ := k.bodyOf(m.r.Name)
	for _, o := range k.c.Plan.InBody[m.r.Name] {
		label := rename[o.Label]
		ls, ok := k.low.byName[label]
		if !ok {
			return fmt.Errorf("body data %s absent from the re-assembled object", label)
		}
		lab, err := k.llc.label(o.Label)
		if err != nil {
			return err
		}
		if err := k.sameLabels(o, lab, func(a string) bool { return rename[a] == label }); err != nil {
			return err
		}
		owned[o.Label] = true
		for _, a := range o.Aliases {
			owned[a] = true
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return err
		}
		// The data sits after the code, inside the function's symbol.
		if ls.Value < m.lowSym.Value+codeLen || ls.Value+uint64(n) > m.lowSym.Value+m.lowSym.Size {
			return fmt.Errorf("body data %s lies outside the function's data area", o.Label)
		}
		at := m.body + (ls.Value - m.lowSym.Value)
		got, err := k.bin.read(at, n)
		if err != nil {
			return err
		}
		si := int(lab.Section)
		want := k.llc.data[si][lab.Value : lab.Value+uint64(n)]
		for i := 0; i < n; {
			r, rel := k.llc.rel[si][lab.Value+uint64(i)]
			if !rel {
				if got[i] != want[i] {
					return fmt.Errorf("body data %s: byte %d is %#02x in the binary, %#02x in llc's object", o.Label, i, got[i], want[i])
				}
				i++
				continue
			}
			if r.typ != elf.R_X86_64_PC32 || int(r.sym.Section) != k.llc.text || i+4 > n {
				return fmt.Errorf("body data %s: a %v relocation at byte %d", o.Label, r.typ, i)
			}
			lt := uint64(int64(r.sym.Value) + r.add - int64(i))
			li, ok := m.L.at[lt]
			if !ok || li >= len(m.L.ins) || m.bStart[li] < 0 {
				return fmt.Errorf("body data %s: entry %d reaches %#x, no instruction of the body", o.Label, i/4, lt)
			}
			v := int32(binary.LittleEndian.Uint32(got[i:]))
			bt := uint64(int64(at) + int64(v))
			if bi, ok := m.B.at[bt]; !ok || bi != m.bStart[li] {
				return fmt.Errorf("body data %s: entry %d reaches %#x in the binary, not where llc's %#x went", o.Label, i/4, bt, lt)
			}
			i += 4
		}
		k.rep.Checked["body data objects"]++
	}
	for l := range rename {
		if !owned[l] {
			return fmt.Errorf("the body's label for %s belongs to none of its data objects", l)
		}
	}
	return nil
}

// ---- register-entry leaves ----

func (k *checker) checkLeaf(meta Meta) error {
	r := meta.Rec
	L, _, err := k.llcFunc(r.Name)
	if err != nil {
		return err
	}
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[r.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	// Go's assembler saves BP in every function with a frame or a call, so
	// also in a leaf that saves nothing but calls another leaf.
	if g.split || g.frame != int64(r.Frame) || g.bp != (r.Frame > 0 || len(r.Calls) > 0) {
		return fmt.Errorf("Go prologue of a leaf: split %v, BP %v, frame %d (declared %d)", g.split, g.bp, g.frame, r.Frame)
	}
	body := k.bodyAt(r.Name, g.afterLead)
	if err := k.allPad(g.afterLead, body); err != nil {
		return err
	}
	lowSym, codeLen, err := k.bodyOf(r.Name)
	if err != nil {
		return err
	}
	code, err := k.bin.read(body, int(codeLen))
	if err != nil {
		return err
	}
	B, err := decodeStream(code, body)
	if err != nil {
		return err
	}
	e := body + lowSym.Size
	if err := k.goEpilogue(g, e, bsym); err != nil {
		return err
	}
	m := &matcher{k: k, r: r, L: L, B: B, bStart: make([]int, len(L.ins)), body: body, lowSym: lowSym}
	for i := range m.bStart {
		m.bStart[i] = -1
	}
	// A leaf keeps C's registers, so only the targets and the counts of
	// its calls and returns are held to llc's facts.
	var ok bool
	if m.facts, ok = k.c.Facts[r.Name]; !ok {
		return fmt.Errorf("llc wrote no register facts of it")
	}
	// The saves are llc's pushes at the start, each to its own 8-byte slot
	// inside the Go frame, the first push highest as on the native stack.
	var pushed []string
	for len(pushed) < len(L.ins) && L.ins[len(pushed)].in.Op == x86asm.PUSH {
		reg, ok := L.ins[len(pushed)].in.Args[0].(x86asm.Reg)
		if !ok || !calleeSavedRegs[reg] {
			return fmt.Errorf("llc's leaf pushes %v at its start", L.ins[len(pushed)].in.Args[0])
		}
		pushed = append(pushed, strings.ToLower(reg.String()))
	}
	if strings.Join(pushed, ",") != strings.Join(r.Pushed, ",") {
		return fmt.Errorf("llc's leaf pushes %v, the lowering declared %v", pushed, r.Pushed)
	}
	// The slots are 0 to 8n-8 above SP: all inside a frame of 8n bytes.
	n := len(pushed)
	if g.frame < int64(8*n) {
		return fmt.Errorf("the leaf's %d-byte Go frame cannot hold its %d saved registers", g.frame, n)
	}
	slot := map[x86asm.Reg]int64{}
	i := 0
	for ; i < n; i++ {
		reg := gpRegs[pushed[i]]
		slot[reg] = int64(8 * (n - 1 - i))
		b, err := m.next("a register save")
		if err != nil {
			return err
		}
		if !isStackMove(b.in, true, reg, slot[reg], false) {
			return fmt.Errorf("leaf: %v at %#x is not the save of %s", b.in, b.at, pushed[i])
		}
	}
	for ; i < len(L.ins); i++ {
		m.bStart[i] = m.j
		x := L.ins[i]
		switch x.in.Op {
		case x86asm.POP:
			// The pops before a return, in reverse, each a load from its
			// slot.
			for p := n - 1; p >= 0; p-- {
				reg := gpRegs[pushed[p]]
				if i >= len(L.ins) || L.ins[i].in.Op != x86asm.POP || !isReg(L.ins[i].in.Args[0], reg) {
					return fmt.Errorf("llc's leaf at %#x does not pop %v", x.at, reg)
				}
				m.bStart[i] = m.j
				b, err := m.next("a register restore")
				if err != nil {
					return err
				}
				if !isStackMove(b.in, false, reg, slot[reg], false) {
					return fmt.Errorf("leaf: %v at %#x is not the restore of %v", b.in, b.at, reg)
				}
				i++
			}
			if i >= len(L.ins) || L.ins[i].in.Op != x86asm.RET {
				return fmt.Errorf("llc's leaf pops at %#x but does not return", x.at)
			}
			i--
			continue
		case x86asm.RET:
			b, err := m.next("the jump to the epilogue")
			if err != nil {
				return err
			}
			rel, ok := b.in.Args[0].(x86asm.Rel)
			if b.in.Op != x86asm.JMP || !ok || !plain(b.in) {
				return fmt.Errorf("leaf return: %v at %#x is not a jump to the epilogue", b.in, b.at)
			}
			m.rets = append(m.rets, uint64(int64(b.at)+int64(b.in.Len)+int64(rel)))
			m.nrets++
			continue
		case x86asm.CALL:
			if err := m.call(x, k.llc.relIn(k.llc.text, x)); err != nil {
				return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
			}
			if c := r.Calls[m.calls-1]; c.Kind != CallLeaf {
				return fmt.Errorf("a leaf makes a %v call", c.Kind)
			}
			continue
		case x86asm.PUSH:
			return fmt.Errorf("llc's leaf pushes at %#x", x.at)
		}
		if mem, _, ok := memOf(x.in); ok && (frameRegs[mem.Base] || mem.Base == x86asm.RIP) {
			return fmt.Errorf("llc's leaf at %#x addresses memory from %v", x.at, mem.Base)
		}
		if err := m.one(x); err != nil {
			return fmt.Errorf("llc instruction at %#x (%v): %w", x.at, x.in, err)
		}
	}
	if m.j != len(B.ins) {
		return fmt.Errorf("generated instruction at %#x explained by no llc instruction: %v", B.ins[m.j].at, B.ins[m.j].in)
	}
	for _, t := range m.rets {
		if bi, ok := B.at[t]; ok && bi < len(B.ins) {
			t = B.ins[bi].at
		}
		if t != e {
			return fmt.Errorf("a return jumps to %#x, not to the Go epilogue", t)
		}
	}
	for _, br := range m.branches {
		li, ok := L.at[br.llcTarget]
		if !ok || li >= len(L.ins) || m.bStart[li] < 0 {
			return fmt.Errorf("branch at %#x: llc's target %#x is no instruction of the body", br.at, br.llcTarget)
		}
		if bi, ok := B.at[br.binTarget]; !ok || bi != m.bStart[li] {
			return fmt.Errorf("branch at %#x goes to %#x, not to where llc's target %#x went", br.at, br.binTarget, br.llcTarget)
		}
	}
	if m.calls != len(r.Calls) {
		return fmt.Errorf("the lowering declared %d calls, llc's object has %d", len(r.Calls), m.calls)
	}
	if m.calls != len(m.facts.Calls) || m.nrets != len(m.facts.Rets) {
		return fmt.Errorf("llc's facts list %d calls and %d returns, its object has %d and %d",
			len(m.facts.Calls), len(m.facts.Rets), m.calls, m.nrets)
	}
	return nil
}

// ---- stubs ----

// checkStub confirms a function the generator could not lower reports its
// index: MOVQ $i, 0(SP); CALL goasmMissing.
func (k *checker) checkStub(meta Meta, idx int) error {
	bsym, _ := k.bin.abi0(k.c.Pkg, k.c.Names[meta.Name])
	g, err := k.goPrologue(bsym.Value)
	if err != nil {
		return err
	}
	if !g.split || !g.bp || g.frame != 8 {
		return fmt.Errorf("Go prologue of a stub: split %v, BP %v, frame %d", g.split, g.bp, g.frame)
	}
	a := g.afterLead
	in, err := k.dec(a)
	if err != nil {
		return err
	}
	m, ok := in.Args[0].(x86asm.Mem)
	imm, ok2 := in.Args[1].(x86asm.Imm)
	if in.Op != x86asm.MOV || !ok || !ok2 || normMem(m) != (x86asm.Mem{Base: x86asm.RSP}) || int64(imm) != int64(idx) || in.DataSize != 64 {
		return fmt.Errorf("stub does not pass its index %d: %v", idx, in)
	}
	a += uint64(in.Len)
	if in, err = k.dec(a); err != nil {
		return err
	}
	miss, ok := k.bin.abi0(k.c.Pkg, "goasmMissing")
	rel, ok2 := in.Args[0].(x86asm.Rel)
	if !ok || in.Op != x86asm.CALL || !ok2 || uint64(int64(a)+int64(in.Len)+int64(rel)) != miss.Value {
		return fmt.Errorf("stub does not call goasmMissing: %v", in)
	}
	return k.goEpilogue(g, a+uint64(in.Len), bsym)
}

// ---- engine_rodata ----

// checkRodata compares engine_rodata in the binary with llc's object: every
// blob object's bytes, every pointer (an absolute address the linker wrote,
// which must be the same llc label plus offset, mapped into the binary) and
// every resolved difference of labels. Bytes no object covers must be zero,
// and every pointer and difference the plan declared must belong to an
// object llc's object has one in.
func (k *checker) checkRodata() error {
	p := k.c.Plan
	rs, ok := k.bin.syms[k.c.Pkg+"."+RodataGo]
	if !ok {
		if len(p.Blob) == 0 {
			return nil
		}
		return fmt.Errorf("no engine_rodata in the binary")
	}
	if int(rs.Size) < len(p.Blob) {
		return fmt.Errorf("engine_rodata is %d bytes in the binary, %d planned", rs.Size, len(p.Blob))
	}
	blob, err := k.bin.read(rs.Value, int(rs.Size))
	if err != nil {
		return err
	}
	relocs := map[int]BlobReloc{}
	for _, r := range p.Relocs {
		relocs[r.Off] = r
	}
	diffs := map[int]BlobDiff{}
	for _, d := range p.Diffs {
		diffs[d.Off] = d
	}
	used := map[int]bool{}
	owned := map[string]bool{}
	covered := make([]bool, len(blob))
	for _, o := range k.c.Unit.DataOrder {
		boff, ok := p.BlobOff[o.Label]
		if !ok {
			continue
		}
		lab, err := k.llc.label(o.Label)
		if err != nil {
			return err
		}
		if err := k.sameLabels(o, lab, func(a string) bool { off, ok := p.BlobOff[a]; return ok && off == boff }); err != nil {
			return err
		}
		owned[o.Label] = true
		for _, a := range o.Aliases {
			owned[a] = true
		}
		n, err := k.llc.extent(lab)
		if err != nil {
			return err
		}
		if boff < 0 || boff+n > len(blob) {
			return fmt.Errorf("data %s runs past engine_rodata", o.Label)
		}
		si := int(lab.Section)
		want := k.llc.data[si]
		if want == nil || int(lab.Value)+n > len(want) {
			return fmt.Errorf("data %s: no bytes in llc's object", o.Label)
		}
		for i := 0; i < n; {
			bo := boff + i
			lo := lab.Value + uint64(i)
			width, err := k.blobSlot(blob, rs.Value, bo, si, lo, relocs, diffs)
			if err != nil {
				return fmt.Errorf("data %s, byte %d: %w", o.Label, i, err)
			}
			if i+width > n {
				return fmt.Errorf("data %s, byte %d: a %d-byte value runs past the object", o.Label, i, width)
			}
			if _, ok := relocs[bo]; ok {
				used[bo] = true
			}
			if _, ok := diffs[bo]; ok {
				used[bo] = true
			}
			for w := 0; w < width; w++ {
				if covered[bo+w] {
					return fmt.Errorf("data %s overlaps another object in engine_rodata at %d", o.Label, bo+w)
				}
				covered[bo+w] = true
			}
			i += width
		}
		k.rep.Checked["engine_rodata objects"]++
	}
	for l := range p.BlobOff {
		if !owned[l] {
			return fmt.Errorf("engine_rodata: the label %s belongs to none of its objects", l)
		}
	}
	for off := range relocs {
		if !used[off] {
			return fmt.Errorf("engine_rodata: a planned pointer at %d belongs to no object llc relocates there", off)
		}
	}
	for off := range diffs {
		if !used[off] {
			return fmt.Errorf("engine_rodata: a planned difference at %d belongs to no object", off)
		}
	}
	for i, c := range covered {
		if !c && blob[i] != 0 {
			return fmt.Errorf("engine_rodata: byte %d (%#02x) belongs to no object", i, blob[i])
		}
	}
	return nil
}

// blobSlot checks the value at blob offset bo against llc's section si at
// lo, and returns how many bytes it covered: 8 for a pointer, the size of a
// difference, else one literal byte.
func (k *checker) blobSlot(blob []byte, base uint64, bo, si int, lo uint64, relocs map[int]BlobReloc, diffs map[int]BlobDiff) (int, error) {
	p := k.c.Plan
	r, rel := k.llc.rel[si][lo]
	br, isPtr := relocs[bo]
	d, isDiff := diffs[bo]
	switch {
	case rel && r.typ == elf.R_X86_64_64:
		if !isPtr {
			return 0, fmt.Errorf("llc relocates it, the plan holds no pointer there")
		}
		if err := k.llc.llcIs(loc{int(r.sym.Section), int64(r.sym.Value) + r.add}, &r.sym, br.Label, br.LabelAdd); err != nil {
			return 0, err
		}
		var want uint64
		if br.Func {
			a, err := k.funcAddr(br.Label)
			if err != nil {
				return 0, err
			}
			want = uint64(int64(a) + br.LabelAdd)
		} else {
			lb, ok := p.BlobOff[br.Label]
			if !ok {
				return 0, fmt.Errorf("points to %s, which is not in engine_rodata", br.Label)
			}
			want = uint64(int64(base) + int64(lb) + br.LabelAdd)
		}
		if got := binary.LittleEndian.Uint64(blob[bo:]); got != want {
			return 0, fmt.Errorf("the pointer is %#x in the binary, should be %#x (%s%+d)", got, want, br.Label, br.LabelAdd)
		}
		return 8, nil
	case rel && r.typ == elf.R_X86_64_PC32:
		if !isDiff || d.Size != 4 {
			return 0, fmt.Errorf("llc relocates it PC-relatively, the plan holds no 4-byte difference there")
		}
		pos, err := k.llc.label(d.Pos)
		if err != nil {
			return 0, err
		}
		neg := int64(lo)
		if d.Neg != "." {
			nl, err := k.llc.label(d.Neg)
			if err != nil {
				return 0, err
			}
			if int(nl.Section) != si {
				return 0, fmt.Errorf("a difference from %s, in another section", d.Neg)
			}
			neg = int64(nl.Value)
		}
		// S + A - P = Pos + K - Neg, with the slot P and Neg in this section.
		if int(r.sym.Section) != int(pos.Section) || int64(r.sym.Value)+r.add != int64(pos.Value)+d.K+(int64(lo)-neg) {
			return 0, fmt.Errorf("llc's entry is not %s-%s%+d", d.Pos, d.Neg, d.K)
		}
		return 4, k.blobDiff(blob, bo, d)
	case rel:
		return 0, fmt.Errorf("a %v relocation", r.typ)
	case isPtr:
		return 0, fmt.Errorf("the plan holds a pointer where llc has none")
	case isDiff:
		// llc resolved the difference itself: both labels in one section.
		pos, err := k.llc.label(d.Pos)
		if err != nil {
			return 0, err
		}
		neg := int64(lo)
		if d.Neg != "." {
			nl, err := k.llc.label(d.Neg)
			if err != nil {
				return 0, err
			}
			if nl.Section != pos.Section {
				return 0, fmt.Errorf("a difference of %s and %s, in two sections, that llc did not relocate", d.Pos, d.Neg)
			}
			neg = int64(nl.Value)
		} else if int(pos.Section) != si {
			return 0, fmt.Errorf("a difference of %s and a place in another section that llc did not relocate", d.Pos)
		}
		if int(lo)+d.Size > len(k.llc.data[si]) {
			return 0, fmt.Errorf("a difference past the section")
		}
		for b := 1; b < d.Size; b++ {
			if _, r := k.llc.rel[si][lo+uint64(b)]; r {
				return 0, fmt.Errorf("a relocation inside a difference")
			}
		}
		if lv, v := readInt(k.llc.data[si][lo:], d.Size), int64(pos.Value)-neg+d.K; lv != v {
			return 0, fmt.Errorf("llc's value is %d, %s-%s%+d is %d", lv, d.Pos, d.Neg, d.K, v)
		}
		return d.Size, k.blobDiff(blob, bo, d)
	}
	if blob[bo] != k.llc.data[si][lo] {
		return 0, fmt.Errorf("%#02x in the binary, %#02x in llc's object", blob[bo], k.llc.data[si][lo])
	}
	return 1, nil
}

// blobDiff checks a resolved difference in the linked blob: the distance of
// its two labels in engine_rodata plus its constant.
func (k *checker) blobDiff(blob []byte, bo int, d BlobDiff) error {
	p := k.c.Plan
	pos, ok1 := p.BlobOff[d.Pos]
	neg, ok2 := p.BlobOff[d.Neg]
	if d.Neg == "." {
		neg, ok2 = bo, true
	}
	if !ok1 || !ok2 {
		return fmt.Errorf("the difference %s-%s names data outside engine_rodata", d.Pos, d.Neg)
	}
	if got, want := readInt(blob[bo:], d.Size), int64(pos-neg)+d.K; got != want {
		return fmt.Errorf("the difference is %d in the binary, %s-%s%+d is %d", got, d.Pos, d.Neg, d.K, want)
	}
	return nil
}

func readInt(b []byte, n int) int64 {
	switch n {
	case 1:
		return int64(int8(b[0]))
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(b)))
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(b)))
	}
	return int64(binary.LittleEndian.Uint64(b))
}
